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
	"testing"
	"time"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/network"
	"github.com/0xsoniclabs/norma/driver/network/local"
	"github.com/0xsoniclabs/norma/load/app"
	"github.com/ethereum/go-ethereum/core/types"
)

const PrivateKey = "163f5f0f9a621d72fedd85ffca3d08d131ab4e812181e0d30ffd1c885d20aac7" // Fakenet validator 1
const FakeNetworkID = 0xfa3

func TestGenerators(t *testing.T) {
	// run local network of one node
	net, err := local.NewLocalNetwork(&driver.NetworkConfig{
		Validators: driver.DefaultValidators,
		NetworkRules: map[string]string{
			"UPGRADES_ALLEGRO": "true",
		},
	})
	if err != nil {
		t.Fatalf("failed to create new local network: %v", err)
	}
	t.Cleanup(func() { net.Shutdown() })

	primaryAccount, err := app.NewAccount(0, PrivateKey, nil, FakeNetworkID)
	if err != nil {
		t.Fatal(err)
	}

	context, err := app.NewContext(net, primaryAccount)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("Counter", func(t *testing.T) {
		counterApp, err := app.NewCounterApplication(context, 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		testGenerator(t, counterApp, context)
	})
	t.Run("ERC20", func(t *testing.T) {
		erc20app, err := app.NewERC20Application(context, 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		testGenerator(t, erc20app, context)
	})
	t.Run("Store", func(t *testing.T) {
		storeApp, err := app.NewStoreApplication(context, 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		testGenerator(t, storeApp, context)
	})
	t.Run("Uniswap", func(t *testing.T) {
		uniswapApp, err := app.NewUniswapApplication(context, 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		testGenerator(t, uniswapApp, context)
	})
	t.Run("SmartAccount", func(t *testing.T) {
		smartAccountApp, err := app.NewSmartAccountApplication(context, 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		testGenerator(t, smartAccountApp, context)
	})
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
		if err := rpcClient.SendTransaction(context.Background(), tx); err != nil {
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
			t.Fatalf("transaction failed, receipt status: %v", receipt.Status)
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
