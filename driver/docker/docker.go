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

package docker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/0xsoniclabs/norma/driver/network"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	dockerNetwork "github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
)

// projectLabel is the label used to identify objects created by norma.
const objectsLabel = "norma"

// Signal represents a signal that can be sent to a Docker container.
type Signal string

// SigHup is the SIGHUP signal.
var SigHup Signal = "SIGHUP"
var SigKill Signal = "SIGKILL"
var SigInt Signal = "SIGINT"

// Client provides means to spawn Docker containers capable of hosting
// services like the go-opera client.
type Client struct {
	cli *client.Client
}

// Network represents a Docker network. It is used to connect Containers
// to each other.
type Network struct {
	id      string
	name    string
	client  *Client
	cleaned bool
}

// ID returns the Docker network ID.
func (n *Network) ID() string { return n.id }

// Name returns the Docker network name.
func (n *Network) Name() string { return n.name }

// Container represents a Docker Container, typically used for running a
// Fantom network Node, thus an instance of the go-opera client.
// *Container implements the driver.Host interface.
type Container struct {
	id      string
	ip      string
	client  *Client
	config  *ContainerConfig
	stopped bool
	cleaned bool

	// execLogs carries the output of background execs. It is only used
	// once a background exec has been started; until then the container's
	// own stdout (via ContainerLogs) remains the log source.
	execLogs *logBroadcaster
	// execLogPath is the host path of the most recent background exec log.
	execLogPath string
	// execLogMutex guards execLogs and execLogPath.
	execLogMutex sync.Mutex
}

// ExecHandle represents a background exec process running inside a
// container.
//
// Done is closed once the process has exited, its output has been fully
// drained and its exit status has been collected; after that, Err
// reports either a streaming failure or a non-zero exit status.
//
// Note: Docker's ContainerExecInspect.Pid returns the host-namespace
// PID, which is not usable for `kill` executed inside the container
// (different PID namespace). Callers that need to signal the process
// must discover its container-namespace PID from inside the container
// (see OperaNode.signalSonicd).
type ExecHandle struct {
	// ExecID is the docker exec instance ID, used in diagnostics.
	ExecID string
	// LogPath is the host path the output was written to, or empty when
	// output was not persisted.
	LogPath string
	// Done is closed when the process has exited and Err is final.
	Done <-chan struct{}

	mu       sync.Mutex
	err      error
	exitCode int
}

// Err returns the error that terminated the process: a streaming
// failure, or a non-zero exit status. It returns nil while the process
// is still running, so callers should only consult it after <-Done.
func (h *ExecHandle) Err() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.err
}

// ExitCode returns the process exit status. It is only meaningful after
// <-Done.
func (h *ExecHandle) ExitCode() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.exitCode
}

func (h *ExecHandle) setResult(exitCode int, err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.exitCode = exitCode
	if h.err == nil {
		h.err = err
	}
}

// ContainerConfig defines parameters for running Docker Containers.
type ContainerConfig struct {
	Hostname        string
	ImageName       string
	ShutdownTimeout *time.Duration
	Environment     map[string]string
	Entrypoint      []string // Entrypoint to run when starting the container. Optional.
	Network         *Network // Docker network to join
	DataDirBinding  *string  // mount client datadir to this path on host
	GenesisFileBind *string  // mount genesis file on host to /genesis.json:ro in container
	KeystoreBinding *string  // mount keystore dir on host to /datadir/keystore:ro in container
	LogsDir         *string  // host directory for exec output logs
}

// NewClient creates a new client facilitating the creation of Docker
// Containers capable of hosting services. Clients successfully created
// through this function should be Closed() eventually.
func NewClient() (*Client, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return nil, err
	}
	return &Client{cli}, nil
}

// Purge removes all Docker objects created by norma.
func Purge(ctx context.Context) error {
	cli, err := NewClient()
	if err != nil {
		return err
	}

	// get all containers created by norma
	containers, err := cli.listContainers(ctx)
	if err != nil {
		return err
	}

	// remove all containers
	for _, c := range containers {
		// remove the container
		err = cli.cli.ContainerRemove(ctx, c.ID, container.RemoveOptions{Force: true})
		if err != nil {
			return err
		}
	}

	// get all networks created by norma
	networks, err := cli.listNetworks(ctx)
	if err != nil {
		return err
	}

	// remove all networks
	for _, n := range networks {
		err = cli.cli.NetworkRemove(ctx, n.ID)
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *Client) Close() error {
	return c.cli.Close()
}

// Start creates and runs one Container. The provided configuration allows
// to configure the Docker image to run inside the container -- and thus the
// services to be offered. When a Network is provided, the container's IP on
// that network is resolved and used to reach services directly, without
// port forwarding.
func (c *Client) Start(ctx context.Context, config *ContainerConfig) (*Container, error) {
	envVars := []string{}
	for key, value := range config.Environment {
		envVars = append(envVars, fmt.Sprintf("%s=%s", key, value))
	}

	var binds []string
	if config.DataDirBinding != nil {
		binds = append(binds, *config.DataDirBinding)
	}
	if config.GenesisFileBind != nil {
		// ensure the genesis file exists on the host before starting the container
		genesisPath := strings.Split(*config.GenesisFileBind, ":")[0]
		if _, err := os.Stat(genesisPath); err != nil {
			return nil, fmt.Errorf("genesis file %s does not exist: %w", genesisPath, err)
		}
		binds = append(binds, *config.GenesisFileBind)
	}
	if config.KeystoreBinding != nil {
		binds = append(binds, *config.KeystoreBinding)
	}

	init := true
	stopTimeout := int(config.ShutdownTimeout.Seconds())
	resp, err := c.cli.ContainerCreate(ctx, &container.Config{
		Image:      config.ImageName,
		Tty:        false,
		Env:        envVars,
		Entrypoint: config.Entrypoint,
		Labels: map[string]string{
			objectsLabel: "true",
		},
		StopTimeout: &stopTimeout,
	}, &container.HostConfig{
		Init:   &init,
		CapAdd: []string{"NET_ADMIN"},
		Binds:  binds,
	}, nil, nil, config.Hostname)
	if err != nil {
		return nil, err
	}

	if config.Network != nil {
		err := c.cli.NetworkConnect(ctx, config.Network.id, resp.ID, nil)
		if err != nil {
			return nil, err
		}
	}

	if err := network.Retry(ctx, network.DefaultRetryAttempts, 1*time.Second,
		func(ctx context.Context) error {
			return c.cli.ContainerStart(ctx, resp.ID, container.StartOptions{})
		}); err != nil {
		return nil, err
	}

	ctr := &Container{
		id:      resp.ID,
		client:  c,
		config:  config,
		stopped: false,
		cleaned: false,
	}

	if config.Network != nil {
		if err := ctr.resolveIP(); err != nil {
			return nil, err
		}
	}

	return ctr, nil
}

// CreateBridgeNetwork creates a new Docker bridge network.
func (c *Client) CreateBridgeNetwork(ctx context.Context) (*Network, error) {
	// generate random name for network
	name := fmt.Sprintf("norma_network_%d", rand.Int())

	// create new network
	resp, err := c.cli.NetworkCreate(ctx, name, dockerNetwork.CreateOptions{
		Labels: map[string]string{
			objectsLabel: "true",
		},
	})
	if err != nil {
		return nil, err
	}

	return &Network{
		id:     resp.ID,
		name:   name,
		client: c,
	}, nil
}

// CreateTestBridgeNetwork creates a Docker bridge network for use in
// tests. When t is non-nil a cleanup function is registered that removes
// the network after the test completes. When t is nil an error is
// returned.
func (c *Client) CreateTestBridgeNetwork(t *testing.T) *Network {
	t.Helper()
	dn, err := c.CreateBridgeNetwork(t.Context())
	if err != nil {
		t.Fatalf("failed to create docker network: %v", err)
	}
	t.Cleanup(func() {
		if err := dn.Cleanup(context.Background()); err != nil {
			t.Errorf("failed to cleanup docker network: %v", err)
		}
	})
	return dn
}

// Hostname returns the hostname of the Container. In this case it is the ID of the
// Docker Container.
func (c *Container) Hostname() string {
	// return the truncated container ID
	return c.id[:12]
}

// IsRunning returns true if the Container has not been stopped yet and is
// expected to offer its services.
func (c *Container) IsRunning() bool {
	return !c.stopped
}

// CheckRunning returns an error if the container process is no longer running,
// either because it exited on its own or because its state cannot be determined.
func (c *Container) CheckRunning(ctx context.Context) error {
	info, err := c.client.cli.ContainerInspect(ctx, c.id)
	if err != nil {
		return fmt.Errorf("failed to inspect container: %w", err)
	}
	if !info.State.Running {
		return fmt.Errorf("container exited with code %d", info.State.ExitCode)
	}
	return nil
}

// Stop terminates this container. Services within the container will be
// signaled about the upcoming termination followed by being killed after a set
// timeout (see ContainerConfig.ShutdownTimeout).
func (c *Container) Stop(ctx context.Context) error {
	if c.stopped {
		return nil
	}
	c.stopped = true
	timeout := int(c.config.ShutdownTimeout.Seconds())
	err := c.client.cli.ContainerStop(ctx, c.id, container.StopOptions{
		Signal: string(SigInt), Timeout: &timeout})

	// Release log subscribers so consumers tailing the log observe EOF
	// instead of blocking forever on a container that will not speak again.
	if logs := c.logSource(); logs != nil {
		logs.close()
	}
	return err
}

// Cleanup stops the container (unless it is already stopped) and frees any
// resources associated to it. After the operation, the Container is to be
// considered invalid.
func (c *Container) Cleanup(ctx context.Context) error {
	if c.cleaned {
		return nil
	}
	start := time.Now()
	defer func() {
		slog.Debug("container cleanup completed", "container", c.id, "duration", time.Since(start))
	}()
	if err := c.Stop(ctx); err != nil {
		return err
	}
	c.cleaned = true
	return c.client.cli.ContainerRemove(ctx, c.id, container.RemoveOptions{})
}

// GetAddressForService retrieves the Address of a service running in this
// Container. Services are reached via the container's IP on the Docker
// network using the service's internal port. If the IP was not resolved
// at start time, it is looked up on demand via container inspection.
func (c *Container) GetAddressForService(service *network.ServiceDescription) (*network.AddressPort, error) {
	if c.ip == "" {
		if err := c.resolveIP(); err != nil {
			return nil, fmt.Errorf("failed to resolve container IP: %w", err)
		}
	}
	res := network.AddressPort(fmt.Sprintf("%s:%d", c.ip, service.Port))
	return &res, nil
}

// IP returns the container's IP address on the Docker network.
func (c *Container) IP() string {
	return c.ip
}

// resolveIP inspects the container and populates c.ip. When the
// container was started with a specific network, only that network is
// considered; otherwise the first available IP is used.
func (c *Container) resolveIP() error {
	info, err := c.client.cli.ContainerInspect(context.Background(), c.id)
	if err != nil {
		return fmt.Errorf("failed to inspect container: %w", err)
	}
	if c.config.Network != nil {
		ep, ok := info.NetworkSettings.Networks[c.config.Network.name]
		if !ok || ep.IPAddress == "" {
			return fmt.Errorf("container has no IP on network %s", c.config.Network.name)
		}
		c.ip = ep.IPAddress
		return nil
	}
	for _, ep := range info.NetworkSettings.Networks {
		if ep.IPAddress != "" {
			c.ip = ep.IPAddress
			return nil
		}
	}
	return fmt.Errorf("container %s has no IP address", c.id)
}

// SaveLogTo fetches the log of the container and saves it to the given directory.
func (c *Container) SaveLogTo(ctx context.Context, directory string) error {
	dst := filepath.Join(directory,
		fmt.Sprintf("%s_%s.log", c.config.ImageName, c.id))

	// When the payload process runs as a background exec, its output is
	// not part of the container's stdout but is already captured in full
	// on the host; copy that file rather than the (empty) container log.
	c.execLogMutex.Lock()
	execLogPath := c.execLogPath
	c.execLogMutex.Unlock()
	if execLogPath != "" {
		return copyFile(execLogPath, dst)
	}

	opt := container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
	}

	// TODO if this proves insufficient, an alternative would be to mount certain directories from
	// the container to temp on the host and here just copy local directories
	reader, err := c.client.cli.ContainerLogs(ctx, c.id, opt)
	if err != nil {
		return err
	}
	defer func() { _ = reader.Close() }()

	file, err := os.Create(dst) //#nosec G304 -- path is constructed internally
	if err != nil {
		return err
	}

	_, err = stdcopy.StdCopy(file, file, reader)
	return errors.Join(err, file.Close())
}

// StreamLog returns a follow-mode reader over the container's log. Once a
// background exec has been started, its output is the log of interest and
// is served from the container's broadcaster; otherwise the container's
// own stdout is streamed.
func (c *Container) StreamLog(ctx context.Context) (io.ReadCloser, error) {
	if logs := c.logSource(); logs != nil {
		return logs.subscribe(ctx), nil
	}

	opt := container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
	}

	reader, err := c.client.cli.ContainerLogs(ctx, c.id, opt)
	if err != nil {
		return nil, err
	}

	return demuxContainerLog(reader), nil
}

// demuxContainerLog decodes docker's stream multiplexing protocol, which
// ContainerLogs uses whenever the container was created without a TTY.
// Without this, every frame's 8-byte binary header would end up inline in
// the log text and corrupt line-based parsing.
func demuxContainerLog(src io.ReadCloser) io.ReadCloser {
	pr, pw := io.Pipe()
	go func() {
		_, err := stdcopy.StdCopy(pw, pw, src)
		_ = src.Close()
		_ = pw.CloseWithError(err)
	}()
	return &demuxedLog{Reader: pr, closer: pr}
}

// demuxedLog ties the reader end of the demultiplexing pipe to its
// lifecycle, so closing it also unblocks the decoding goroutine.
type demuxedLog struct {
	io.Reader
	closer io.Closer
}

func (d *demuxedLog) Close() error {
	return d.closer.Close()
}

// copyFile copies the contents of src to dst.
func copyFile(src, dst string) error {
	in, err := os.Open(src) //#nosec G304 -- path is constructed internally
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	out, err := os.Create(dst) //#nosec G304 -- path is constructed internally
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	return errors.Join(err, out.Close())
}

// SendSignal sends a signal to the container.
func (c *Container) SendSignal(ctx context.Context, signal Signal) error {
	return c.client.cli.ContainerKill(ctx, c.id, string(signal))
}

// Exec executes a command in the container.
// This method is blocking until the command has finished.
// The output of the command is returned as a string (stdout + stderr).
// The command is required to be tokenized and interpreted in shell's exec form.
func (c *Container) Exec(ctx context.Context, cmd []string) (string, error) {
	return c.ExecWithOptions(ctx, cmd, ExecOptions{})
}

// ExecOptions carries the optional parameters of an exec invocation.
type ExecOptions struct {
	// Env are additional environment variables for the process. The
	// container's own environment is inherited regardless.
	Env []string
	// LogName, when set together with a configured LogsDir, persists the
	// process output to a timestamped file named
	// <hostname>_<LogName>_<timestamp>.log in that directory.
	LogName string
}

// ExecWithOptions executes a command in the container and blocks until it
// has finished, returning its interleaved stdout and stderr.
func (c *Container) ExecWithOptions(
	ctx context.Context,
	cmd []string,
	opts ExecOptions,
) (string, error) {
	execID, resp, err := c.startExec(ctx, cmd, opts.Env)
	if err != nil {
		return "", err
	}
	defer resp.Close()

	var output bytes.Buffer
	// stdout and stderr are demultiplexed into one buffer: callers treat
	// exec output as a single diagnostic stream, and keeping the relative
	// order of the two streams matters more than telling them apart.
	if _, err := stdcopy.StdCopy(&output, &output, resp.Reader); err != nil {
		return output.String(), fmt.Errorf("failed to read exec output: %w", err)
	}

	if opts.LogName != "" && c.config.LogsDir != nil {
		if _, writeErr := c.writeExecLog(opts.LogName, output.Bytes()); writeErr != nil {
			slog.Warn("failed to write exec log",
				"name", opts.LogName,
				"error", writeErr)
		}
	}

	execInspect, err := c.client.cli.ContainerExecInspect(ctx, execID)
	if err != nil {
		return output.String(), fmt.Errorf("failed to inspect exec instance: %w", err)
	}
	if execInspect.ExitCode != 0 {
		return output.String(),
			fmt.Errorf("command '%s' execution failed with exit code %d",
				strings.Join(cmd, " "), execInspect.ExitCode)
	}

	return output.String(), nil
}

// ExecBackground starts a long-running command in the container without
// waiting for it to finish. Its output is split into lines and forwarded
// both to a host-side log file (when opts.LogName and LogsDir are set)
// and to the container's log broadcaster, which is what StreamLog serves
// from once a background exec exists.
//
// The returned handle's Done channel is closed after the process exited,
// its output was drained and its exit status was recorded.
func (c *Container) ExecBackground(
	ctx context.Context,
	cmd []string,
	opts ExecOptions,
) (*ExecHandle, error) {
	logs := c.logBroadcasterForExec()

	var logFile *os.File
	logPath := ""
	if opts.LogName != "" && c.config.LogsDir != nil {
		f, path, err := c.createExecLogFile(opts.LogName)
		if err != nil {
			return nil, fmt.Errorf("failed to create exec log file: %w", err)
		}
		logFile, logPath = f, path
	}

	execID, resp, err := c.startExec(ctx, cmd, opts.Env)
	if err != nil {
		if logFile != nil {
			_ = logFile.Close()
		}
		return nil, err
	}

	c.execLogMutex.Lock()
	c.execLogPath = logPath
	c.execLogMutex.Unlock()

	done := make(chan struct{})
	handle := &ExecHandle{ExecID: execID, LogPath: logPath, Done: done}

	go func() {
		defer close(done)
		defer resp.Close()

		splitter := &lineSplitter{emit: func(line []byte) {
			if logFile != nil {
				if _, err := logFile.Write(line); err != nil {
					slog.Warn("failed to write exec log line",
						"path", logPath, "error", err)
				}
			}
			logs.publish(line)
		}}

		_, copyErr := stdcopy.StdCopy(splitter, splitter, resp.Reader)
		splitter.flush()

		if logFile != nil {
			if err := logFile.Close(); err != nil {
				slog.Warn("failed to close exec log file",
					"path", logPath, "error", err)
			}
		}

		if copyErr != nil && ctx.Err() == nil {
			handle.setResult(-1, fmt.Errorf(
				"failed to stream output of exec %s: %w", execID, copyErr))
			return
		}

		// The exit status is only available after the output stream ended.
		// Use a context detached from ctx so a cancelled scenario still
		// records why the process stopped.
		inspectCtx, cancel := context.WithTimeout(
			context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()

		inspect, err := c.client.cli.ContainerExecInspect(inspectCtx, execID)
		if err != nil {
			handle.setResult(-1, fmt.Errorf(
				"failed to inspect exec %s: %w", execID, err))
			return
		}
		if inspect.ExitCode != 0 {
			handle.setResult(inspect.ExitCode, fmt.Errorf(
				"command '%s' terminated with exit code %d",
				strings.Join(cmd, " "), inspect.ExitCode))
			return
		}
		handle.setResult(0, nil)
	}()

	return handle, nil
}

// startExec creates and attaches to an exec instance, returning its ID
// and the attached stream.
//
// Tty is deliberately left false: a pty would merge stdout and stderr
// into an undemultiplexable stream and make the client emit ANSI colour
// escapes, which the log parsers would then have to strip.
func (c *Container) startExec(
	ctx context.Context,
	cmd []string,
	env []string,
) (string, types.HijackedResponse, error) {
	execConfig := container.ExecOptions{
		Tty:          false,
		Cmd:          cmd,
		Env:          env,
		AttachStdout: true,
		AttachStderr: true,
	}
	execResp, err := c.client.cli.ContainerExecCreate(ctx, c.id, execConfig)
	if err != nil {
		return "", types.HijackedResponse{},
			fmt.Errorf("failed to create exec instance: %w", err)
	}
	if execResp.ID == "" {
		return "", types.HijackedResponse{},
			errors.New("failed to create exec instance: empty exec ID")
	}

	resp, err := c.client.cli.ContainerExecAttach(ctx, execResp.ID,
		container.ExecStartOptions{})
	if err != nil {
		return "", types.HijackedResponse{},
			fmt.Errorf("failed to attach to exec instance: %w", err)
	}
	return execResp.ID, resp, nil
}

// logBroadcasterForExec returns the container's log broadcaster,
// creating it on first use. Its existence is what switches StreamLog
// over from the container's stdout to the exec output.
func (c *Container) logBroadcasterForExec() *logBroadcaster {
	c.execLogMutex.Lock()
	defer c.execLogMutex.Unlock()
	if c.execLogs == nil {
		c.execLogs = newLogBroadcaster()
	}
	return c.execLogs
}

// logSource returns the broadcaster to read logs from, or nil when the
// container's own stdout is still the source.
func (c *Container) logSource() *logBroadcaster {
	c.execLogMutex.Lock()
	defer c.execLogMutex.Unlock()
	return c.execLogs
}

// writeExecLog writes output data to a timestamped log file and returns
// its path.
func (c *Container) writeExecLog(name string, data []byte) (string, error) {
	f, path, err := c.createExecLogFile(name)
	if err != nil {
		return "", err
	}
	if _, err := f.Write(data); err != nil {
		return path, errors.Join(err, f.Close())
	}
	return path, f.Close()
}

// createExecLogFile creates a new timestamped log file in the
// configured LogsDir and returns it along with its path.
func (c *Container) createExecLogFile(name string) (*os.File, string, error) {
	ts := time.Now().UTC().Format("20060102T150405.000Z")
	filename := fmt.Sprintf("%s_%s_%s.log", c.config.Hostname, name, ts)
	path := filepath.Join(*c.config.LogsDir, filename)
	f, err := os.Create(path) //#nosec G304 -- path is constructed internally
	if err != nil {
		return nil, "", err
	}
	return f, path, nil
}

// Cleanup removes the network from the Docker host.
func (n *Network) Cleanup(ctx context.Context) error {
	if n.cleaned {
		return nil
	}
	// remove all containers from the network, so we can remove the network
	containers, err := n.client.listContainers(ctx)
	if err != nil {
		return err
	}
	for _, c := range containers {
		for _, cn := range c.NetworkSettings.Networks {
			if cn.NetworkID == n.id {
				if err := n.client.cli.NetworkDisconnect(ctx, n.id, c.ID, true); err != nil {
					return err
				}
			}
		}
	}
	n.cleaned = true
	// remove the network
	return n.client.cli.NetworkRemove(ctx, n.id)
}

// listNetworks returns a list of all networks on the Docker host filtered by label.
func (c *Client) listNetworks(ctx context.Context) ([]dockerNetwork.Inspect, error) {
	return c.cli.NetworkList(ctx, dockerNetwork.ListOptions{
		Filters: filters.NewArgs(getObjectsLabelFilter()),
	})
}

// ContainerExists returns true if a container with the given name is
// currently running on the Docker host.
func (c *Client) ContainerExists(name string) (bool, error) {
	containers, err := c.cli.ContainerList(context.Background(), container.ListOptions{
		Filters: filters.NewArgs(filters.Arg("name", fmt.Sprintf("^/%s$", name))),
	})
	if err != nil {
		return false, err
	}
	return len(containers) > 0, nil
}

// listContainers returns a list of all containers (running and stopped) on the
// Docker host created by norma.
func (c *Client) listContainers(ctx context.Context) ([]types.Container, error) {
	return c.cli.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: filters.NewArgs(getObjectsLabelFilter()),
	})
}

// getObjectsLabelFilter returns a filter for the objects label.
func getObjectsLabelFilter() filters.KeyValuePair {
	return filters.Arg("label", fmt.Sprintf("%s=true", objectsLabel))
}
