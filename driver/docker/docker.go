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
	"math/rand"
	"os"
	"strings"
	"time"

	"github.com/0xsoniclabs/norma/driver/network"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	dockerNetwork "github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
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

// Container represents a Docker Container, typically used for running a
// Fantom network Node, thus an instance of the go-opera client.
// *Container implements the driver.Host interface.
type Container struct {
	id      string
	client  *Client
	config  *ContainerConfig
	stopped bool
	cleaned bool
}

// ContainerConfig defines parameters for running Docker Containers.
type ContainerConfig struct {
	Hostname        string
	ImageName       string
	ShutdownTimeout *time.Duration
	PortForwarding  map[network.Port]network.Port // Container Port => Host Port
	Environment     map[string]string
	Entrypoint      []string // Entrypoint to run when starting the container. Optional.
	Network         *Network // Docker network to join, nil to join bridge network
	DataDirBinding  *string  // mount client datadir to this path on host
	GenesisFileBind *string  // mount genesis file on host to /genesis.json:ro in container
	KeystoreBinding *string  // mount keystore dir on host to /datadir/keystore:ro in container
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
// services to be offered -- and port-forwarding specifications to make those
// services reachable from outside the Docker container (e.g. by the
// application running this code).
func (c *Client) Start(ctx context.Context, config *ContainerConfig) (*Container, error) {
	envVars := []string{}
	for key, value := range config.Environment {
		envVars = append(envVars, fmt.Sprintf("%s=%s", key, value))
	}

	portMapping := nat.PortMap{}
	for inner, outer := range config.PortForwarding {
		portMapping[nat.Port(fmt.Sprintf("%d/tcp", inner))] = []nat.PortBinding{{
			HostIP:   "0.0.0.0",
			HostPort: fmt.Sprintf("%d/tcp", outer),
		}}
	}

	var binds []string
	if config.DataDirBinding != nil {
		binds = append(binds, *config.DataDirBinding)
	}
	if config.GenesisFileBind != nil {
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
		PortBindings: portMapping,
		Init:         &init,
		CapAdd:       []string{"NET_ADMIN"},
		Binds:        binds,
	}, nil, nil, config.Hostname)
	if err != nil {
		return nil, err
	}

	// connect to custom network if specified
	// this way the container will be connected to bridge network and
	// custom network at the same time (otherwise on network cleanup the
	// forwarded ports would be lost)
	if config.Network != nil {
		err = c.cli.NetworkConnect(ctx, config.Network.id, resp.ID, nil)
		if err != nil {
			return nil, err
		}
	}

	if err := network.Retry(ctx, network.DefaultRetryAttempts, 1*time.Second, func() error {
		return c.cli.ContainerStart(ctx, resp.ID, container.StartOptions{})
	}); err != nil {
		return nil, err
	}

	return &Container{resp.ID, c, config, false, false}, nil
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
	if err := c.Stop(ctx); err != nil {
		return err
	}
	c.cleaned = true
	return c.client.cli.ContainerRemove(ctx, c.id, container.RemoveOptions{})
}

// GetAddressForService retrieves the Address of a service running in this
// Container and being exported to the Docker's host environment. If there is
// no such service (e.g., because it was not marked as to be exported during
// the Start of the Container), nil will be returned.
func (c *Container) GetAddressForService(service *network.ServiceDescription) *network.AddressPort {
	// All services inside the container are reached through port-forwarding
	// on the localhost. Non-forwarded services are not supported.
	port, ok := c.config.PortForwarding[service.Port]
	if !ok {
		return nil
	}
	res := network.AddressPort(fmt.Sprintf("%s:%d", "0.0.0.0", port))
	return &res
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
	// Create a container exec instance
	execConfig := container.ExecOptions{
		Tty:          true,
		Cmd:          cmd,
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

	// Wait for the exec command to finish
	execInspect, err := c.client.cli.ContainerExecInspect(ctx, execResp.ID)
	if err != nil {
		return (string)(output), fmt.Errorf("failed to inspect exec instance: %s", err)
	}

	// Check the exit code of the executed command
	if execInspect.ExitCode != 0 {
		return (string)(output), fmt.Errorf(
			"command '%s' execution failed with exit code %d", strings.Join(cmd, " "), execInspect.ExitCode)
	}

	return (string)(output), nil
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
