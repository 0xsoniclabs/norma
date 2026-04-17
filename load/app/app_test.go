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
	"github.com/0xsoniclabs/norma/load/app"
	"github.com/0xsoniclabs/sonic/gossip/blockproc/bundle"
	"github.com/ethereum/go-ethereum/core/types"
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
		"Mix": {
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
			net, err := local.NewLocalNetwork(&driver.NetworkConfig{
				Validators:   driver.DefaultValidators,
				NetworkRules: getCumulativeUpgrades(upgrade),
			})
			if err != nil {
				t.Fatalf("failed to create new local network: %v", err)
			}
			t.Cleanup(func() {
				if err := net.Shutdown(); err != nil {
					t.Fatalf("failed to shutdown network: %v", err)
				}
			})

			primaryAccount, err := app.NewAccount(0, PrivateKey, FakeNetworkID)
			if err != nil {
				t.Fatal(err)
			}

			appCtx, err := app.NewContext(net, primaryAccount)
			if err != nil {
				t.Fatal(err)
			}

			for name, test := range tests {
				if !slices.Contains(test.availableInUpgrades, upgrade) {
					continue
				}
				t.Run(name, func(t *testing.T) {
					application, err := app.NewApplication(name, appCtx, 0, 0)
					if err != nil {
						t.Fatal(err)
					}
					testGenerator(t, application, appCtx)
				})
			}
		})
	}
}

// getCumulativeUpgrades returns a map of upgrades that are enabled
// up to and including the lastSupported upgrade.
// This function is needed because upgrades should be cumulative, not exclusive.
func getCumulativeUpgrades(lastSupported string) map[string]string {
	upgrades := map[string][]string{
		"UPGRADES_SONIC":   {"UPGRADES_SONIC"},
		"UPGRADES_ALLEGRO": {"UPGRADES_SONIC", "UPGRADES_ALLEGRO"},
		"UPGRADES_BRIO":    {"UPGRADES_SONIC", "UPGRADES_ALLEGRO", "UPGRADES_BRIO"},
	}
	result := make(map[string]string)
	for _, upgrade := range upgrades[lastSupported] {
		result[upgrade] = "true"
	}
	return result
}

func TestGenerators_Subsidies(t *testing.T) {
	net, err := local.NewLocalNetwork(&driver.NetworkConfig{
		Validators: driver.DefaultValidators,
		NetworkRules: map[string]string{
			"UPGRADES_GAS_SUBSIDIES": "true",
		},
	})
	if err != nil {
		t.Fatalf("failed to create new local network: %v", err)
	}
	t.Cleanup(func() {
		if err := net.Shutdown(); err != nil {
			t.Fatalf("failed to shutdown network: %v", err)
		}
	})

	primaryAccount, err := app.NewAccount(0, PrivateKey, FakeNetworkID)
	if err != nil {
		t.Fatal(err)
	}

	appCtx, err := app.NewContext(net, primaryAccount)
	if err != nil {
		t.Fatal(err)
	}

	subsidiesApp, err := app.NewSubsidiesApplication(appCtx, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	testGenerator(t, subsidiesApp, appCtx)
}

func testGenerator(t *testing.T, app app.Application, ctxt app.AppContext) {
	users, err := app.CreateUsers(ctxt, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 1 {
		t.Fatalf("unexpected number of users created, wanted 1, got %d", len(users))
	}
	user := users[0]

	rpcClient := ctxt.GetClient()
	numTransactions := 10
	transactions := []*types.Transaction{}
	for range numTransactions {
		tx, err := user.GenerateTx()
		if err != nil {
			t.Fatal(err)
		}
		if tx == nil {
			t.Fatal("generated transaction is nil")
		}

		if err := rpcClient.SendTransaction(t.Context(), tx); err != nil {
			t.Fatal(err)
		}
		transactions = append(transactions, tx)
	}

	// wait for the transactions to be processed
	for _, tx := range transactions {
		receipt, err := ctxt.GetReceipt(tx.Hash())
		if err != nil {
			t.Fatal(err)
		}
		if receipt.Status != types.ReceiptStatusSuccessful {
			t.Fatalf("transaction failed, receipt status: %v (gas limit %d used %d)", receipt.Status, tx.Gas(), receipt.GasUsed)
		}
		if tx.Gas() > 2*receipt.GasUsed {
			t.Errorf("gas limit unnecessary high: limit %d used %d", tx.Gas(), receipt.GasUsed)
		}
	}

	if got, want := user.GetSentTransactions(), numTransactions; got != uint64(want) {
		t.Errorf("invalid number of sent transactions reported, wanted %d, got %d", want, got)
	}

	err = network.Retry(network.DefaultRetryAttempts, 1*time.Second, func() error {
		received, err := app.GetReceivedTransactions(rpcClient)
		if err != nil {
			return fmt.Errorf("unable to get amount of received txs; %v", err)
		}
		if received != 10 {
			return fmt.Errorf("unexpected amount of txs in chain (%d)", received)
		}
		return nil
	})
	if err != nil {
		t.Error(err)
	}
}

func TestGenerators_Bundles(t *testing.T) {
	rules := getCumulativeUpgrades("UPGRADES_BRIO")
	rules["UPGRADES_TRANSACTION_BUNDLES"] = "true"
	net, err := local.NewLocalNetwork(&driver.NetworkConfig{
		Validators:   driver.DefaultValidators,
		NetworkRules: rules,
	})
	if err != nil {
		t.Fatalf("failed to create new local network: %v", err)
	}
	t.Cleanup(func() {
		if err := net.Shutdown(); err != nil {
			t.Fatalf("failed to shutdown network: %v", err)
		}
	})

	primaryAccount, err := app.NewAccount(0, PrivateKey, FakeNetworkID)
	if err != nil {
		t.Fatal(err)
	}

	context, err := app.NewContext(net, primaryAccount)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("AllOfBundle", func(t *testing.T) {
		allOfBundleApp, err := app.NewAllOfBundleApplication(context, 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		testBundleGenerator(t, allOfBundleApp, context)
	})
}

func testBundleGenerator(t *testing.T, app app.Application, ctxt app.AppContext) {
	users, err := app.CreateUsers(ctxt, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 1 {
		t.Fatalf("unexpected number of users created, wanted 1, got %d", len(users))
	}
	user := users[0]

	rpcClient := ctxt.GetClient()
	numTransactions := 10
	envelopeTxs := []*types.Transaction{}
	for range numTransactions {
		tx, err := user.GenerateTx()
		if err != nil {
			t.Fatal(err)
		}
		if tx == nil {
			t.Fatal("generated transaction is nil")
		}

		if err := rpcClient.SendTransaction(context.Background(), tx); err != nil {
			t.Fatal(err)
		}
		envelopeTxs = append(envelopeTxs, tx)
	}

	// The envelope itself has no receipt - wait for each inner step tx to land on
	// chain individually, mirroring how testGenerator waits for tx receipts.
	chainID, err := rpcClient.ChainID(context.Background())
	if err != nil {
		t.Fatalf("failed to get chain ID: %v", err)
	}
	signer := types.NewCancunSigner(chainID)

	for _, envelopeTx := range envelopeTxs {
		txBundle, openErr := bundle.OpenEnvelope(signer, envelopeTx)
		if openErr != nil {
			t.Fatalf("failed to open bundle envelopeTx: %v", openErr)
		}
		for _, innerTx := range txBundle.GetTransactionsInReferencedOrder() {
			if _, err := ctxt.GetReceipt(innerTx.Hash()); err != nil {
				t.Errorf("inner step tx receipt not available: %v", err)
			}
		}
	}

	err = network.Retry(network.DefaultRetryAttempts, 1*time.Second, func() error {
		sent := user.GetSentTransactions()
		received, err := app.GetReceivedTransactions(rpcClient)
		if err != nil {
			return fmt.Errorf("unable to get amount of received txs; %v", err)
		}
		if received != sent {
			return fmt.Errorf("unexpected amount of txs in chain (sent %d, received%d)", sent, received)
		}
		return nil
	})
	if err != nil {
		t.Error(err)
	}
}
