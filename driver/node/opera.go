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

package node

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"golang.org/x/exp/maps"

	"github.com/0xsoniclabs/norma/driver/parser"
	rpcdriver "github.com/0xsoniclabs/norma/driver/rpc"
	"github.com/0xsoniclabs/norma/genesis"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/docker"
	"github.com/0xsoniclabs/norma/driver/network"
	"github.com/0xsoniclabs/sonic/opera"
	"github.com/ethereum/go-ethereum/rpc"
)

var OperaRpcService = network.ServiceDescription{
	Name:     "OperaRPC",
	Port:     18545,
	Protocol: "http",
}

var OperaWsService = network.ServiceDescription{
	Name:     "OperaWs",
	Port:     18546,
	Protocol: "ws",
}

var OperaDebugService = network.ServiceDescription{
	Name:     "OperaPprof",
	Port:     6060,
	Protocol: "http",
}

var operaServices = network.ServiceGroup{}

func init() {
	if err := operaServices.RegisterService(&OperaRpcService); err != nil {
		panic(err)
	}
	if err := operaServices.RegisterService(&OperaWsService); err != nil {
		panic(err)
	}
	if err := operaServices.RegisterService(&OperaDebugService); err != nil {
		panic(err)
	}
}

// OperaNode implements the driver's Node interface by running a go-opera
// client on a generic host.
type OperaNode struct {
	host      network.Host
	container *docker.Container
	config    *OperaNodeConfig
	tempDirs  []string
}

type OperaNodeConfig struct {
	// The label to be used to name this node. The label should not be empty.
	Label string
	// Failing if true, the node is expected to fail at some point of execution.
	Failing bool
	// The Docker image to use for the node.
	Image string
	// The ID of the validator, nil if the node should not be a validator.
	ValidatorId *int
	// The configuration of the network the configured node should be part of.
	NetworkConfig *driver.NetworkConfig
	// ValidatorPubkey is nil if not a validator, else used as pubkey for the validator.
	ValidatorPubkey *string
	// MountDataDir is the directory where the node should store its state.
	// Temporary location is used if nil.
	MountDataDir *string
	// GenesisJsonPath is the path to the host-generated genesis file mounted into the container.
	GenesisJsonPath *string
	// ExtraArguments are additional command line arguments to pass to the node.
	ExtraArguments string
}

// imageEnsureState stores the completion signal and final error for one
// in-flight image provisioning operation.
type imageEnsureState struct {
	done chan struct{}
	err  error
}

var (
	imageEnsureMutex sync.Mutex
	// imageEnsureInFlight tracks in-progress image provisioning by image tag.
	//
	// This allows concurrent node startups using the same image to share one
	// EnsureImages call instead of triggering duplicate pull/build operations.
	imageEnsureInFlight = map[string]*imageEnsureState{}
)

// ensureImageAvailable ensures the given image is locally available and
// deduplicates concurrent ensure calls for the same image.
//
// If another goroutine is already provisioning the same image, this function
// waits for that operation to complete and returns its result.
func ensureImageAvailable(ctx context.Context, image string) error {
	imageEnsureMutex.Lock()
	if state, found := imageEnsureInFlight[image]; found {
		imageEnsureMutex.Unlock()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-state.done:
			return state.err
		}
	}

	state := &imageEnsureState{done: make(chan struct{})}
	imageEnsureInFlight[image] = state
	imageEnsureMutex.Unlock()

	err := docker.EnsureImages(ctx, []string{image}, "")

	imageEnsureMutex.Lock()
	state.err = err
	close(state.done)
	delete(imageEnsureInFlight, image)
	imageEnsureMutex.Unlock()

	return err
}

// StartOperaDockerNode creates a new OperaNode running in a Docker container.
func StartOperaDockerNode(ctx context.Context, client *docker.Client, dn *docker.Network, config *OperaNodeConfig) (*OperaNode, error) {
	// avoid slashes and underscores in labels
	config.Label = strings.ReplaceAll(config.Label, "/", "-")
	config.Label = strings.ReplaceAll(config.Label, "_", "-")
	if !parser.NamePattern.MatchString(config.Label) {
		return nil, fmt.Errorf("invalid label for node: '%v'", config.Label)
	}

	exists, err := client.ContainerExists(config.Label)
	if err != nil {
		return nil, fmt.Errorf("failed to start docker node: %w", err)
	}
	if exists {
		return nil, fmt.Errorf("failed to start docker node: container %q already running", config.Label)
	}

	image := driver.ResolveClientImageName(config.Image)
	if err := ensureImageAvailable(ctx, image); err != nil {
		return nil, fmt.Errorf("failed to ensure image %q: %w", image, err)
	}

	shutdownTimeout := 180 * time.Second

	validatorId := "0"
	isValidator := config.ValidatorId != nil && *config.ValidatorId > 0
	tempDirs := make([]string, 0)
	genesisJSONPath := ""
	if config.ValidatorId != nil {
		validatorId = fmt.Sprintf("%d", *config.ValidatorId)
	}
	if config.GenesisJsonPath == nil || *config.GenesisJsonPath == "" {
		if config.NetworkConfig == nil {
			return nil, fmt.Errorf("missing network config for genesis generation")
		}

		tmpDir, err := os.MkdirTemp("", "norma-node-genesis-*")
		if err != nil {
			return nil, fmt.Errorf("failed to create temporary genesis dir: %w", err)
		}
		tempDirs = append(tempDirs, tmpDir)

		rules := opera.FakeNetRules(opera.GetSonicUpgrades())
		if err := genesis.ConfigureNetworkRulesMap(&rules, config.NetworkConfig.NetworkRules); err != nil {
			return nil, fmt.Errorf("failed to configure rules for temporary genesis: %w", err)
		}

		genesisPath := filepath.Join(tmpDir, "genesis.json")
		if err := genesis.GenerateJsonGenesis(genesisPath, driver.GetValidatorStakes(config.NetworkConfig.Validators), &rules); err != nil {
			return nil, fmt.Errorf("failed to generate temporary genesis: %w", err)
		}

		genesisJSONPath = genesisPath
	} else {
		genesisJSONPath = *config.GenesisJsonPath
	}

	host, err := network.RetryReturn(ctx, network.DefaultRetryAttempts, 1*time.Second, func() (*docker.Container, error) {
		ports, err := network.GetFreePorts(len(operaServices.Services()))
		if err != nil {
			return nil, err
		}

		portForwarding := make(map[network.Port]network.Port, len(ports))
		for i, service := range operaServices.Services() {
			portForwarding[service.Port] = ports[i]
		}

		envs := map[string]string{
			"VALIDATOR_ID":     validatorId,
			"VALIDATORS_COUNT": fmt.Sprintf("%d", config.NetworkConfig.Validators.GetNumValidators()),
			"NETWORK_LATENCY":  fmt.Sprintf("%v", config.NetworkConfig.RoundTripTime/2),
			"EXTRA_ARGUMENTS":  config.ExtraArguments,
		}

		const dataDir = "/datadir"
		envs["STATE_DB_DATADIR"] = dataDir

		// when configured, mount the datadir to the host
		var dataDirBinding *string
		if config.MountDataDir != nil {
			if err := os.MkdirAll(*config.MountDataDir, 0777); err != nil {
				return nil, err
			}

			dataDirBinding = new(string)
			*dataDirBinding = fmt.Sprintf("%s:%s", *config.MountDataDir, dataDir)
		}

		genesisBind := fmt.Sprintf("%s:/genesis.json:ro", genesisJSONPath)

		var keystoreBinding *string
		if isValidator {
			privKey, pubKey, address, err := genesis.DeriveValidatorKey(*config.ValidatorId)
			if err != nil {
				return nil, fmt.Errorf("failed to derive validator key: %w", err)
			}
			envs["VALIDATOR_PUBKEY"] = pubKey
			envs["VALIDATOR_ADDRESS"] = address

			if config.MountDataDir != nil {
				if err := genesis.WriteValidatorKeystore(privKey, *config.MountDataDir); err != nil {
					return nil, fmt.Errorf("failed to write validator keystore in mounted datadir: %w", err)
				}
			} else {
				validatorDir, err := os.MkdirTemp("", fmt.Sprintf("norma-validator-%d-*", *config.ValidatorId))
				if err != nil {
					return nil, fmt.Errorf("failed to create validator temp dir: %w", err)
				}
				tempDirs = append(tempDirs, validatorDir)

				if err := genesis.WriteValidatorKeystore(privKey, validatorDir); err != nil {
					return nil, fmt.Errorf("failed to write validator keystore: %w", err)
				}

				keystorePath := filepath.Join(validatorDir, "keystore")
				keystoreBinding = new(string)
				*keystoreBinding = fmt.Sprintf("%s:%s/keystore:ro", keystorePath, dataDir)
			}
		}

		maps.Copy(envs, config.NetworkConfig.NetworkRules) // put in the network rules

		return client.Start(ctx,
			&docker.ContainerConfig{
				Hostname:        config.Label,
				ImageName:       image,
				ShutdownTimeout: &shutdownTimeout,
				PortForwarding:  portForwarding,
				Environment:     envs,
				Network:         dn,
				DataDirBinding:  dataDirBinding,
				GenesisFileBind: &genesisBind,
				KeystoreBinding: keystoreBinding,
			})
	})

	if err != nil {
		return nil, err
	}

	// Use a private copy of the config to avoid modifying the original.
	nodeConfig := *config
	nodeConfig.Image = image
	nodeConfig.GenesisJsonPath = nil
	if genesisJSONPath != "" {
		nodeConfig.GenesisJsonPath = new(string)
		*nodeConfig.GenesisJsonPath = genesisJSONPath
	}
	if config.ValidatorId != nil {
		nodeConfig.ValidatorId = new(int)
		*nodeConfig.ValidatorId = *config.ValidatorId
	}
	node := &OperaNode{
		host:      host,
		container: host,
		config:    &nodeConfig,
		tempDirs:  tempDirs,
	}

	// Wait until the OperaNode inside the Container is ready.
	err = network.Retry(ctx, network.DefaultRetryAttempts, 1*time.Second, func() error {
		if err := node.host.CheckRunning(ctx); err != nil {
			return fmt.Errorf("%w: %w", err, network.ErrPermanent)
		}
		_, err = node.GetNodeID()
		return err
	})
	if err == nil {
		return node, nil
	}

	// The node did not show up in time, so we consider the start to have failed.
	return nil, errors.Join(
		printLog(ctx, node),
		fmt.Errorf("failed to get node online, %w", err),
		node.Cleanup(ctx),
	)
}

// printLog streams and prints the logs of the given OperaNode, to debug cause of
// startup failure.
func printLog(ctx context.Context, node *OperaNode) error {
	reader, err := node.StreamLog(ctx)
	if err != nil {
		return fmt.Errorf("cannot read node logs: %w", err)
	}
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		fmt.Printf("[Opera Node %s] %s\n", node.GetLabel(), scanner.Text())
	}
	return reader.Close()
}

func (n *OperaNode) GetLabel() string {
	return n.config.Label
}

func (n *OperaNode) IsExpectedFailure() bool {
	return n.config.Failing
}

// Hostname returns the hostname of the node.
// The hostname is accessible only inside the Docker network.
func (n *OperaNode) Hostname() string {
	return n.host.Hostname()
}

// MetricsPort returns the port on which the node exports its metrics.
// The port is accessible only inside the Docker network.
func (n *OperaNode) MetricsPort() int {
	return 6060
}

func (n *OperaNode) IsRunning() bool {
	return n.host.IsRunning()
}

func (n *OperaNode) GetServiceUrl(service *network.ServiceDescription) *driver.URL {
	addr := n.host.GetAddressForService(service)
	if addr == nil {
		return nil
	}
	url := driver.URL(fmt.Sprintf("%s://%s", service.Protocol, *addr))
	return &url
}

func (n *OperaNode) GetNodeID() (driver.NodeID, error) {
	url := n.GetServiceUrl(&OperaRpcService)
	if url == nil {
		return "", fmt.Errorf("node does not export an RPC server")
	}
	rpcClient, err := rpc.DialContext(context.Background(), string(*url))
	if err != nil {
		return "", err
	}
	var result struct {
		Enode string
	}
	err = rpcClient.Call(&result, "admin_nodeInfo")
	if err != nil {
		return "", err
	}
	return driver.NodeID(result.Enode), nil
}

func (n *OperaNode) GetValidatorId() *int {
	return n.config.ValidatorId
}

func (n *OperaNode) StreamLog(ctx context.Context) (io.ReadCloser, error) {
	return n.host.StreamLog(ctx)
}

func (n *OperaNode) Stop(ctx context.Context) error {
	return n.host.Stop(ctx)
}

func (n *OperaNode) Cleanup(ctx context.Context) error {
	err := n.host.Cleanup(ctx)
	for _, dir := range n.tempDirs {
		if cleanupErr := os.RemoveAll(dir); cleanupErr != nil {
			err = errors.Join(err, cleanupErr)
		}
	}
	n.tempDirs = nil
	return err
}

func (n *OperaNode) DialRpc(ctx context.Context) (rpcdriver.Client, error) {
	url := n.GetServiceUrl(&OperaRpcService)
	if url == nil {
		return nil, fmt.Errorf("node %s does not export an RPC server", n.GetLabel())
	}

	rpcClient, err := network.RetryReturn(ctx, network.DefaultRetryAttempts, 1*time.Second, func() (*rpc.Client, error) {
		return rpc.DialContext(ctx, string(*url))
	})
	if err != nil {
		return nil, fmt.Errorf("failed to dial RPC for node %s; %v", n.GetLabel(), err)
	}
	return rpcdriver.WrapRpcClient(rpcClient), nil
}

// AddPeer informs the client instance represented by the OperaNode about the
// existence of another node, to which it may establish a connection.
func (n *OperaNode) AddPeer(ctx context.Context, id driver.NodeID) error {
	rpcClient, err := n.DialRpc(ctx)
	if err != nil {
		return err
	}
	return network.Retry(ctx, network.DefaultRetryAttempts, 1*time.Second, func() error {
		if err := rpcClient.Call(nil, "admin_addTrustedPeer", id); err != nil {
			return fmt.Errorf("failed to add trusted peer on node %s: %v", id, err)
		}
		return rpcClient.Call(nil, "admin_addPeer", id)
	})
}

// RemovePeer informs the client instance represented by the OperaNode
// that the input node is no more available in the network.
func (n *OperaNode) RemovePeer(ctx context.Context, id driver.NodeID) error {
	rpcClient, err := n.DialRpc(ctx)
	if err != nil {
		return err
	}
	return network.Retry(ctx, network.DefaultRetryAttempts, 1*time.Second, func() error {
		return rpcClient.Call(nil, "admin_removePeer", id)
	})
}

// Kill sends a SigKill signal to node.
func (n *OperaNode) Kill(ctx context.Context) error {
	return n.container.SendSignal(ctx, docker.SigKill)
}

// GetRoundTripTime returns the median network round-trip time to the given host.
func (n *OperaNode) GetRoundTripTime(host string) (time.Duration, error) {
	output, err := n.container.Exec(context.Background(), []string{"ping", "-c", "5", host})
	if err != nil {
		return 0, err
	}
	regex := regexp.MustCompile("time=([0-9.]+) ms")
	matches := regex.FindAllStringSubmatch(string(output), -1)

	durations := make([]time.Duration, 0, len(matches))
	for _, match := range matches {
		duration, err := time.ParseDuration(match[1] + "ms")
		if err != nil {
			return 0, err
		}
		durations = append(durations, duration)
	}
	slices.Sort(durations)
	return durations[len(durations)/2], nil
}
