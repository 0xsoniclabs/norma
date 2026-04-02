// Copyright 2026 Fantom Foundation
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

package app

import (
	"fmt"
	"math/rand"
	"sync/atomic"

	"github.com/0xsoniclabs/norma/driver/rpc"
	"github.com/ethereum/go-ethereum/core/types"
)

// mixEntry pairs an application type name with a relative weight.
// The probability of picking an app equals its weight divided by the sum of
// all weights.
type mixEntry struct {
	appType string
	weight  int
}

// mixAppTypes lists the application types included in the mix together with
// their relative selection weights.
var mixAppTypes = []mixEntry{
	{"erc20", 10},
	{"counter", 10},
	{"store", 10},
	{"uniswap", 5},
	{"smartaccount", 1},
	{"subsidies", 1},
	{"transient", 1},
	{"selfdestructor", 1},
	{"instantselfdestructor", 1},
	{"bundlesubsidy", 4},
}

// MixApplication initialises one instance of every application type and
// dispatches transactions across all of them with weighted random selection.
type MixApplication struct {
	apps        []Application
	totalWeight int
	// cumulativeWeights[i] is the sum of weights[0..i] (exclusive upper bound
	// for app i when sampling in [0, totalWeight)).
	cumulativeWeights []int
}

func NewMixApplication(appContext AppContext, feederId, appId uint32) (Application, error) {
	apps := make([]Application, 0, len(mixAppTypes))
	cumulativeWeights := make([]int, 0, len(mixAppTypes))
	totalWeight := 0

	for i, entry := range mixAppTypes {
		if entry.weight <= 0 {
			return nil, fmt.Errorf("mix: weight for %q must be positive, got %d", entry.appType, entry.weight)
		}
		factory := getFactory(entry.appType)
		if factory == nil {
			return nil, fmt.Errorf("mix: unknown application type %q", entry.appType)
		}
		// Sub-app IDs are placed in the upper half of the uint32 space to avoid
		// colliding with the sequentially assigned appIds of standalone apps in
		// the same scenario (which start from 0 and stay small).
		// Layout: [mixAppIdOffset + appId*len(mixAppTypes) + i]
		// mixAppIdOffset ensures sub-app IDs never overlap with scenario appIds
		// even when the mix app itself has appId=0.
		const mixAppIdOffset = 1 << 16
		subAppId := mixAppIdOffset + appId*uint32(len(mixAppTypes)) + uint32(i)
		a, err := factory(appContext, feederId, subAppId)
		if err != nil {
			return nil, fmt.Errorf("mix: failed to initialise %q: %w", entry.appType, err)
		}
		apps = append(apps, a)
		totalWeight += entry.weight
		cumulativeWeights = append(cumulativeWeights, totalWeight)
	}

	return &MixApplication{
		apps:              apps,
		totalWeight:       totalWeight,
		cumulativeWeights: cumulativeWeights,
	}, nil
}

// pick returns the index of the app selected by a weighted random draw.
func (m *MixApplication) pick() int {
	r := rand.Intn(m.totalWeight)
	for i, cum := range m.cumulativeWeights {
		if r < cum {
			return i
		}
	}
	return len(m.apps) - 1 // unreachable, but satisfies the compiler
}

// CreateUsers creates numUsers users per sub-application and returns them in a
// flat slice. Each MixUser independently draws a weighted random sub-app on
// every GenerateTx call.
func (m *MixApplication) CreateUsers(appContext AppContext, numUsers int) ([]User, error) {
	subUsers := make([][]User, len(m.apps))
	for i, a := range m.apps {
		users, err := a.CreateUsers(appContext, numUsers)
		if err != nil {
			return nil, fmt.Errorf("mix: failed to create users for app %d: %w", i, err)
		}
		subUsers[i] = users
	}

	result := make([]User, numUsers)
	for j := 0; j < numUsers; j++ {
		peers := make([]User, len(m.apps))
		for i := range m.apps {
			peers[i] = subUsers[i][j]
		}
		result[j] = &MixUser{
			peers:             peers,
			totalWeight:       m.totalWeight,
			cumulativeWeights: m.cumulativeWeights,
		}
	}
	return result, nil
}

// GetReceivedTransactions sums received transaction counts across all sub-apps.
func (m *MixApplication) GetReceivedTransactions(rpcClient rpc.Client) (uint64, error) {
	var total uint64
	for _, a := range m.apps {
		count, err := a.GetReceivedTransactions(rpcClient)
		if err != nil {
			return 0, err
		}
		total += count
	}
	return total, nil
}

// MixUser holds one user per sub-application and picks one with weighted
// random selection on each GenerateTx call.
type MixUser struct {
	peers             []User
	totalWeight       int
	cumulativeWeights []int
	sentTxs           atomic.Uint64
}

func (u *MixUser) GenerateTx() (*types.Transaction, error) {
	// Weighted pick: draw r in [0, totalWeight) and find the first app whose
	// cumulative weight exceeds r.
	r := rand.Intn(u.totalWeight)
	chosen := u.peers[len(u.peers)-1]
	for i, cum := range u.cumulativeWeights {
		if r < cum {
			chosen = u.peers[i]
			break
		}
	}

	tx, err := chosen.GenerateTx()
	if err != nil {
		return nil, err
	}
	u.sentTxs.Add(1)
	return tx, nil
}

func (u *MixUser) GetSentTransactions() uint64 {
	return u.sentTxs.Load()
}
