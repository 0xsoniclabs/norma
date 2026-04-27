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

// mixEntry pairs an application type name with a relative weight and the set
// of NetworkRules that must be set for this app to be supported.
type mixEntry struct {
	appType       string
	weight        int
	requiredRules []string
}

func (e mixEntry) isEnabled(have map[string]string) bool {
	for _, name := range e.requiredRules {
		if have[name] != "true" {
			return false
		}
	}
	return true
}

// mixAppTypes lists the application types included in the mix together with
// their relative selection weights and required network rules.
var mixAppTypes = []mixEntry{
	{"erc20", 10, nil},
	{"counter", 10, nil},
	{"store", 10, nil},
	{"uniswap", 5, nil},
	{"smartaccount", 1, nil},
	{"subsidies", 1, []string{"UPGRADES_GAS_SUBSIDIES"}},
	{"transient", 1, nil},
	{"selfdestructoldcontract", 1, nil},
	{"selfdestructnewcontract", 1, nil},
	{"ecdsa", 2, nil},
	{"largecontract", 1, nil},
	{"allofbundle", 3, []string{"UPGRADES_TRANSACTION_BUNDLES"}},
	{"oneofbundle", 3, []string{"UPGRADES_TRANSACTION_BUNDLES"}},
	{"subsidizedbundle", 3, []string{"UPGRADES_GAS_SUBSIDIES", "UPGRADES_TRANSACTION_BUNDLES"}},
	{"failingbundle", 1, []string{"UPGRADES_TRANSACTION_BUNDLES"}},
}

// MixApplication initialises one instance of every application type and
// dispatches transactions across all of them with weighted random selection.
type MixApplication struct {
	apps              []Application
	totalWeight       int
	cumulativeWeights []int
}

func NewMixApplication(appContext AppContext, feederId, appId uint32) (Application, error) {
	apps := make([]Application, 0, len(mixAppTypes))
	cumulativeWeights := make([]int, 0, len(mixAppTypes))
	totalWeight := 0

	networkRules := appContext.GetNetworkRules()
	for i, entry := range mixAppTypes {
		if entry.weight <= 0 {
			return nil, fmt.Errorf("mix: weight for %q must be positive, got %d", entry.appType, entry.weight)
		}
		if !entry.isEnabled(networkRules) {
			continue
		}

		// choose subAppId to avoid collision with regular apps, even with sub-apps of other Mix apps
		const mixAppIdOffset = 1 << 16
		mixSubAppsCount := uint32(len(mixAppTypes))
		subAppId := mixAppIdOffset + appId*mixSubAppsCount + uint32(i)

		application, err := NewApplication(entry.appType, appContext, feederId, subAppId)
		if err != nil {
			return nil, fmt.Errorf("mix: failed to initialise sub-app %q: %w", entry.appType, err)
		}
		apps = append(apps, application)
		totalWeight += entry.weight
		cumulativeWeights = append(cumulativeWeights, totalWeight)
	}

	return &MixApplication{
		apps:              apps,
		totalWeight:       totalWeight,
		cumulativeWeights: cumulativeWeights,
	}, nil
}

// CreateUsers creates numUsers users per sub-application and returns them in a
// flat slice. Each MixUser independently draws a weighted random sub-app on
// every GenerateTx call.
func (m *MixApplication) CreateUsers(appContext AppContext, numUsers int) ([]User, error) {
	var err error
	subAppUsers := make([][]User, len(m.apps))
	for i, subApp := range m.apps {
		subAppUsers[i], err = subApp.CreateUsers(appContext, numUsers)
		if err != nil {
			return nil, fmt.Errorf("mix: failed to create users for app %d: %w", i, err)
		}
	}

	result := make([]User, numUsers)
	for userIndex := 0; userIndex < numUsers; userIndex++ {
		users := make([]User, len(m.apps))
		for subAppIndex := range m.apps {
			users[subAppIndex] = subAppUsers[subAppIndex][userIndex]
		}
		result[userIndex] = &MixUser{
			users:             users,
			totalWeight:       m.totalWeight,
			cumulativeWeights: m.cumulativeWeights,
		}
	}
	return result, nil
}

// GetReceivedTransactions sums received transaction counts across all sub-apps.
func (m *MixApplication) GetReceivedTransactions(rpcClient rpc.Client) (uint64, error) {
	var total uint64
	for _, application := range m.apps {
		count, err := application.GetReceivedTransactions(rpcClient)
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
	users             []User
	totalWeight       int
	cumulativeWeights []int
	sentTxs           atomic.Uint64
}

// pickRandomUser returns the index of the app selected by a weighted random draw.
func (u *MixUser) pickRandomUser() int {
	randomNumber := rand.Intn(u.totalWeight)
	for appIndex, cumulativeWeight := range u.cumulativeWeights {
		if randomNumber < cumulativeWeight {
			return appIndex
		}
	}
	return len(u.cumulativeWeights) - 1
}

func (u *MixUser) GenerateTx() (*types.Transaction, error) {
	chosen := u.pickRandomUser()
	tx, err := u.users[chosen].GenerateTx()
	if err != nil {
		return nil, err
	}
	u.sentTxs.Add(1)
	return tx, nil
}

func (u *MixUser) GetSentTransactions() uint64 {
	return u.sentTxs.Load()
}
