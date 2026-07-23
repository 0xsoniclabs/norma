// Copyright 2024 Fantom Foundation
// This file is part of Norma System Testing Infrastructure for Sonic.
//
// Norma is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// Norma is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with Norma. If not, see <http://www.gnu.org/licenses/>.

package local

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/docker"
	"github.com/0xsoniclabs/norma/driver/network"
	"github.com/0xsoniclabs/norma/driver/network/rpc"
	"github.com/0xsoniclabs/norma/driver/node"
	rpcdriver "github.com/0xsoniclabs/norma/driver/rpc"
	"github.com/0xsoniclabs/norma/genesis"
	"github.com/0xsoniclabs/norma/load/app"
	"github.com/0xsoniclabs/norma/load/controller"
	"github.com/0xsoniclabs/norma/load/shaper"
	"github.com/0xsoniclabs/sonic/opera"
	"github.com/ethereum/go-ethereum/core/types"
)

// LocalNetwork is a Docker based network running each individual node
// within its own, dedicated Docker Container.
type LocalNetwork struct {
	ctx            context.Context
	docker         *docker.Client
	network        *docker.Network
	config         driver.NetworkConfig
	primaryAccount *app.Account

	// nodes provide a register for all nodes in the network, including
	// validator nodes created during startup.
	nodes map[driver.NodeID]*node.OperaNode

	// nodesMutex synchronizes access to the list of nodes.
	nodesMutex sync.Mutex

	// apps maintains a list of all applications created on the network.
	apps []driver.Application

	// appsMutex synchronizes access to the list of applications.
	appsMutex sync.Mutex

	// nextAppId is the id to use for next created applications.
	nextAppId atomic.Uint32

	// listeners is the set of registered NetworkListeners.
	listeners map[driver.NetworkListener]bool

	// listenerMutex is synching access to listeners
	listenerMutex sync.Mutex

	rpcWorkerPool *rpc.RpcWorkerPool

	// a context for app management operations on the network
	appContext app.AppContext

	// appContextMu guards lazy initialization of appContext.
	appContextMu sync.Mutex

	// temporary host directory used while preparing genesis artifacts
	genesisTmpDir string
	// host path to the generated genesis.json file
	genesisJsonPath string

	// runConsensusChain tracks whether the network rules enable the Sonic
	// consensus-chain engine, whose libp2p mesh requires explicit seeding
	// (see seedConsensusChainNode). It is set from the initial rules and
	// updated when a rules patch toggles the RunConsensusChain upgrade.
	runConsensusChain atomic.Bool
}

// NewLocalLegacyNetwork creates a network and starts all validators
// defined in the configuration. It also eagerly initializes the app
// context. Use this for the legacy (time-based) executor and tests
// that expect nodes to be running immediately after construction.
func NewLocalLegacyNetwork(
	ctx context.Context,
	config *driver.NetworkConfig,
) (*LocalNetwork, error) {
	net, err := NewLocalNetwork(ctx, config)
	if err != nil {
		return nil, err
	}

	if err := net.startGenesisValidators(ctx); err != nil {
		return nil, errors.Join(err, net.Shutdown())
	}

	if err := net.ensureAppContext(); err != nil {
		return nil, errors.Join(err, net.Shutdown())
	}

	return net, nil
}

func NewLocalNetwork(ctx context.Context, config *driver.NetworkConfig) (*LocalNetwork, error) {
	client, err := docker.NewClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create docker client; %v", err)
	}

	dn, err := client.CreateBridgeNetwork(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create bridge network; %v", err)
	}

	// Create chain account, which will be used for the initialization
	primaryAccount, err := app.NewAccount(0, treasureAccountPrivateKey, fakeNetworkID)
	if err != nil {
		return nil, fmt.Errorf("failed to create primary account; %v", err)
	}

	// Create the empty network.
	net := &LocalNetwork{
		ctx:            ctx,
		docker:         client,
		network:        dn,
		config:         *config,
		primaryAccount: primaryAccount,
		nodes:          map[driver.NodeID]*node.OperaNode{},
		apps:           []driver.Application{},
		listeners:      map[driver.NetworkListener]bool{},
		rpcWorkerPool:  rpc.NewRpcWorkerPool(ctx),
	}

	if err := net.prepareGenesis(); err != nil {
		return nil, errors.Join(fmt.Errorf("failed to prepare genesis: %w", err), net.Shutdown())
	}

	// Let the RPC pool to start RPC workers when a node start.
	net.RegisterListener(net.rpcWorkerPool)

	return net, nil
}

// startGenesisValidators boots all validators defined in the network
// configuration sequentially, matching the genesis validator IDs.
func (n *LocalNetwork) startGenesisValidators(
	ctx context.Context,
) error {
	var idx int
	for _, validator := range n.config.Validators {
		for j := range validator.Instances {
			label := fmt.Sprintf("validator-%d", j)
			if len(validator.Name) != 0 {
				label = fmt.Sprintf("%s-%d", validator.Name, j)
			}
			validatorId := idx + 1
			cfg := node.OperaNodeConfig{
				ValidatorId:     &validatorId,
				Failing:         validator.Failing,
				Image:           validator.ImageName,
				NetworkConfig:   &n.config,
				Label:           label,
				GenesisJsonPath: &n.genesisJsonPath,
				ExtraArguments:  validator.ExtraArguments,
			}
			if _, err := n.createNode(ctx, &cfg); err != nil {
				return fmt.Errorf(
					"validator %q (idx=%d, image=%s): %w",
					label, idx, validator.ImageName, err,
				)
			}
			idx++
		}
	}
	return nil
}

// addNodeIntoNetwork connects the node with other nodes in the network, adds it into the list of nodes.
// It is best-effort: if some existing nodes are unreachable, the new node will
// still join the network as long as at least one peer connection succeeds.
func (n *LocalNetwork) addNodeIntoNetwork(ctx context.Context, node *node.OperaNode) error {
	n.nodesMutex.Lock()
	defer n.nodesMutex.Unlock()

	id, err := node.GetNodeID()
	if err != nil {
		return fmt.Errorf("failed to get node id; %v", err)
	}
	var succeeded int
	for _, other := range n.nodes {
		if err = other.AddPeer(ctx, id); err != nil {
			label := other.GetLabel()
			if checkErr := other.CheckRunning(ctx); checkErr != nil {
				slog.Error("node has crashed", "node", label, "status", checkErr)
			} else {
				slog.Warn("failed to add peer to node", "node", label, "error", err)
			}
		} else {
			succeeded++
		}
	}
	if len(n.nodes) > 0 && succeeded == 0 {
		return fmt.Errorf("failed to add peer; no existing node was reachable")
	}

	// When the consensus-chain engine is running, its libp2p mesh uses
	// ephemeral peer identities that are not known ahead of time, so the new
	// node must additionally be seeded into the mesh through an existing one.
	if n.runConsensusChain.Load() && len(n.nodes) > 0 {
		if err := seedConsensusChainNode(ctx, node, n.nodes); err != nil {
			return err
		}
	}

	n.nodes[id] = node
	return nil
}

// seedConsensusChainNode dials one of the given existing nodes from the new
// node, seeding it into the consensus-chain mesh; the validator directory
// carries the remaining membership. The bootstrap RPCs answer with an error
// until the engine is up, so both sides are retried.
func seedConsensusChainNode(ctx context.Context, newNode *node.OperaNode, others map[driver.NodeID]*node.OperaNode) error {
	addresses, err := network.RetryReturn(ctx, network.DefaultRetryAttempts, 1*time.Second,
		func(ctx context.Context) ([]string, error) {
			for _, other := range others {
				if addresses, err := other.ConsensusChainAddresses(ctx); err == nil && len(addresses) > 0 {
					return addresses, nil
				}
			}
			return nil, fmt.Errorf("no existing node answered the consensus-chain bootstrap RPC")
		})
	if err != nil {
		return fmt.Errorf("failed to get consensus-chain addresses to seed node %s: %w", newNode.GetLabel(), err)
	}
	return network.Retry(ctx, network.DefaultRetryAttempts, 1*time.Second,
		func(ctx context.Context) error {
			return newNode.ConsensusChainConnect(ctx, addresses)
		})
}

// connectConsensusChainMesh seeds every node in the network into the
// consensus-chain mesh by dialing one common node from all others. It is used
// after the RunConsensusChain upgrade is enabled on a running network, where
// all nodes bring up fresh engines at once; for an already-formed mesh it is
// a cheap no-op.
func (n *LocalNetwork) connectConsensusChainMesh(ctx context.Context) error {
	n.nodesMutex.Lock()
	nodes := make([]*node.OperaNode, 0, len(n.nodes))
	for _, item := range n.nodes {
		nodes = append(nodes, item)
	}
	n.nodesMutex.Unlock()

	if len(nodes) < 2 {
		return nil
	}

	first := nodes[0]
	addresses, err := network.RetryReturn(ctx, network.DefaultRetryAttempts, 1*time.Second,
		func(ctx context.Context) ([]string, error) {
			return first.ConsensusChainAddresses(ctx)
		})
	if err != nil {
		return fmt.Errorf("node %s's consensus-chain engine did not come up: %w", first.GetLabel(), err)
	}

	for _, item := range nodes[1:] {
		err := network.Retry(ctx, network.DefaultRetryAttempts, 1*time.Second,
			func(ctx context.Context) error {
				return item.ConsensusChainConnect(ctx, addresses)
			})
		if err != nil {
			return err
		}
	}
	return nil
}

// createNode is an internal version of CreateNode enabling the creation
// of validator and non-validator nodes in the network.
func (n *LocalNetwork) createNode(ctx context.Context, nodeConfig *node.OperaNodeConfig) (*node.OperaNode, error) {
	nodeConfig.ConsensusChainEnabled = n.runConsensusChain.Load()
	node, err := node.StartOperaDockerNode(ctx, n.docker, n.network, nodeConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to start opera docker; %v", err)
	}
	if err := n.addNodeIntoNetwork(ctx, node); err != nil {
		return nil, fmt.Errorf("failed to connect node; %w", err)
	}
	n.listenerMutex.Lock()
	for listener := range n.listeners {
		listener.AfterNodeCreation(node)
	}
	n.listenerMutex.Unlock()
	return node, nil
}

// CreateNode creates nodes in the network during run.
func (n *LocalNetwork) CreateNode(config *driver.NodeConfig) (driver.Node, error) {
	if config.Cheater {
		_, err := n.createNode(n.ctx, &node.OperaNodeConfig{
			Label:          "cheater-" + config.Name,
			Failing:        config.Failing,
			Image:          config.Image,
			NetworkConfig:  &n.config,
			ValidatorId:    config.ValidatorId,
			ExtraArguments: config.ExtraArguments,
		})
		if err != nil {
			return nil, err
		}
	}

	var datadir *string
	if config.DataVolume != nil {
		datadir = new(string)
		*datadir = fmt.Sprintf("%s/%s", n.config.OutputDir, *config.DataVolume)
	}

	return n.createNode(n.ctx, &node.OperaNodeConfig{
		Label:           config.Name,
		Failing:         config.Failing,
		Image:           config.Image,
		NetworkConfig:   &n.config,
		ValidatorId:     config.ValidatorId,
		GenesisJsonPath: &n.genesisJsonPath,
		MountDataDir:    datadir,
		ExtraArguments:  config.ExtraArguments,
	})
}

// prepareGenesis generates the genesis.json file for the network based on the
// configuration provided at startup and stores it in a temporary directory.
// The path to the generated genesis.json file is stored in the LocalNetwork
// struct for later use during node creation.
func (n *LocalNetwork) prepareGenesis() error {
	tmpDir, err := os.MkdirTemp("", "norma-genesis-*")
	if err != nil {
		return fmt.Errorf("failed to create genesis temp dir: %w", err)
	}

	rules := opera.FakeNetRules(opera.GetSonicUpgrades())
	if err := genesis.ApplyNetworkRulesPatch(&rules, n.config.NetworkRules); err != nil {
		return errors.Join(fmt.Errorf("failed to apply network rules to genesis: %w", err), os.RemoveAll(tmpDir))
	}
	n.runConsensusChain.Store(rules.Upgrades.RunConsensusChain)

	genesisPath := filepath.Join(tmpDir, "genesis.json")
	useConsensusChain := n.config.NetworkRules.UseConsensusChain != nil && *n.config.NetworkRules.UseConsensusChain
	if err := genesis.GenerateJsonGenesis(genesisPath, driver.GetValidatorStakes(n.config.Validators), &rules, useConsensusChain); err != nil {
		return errors.Join(fmt.Errorf("failed to generate genesis file: %w", err), os.RemoveAll(tmpDir))
	}

	n.genesisTmpDir = tmpDir
	n.genesisJsonPath = genesisPath
	return nil
}
func (n *LocalNetwork) RemoveNode(node driver.Node) error {
	n.listenerMutex.Lock()
	for listener := range n.listeners {
		listener.BeforeNodeRemoval(node)
	}
	n.listenerMutex.Unlock()

	n.nodesMutex.Lock()
	defer n.nodesMutex.Unlock()
	id, err := node.GetNodeID()
	if err != nil {
		return fmt.Errorf("failed to get node id; %v", err)
	}

	delete(n.nodes, id)
	for _, other := range n.nodes {
		if err = other.RemovePeer(n.ctx, id); err != nil {
			n.nodesMutex.Unlock()
			return fmt.Errorf("failed to remove peer; %v", err)
		}
	}

	return nil
}

func (n *LocalNetwork) SendTransaction(tx *types.Transaction, source string) {
	n.rpcWorkerPool.SendTransaction(tx, source)
}

func (n *LocalNetwork) DialRandomRpc() (rpcdriver.Client, error) {
	nodes := n.GetActiveNodes()
	if len(nodes) == 0 {
		return nil, driver.ErrEmptyNetwork
	}
	reliable := make([]driver.Node, 0, len(nodes))
	for _, node := range nodes {
		if !node.IsExpectedFailure() {
			reliable = append(reliable, node)
		}
	}
	if len(reliable) == 0 {
		reliable = nodes // use failing nodes if there are no reliable nodes
	}
	// Shuffle and try nodes in order, skipping unresponsive ones.
	perm := rand.Perm(len(reliable))
	for _, i := range perm {
		chosen := reliable[i]
		client, err := chosen.DialRpc(n.ctx)
		if err != nil {
			slog.Warn("node unreachable in DialRandomRpc, skipping",
				"node", chosen.GetLabel(), "error", err)
			continue
		}
		// Verify the connection is actually alive with a quick call.
		probeCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_, probeErr := client.BlockNumber(probeCtx)
		cancel()
		if probeErr != nil {
			slog.Warn("node not responding in DialRandomRpc, skipping",
				"node", chosen.GetLabel(), "error", probeErr)
			client.Close()
			continue
		}
		return client, nil
	}
	return nil, fmt.Errorf("no reachable node found among %d active nodes", len(reliable))
}

func (n *LocalNetwork) ApplyNetworkRules(ctx context.Context, rules driver.NetworkRules) error {
	client, err := n.DialRandomRpc()
	if err != nil {
		return fmt.Errorf("failed to connect to network: %w", err)
	}
	defer client.Close()

	if err := network.ApplyNetworkRules(ctx, client, rules); err != nil {
		return err
	}

	// A patch toggling the RunConsensusChain upgrade changes whether nodes run
	// the consensus-chain engine. The engines come up on the next epoch seal,
	// so the mesh is seeded in AdvanceEpoch rather than here.
	if rules.Upgrades != nil && rules.Upgrades.RunConsensusChain != nil {
		n.runConsensusChain.Store(*rules.Upgrades.RunConsensusChain)
	}
	return nil
}

func (n *LocalNetwork) HandOverToConsensusChain(ctx context.Context) error {
	if !n.runConsensusChain.Load() {
		return fmt.Errorf("cannot hand over: the consensus-chain engine is not running (enable the RunConsensusChain upgrade first)")
	}
	client, err := n.DialRandomRpc()
	if err != nil {
		return fmt.Errorf("failed to connect to network: %w", err)
	}
	defer client.Close()

	return network.HandOverToConsensusChain(ctx, client)
}

func (n *LocalNetwork) AdvanceEpoch(ctx context.Context, epochIncrement int) error {
	client, err := n.DialRandomRpc()
	if err != nil {
		return fmt.Errorf("failed to connect to network: %w", err)
	}
	defer client.Close()

	if err := network.AdvanceEpoch(ctx, client, epochIncrement); err != nil {
		return err
	}

	// When the RunConsensusChain upgrade was enabled by a rules patch, the
	// engines are built by the nodes on the epoch seal that just happened, and
	// their fresh mesh identities must be connected. This is a no-op for an
	// already-formed mesh, as re-dialing a connected peer does nothing.
	if n.runConsensusChain.Load() {
		if err := n.connectConsensusChainMesh(ctx); err != nil {
			return fmt.Errorf("failed to seed the consensus-chain mesh: %w", err)
		}
	}
	return nil
}

func (n *LocalNetwork) WaitForEpochChange(ctx context.Context) error {
	client, err := n.DialRandomRpc()
	if err != nil {
		return fmt.Errorf("failed to connect to network: %w", err)
	}
	defer client.Close()

	return network.WaitForEpochChange(ctx, client)
}

// treasureAccountPrivateKey is an account with tokens that can be used to
// initiate test applications and accounts.
const treasureAccountPrivateKey = "163f5f0f9a621d72fedd85ffca3d08d131ab4e812181e0d30ffd1c885d20aac7" // Fakenet validator 1

const fakeNetworkID = 0xfa3

type localApplication struct {
	name       string
	controller *controller.AppController
	config     *driver.ApplicationConfig
	cancel     context.CancelFunc
	done       *sync.WaitGroup
}

func (a *localApplication) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	a.cancel = cancel

	a.done.Add(1)
	go func() {
		defer a.done.Done()
		err := a.controller.Run(ctx)
		if err != nil {
			slog.Error("failed to run load app", "error", err)
		}
	}()
	return nil
}

func (a *localApplication) Stop() error {
	if a.cancel != nil {
		a.cancel()
	}
	a.cancel = nil
	slog.Info("waiting for application to stop", "app", a.name)
	a.done.Wait()
	slog.Info("application has stopped", "app", a.name)
	return nil
}

func (a *localApplication) Config() *driver.ApplicationConfig {
	return a.config
}

func (a *localApplication) GetNumberOfUsers() int {
	return a.controller.GetNumberOfUsers()
}

func (a *localApplication) GetSentTransactions(user int) (uint64, error) {
	return a.controller.GetTransactionsSentBy(user)
}

func (a *localApplication) GetReceivedTransactions() (uint64, error) {
	return a.controller.GetReceivedTransactions()
}

// ensureAppContext initializes the appContext lazily on first use.
// It requires at least one node to be running (for RPC connectivity).
func (n *LocalNetwork) ensureAppContext() error {
	n.appContextMu.Lock()
	defer n.appContextMu.Unlock()
	if n.appContext != nil {
		return nil
	}
	appCtx, err := app.NewContext(n.ctx, n, n.primaryAccount, n.config.NetworkRules)
	if err != nil {
		return fmt.Errorf("failed to create app context: %w", err)
	}
	n.appContext = appCtx
	return nil
}

func (n *LocalNetwork) CreateApplication(ctx context.Context, config *driver.ApplicationConfig) (driver.Application, error) {
	if err := n.ensureAppContext(); err != nil {
		return nil, fmt.Errorf("failed to initialize app context: %w", err)
	}

	rpcClient, err := n.DialRandomRpc()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to RPC to initialize the application; %v", err)
	}
	defer rpcClient.Close()

	appId := n.nextAppId.Add(1)
	application, err := app.NewApplication(config.Type, n.appContext, 0, appId)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize on-chain app; %v", err)
	}

	sh, err := shaper.ParseRate(config.Rate)
	if err != nil {
		return nil, fmt.Errorf("failed to parse shaper; %v", err)
	}

	appController, err := controller.NewAppController(application, sh, config.Users, n.appContext, n)
	if err != nil {
		return nil, err
	}

	app := &localApplication{
		name:       config.Name,
		controller: appController,
		config:     config,
		done:       &sync.WaitGroup{},
	}

	n.appsMutex.Lock()
	n.apps = append(n.apps, app)
	n.appsMutex.Unlock()

	n.listenerMutex.Lock()
	for listener := range n.listeners {
		listener.AfterApplicationCreation(app)
	}
	n.listenerMutex.Unlock()

	return app, nil
}

func (n *LocalNetwork) GetActiveNodes() []driver.Node {
	n.nodesMutex.Lock()
	defer n.nodesMutex.Unlock()
	res := make([]driver.Node, 0, len(n.nodes))
	for _, node := range n.nodes {
		if node.IsRunning() {
			res = append(res, node)
		}
	}
	return res
}

func (n *LocalNetwork) GetActiveApplications() []driver.Application {
	n.appsMutex.Lock()
	defer n.appsMutex.Unlock()
	return n.apps
}

func (n *LocalNetwork) RegisterListener(listener driver.NetworkListener) {
	n.listenerMutex.Lock()
	n.listeners[listener] = true
	n.listenerMutex.Unlock()
}

func (n *LocalNetwork) UnregisterListener(listener driver.NetworkListener) {
	n.listenerMutex.Lock()
	delete(n.listeners, listener)
	n.listenerMutex.Unlock()
}

func (n *LocalNetwork) Shutdown() error {
	var errs []error

	// cleanups shall be completed, even after execution context is canceled.
	ctx := context.Background()

	// First stop all generators.
	for _, app := range n.apps {
		// TODO: shutdown apps in parallel.
		if err := app.Stop(); err != nil {
			errs = append(errs, err)
		}
	}
	n.apps = n.apps[:0]

	if n.appContext != nil {
		n.appContext.Close()
	}

	// Second, shut down the nodes.
	for _, node := range n.nodes {
		// TODO: shutdown nodes in parallel.
		if err := node.Stop(ctx); err != nil {
			errs = append(errs, err)
		}
		if err := node.Cleanup(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	n.nodes = map[driver.NodeID]*node.OperaNode{}

	// Third, shut down the docker network.
	if n.network != nil {
		if err := n.network.Cleanup(ctx); err != nil {
			errs = append(errs, err)
		}
	}

	errs = append(errs, n.rpcWorkerPool.Close())
	if n.genesisTmpDir != "" {
		errs = append(errs, os.RemoveAll(n.genesisTmpDir))
		n.genesisTmpDir = ""
		n.genesisJsonPath = ""
	}

	return errors.Join(errs...)
}

// GetDockerNetwork returns the underlying docker network.
func (n *LocalNetwork) GetDockerNetwork() *docker.Network {
	return n.network
}
