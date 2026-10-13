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

package steps

import (
	"crypto/ecdsa"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/checking"
	"github.com/ethereum/go-ethereum/common"
)

// Env is what steps run against: the network, and the state the steps of a
// scenario build up for the ones after them.
type Env struct {
	Net        driver.Network
	Checks     checking.Checks
	Validators ValidatorRegistry

	// Nodes maps node labels to the active nodes.
	Nodes map[string]driver.Node
	// Apps maps app names to the running applications.
	Apps map[string]driver.Application
	// NodeHistory holds every label started before, to detect a rejoin.
	NodeHistory map[string]bool
	// ValidatorIds keeps the validator id of each validator label, including
	// stopped ones, so they can rejoin under it.
	ValidatorIds map[string]int
	// Stakes is the stake moved through named delegator accounts.
	Stakes Ledger
}

// NewEnv returns an Env with no nodes, apps or stakes.
func NewEnv(net driver.Network, checks checking.Checks, validators ValidatorRegistry) *Env {
	return &Env{
		Net:          net,
		Checks:       checks,
		Validators:   validators,
		Nodes:        map[string]driver.Node{},
		Apps:         map[string]driver.Application{},
		NodeHistory:  map[string]bool{},
		ValidatorIds: map[string]int{},
		Stakes: Ledger{
			Delegators: map[string]*Delegator{},
			Expected:   map[StakeKey]uint64{},
		},
	}
}

// Ledger tracks the external delegator accounts a scenario created and the
// stake each is expected to hold, for verifyStakes to compare with the chain.
type Ledger struct {
	// Delegators maps scenario-scoped delegator names to their accounts,
	// created on first use.
	Delegators map[string]*Delegator
	// Expected is the running stake (in S) of every pair touched so far.
	// Fully undelegated pairs stay tracked with an expected stake of 0.
	Expected map[StakeKey]uint64
}

// StakeKey identifies a tracked (delegator, validator) stake pair.
type StakeKey struct {
	Delegator   string
	ValidatorId int
}

// Delegator is a named external delegator account.
type Delegator struct {
	Name       string
	PrivateKey *ecdsa.PrivateKey
	Address    common.Address
}
