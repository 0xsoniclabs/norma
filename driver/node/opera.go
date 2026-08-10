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
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

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
// client inside a Docker container.
//
// The client is started as a `docker exec` rather than as the container's
// entrypoint, which is what allows a scenario to stop, kill, heal and
// restart it without discarding the container and its data directory.
// Because that lifecycle control is docker-specific, the node holds a
// *docker.Container directly instead of the generic network.Host.
type OperaNode struct {
	container *docker.Container
	config    *OperaNodeConfig
	tempDirs  []string

	// The fields below are all guarded by stateMutex.
	stateMutex sync.Mutex
	state      NodeState
	// sonicd is the handle of the most recently started client process, or
	// nil when none has been started yet.
	sonicd *docker.ExecHandle
	// clientGen counts client starts. It lets an exit watcher tell whether
	// the process it was watching is still the current one.
	clientGen uint64
	// clientExitWasUnexpected records that the node reached
	// NodeStateKilled because its client died rather than was killed.
	clientExitWasUnexpected bool
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
	// NetworkBootstrap is true if this node starts a brand-new network, i.e.
	// there is no running node it could sync with.
	NetworkBootstrap bool
	// PubKey is the public key of the validator, if the node is a validator.
	PubKey string
	// PrivKey is the private key of the validator, if the node is a validator.
	PrivKey string
	// Address is the address of the validator, if the node is a validator.
	Address string
	// LogsDir is the host directory the client's output is written to. When
	// empty, a temporary directory is used that is removed on cleanup, so
	// callers that want the logs to outlive the run must set this.
	LogsDir string
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
func StartOperaDockerNode(
	ctx context.Context,
	client *docker.Client,
	dn *docker.Network,
	config *OperaNodeConfig,
) (*OperaNode, error) {
	// avoid slashes and underscores in labels
	config.Label = strings.ReplaceAll(config.Label, "/", "-")
	config.Label = strings.ReplaceAll(config.Label, "_", "-")
	if !parser.NamePattern.MatchString(config.Label) {
		return nil, fmt.Errorf("invalid label for node: '%v'", config.Label)
	}

	if dn == nil {
		return nil, fmt.Errorf("docker network is required to start an Opera node")
	}

	exists, err := client.ContainerExists(config.Label)
	if err != nil {
		return nil, fmt.Errorf("failed to start docker node: %w", err)
	}
	if exists {
		return nil, fmt.Errorf("failed to start docker node: container %q already running", config.Label)
	}

	if config.ValidatorId != nil && *config.ValidatorId > 0 {
		var privKey, pubKey, address string
		if privKey, pubKey, address, err = genesis.DeriveValidatorKey(*config.ValidatorId); err != nil {
			return nil, fmt.Errorf("failed to derive validator key: %w", err)
		}
		config.PrivKey = privKey
		config.PubKey = pubKey
		config.Address = address
	}

	node, err := NewOperaNode(ctx, client, dn, config)
	if err != nil {
		return nil, fmt.Errorf("failed to start docker node: %w", err)
	}

	// Ensure the container and temp dirs are cleaned up if any
	// subsequent exec step fails before we return the OperaNode.
	started := false
	defer func() {
		if !started {
			// defer needs its own context
			_ = node.Cleanup(context.Background())
		}
	}()

	// --- Exec-based startup sequence ---

	// Initialize datadir with sonictool.
	if err = node.Initialize(ctx); err != nil {
		return nil, fmt.Errorf("failed to initialize datadir: %w", err)
	}

	// Start sonicd in the background.
	if err = node.StartSonicd(ctx); err != nil {
		return nil, fmt.Errorf("failed to start sonicd: %w", err)
	}

	// Wait for the node to sync and become ready.
	if err = node.WaitForSync(ctx); err != nil {
		return nil, errors.Join(
			printLog(ctx, node),
			fmt.Errorf("failed to get node online: %w", err),
		)
	}

	started = true
	return node, nil
}

// connectivityCheck attempts to connect to the Opera RPC service of the given host.
func connectivityCheck(ctx context.Context, node *OperaNode) error {
	addr, err := node.container.GetAddressForService(&OperaRpcService)
	if err != nil {
		return fmt.Errorf("failed to get RPC service address: %w", err)
	}

	conn, dialErr := net.DialTimeout("tcp", string(*addr), 5*time.Second)
	if dialErr != nil {
		return fmt.Errorf("failed to connect to RPC service at %s: %w",
			string(*addr), dialErr)
	}
	if err := conn.Close(); err != nil {
		return fmt.Errorf("failed to close connection to RPC service at %s: %w",
			string(*addr), err)
	}
	return nil
}

// startupLogTimeout bounds how long printLog collects output. The log
// stream follows the node and only ends when the container stops, so the
// read has to be bounded explicitly or a failed startup would hang instead
// of reporting why it failed.
const startupLogTimeout = 10 * time.Second

// printLog streams and prints the logs of the given OperaNode, to help
// diagnose the cause of a startup failure. Emits structured slog
// entries tagged with the node label so the output is greppable and
// consistent with the rest of the driver's logging.
func printLog(ctx context.Context, node *OperaNode) error {
	logCtx, cancel := context.WithTimeout(ctx, startupLogTimeout)
	defer cancel()

	reader, err := node.StreamLog(logCtx)
	if err != nil {
		return fmt.Errorf("cannot read node logs: %w", err)
	}
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		slog.Info("opera node log",
			"node", node.GetLabel(),
			"line", scanner.Text())
	}
	return errors.Join(scanner.Err(), reader.Close())
}

// dataDir is the path where the node stores its state inside the container.
const dataDir = "/datadir"

// containerShutdownTimeout bounds how long docker waits for the container
// to exit after being signalled before it is killed.
const containerShutdownTimeout = 180 * time.Second

// containerEntrypoint keeps the container alive without running the
// client: the client itself is started later as a background exec, so that
// it can be stopped and restarted without losing the container.
var containerEntrypoint = []string{"sleep", "infinity"}

// NewOperaNode creates the container hosting an Opera node and returns it
// in NodeStateUninitialized. The client process is not started; see
// Initialize and StartSonicd.
func NewOperaNode(
	ctx context.Context,
	client *docker.Client,
	dn *docker.Network,
	config *OperaNodeConfig,
) (*OperaNode, error) {
	image := driver.ResolveClientImageName(config.Image)
	if err := ensureImageAvailable(ctx, image); err != nil {
		return nil, fmt.Errorf("failed to ensure image %q: %w", image, err)
	}

	// tempDirs collects host directories owned by this node so they can be
	// released by Cleanup, or right here if construction fails. The helpers
	// below report a directory they created even when they then fail, so
	// that a partially prepared directory is still cleaned up.
	tempDirs := make([]string, 0)
	cleanupTempDirs := func() {
		for _, dir := range tempDirs {
			_ = os.RemoveAll(dir)
		}
	}
	track := func(dir string) {
		if dir != "" {
			tempDirs = append(tempDirs, dir)
		}
	}

	genesisJSONPath, genesisTempDir, err := resolveGenesisFile(config)
	track(genesisTempDir)
	if err != nil {
		cleanupTempDirs()
		return nil, err
	}

	envs := map[string]string{
		// The remaining client environment (STATE_DB_IMPL, VM_IMPL,
		// LD_LIBRARY_PATH, GOMEMLIMIT) is baked into the image; exec'd
		// processes inherit it from the container.
		"STATE_DB_DATADIR": dataDir,
	}

	var dataDirBinding *string
	if config.MountDataDir != nil {
		if err := os.MkdirAll(*config.MountDataDir, 0777); err != nil {
			cleanupTempDirs()
			return nil, fmt.Errorf("failed to create mount data dir: %w", err)
		}

		dataDirBinding = new(string)
		*dataDirBinding = fmt.Sprintf("%s:%s", *config.MountDataDir, dataDir)
	}

	genesisBind := fmt.Sprintf("%s:/genesis.json:ro", genesisJSONPath)

	keystoreBinding, keystoreTempDir, err := resolveValidatorKeystore(config, envs)
	track(keystoreTempDir)
	if err != nil {
		cleanupTempDirs()
		return nil, err
	}

	logsDir, logsDirIsTemporary, err := resolveLogsDir(config)
	if err != nil {
		cleanupTempDirs()
		return nil, err
	}
	if logsDirIsTemporary {
		track(logsDir)
	}
	slog.Debug("node logs directory resolved",
		"node", config.Label, "path", logsDir,
		"temporary", logsDirIsTemporary)

	shutdownTimeout := containerShutdownTimeout
	container, err := client.Start(ctx,
		&docker.ContainerConfig{
			Hostname:        config.Label,
			ImageName:       image,
			ShutdownTimeout: &shutdownTimeout,
			Environment:     envs,
			Entrypoint:      containerEntrypoint,
			Network:         dn,
			DataDirBinding:  dataDirBinding,
			GenesisFileBind: &genesisBind,
			KeystoreBinding: keystoreBinding,
			LogsDir:         &logsDir,
		})
	if err != nil {
		cleanupTempDirs()
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

	return &OperaNode{
		container: container,
		config:    &nodeConfig,
		tempDirs:  tempDirs,
		state:     NodeStateUninitialized,
	}, nil
}

// resolveGenesisFile returns the host path of the genesis file to mount.
// When the config does not name one, a genesis is generated into a fresh
// temporary directory, whose path is returned as the second result so the
// caller can register it for cleanup.
func resolveGenesisFile(config *OperaNodeConfig) (path, tempDir string, err error) {
	if config.GenesisJsonPath != nil && *config.GenesisJsonPath != "" {
		return *config.GenesisJsonPath, "", nil
	}
	if config.NetworkConfig == nil {
		return "", "", fmt.Errorf("missing network config for genesis generation")
	}

	tempDir, err = os.MkdirTemp("", "norma-node-genesis-*")
	if err != nil {
		return "", "", fmt.Errorf("failed to create temporary genesis dir: %w", err)
	}

	rules := opera.FakeNetRules(opera.GetSonicUpgrades())
	if err := genesis.ApplyNetworkRulesPatch(
		&rules, config.NetworkConfig.NetworkRules); err != nil {
		return "", tempDir, fmt.Errorf(
			"failed to configure rules for temporary genesis: %w", err)
	}

	path = filepath.Join(tempDir, "genesis.json")
	if err := genesis.GenerateJsonGenesis(path,
		driver.GetValidatorStakes(config.NetworkConfig.Validators),
		&rules); err != nil {
		return "", tempDir, fmt.Errorf(
			"failed to generate temporary genesis: %w", err)
	}

	// A missing or empty genesis file surfaces as an obscure client startup
	// failure much later, so fail here where the cause is still obvious.
	info, err := os.Stat(path)
	if err != nil {
		return "", tempDir, fmt.Errorf(
			"failed to verify temporary genesis file: %w", err)
	}
	if info.IsDir() {
		return "", tempDir, fmt.Errorf(
			"temporary genesis path is a directory, expected a file: %s", path)
	}
	if info.Size() == 0 {
		return "", tempDir, fmt.Errorf("temporary genesis file is empty: %s", path)
	}
	return path, tempDir, nil
}

// resolveValidatorKeystore writes the validator keystore for validator
// nodes and returns the bind mount exposing it to the container, if any.
// It also records the validator identity in envs. The returned tempDir is
// non-empty when a temporary directory was created for the keystore.
func resolveValidatorKeystore(
	config *OperaNodeConfig,
	envs map[string]string,
) (binding *string, tempDir string, err error) {
	if config.ValidatorId == nil || *config.ValidatorId <= 0 {
		return nil, "", nil
	}

	envs["VALIDATOR_PUBKEY"] = config.PubKey
	envs["VALIDATOR_ADDRESS"] = config.Address

	if config.MountDataDir != nil {
		if err := genesis.WriteValidatorKeystore(
			config.PrivKey, *config.MountDataDir); err != nil {
			return nil, "", fmt.Errorf(
				"failed to write validator keystore in mounted datadir: %w", err)
		}
		return nil, "", nil
	}

	tempDir, err = os.MkdirTemp("",
		fmt.Sprintf("norma-validator-%d-*", *config.ValidatorId))
	if err != nil {
		return nil, "", fmt.Errorf("failed to create validator temp dir: %w", err)
	}
	if err := genesis.WriteValidatorKeystore(config.PrivKey, tempDir); err != nil {
		return nil, tempDir, fmt.Errorf("failed to write validator keystore: %w", err)
	}

	binding = new(string)
	*binding = fmt.Sprintf("%s:%s/keystore:ro",
		filepath.Join(tempDir, "keystore"), dataDir)
	return binding, tempDir, nil
}

// resolveLogsDir determines where the client's output is written on the
// host. When the config names a directory (the scenario's output
// directory) the logs are placed there and survive teardown, which is the
// whole point of keeping them; temporary is true only for the unconfigured
// fallback, whose directory the caller registers for cleanup.
func resolveLogsDir(config *OperaNodeConfig) (dir string, temporary bool, err error) {
	if config.LogsDir == "" {
		dir, err = os.MkdirTemp("", "norma-node-logs-*")
		if err != nil {
			return "", false, fmt.Errorf("failed to create logs dir: %w", err)
		}
		return dir, true, nil
	}
	dir = filepath.Join(config.LogsDir, "client-logs")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", false, fmt.Errorf("failed to create logs dir %s: %w", dir, err)
	}
	return dir, false, nil
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
	return n.container.Hostname()
}

// MetricsPort returns the port on which the node exports its metrics.
// The port is accessible only inside the Docker network.
func (n *OperaNode) MetricsPort() int {
	return 6060
}

func (n *OperaNode) IsRunning() bool {
	return n.container.IsRunning()
}

// CheckRunning returns an error if the node's container process is no longer
// running (e.g. it crashed or exited unexpectedly).
func (n *OperaNode) CheckRunning(ctx context.Context) error {
	return n.container.CheckRunning(ctx)
}

func (n *OperaNode) GetServiceUrl(service *network.ServiceDescription) (*driver.URL, error) {
	addr, err := n.container.GetAddressForService(service)
	if err != nil {
		return nil, fmt.Errorf("failed to get service address for %s: %w", service.Name, err)
	}
	url := driver.URL(fmt.Sprintf("%s://%s", service.Protocol, *addr))
	return &url, nil
}

func (n *OperaNode) GetNodeID() (driver.NodeID, error) {
	url, err := n.GetServiceUrl(&OperaRpcService)
	if err != nil {
		return "", fmt.Errorf("failed to get RPC service URL: %w", err)
	}
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
	return n.container.StreamLog(ctx)
}

// StreamExecLog opens the client's log file for a one-shot read of the
// complete output of the most recent client process. Use this only after
// the process has exited, i.e. after <-ExecDone; for continuous tailing,
// use StreamLog instead.
func (n *OperaNode) StreamExecLog() (io.ReadCloser, error) {
	handle := n.clientHandle()
	if handle == nil {
		return nil, fmt.Errorf("node %q has no client process", n.GetLabel())
	}
	if handle.LogPath == "" {
		return nil, fmt.Errorf(
			"node %q does not persist client output; no logs directory configured",
			n.GetLabel())
	}
	return os.Open(handle.LogPath) //#nosec G304 -- path is constructed internally
}

// Exec runs a command inside the node's container and returns its output.
func (n *OperaNode) Exec(ctx context.Context, cmd []string) (string, error) {
	return n.container.Exec(ctx, cmd)
}

// ExecDone returns a channel that is closed once the client process has
// exited and its log file has been flushed and closed.
func (n *OperaNode) ExecDone() <-chan struct{} {
	handle := n.clientHandle()
	if handle == nil {
		ch := make(chan struct{})
		close(ch)
		return ch
	}
	return handle.Done
}

// Stop shuts the node down: the client process first, then the container.
//
// The client is stopped on a best-effort basis. A node that was killed,
// never started, or already stopped is not an error here: Stop is also the
// teardown path, and refusing to release the container because the client
// was already gone would leak it and fail otherwise-successful scenarios.
func (n *OperaNode) Stop(ctx context.Context) error {
	if err := n.StopSonicd(ctx); err != nil {
		slog.Debug("client process was not stopped gracefully",
			"node", n.GetLabel(), "error", err)
	}

	// Fix permissions on the bind-mounted datadir while the container is
	// still running, so non-root host users can clean up afterwards.
	if n.container != nil && n.config.MountDataDir != nil {
		if _, err := n.container.Exec(ctx,
			[]string{"chmod", "-R", "777", dataDir}); err != nil {
			slog.Debug("failed to relax datadir permissions",
				"node", n.GetLabel(), "error", err)
		}
	}

	if n.container == nil {
		return nil
	}
	return n.container.Stop(ctx)
}

func (n *OperaNode) Cleanup(ctx context.Context) error {
	var err error
	if n.container != nil {
		err = n.container.Cleanup(ctx)
	}
	for _, dir := range n.tempDirs {
		if cleanupErr := os.RemoveAll(dir); cleanupErr != nil {
			err = errors.Join(err, cleanupErr)
		}
	}
	n.tempDirs = nil
	return err
}

func (n *OperaNode) DialRpc(ctx context.Context) (rpcdriver.Client, error) {
	url, err := n.GetServiceUrl(&OperaRpcService)
	if err != nil {
		return nil, fmt.Errorf("node %s does not export an RPC server: %w", n.GetLabel(), err)
	}

	rpcClient, err := network.RetryReturn(ctx, network.DefaultRetryAttempts, 1*time.Second,
		func(ctx context.Context) (*rpc.Client, error) {
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
	return network.Retry(ctx, network.DefaultRetryAttempts, 1*time.Second,
		func(ctx context.Context) error {
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
	return network.Retry(ctx, network.DefaultRetryAttempts, 1*time.Second,
		func(ctx context.Context) error {
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

// GetState returns the current state of the node.
func (n *OperaNode) GetState() NodeState {
	n.stateMutex.Lock()
	defer n.stateMutex.Unlock()
	return n.state
}

// transition atomically moves the node from the expected `from` state to
// the target `to` state. It returns an error if the current state does
// not match `from`, which prevents concurrent actions from interleaving.
// This is the only supported way to mutate the node's state.
func (n *OperaNode) transition(from, to NodeState) error {
	return n.transitionFromAny(to, from)
}

// transitionFromAny is transition for actions reachable from more than one
// state, moving the node to `to` if its current state is any of `from`.
func (n *OperaNode) transitionFromAny(to NodeState, from ...NodeState) error {
	n.stateMutex.Lock()
	defer n.stateMutex.Unlock()
	if !slices.Contains(from, n.state) {
		return fmt.Errorf(
			"node %q: cannot transition %s\u2192%s (currently %s)",
			n.GetLabel(), formatStates(from), to, n.state)
	}
	n.state = to
	return nil
}

// formatStates renders the accepted source states of a transition.
func formatStates(states []NodeState) string {
	names := make([]string, 0, len(states))
	for _, state := range states {
		names = append(names, state.String())
	}
	return strings.Join(names, "|")
}

// clientHandle returns the handle of the most recently started client
// process, or nil if none was started.
func (n *OperaNode) clientHandle() *docker.ExecHandle {
	n.stateMutex.Lock()
	defer n.stateMutex.Unlock()
	return n.sonicd
}

// beginClientStart claims the Ready\u2192Syncing transition for a new client
// process and returns the generation identifying it. Bumping the
// generation here, before the process exists, retires the previous
// process's exit watcher so it cannot be attributed to this start.
func (n *OperaNode) beginClientStart() (uint64, error) {
	n.stateMutex.Lock()
	defer n.stateMutex.Unlock()
	if n.state != NodeStateReady {
		return 0, fmt.Errorf(
			"node %q: cannot transition %s\u2192%s (currently %s)",
			n.GetLabel(), NodeStateReady, NodeStateSyncing, n.state)
	}
	n.state = NodeStateSyncing
	n.clientGen++
	n.clientExitWasUnexpected = false
	return n.clientGen, nil
}

// attachClient records the handle of a started client process, unless the
// start it belongs to has already been superseded.
func (n *OperaNode) attachClient(gen uint64, handle *docker.ExecHandle) {
	n.stateMutex.Lock()
	defer n.stateMutex.Unlock()
	if n.clientGen == gen {
		n.sonicd = handle
	}
}

// forceSetState unconditionally sets the node's state. It is intended
// only for construction (setting the initial state) and must not be
// used from action methods; use transition instead.
func (n *OperaNode) forceSetState(s NodeState) {
	n.stateMutex.Lock()
	defer n.stateMutex.Unlock()
	n.state = s
}

// buildSonicdCmd constructs the sonicd argument vector.
//
// The command is returned in exec form and is run without a shell: the
// output is captured from the exec stream on the host, so there is nothing
// to redirect, and keeping the arguments as separate argv entries means
// values are never re-parsed or word-split by a shell.
func buildSonicdCmd(
	validatorId *int,
	pubKey, externalIP string,
	extraArguments string,
) []string {
	args := []string{
		sonicdBinaryPath,
		"--datadir=" + dataDir,
		"--http", "--http.addr", "0.0.0.0",
		"--http.port", "18545",
		"--http.api", "admin,dag,eth,sonic,txpool",
		"--ws", "--ws.addr", "0.0.0.0",
		"--ws.port", "18546",
		"--ws.api", "admin,dag,eth,sonic,txpool",
		"--pprof", "--pprof.addr", "0.0.0.0",
		"--nat", "extip:" + externalIP,
		"--metrics", "--metrics.expensive",
		"--config", configFilePath,
		"--datadir.minfreedisk", "0",
		"--statedb.livecache", "1",
	}

	if validatorId != nil && *validatorId > 0 {
		args = append(args,
			"--validator.id", fmt.Sprintf("%d", *validatorId),
			"--validator.pubkey", pubKey,
			"--validator.password", passwordFilePath,
			"--mode", "rpc",
		)
	}

	if extraArguments != "" {
		args = append(args, strings.Fields(extraArguments)...)
	}

	return args
}
