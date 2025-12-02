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
	"regexp"
	"slices"
	"time"

	"golang.org/x/exp/maps"

	rpcdriver "github.com/0xsoniclabs/norma/driver/rpc"
	"github.com/0xsoniclabs/norma/genesistools/genesis"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/docker"
	"github.com/0xsoniclabs/norma/driver/network"
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
}

// labelPattern restricts labels for nodes to non-empty alpha-numerical strings
// with underscores and hyphens.
var labelPattern = regexp.MustCompile("[A-Za-z0-9_-]+")

// StartOperaDockerNode creates a new OperaNode running in a Docker container.
func StartOperaDockerNode(client *docker.Client, dn *docker.Network, config *OperaNodeConfig) (*OperaNode, error) {
	if !labelPattern.Match([]byte(config.Label)) {
		return nil, fmt.Errorf("invalid label for node: '%v'", config.Label)
	}

	shutdownTimeout := 180 * time.Second

	validatorId := "0"
	if config.ValidatorId != nil {
		validatorId = fmt.Sprintf("%d", *config.ValidatorId)
	}

	host, err := network.RetryReturn(network.DefaultRetryAttempts, 1*time.Second, func() (*docker.Container, error) {
		ports, err := network.GetFreePorts(len(operaServices.Services()))
		if err != nil {
			return nil, err
		}

		portForwarding := make(map[network.Port]network.Port, len(ports))
		for i, service := range operaServices.Services() {
			portForwarding[service.Port] = ports[i]
		}

		stakes := []uint64{}
		for _, val := range config.NetworkConfig.Validators {
			for range max(val.Instances, 1) {
				stakes = append(stakes, uint64(val.Stake))
			}
		}

		envs := map[string]string{
			"VALIDATOR_ID":      validatorId,
			"VALIDATORS_COUNT":  fmt.Sprintf("%d", config.NetworkConfig.Validators.GetNumValidators()),
			"VALIDATORS_STAKES": genesis.GetStakesString(stakes),
			"NETWORK_LATENCY":   fmt.Sprintf("%v", config.NetworkConfig.RoundTripTime/2),
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
	node := &OperaNode{
		host:      host,
		container: host,
		config:    &nodeConfig,
	}

	// Wait until the OperaNode inside the Container is ready.
	err = network.Retry(network.DefaultRetryAttempts, 1*time.Second, func() error {
		_, err := node.GetNodeID()
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

// printLog streams and prints the logs of the given OperaNode, to debug cause of
// startup failure.
func printLog(node *OperaNode) error {
	reader, err := node.StreamLog()
	if err != nil {
		return fmt.Errorf("cannot read node logs: %e", err)
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

func (n *OperaNode) StreamLog() (io.ReadCloser, error) {
	return n.host.StreamLog()
}

func (n *OperaNode) Stop() error {
	return n.host.Stop()
}

func (n *OperaNode) Cleanup() error {
	return n.host.Cleanup()
}

func (n *OperaNode) DialRpc() (rpcdriver.Client, error) {
	url := n.GetServiceUrl(&OperaRpcService)
	if url == nil {
		return nil, fmt.Errorf("node %s does not export an RPC server", n.GetLabel())
	}

	rpcClient, err := network.RetryReturn(network.DefaultRetryAttempts, 1*time.Second, func() (*rpc.Client, error) {
		return rpc.DialContext(context.Background(), string(*url))
	})
	if err != nil {
		return nil, fmt.Errorf("failed to dial RPC for node %s; %v", n.GetLabel(), err)
	}
	return rpcdriver.WrapRpcClient(rpcClient), nil
}

// AddPeer informs the client instance represented by the OperaNode about the
// existence of another node, to which it may establish a connection.
func (n *OperaNode) AddPeer(id driver.NodeID) error {
	rpcClient, err := n.DialRpc()
	if err != nil {
		return err
	}
	return network.Retry(network.DefaultRetryAttempts, 1*time.Second, func() error {
		return rpcClient.Call(nil, "admin_addPeer", id)
	})
}

// RemovePeer informs the client instance represented by the OperaNode
// that the input node is no more available in the network.
func (n *OperaNode) RemovePeer(id driver.NodeID) error {
	rpcClient, err := n.DialRpc()
	if err != nil {
		return err
	}
	return network.Retry(network.DefaultRetryAttempts, 1*time.Second, func() error {
		return rpcClient.Call(nil, "admin_removePeer", id)
	})
}

// Kill sends a SigKill singal to node.
func (n *OperaNode) Kill() error {
	return n.container.SendSignal(docker.SigKill)
}

// GetRoundTripTime returns the median network round-trip time to the given host.
func (n *OperaNode) GetRoundTripTime(host string) (time.Duration, error) {
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
