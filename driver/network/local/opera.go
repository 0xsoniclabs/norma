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
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"slices"
	"time"

	"golang.org/x/exp/maps"

	"github.com/0xsoniclabs/norma/driver/node"
	rpcdriver "github.com/0xsoniclabs/norma/driver/rpc"
	"github.com/0xsoniclabs/norma/genesistools/genesis"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/docker"
	"github.com/0xsoniclabs/norma/driver/network"
	"github.com/ethereum/go-ethereum/rpc"
)

// operaNode implements the driver's Node interface by running a go-opera
// client on a generic host.
type operaNode struct {
	host      network.Host
	container *docker.Container
	config    *operaNodeConfig
}

type operaNodeConfig struct {
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
	// ExtraArguments are additional command line arguments to pass to the node.
	ExtraArguments string
}

// labelPattern restricts labels for nodes to non-empty alpha-numerical strings
// with underscores and hyphens.
var labelPattern = regexp.MustCompile("[A-Za-z0-9_-]+")

// startOperaDockerNode creates a new operaNode running in a Docker container.
func startOperaDockerNode(ctx context.Context, client *docker.Client, dn *docker.Network, config *operaNodeConfig) (*operaNode, error) {
	if !labelPattern.Match([]byte(config.Label)) {
		return nil, fmt.Errorf("invalid label for node: '%v'", config.Label)
	}

	shutdownTimeout := 180 * time.Second

	validatorId := "0"
	if config.ValidatorId != nil {
		validatorId = fmt.Sprintf("%d", *config.ValidatorId)
	}

	host, err := network.RetryReturn(ctx, network.DefaultRetryAttempts, 1*time.Second, func() (*docker.Container, error) {
		ports, err := network.GetFreePorts(len(node.OperaServices.Services()))
		if err != nil {
			return nil, err
		}

		portForwarding := make(map[network.Port]network.Port, len(ports))
		for i, service := range node.OperaServices.Services() {
			portForwarding[service.Port] = ports[i]
		}

		stakes := []uint64{}
		for _, val := range config.NetworkConfig.Validators {
			for range max(val.Instances, 1) {
				if val.Stake == 0 {
					// if no stake defined, use default
					stakes = append(stakes, 5_000_000)
					continue
				}
				stakes = append(stakes, uint64(val.Stake))
			}
		}

		envs := map[string]string{
			"VALIDATOR_ID":      validatorId,
			"VALIDATORS_COUNT":  fmt.Sprintf("%d", config.NetworkConfig.Validators.GetNumValidators()),
			"VALIDATORS_STAKES": genesis.GetStakesString(stakes),
			"NETWORK_LATENCY":   fmt.Sprintf("%v", config.NetworkConfig.RoundTripTime/2),
			"EXTRA_ARGUMENTS":   config.ExtraArguments,
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

		maps.Copy(envs, config.NetworkConfig.NetworkRules) // put in the network rules

		return client.Start(&docker.ContainerConfig{
			ImageName:       config.Image,
			ShutdownTimeout: &shutdownTimeout,
			PortForwarding:  portForwarding,
			Environment:     envs,
			Network:         dn,
			DataDirBinding:  dataDirBinding,
		})
	})

	if err != nil {
		return nil, err
	}

	// Use a private copy of the config to avoid modifying the original.
	nodeConfig := *config
	if config.ValidatorId != nil {
		nodeConfig.ValidatorId = new(int)
		*nodeConfig.ValidatorId = *config.ValidatorId
	}
	node := &operaNode{
		host:      host,
		container: host,
		config:    &nodeConfig,
	}

	// Wait until the operaNode inside the Container is ready.
	err = network.Retry(ctx, network.DefaultRetryAttempts, 1*time.Second, func() error {
		if err := node.host.CheckRunning(); err != nil {
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
		printLog(node),
		fmt.Errorf("failed to get node online, %w", err),
		node.host.Cleanup(),
	)
}

// printLog streams and prints the logs of the given operaNode, to debug cause of
// startup failure.
func printLog(node *operaNode) error {
	reader, err := node.StreamLog()
	if err != nil {
		return fmt.Errorf("cannot read node logs: %w", err)
	}
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		fmt.Printf("[Opera Node %s] %s\n", node.GetLabel(), scanner.Text())
	}
	return reader.Close()
}

func (n *operaNode) GetLabel() string {
	return n.config.Label
}

func (n *operaNode) IsExpectedFailure() bool {
	return n.config.Failing
}

// Hostname returns the hostname of the node.
// The hostname is accessible only inside the Docker network.
func (n *operaNode) Hostname() string {
	return n.host.Hostname()
}

// MetricsPort returns the port on which the node exports its metrics.
// The port is accessible only inside the Docker network.
func (n *operaNode) MetricsPort() int {
	return 6060
}

func (n *operaNode) IsRunning() bool {
	return n.host.IsRunning()
}

func (n *operaNode) GetServiceUrl(service *network.ServiceDescription) *driver.URL {
	addr := n.host.GetAddressForService(service)
	if addr == nil {
		return nil
	}
	url := driver.URL(fmt.Sprintf("%s://%s", service.Protocol, *addr))
	return &url
}

func (n *operaNode) GetNodeID() (driver.NodeID, error) {
	url := n.GetServiceUrl(&node.OperaRpcService)
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

func (n *operaNode) GetValidatorId() *int {
	return n.config.ValidatorId
}

func (n *operaNode) StreamLog() (io.ReadCloser, error) {
	return n.host.StreamLog()
}

func (n *operaNode) Stop() error {
	return n.host.Stop()
}

func (n *operaNode) Cleanup() error {
	return n.host.Cleanup()
}

func (n *operaNode) DialRpc() (rpcdriver.Client, error) {
	url := n.GetServiceUrl(&node.OperaRpcService)
	if url == nil {
		return nil, fmt.Errorf("node %s does not export an RPC server", n.GetLabel())
	}

	rpcClient, err := network.RetryReturn(context.Background(), network.DefaultRetryAttempts, 1*time.Second, func() (*rpc.Client, error) {
		return rpc.DialContext(context.Background(), string(*url))
	})
	if err != nil {
		return nil, fmt.Errorf("failed to dial RPC for node %s; %v", n.GetLabel(), err)
	}
	return rpcdriver.WrapRpcClient(rpcClient), nil
}

// AddPeer informs the client instance represented by the operaNode about the
// existence of another node, to which it may establish a connection.
func (n *operaNode) AddPeer(id driver.NodeID) error {
	rpcClient, err := n.DialRpc()
	if err != nil {
		return err
	}
	return network.Retry(context.Background(), network.DefaultRetryAttempts, 1*time.Second, func() error {
		return rpcClient.Call(nil, "admin_addPeer", id)
	})
}

// RemovePeer informs the client instance represented by the operaNode
// that the input node is no more available in the network.
func (n *operaNode) RemovePeer(id driver.NodeID) error {
	rpcClient, err := n.DialRpc()
	if err != nil {
		return err
	}
	return network.Retry(context.Background(), network.DefaultRetryAttempts, 1*time.Second, func() error {
		return rpcClient.Call(nil, "admin_removePeer", id)
	})
}

// Kill sends a SigKill singal to node.
func (n *operaNode) Kill() error {
	return n.container.SendSignal(docker.SigKill)
}

// GetRoundTripTime returns the median network round-trip time to the given host.
func (n *operaNode) GetRoundTripTime(host string) (time.Duration, error) {
	output, err := n.container.Exec([]string{"ping", "-c", "5", host})
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
