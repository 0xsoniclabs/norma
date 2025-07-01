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
	"time"

	"github.com/0xsoniclabs/norma/driver/parser"
	"github.com/0xsoniclabs/norma/driver/rpc"
	"github.com/ethereum/go-ethereum/core/types"
)

//go:generate mockgen -source network.go -destination network_mock.go -package driver

// DefaultClientDockerImageName is the name of the docker image to use for clients.
const DefaultClientDockerImageName = "sonic"

// DefaultValidators is a default configuration for a single validator.
var DefaultValidators = NewDefaultValidators(1)

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
	CreateApplication(config *ApplicationConfig) (Application, error)

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

	SendTransaction(tx *types.Transaction)

	DialRandomRpc() (rpc.Client, error)

	// ApplyNetworkRules applies the given network rules to the network.
	ApplyNetworkRules(rules NetworkRules) error

	// AdvanceEpoch advances an epoch by the given number.
	AdvanceEpoch(epochIncrement int) error
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
type NetworkRules map[string]string

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
	Name       string
	Failing    bool
	Validator  bool
	Cheater    bool
	Image      string
	DataVolume *string
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
	Name      string
	Failing   bool
	Instances int
	ImageName string
}

// NewValidator creates a new Validator from a parser.Validator.
func NewValidator(v parser.Validator) Validator {
	instances := 1
	if v.Instances != nil {
		instances = *v.Instances
	}
	imageName := DefaultClientDockerImageName
	if v.ImageName != "" {
		imageName = v.ImageName
	}
	return Validator{
		Name:      v.Name,
		Failing:   v.Failing,
		Instances: instances,
		ImageName: imageName,
	}
}

type Validators []Validator

// NewDefaultValidators creates a new Validators with a single validator defining only the number of instances,
// using the default client docker image.
func NewDefaultValidators(instances int) Validators {
	return []Validator{{Name: "validator", Instances: instances, ImageName: DefaultClientDockerImageName}}
}

// NewValidators creates a new Validators from a parser.Validators.
func NewValidators(v []parser.Validator) Validators {
	if len(v) == 0 {
		return NewDefaultValidators(1)
	}

	validators := make([]Validator, len(v))
	for i, val := range v {
		validators[i] = NewValidator(val)
	}
	return validators
}

func (v Validators) GetNumValidators() int {
	num := 0
	for _, val := range v {
		num += val.Instances
	}
	return num
}
