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

package app_test

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/network"
	"github.com/0xsoniclabs/norma/driver/network/local"
	"github.com/0xsoniclabs/norma/genesis"
	"github.com/0xsoniclabs/norma/load/app"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/require"
)

const PrivateKey = "163f5f0f9a621d72fedd85ffca3d08d131ab4e812181e0d30ffd1c885d20aac7" // Fakenet validator 1
const FakeNetworkID = 0xfa3

func TestGenerators(t *testing.T) {

	tests := map[string]struct {
		availableInUpgrades []string
	}{
		"Counter": {
			availableInUpgrades: []string{
				"UPGRADES_SONIC",
				"UPGRADES_ALLEGRO",
				"UPGRADES_BRIO",
			},
		},
		"ERC20": {
			availableInUpgrades: []string{
				"UPGRADES_SONIC",
				"UPGRADES_ALLEGRO",
				"UPGRADES_BRIO",
			},
		},
		"Store": {
			availableInUpgrades: []string{
				"UPGRADES_SONIC",
				"UPGRADES_ALLEGRO",
				"UPGRADES_BRIO",
			},
		},
		"Uniswap": {
			availableInUpgrades: []string{
				"UPGRADES_SONIC",
				"UPGRADES_ALLEGRO",
				"UPGRADES_BRIO",
			},
		},
		"SmartAccount": {
			availableInUpgrades: []string{
				"UPGRADES_ALLEGRO",
				"UPGRADES_BRIO",
			},
		},
		"Transient": {
			availableInUpgrades: []string{
				"UPGRADES_SONIC",
				"UPGRADES_ALLEGRO",
				"UPGRADES_BRIO",
			},
		},
		"SelfDestructOldContract": {
			availableInUpgrades: []string{
				"UPGRADES_SONIC",
				"UPGRADES_ALLEGRO",
				"UPGRADES_BRIO",
			},
		},
		"SelfDestructNewContract": {
			availableInUpgrades: []string{
				"UPGRADES_SONIC",
				"UPGRADES_ALLEGRO",
				"UPGRADES_BRIO",
			},
		},
		"Ecdsa": {
			availableInUpgrades: []string{
				"UPGRADES_BRIO",
			},
		},
		"LargeContract": {
			availableInUpgrades: []string{
				"UPGRADES_BRIO",
			},
		},
	}

	for _, upgrade := range []string{
		"UPGRADES_SONIC",
		"UPGRADES_ALLEGRO",
		"UPGRADES_BRIO",
	} {
		t.Run(upgrade, func(t *testing.T) {
			// run local network of one node
			rules := getCumulativeUpgrades(upgrade)
			net, err := local.NewLocalNetwork(t.Context(), &driver.NetworkConfig{
				Validators:   driver.DefaultValidators(t.Name()),
				NetworkRules: rules,
			})
			require.NoError(t, err, "failed to create new local network")
			t.Cleanup(func() {
				require.NoError(t, net.Shutdown(), "failed to shutdown network")
			})

			primaryAccount, err := app.NewAccount(0, PrivateKey, FakeNetworkID)
			require.NoError(t, err, "failed to create primary account")

			appCtx, err := app.NewContext(net, primaryAccount, rules)
			require.NoError(t, err, "failed to create application context")

			for name, test := range tests {
				if !slices.Contains(test.availableInUpgrades, upgrade) {
					continue
				}
				t.Run(name, func(t *testing.T) {
					application, err := app.NewApplication(name, appCtx, 0, 0)
					require.NoError(t, err, "failed to create application")
					testGenerator(t, application, appCtx)
				})
			}
		})
	}
}

// getCumulativeUpgrades returns a map of upgrades that are enabled
// up to and including the lastSupported upgrade.
// This function is needed because upgrades should be cumulative, not exclusive.
func getCumulativeUpgrades(lastSupported string) driver.NetworkRules {
	sonic := false
	allegro := false
	brio := false

	switch lastSupported {
	case "UPGRADES_SONIC":
		sonic = true
	case "UPGRADES_ALLEGRO":
		sonic = true
		allegro = true
	case "UPGRADES_BRIO":
		sonic = true
		allegro = true
		brio = true
	}

	return driver.NetworkRules{
		Upgrades: &genesis.UpgradesPatch{
			Sonic:   new(sonic),
			Allegro: new(allegro),
			Brio:    new(brio),
		},
	}
}

func TestGenerators_Subsidies(t *testing.T) {
	rules := driver.NetworkRules{
		Upgrades: &genesis.UpgradesPatch{
			GasSubsidies: new(true),
		},
	}
	net, err := local.NewLocalNetwork(t.Context(), &driver.NetworkConfig{
		Validators:   driver.DefaultValidators(t.Name()),
		NetworkRules: rules,
	})
	require.NoError(t, err, "failed to create new local network")
	t.Cleanup(func() {
		require.NoError(t, net.Shutdown(), "failed to shutdown network")
	})

	primaryAccount, err := app.NewAccount(0, PrivateKey, FakeNetworkID)
	require.NoError(t, err, "failed to create primary account")

	appCtx, err := app.NewContext(net, primaryAccount, rules)
	require.NoError(t, err, "failed to create application context")

	subsidiesApp, err := app.NewSubsidiesApplication(appCtx, 0, 0)
	require.NoError(t, err, "failed to create subsidies application")
	testGenerator(t, subsidiesApp, appCtx)
}

func testGenerator(t *testing.T, app app.Application, ctxt app.AppContext) {
	users, err := app.CreateUsers(ctxt, 1)
	require.NoError(t, err, "failed to create users for application")
	require.Len(t, users, 1, "unexpected number of users created")
	user := users[0]

	rpcClient := ctxt.GetClient()
	numTransactions := 10
	transactions := []*types.Transaction{}
	for range numTransactions {
		tx, err := user.GenerateTx()
		require.NoError(t, err, "failed to generate transaction")
		require.NotNil(t, tx, "generated transaction is nil")

		require.NoError(t, rpcClient.SendTransaction(t.Context(), tx), "failed to send transaction")
		transactions = append(transactions, tx)
	}

	// wait for the transactions to be processed
	for _, tx := range transactions {
		receipt, err := ctxt.GetReceipt(tx.Hash())
		require.NoError(t, err, "failed to get transaction receipt")
		require.Equalf(t, types.ReceiptStatusSuccessful, receipt.Status,
			"transaction failed, receipt status: %v (gas limit %d used %d)", receipt.Status, tx.Gas(), receipt.GasUsed)
		require.LessOrEqualf(t, tx.Gas(), 2*receipt.GasUsed,
			"gas limit unnecessary high: limit %d used %d", tx.Gas(), receipt.GasUsed)
	}
	require.Equal(t, uint64(numTransactions), user.GetSentTransactions(), "invalid number of sent transactions reported")

	err = network.Retry(t.Context(), network.DefaultRetryAttempts, 1*time.Second,
		func(ctx context.Context) error {
			received, err := app.GetReceivedTransactions(rpcClient)
			if err != nil {
				return fmt.Errorf("unable to get amount of received txs; %v", err)
			}
			if received != 10 {
				return fmt.Errorf("unexpected amount of txs in chain (%d)", received)
			}
			return nil
		})
	require.NoError(t, err, "transactions were not processed in time")
}
