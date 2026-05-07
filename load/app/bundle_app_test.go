package app_test

import (
	"context"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/network"
	"github.com/0xsoniclabs/norma/driver/network/local"
	"github.com/0xsoniclabs/norma/load/app"
	"github.com/0xsoniclabs/sonic/gossip/blockproc/bundle"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

func TestGenerators_Bundles(t *testing.T) {
	rules := getCumulativeUpgrades("UPGRADES_BRIO")
	rules["UPGRADES_TRANSACTION_BUNDLES"] = "true"
	rules["UPGRADES_GAS_SUBSIDIES"] = "true"
	net, err := local.NewLocalNetwork(t.Context(), &driver.NetworkConfig{
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

	appCtx, err := app.NewContext(net, primaryAccount, rules)
	if err != nil {
		t.Fatal(err)
	}

	for appId, name := range []string{
		"AllOfBundle",
		"OneOfBundle",
		"SubsidizedBundle",
		"DuplicatedBundle",
	} {
		t.Run(name, func(t *testing.T) {
			application, err := app.NewApplication(name, appCtx, 0, uint32(appId))
			if err != nil {
				t.Fatal(err)
			}
			testBundleGenerator(t, application, appCtx)
		})
	}

	for appId, name := range []string{
		"FailingBundle",
	} {
		t.Run(name, func(t *testing.T) {
			application, err := app.NewApplication(name, appCtx, 0, uint32(appId))
			if err != nil {
				t.Fatal(err)
			}
			testRpcNonceBundleGenerator(t, application, appCtx)
		})
	}
}

func testBundleGenerator(t *testing.T, application app.Application, ctxt app.AppContext) {
	users, err := application.CreateUsers(ctxt, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 1 {
		t.Fatalf("unexpected number of users created, wanted 1, got %d", len(users))
	}
	user := users[0]

	numBundles := 5
	rpcClient := ctxt.GetClient()
	planHashes := make([]common.Hash, 0, numBundles)
	signer := types.LatestSignerForChainID(big.NewInt(FakeNetworkID))
	for i := range numBundles {
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

		txBundle, err := bundle.OpenEnvelope(signer, tx)
		if err != nil {
			t.Fatalf("failed to open bundle envelope: %v", err)
		}
		planHash := txBundle.Plan.Hash()
		fmt.Printf("Sent bundle %d (plan %s)\n", i, planHash)
		planHashes = append(planHashes, planHash)
	}

	// Wait for each successful bundle execution via sonic_getBundleInfo. This detects
	// rolled-back bundles which commit no transactions and have no receipts.
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	for i, planHash := range planHashes {
		fmt.Printf("Awaiting bundle %d (plan %s)...\n", i, planHash)
		info, err := rpcClient.WaitForBundleInfo(ctx, planHash)
		if err != nil {
			t.Fatalf("bundle %d (plan %s) not executed: %v", i, planHash, err)
		}
		fmt.Printf("bundle %d (plan %s) executed: block=%d position=%d count=%d\n", i, planHash, info.Block, info.Position, info.Count)
	}

	err = network.Retry(t.Context(), network.DefaultRetryAttempts, 1*time.Second, func() error {
		sent := user.GetSentTransactions()
		received, err := application.GetReceivedTransactions(rpcClient)
		if err != nil {
			return fmt.Errorf("unable to get amount of received txs; %v", err)
		}
		if received != sent {
			return fmt.Errorf("unexpected amount of txs in chain (sent %d, received %d)", sent, received)
		}
		return nil
	})
	if err != nil {
		t.Error(err)
	}
}

func testRpcNonceBundleGenerator(t *testing.T, application app.Application, ctxt app.AppContext) {
	users, err := application.CreateUsers(ctxt, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 1 {
		t.Fatalf("unexpected number of users created, wanted 1, got %d", len(users))
	}
	user := users[0]

	numBundles := 5
	rpcClient := ctxt.GetClient()
	lastReceived := uint64(0)
	for i := range numBundles {
		tx, err := user.GenerateTx()
		if err != nil {
			t.Fatal(err)
		}
		if tx == nil {
			t.Fatal("generated transaction is nil")
		}

		if err := rpcClient.SendTransaction(t.Context(), tx); err != nil {
			fmt.Printf("eth_sendRawTransaction failed for randomly failing bundle (expected): %v\n", err)
			continue
		}
		fmt.Printf("Sent bundle %d\n", i)

		// wait for tx to be processed (necessary because of nonce loading in GenerateTx())
		_ = network.Retry(t.Context(), 5, 1*time.Second, func() error {
			received, err := application.GetReceivedTransactions(rpcClient)
			if err != nil {
				return fmt.Errorf("unable to get amount of received txs; %v", err)
			}
			if received <= lastReceived {
				fmt.Printf("Waiting for received txs increase before sending next tx (received %d)\n", received)
				return fmt.Errorf("not enough bundled txs received, received %d", received)
			}
			lastReceived = received
			return nil
		})
	}

	err = network.Retry(t.Context(), 10, 1*time.Second, func() error {
		received, err := application.GetReceivedTransactions(rpcClient)
		if err != nil {
			return fmt.Errorf("unable to get amount of received txs; %v", err)
		}
		if received < 2 {
			return fmt.Errorf("not enough bundled txs received, received %d", received)
		}
		return nil
	})
	if err != nil {
		t.Error(err)
	}
}
