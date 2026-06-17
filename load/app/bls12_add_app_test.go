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
	"fmt"
	"testing"
	"time"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/network"
	"github.com/0xsoniclabs/norma/driver/network/local"
	"github.com/0xsoniclabs/norma/load/app"
	"github.com/ethereum/go-ethereum/core/types"
)

// TestBls12AddApplication verifies that the BLS12-381 G1 addition precompile
// application can generate and process transactions on an Allegro network.
func TestBls12AddApplication(t *testing.T) {
	rules := map[string]string{
		"UPGRADES_SONIC":   "true",
		"UPGRADES_ALLEGRO": "true",
	}
	net, err := local.NewLocalNetwork(t.Context(), &driver.NetworkConfig{
		Validators:   driver.DefaultValidators(t.Name()),
		NetworkRules: rules,
	})
	if err != nil {
		t.Fatalf("failed to create local network: %v", err)
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

	ctxt, err := app.NewContext(net, primaryAccount, rules)
	if err != nil {
		t.Fatal(err)
	}
	defer ctxt.Close()

	application, err := app.NewBls12AddApplication(ctxt, 0, 0)
	if err != nil {
		t.Fatal(err)
	}

	users, err := application.CreateUsers(ctxt, 1)
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
	}

	if got, want := user.GetSentTransactions(), numTransactions; got != uint64(want) {
		t.Errorf("invalid number of sent transactions reported, wanted %d, got %d", want, got)
	}

	err = network.Retry(t.Context(), network.DefaultRetryAttempts, 1*time.Second, func() error {
		received, err := application.GetReceivedTransactions(rpcClient)
		if err != nil {
			return fmt.Errorf("unable to get amount of received txs; %v", err)
		}
		if received != uint64(numTransactions) {
			return fmt.Errorf("unexpected amount of txs in chain, wanted %d, got %d", numTransactions, received)
		}
		return nil
	})
	if err != nil {
		t.Error(err)
	}
}
