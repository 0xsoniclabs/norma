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
	"context"
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
}

// ExecHandle represents a background exec process running inside a
// container. It provides access to the exec ID and a channel that is
// closed when the output streaming goroutine finishes.
//
// Note: Docker's ContainerExecInspect.Pid returns the host-namespace
// PID, which is not usable for `kill` executed inside the container
// (different PID namespace). Callers that need to signal the process
// must discover its container-namespace PID from inside the container
// (see OperaNode.signalSonicd).
type ExecHandle struct {
	ExecID string
	Done   <-chan struct{}
	mu     sync.Mutex
	err    error
}

// Err returns the error encountered by the background streaming
// goroutine, if any. It is safe to call after <-Done.
func (h *ExecHandle) Err() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.err
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
	return c.client.cli.ContainerStop(ctx, c.id, container.StopOptions{
		Signal: string(SigInt), Timeout: &timeout})
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

	file, err := os.Create(fmt.Sprintf("%s/%s_%s.log", directory, c.config.ImageName, c.id))
	if err != nil {
		return err
	}

	_, err = io.Copy(file, reader)
	if err != nil {
		return err
	}

	return nil
}

func (c *Container) StreamLog(ctx context.Context) (io.ReadCloser, error) {
	opt := container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
	}

	reader, err := c.client.cli.ContainerLogs(ctx, c.id, opt)
	if err != nil {
		return nil, err
	}

	return reader, nil
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
	return c.ExecWithEnv(ctx, cmd, nil, "")
}

// ExecWithEnv executes a command in the container with the given
// environment variables. If logName is non-empty and a LogsDir was
// configured, the output is also written to a timestamped log file
// named <logName>_<timestamp>.log inside that directory.
func (c *Container) ExecWithEnv(
	ctx context.Context,
	cmd []string,
	env []string,
	logName string,
) (string, error) {
	execConfig := container.ExecOptions{
		Tty:          true,
		Cmd:          cmd,
		Env:          env,
		AttachStdout: true,
		AttachStderr: true,
	}
	execResp, err := c.client.cli.ContainerExecCreate(ctx, c.id, execConfig)
	if err != nil {
		return "", fmt.Errorf("failed to create exec instance: %s", err)
	}

	// Check if any error occurred during exec creation
	if execResp.ID == "" {
		return "", fmt.Errorf("failed to create exec instance: Empty exec ID")
	}

	// Attach to the exec instance
	resp, err := c.client.cli.ContainerExecAttach(ctx, execResp.ID, container.ExecStartOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to attach to exec instance: %s", err)
	}
	defer resp.Close()

	// Capture the output and errors
	output, err := io.ReadAll(resp.Reader)
	if err != nil {
		return "", fmt.Errorf("failed to read exec output: %s", err)
	}

	// Persist output to a log file when configured.
	if logName != "" && c.config.LogsDir != nil {
		if writeErr := c.writeExecLog(logName, output); writeErr != nil {
			slog.Warn("failed to write exec log",
				"name", logName,
				"error", writeErr)
		}
	}

	execInspect, err := c.client.cli.ContainerExecInspect(ctx, execResp.ID)
	if err != nil {
		return string(output), fmt.Errorf("failed to inspect exec instance: %s", err)
	}
	if execInspect.ExitCode != 0 {
		return string(output),
			fmt.Errorf("command '%s' execution failed with exit code %d",
				strings.Join(cmd, " "), execInspect.ExitCode)
	}

	return string(output), nil
}

// ExecBackground starts a long-running command in the container and
// streams its output to the given file on the host. The method returns
// immediately with an ExecHandle. The streaming goroutine runs until
// the command exits or the context is cancelled.
func (c *Container) ExecBackground(
	ctx context.Context,
	cmd []string,
	env []string,
	logName string,
) (*ExecHandle, error) {
	execConfig := container.ExecOptions{
		Tty:          true,
		Cmd:          cmd,
		Env:          env,
		AttachStdout: true,
		AttachStderr: true,
	}
	execResp, err := c.client.cli.ContainerExecCreate(ctx, c.id, execConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create background exec: %s", err)
	}
	if execResp.ID == "" {
		return nil, fmt.Errorf("failed to create background exec: Empty exec ID")
	}

	resp, err := c.client.cli.ContainerExecAttach(ctx,
		execResp.ID,
		container.ExecStartOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to attach to background exec: %s", err)
	}

	done := make(chan struct{})
	handle := &ExecHandle{ExecID: execResp.ID, Done: done}

	go func() {
		defer close(done)
		defer resp.Close()

		w := io.Discard
		if logName != "" && c.config.LogsDir != nil {
			f, ferr := c.createExecLogFile(logName)
			if ferr != nil {
				slog.Warn("failed to create background exec log",
					"name", logName,
					"error", ferr)
			}
			defer f.Close()
			w = f
		}

		_, copyErr := io.Copy(w, resp.Reader)
		if copyErr != nil && ctx.Err() == nil {
			handle.mu.Lock()
			handle.err = copyErr
			handle.mu.Unlock()
		}
	}()

	return handle, nil
}

// writeExecLog writes output data to a timestamped log file.
func (c *Container) writeExecLog(name string, data []byte) error {
	f, err := c.createExecLogFile(name)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(data)
	return err
}

// createExecLogFile creates a new timestamped log file in the
// configured LogsDir.
func (c *Container) createExecLogFile(name string) (*os.File, error) {
	ts := time.Now().UTC().Format("20060102T150405Z")
	filename := fmt.Sprintf("%s_%s_%s.log", c.config.Hostname, name, ts)
	path := filepath.Join(*c.config.LogsDir, filename)
	return os.Create(path) //#nosec G304 -- path is constructed internally
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
