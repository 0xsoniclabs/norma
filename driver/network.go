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
//
// This is "sonic:local", which builds from the sonic submodule, so a scenario
// that omits imageName tests the local sources. The alternative, "sonic",
// builds from the remote repository's default branch and therefore ignores the
// checked-out submodule entirely; scenarios that want that must ask for it
// explicitly.
const DefaultClientDockerImageName = "sonic:local"

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

	// SendTransaction sends a transaction to the network, through the RPC
	// interface of whichever node takes it first. The source identifies which
	// load generator produced it, for diagnostics and for attributing it to an
	// application in the monitoring data.
	SendTransaction(tx *types.Transaction, source TransactionSource)

	// SendTransactionTo sends a transaction to the network through the RPC
	// interface of the node with the given label, and nowhere else. It is what an
	// application pinned to a node needs; see ApplicationConfig.RpcNode.
	SendTransactionTo(label string, tx *types.Transaction, source TransactionSource)

	// RegisterTransactionObserver registers an observer to be notified about
	// every transaction submitted through SendTransaction.
	RegisterTransactionObserver(TransactionObserver)

	// Create a connection to a random node on the network. May fail if there
	// is no node on the network with a ErrorEmptyNetwork error.
	DialRandomRpc() (rpc.Client, error)

	// ApplyNetworkRules applies the given network rules to the network.
	ApplyNetworkRules(ctx context.Context, rules NetworkRules) error

	// AdvanceEpoch advances an epoch by the given number.
	AdvanceEpoch(ctx context.Context, epochIncrement int) error

	// WaitForEpochChange waits until the epoch changes.
	WaitForEpochChange(ctx context.Context) error

	// ReconnectNode re-establishes peer connections between the given
	// node and the rest of the network. Use after an in-place sonicd
	// restart where the node's peer table was lost.
	ReconnectNode(ctx context.Context, node Node) error

	// SuspendNode notifies listeners that the given node is temporarily
	// unavailable (e.g. sonicd was killed). Monitoring sensors will stop
	// polling this node until ResumeNode is called.
	SuspendNode(node Node)

	// ResumeNode notifies listeners that the given node is available
	// again (e.g. sonicd was restarted after heal). Monitoring sensors
	// will resume polling this node.
	ResumeNode(node Node)
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

// TransactionSource identifies the load generator a transaction originates
// from: the application it belongs to and which of the application's users
// created it.
type TransactionSource struct {
	App  string
	User int
}

func (s TransactionSource) String() string {
	return fmt.Sprintf("%s/user-%d", s.App, s.User)
}

// TransactionObserver can be registered to networks to be notified about
// transactions submitted to them.
type TransactionObserver interface {
	// OnTransactionSubmitted is called once a transaction has been handed to a
	// node's RPC interface, reporting the moment of the submission and the error
	// the node responded with, if any. Note that this is not the moment the
	// transaction was created, which may be earlier if submissions are queuing
	// up, and that a transaction rejected here never reaches a transaction pool.
	OnTransactionSubmitted(source TransactionSource, tx *types.Transaction, at time.Time, err error)
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

	// Load names the traffic of application types that carry another type's
	// instead of defining their own, such as the priority lanes. It is empty for
	// every other type.
	Load string

	// RpcNode is the label of the node whose RPC interface receives every
	// transaction of this application. Empty, the default, submits through any
	// node of the network, which is what a load that measures the network as a
	// whole wants; naming one node is for asking what the network does with
	// traffic that enters it in a single place.
	RpcNode string

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
