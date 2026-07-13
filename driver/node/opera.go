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
// client on a generic host.
type OperaNode struct {
	host      network.Host
	container *docker.Container
	config    *OperaNodeConfig
	tempDirs  []string
	sonicd    *docker.ExecHandle
	logsDir   string
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
		err = genesis.ApplyNetworkRulesPatch(&rules, config.NetworkConfig.NetworkRules)
		if err != nil {
			return nil, fmt.Errorf("failed to configure rules for temporary genesis: %w", err)
		}

		genesisPath := filepath.Join(tmpDir, "genesis.json")
		err = genesis.GenerateJsonGenesis(
			genesisPath,
			driver.GetValidatorStakes(config.NetworkConfig.Validators),
			&rules)
		if err != nil {
			return nil, fmt.Errorf("failed to generate temporary genesis: %w", err)
		}

		genesisJSONPath = genesisPath
		// verify a file is created in genesisJSONPath
		if info, err := os.Stat(genesisJSONPath); err != nil {
			return nil, fmt.Errorf("failed to verify temporary genesis file: %w", err)
		} else {
			if info.IsDir() {
				return nil, fmt.Errorf("temporary genesis path is a directory, expected a file: %s", genesisJSONPath)
			} else if info.Size() == 0 {
				return nil, fmt.Errorf("temporary genesis file is empty: %s", genesisJSONPath)
			}
		}
	} else {
		genesisJSONPath = *config.GenesisJsonPath
	}

	// Container-level env vars (shared state only).
	envs := map[string]string{
		"STATE_DB_IMPL":   "geth",
		"VM_IMPL":         "geth",
		"LD_LIBRARY_PATH": "./",
		"GOMEMLIMIT":      "1GiB",
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
	var pubKey, address string
	if isValidator {
		var privKey string
		privKey, pubKey, address, err = genesis.DeriveValidatorKey(*config.ValidatorId)
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

	// Create a host-side logs directory for exec output.
	logsDir, err := os.MkdirTemp("", "norma-node-logs-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create logs dir: %w", err)
	}
	tempDirs = append(tempDirs, logsDir)

	host, err := client.Start(ctx,
		&docker.ContainerConfig{
			Hostname:        config.Label,
			ImageName:       image,
			ShutdownTimeout: &shutdownTimeout,
			Environment:     envs,
			Entrypoint:      []string{"sleep", "infinity"},
			Network:         dn,
			DataDirBinding:  dataDirBinding,
			GenesisFileBind: &genesisBind,
			KeystoreBinding: keystoreBinding,
			LogsDir:         &logsDir,
		})
	if err != nil {
		return nil, err
	}

	// Ensure the container and temp dirs are cleaned up if any
	// subsequent exec step fails before we return the OperaNode.
	started := false
	defer func() {
		if !started {
			_ = host.Cleanup(context.Background())
			for _, dir := range tempDirs {
				_ = os.RemoveAll(dir)
			}
		}
	}()

	// --- Exec-based startup sequence ---

	// 1. Initialize datadir with sonictool.
	// Skip initialization only when re-using a populated mount directory.
	needsInit := config.MountDataDir == nil || isDirEmpty(*config.MountDataDir)
	if needsInit {
		mkdirCmd := []string{"mkdir", "-m", "755", "-p", dataDir}
		if output, err := host.ExecWithEnv(ctx, mkdirCmd, nil, ""); err != nil {
			return nil, fmt.Errorf("failed to create datadir: %w - output: %s", err, output)
		}

		sonicToolCmd := []string{
			"./sonictool",
			"--datadir", dataDir,
			"--statedb.livecache", "1",
			"genesis", "json", "--experimental", "/genesis.json",
		}
		output, err := host.ExecWithEnv(ctx, sonicToolCmd, nil, "sonictool")
		if err != nil {
			return nil, fmt.Errorf("sonictool genesis init failed: %w - output: %s", err, output)
		}
	}

	// 2. Write password file for validator keystore decryption.
	passwordCmd := []string{"sh", "-c", "echo password > password.txt"}
	if _, err := host.ExecWithEnv(ctx, passwordCmd, nil, ""); err != nil {
		return nil, fmt.Errorf("failed to write password file: %w", err)
	}

	// 3. Write config.toml (emitter intervals).
	numValidators := config.NetworkConfig.Validators.GetNumValidators()
	dsProtection := "5000000000"
	if numValidators == 1 && validatorId == "1" {
		dsProtection = "0"
	}
	configToml := fmt.Sprintf("[Emitter.EmitIntervals]\nDoublesignProtection = %s\n", dsProtection)
	configCmd := []string{"sh", "-c",
		fmt.Sprintf("printf '%%s' '%s' > config.toml", configToml)}
	if _, err := host.ExecWithEnv(ctx, configCmd, nil, ""); err != nil {
		return nil, fmt.Errorf("failed to write config.toml: %w", err)
	}

	// 4. Network latency simulation via tc netem.
	latency := config.NetworkConfig.RoundTripTime / 2
	if latency > 0 {
		tcCmd := fmt.Sprintf(
			"tc qdisc add dev eth0 root netem delay %v"+
				" && (ip link show eth1 2>/dev/null"+
				" && tc qdisc add dev eth1 root netem delay %v || true)",
			latency, latency)
		cmd := []string{"sh", "-c", tcCmd}
		if _, err := host.ExecWithEnv(ctx, cmd, nil, "tc_setup"); err != nil {
			return nil, fmt.Errorf("failed to configure network latency: %w", err)
		}
	}

	// 5. Build sonicd command line.
	sonicdCmd := buildSonicdCmd(
		dataDir, validatorId, pubKey, address,
		host.IP(), config.ExtraArguments)

	// 6. Start sonicd in the background.
	slog.Info("Starting sonicd", "node", config.Label)
	sonicdHandle, err := host.ExecBackground(
		ctx,
		sonicdCmd,
		[]string{"GOMEMLIMIT=1GiB"},
		"sonicd",
	)
	if err != nil {
		return nil, fmt.Errorf("failed to start sonicd: %w", err)
	}
	slog.Info("Sonicd started")

	// --- End exec startup ---

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
		sonicd:    sonicdHandle,
		logsDir:   logsDir,
	}

	// Wait until the OperaNode inside the Container is ready.
	err = network.Retry(ctx, network.DefaultRetryAttempts, 1*time.Second,
		func(ctx context.Context) error {
			if err := node.host.CheckRunning(ctx); err != nil {
				return fmt.Errorf("%w: %w", err, network.ErrPermanent)
			}
			if err := connectivityCheck(ctx, node); err != nil {
				return err
			}
			_, err = node.GetNodeID()
			return err
		})
	if err == nil {
		started = true
		return node, nil
	}

	// The node did not show up in time, so we consider the start to have failed.
	started = true // node.Cleanup handles teardown; avoid double cleanup
	return nil, errors.Join(
		printLog(ctx, node),
		fmt.Errorf("failed to get node online, %w", err),
		node.Cleanup(ctx),
	)
}

// connectivityCheck attempts to connect to the Opera RPC service of the given host.
func connectivityCheck(ctx context.Context, node *OperaNode) error {
	addr, err := node.host.GetAddressForService(&OperaRpcService)
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

// CheckRunning returns an error if the node's container process is no longer
// running (e.g. it crashed or exited unexpectedly).
func (n *OperaNode) CheckRunning(ctx context.Context) error {
	return n.host.CheckRunning(ctx)
}

func (n *OperaNode) GetServiceUrl(service *network.ServiceDescription) (*driver.URL, error) {
	addr, err := n.host.GetAddressForService(service)
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
	return n.tailExecLog(ctx)
}

// tailExecLog returns a reader that continuously tails the sonicd exec
// log file, similar to `tail -f`. The reader blocks on Read when it
// reaches the end of the file and polls for new data until the context
// is cancelled or the sonicd process exits.
func (n *OperaNode) tailExecLog(ctx context.Context) (io.ReadCloser, error) {
	path, err := n.execLogPath()
	if err != nil {
		return nil, err
	}
	//nolint:gosec // path is constructed internally from logsDir
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open exec log: %w", err)
	}
	return &fileTailer{
		file:    f,
		done:    n.sonicd.Done,
		ctx:     ctx,
		pollInt: 200 * time.Millisecond,
	}, nil
}

// StreamExecLog opens the sonicd exec log file for a one-shot read.
// Use this only after the sonicd process has exited and the log file
// is fully flushed. For continuous tailing, use StreamLog instead.
func (n *OperaNode) StreamExecLog() (io.ReadCloser, error) {
	path, err := n.execLogPath()
	if err != nil {
		return nil, err
	}
	//nolint:gosec // path is constructed internally from logsDir
	return os.Open(path)
}

// execLogPath returns the path to the latest sonicd exec log file.
func (n *OperaNode) execLogPath() (string, error) {
	matches, err := filepath.Glob(
		filepath.Join(n.logsDir, "*_sonicd_*.log"),
	)
	if err != nil {
		return "", fmt.Errorf("failed to glob sonicd logs: %w", err)
	}
	if len(matches) == 0 {
		return "", fmt.Errorf(
			"no sonicd log file found in %s", n.logsDir,
		)
	}
	slices.Sort(matches)
	return matches[len(matches)-1], nil
}

// fileTailer implements io.ReadCloser and tails a file that is being
// written to by another process. When Read reaches EOF it polls for
// new data until the context is cancelled or the done channel closes.
type fileTailer struct {
	file    *os.File
	done    <-chan struct{} // closed when the writing process exits
	ctx     context.Context
	pollInt time.Duration
}

func (t *fileTailer) Read(p []byte) (int, error) {
	for {
		n, err := t.file.Read(p)
		if n > 0 || err != io.EOF {
			return n, err
		}
		// Reached EOF — check if the writer is done.
		select {
		case <-t.done:
			// Writer exited; do one final read then return EOF.
			return t.file.Read(p)
		case <-t.ctx.Done():
			return 0, t.ctx.Err()
		default:
		}
		// Poll for new data.
		select {
		case <-time.After(t.pollInt):
		case <-t.done:
		case <-t.ctx.Done():
			return 0, t.ctx.Err()
		}
	}
}

func (t *fileTailer) Close() error {
	return t.file.Close()
}

// Exec runs a command inside the node's container and returns its output.
func (n *OperaNode) Exec(ctx context.Context, cmd []string) (string, error) {
	return n.container.Exec(ctx, cmd)
}

// ExecDone returns a channel that is closed when the sonicd background
// exec finishes. This can be used to wait for the log file to be fully
// flushed before reading it.
func (n *OperaNode) ExecDone() <-chan struct{} {
	if n.sonicd == nil {
		ch := make(chan struct{})
		close(ch)
		return ch
	}
	return n.sonicd.Done
}

func (n *OperaNode) Stop(ctx context.Context) error {
	// Send SIGINT to sonicd so it can shut down gracefully (close DBs, etc.).
	// The container's PID 1 is "sleep infinity" which doesn't forward signals,
	// so we must explicitly find and signal the sonicd process.
	if n.container != nil && n.sonicd != nil {
		_, _ = n.container.Exec(ctx, []string{
			"sh", "-c",
			`for d in /proc/[0-9]*; do` +
				` grep -ql sonicd "$d/cmdline" 2>/dev/null &&` +
				` kill -INT "${d##*/}" 2>/dev/null; done`,
		})
		// Wait for sonicd to exit gracefully before stopping the container.
		select {
		case <-n.sonicd.Done:
		case <-ctx.Done():
		}
	}

	// Fix permissions on the bind-mounted datadir while the container is
	// still running, so non-root host users can clean up afterwards.
	if n.container != nil && n.config.MountDataDir != nil {
		_, _ = n.container.ExecWithEnv(
			ctx, []string{"chmod", "-R", "777", "/datadir"}, nil, "",
		)
	}

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

// buildSonicdCmd constructs the sonicd command-line from the given
// parameters. It mirrors the flags previously assembled in
// scripts/run_sonic.sh.
func buildSonicdCmd(
	dataDir, validatorId, pubKey, address, externalIP string,
	extraArguments string,
) []string {
	cmd := []string{
		"./sonicd",
		"--datadir=" + dataDir,
		"--http", "--http.addr", "0.0.0.0",
		"--http.port", "18545",
		"--http.api", "admin,eth,sonic,txpool",
		"--ws", "--ws.addr", "0.0.0.0",
		"--ws.port", "18546",
		"--ws.api", "admin,eth,sonic,txpool",
		"--pprof", "--pprof.addr", "0.0.0.0",
		"--nat", "extip:" + externalIP,
		"--metrics", "--metrics.expensive",
		"--config", "config.toml",
		"--datadir.minfreedisk", "0",
		"--statedb.livecache", "1",
	}

	if validatorId != "0" {
		cmd = append(cmd,
			"--validator.id", validatorId,
			"--validator.pubkey", pubKey,
			"--validator.password", "password.txt",
			"--mode", "rpc",
		)
	}

	if extraArguments != "" {
		cmd = append(cmd, strings.Fields(extraArguments)...)
	}

	return cmd
}

// isDirEmpty reports whether the directory at path contains no
// entries (ignoring the "keystore" directory written before init).
func isDirEmpty(path string) bool {
	entries, err := os.ReadDir(path)
	if err != nil {
		return true
	}
	for _, e := range entries {
		if e.Name() != "keystore" {
			return false
		}
	}
	return true
}
