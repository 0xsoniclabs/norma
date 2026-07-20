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

package driver

import (
	"context"
	"fmt"
	"time"

	"github.com/0xsoniclabs/carmen/go/common"
	"github.com/0xsoniclabs/norma/driver/parser"
	"github.com/0xsoniclabs/norma/driver/rpc"
	"github.com/0xsoniclabs/norma/genesis"
	"github.com/ethereum/go-ethereum/core/types"
)

//go:generate mockgen -source network.go -destination network_mock.go -package driver

// DefaultClientDockerImageName is the name of the docker image to use for clients.
const DefaultClientDockerImageName = "sonic"

// DefaultValidators is a default configuration for a single validator.
func DefaultValidators(name string) Validators {
	return NewDefaultTestValidators(name, 1)
}

// ResolveClientImageName returns imageName if set, otherwise the default client
// image name.
func ResolveClientImageName(imageName string) string {
	if imageName != "" {
		return imageName
	}
	return DefaultClientDockerImageName
}

const (
	// ErrEmptyNetwork is returned when trying to connect to an empty network.
	ErrEmptyNetwork = common.ConstError("network is empty")
)

// Network abstracts an execution environment for running scenarios.
// Implementations may run nodes and applications locally, in docker images, or
// remotely, on actual nodes. The interface is used by the scenario driver
// to execute scenario descriptions.
type Network interface {
	// CreateNode creates a new node instance running a network client based on
	// the given configuration. It is used by the scenario executor to add
	// nodes to the network as needed.
	CreateNode(config *NodeConfig) (Node, error)

	// RemoveNode ends the client gracefully and removes node from the network
	RemoveNode(Node) error

	// CreateApplication creates a new application in this network, ready to
	// produce load as defined by its configuration.
	CreateApplication(context.Context, *ApplicationConfig) (Application, error)

	// GetActiveNodes obtains a list of active nodes in the network.
	GetActiveNodes() []Node

	// GetActiveApplications obtains a list of active apps in the network.
	GetActiveApplications() []Application

	// RegisterListener registers a listener to receive updates on network
	// changes, for instance, to update monitoring information. Registering
	// the same listener more than once will have no effect.
	RegisterListener(NetworkListener)

	// UnregisterListener removes the given listener from this network.
	UnregisterListener(NetworkListener)

	// Shutdown stops all applications and nodes in the network and frees
	// any potential other resources.
	Shutdown() error

	// SendTransaction sends a transaction to the network.
	// The source parameter is used for logging and debugging purposes.
	SendTransaction(tx *types.Transaction, source string)

	// Create a connection to a random node on the network. May fail if there
	// is no node on the network with a ErrorEmptyNetwork error.
	DialRandomRpc() (rpc.Client, error)

	// ApplyNetworkRules applies the given network rules to the network.
	ApplyNetworkRules(ctx context.Context, rules NetworkRules) error

	// AdvanceEpoch advances an epoch by the given number.
	AdvanceEpoch(ctx context.Context, epochIncrement int) error

	// WaitForEpochChange waits until the epoch changes.
	WaitForEpochChange(ctx context.Context) error
}

// NetworkConfig is a collection of network parameters to be used by factories
// creating network instances.
type NetworkConfig struct {
	// Validators is a list of validators to start up in the network.
	Validators Validators
	// RoundTripTime is the average round trip time between nodes in the network.
	RoundTripTime time.Duration
	// NetworkRules is a map of network rules to be applied to the network.
	NetworkRules NetworkRules
	// OutputDir is the directory where temp data are written.
	OutputDir string
}

// NetworkRules defines a set of network rules that can be applied to the network.
// Network rules contains all the fields in sonic's opera.Rules, but all fields
// are optional and only the non-nil fields will be applied to the network.
//
// This type is used to define the initial rule set in the genesis, by applying
// the diff to the default rules: opera.FakeNetRules(opera.GetSonicUpgrades())
// Additionally it can be sent serialized using json to change the network rules
// during execution.
type NetworkRules = genesis.NetworkRulesPatch

// NetworkListener can be registered to networks to get callbacks whenever there
// are changes in the network.
type NetworkListener interface {
	// AfterNodeCreation is called whenever a new node has joined the network.
	AfterNodeCreation(Node)
	// BeforeNodeRemoval is called whenever a node is removed from the network.
	BeforeNodeRemoval(Node)
	// AfterApplicationCreation is called after a new application has started.
	AfterApplicationCreation(Application)
}

type NodeConfig struct {
	Name           string
	Failing        bool
	Validator      bool
	ValidatorId    *int
	Cheater        bool
	Image          string
	DataVolume     *string
	ExtraArguments string
}

type ApplicationConfig struct {
	Name string

	// Type defines the on-chain app which should generate the traffic.
	Type string

	// Rate defines the Tx/s config the source should produce while active.
	Rate *parser.Rate

	// Users defines the number of users sending transactions to the app.
	Users int

	// TODO: add other parameters as needed
	//  - application type
}

// Validator is a configuration for a group of network start-up validators.
type Validator struct {
	Name           string
	Failing        bool
	Instances      int
	ImageName      string
	Stake          uint64 // < Stake in FTM
	ExtraArguments string // Extra command line arguments for sonicd
}

type Validators []Validator

// NewDefaultValidators creates a new Validators with a single validator defining only the number of instances,
// using the default client docker image.
func NewDefaultValidators(instances int) Validators {
	return []Validator{{Name: "validator", Instances: instances, ImageName: DefaultClientDockerImageName}}
}

func NewDefaultTestValidators(name string, instances int) Validators {
	return []Validator{{Name: fmt.Sprintf("validator-%s", name), Instances: instances, ImageName: DefaultClientDockerImageName}}
}

func (v Validators) GetNumValidators() int {
	num := 0
	for _, val := range v {
		num += val.Instances
	}
	return num
}
