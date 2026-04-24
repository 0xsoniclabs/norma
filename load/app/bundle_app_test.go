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

	appCtx, err := app.NewContext(net, primaryAccount, rules)
	if err != nil {
		t.Fatal(err)
	}

	for appId, name := range []string{
		"AllOfBundle",
		"OneOfBundle",
		"SubsidizedBundle",
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
			testFailingBundleGenerator(t, application, appCtx)
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
	user, ok := users[0].(app.BundleUser)
	if !ok {
		t.Fatal("User does not implement BundleUser")
	}

	numBundles := 5
	rpcClient := ctxt.GetClient()
	planHashes := make([]common.Hash, 0, numBundles)
	signer := types.LatestSignerForChainID(big.NewInt(FakeNetworkID))
	for range numBundles {
		tx, shouldFail, err := user.GenerateBundle()
		if err != nil {
			t.Fatal(err)
		}
		if tx == nil {
			t.Fatal("generated transaction is nil")
		}

		if err := rpcClient.SendTransaction(t.Context(), tx); err != nil {
			t.Fatal(err)
		}

		if !shouldFail {
			txBundle, err := bundle.OpenEnvelope(signer, tx)
			if err != nil {
				t.Fatalf("failed to open bundle envelope: %v", err)
			}
			planHashes = append(planHashes, txBundle.Plan.Hash())
		}
	}

	// Wait for each successful bundle execution via sonic_getBundleInfo. This detects
	// rolled-back bundles which commit no transactions and have no receipts.
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	for i, planHash := range planHashes {
		info, err := rpcClient.WaitForBundleInfo(ctx, planHash)
		if err != nil {
			t.Fatalf("bundle %d (plan %s) not executed: %v", i, planHash, err)
		}
		fmt.Printf("bundle %d (plan %s) executed: block=%d position=%d count=%d\n", i, planHash, info.Block, info.Position, info.Count)
	}

	err = network.Retry(network.DefaultRetryAttempts, 1*time.Second, func() error {
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

func testFailingBundleGenerator(t *testing.T, application app.Application, ctxt app.AppContext) {
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
	for range numBundles {
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

		// wait for tx to be processed (necessary because of nonce loading in GenerateTx())
		_ = network.Retry(5, 1*time.Second, func() error {
			received, err := application.GetReceivedTransactions(rpcClient)
			if err != nil {
				return fmt.Errorf("unable to get amount of received txs; %v", err)
			}
			if received <= lastReceived {
				fmt.Printf("wating for tx to be or not to be processed (%d vs %d)\n", received, lastReceived)
				return fmt.Errorf("not enough bundled txs received, received %d", received)
			}
			lastReceived = received
			return nil
		})
	}

	err = network.Retry(10, 1*time.Second, func() error {
		received, err := application.GetReceivedTransactions(rpcClient)
		if err != nil {
			return fmt.Errorf("unable to get amount of received txs; %v", err)
		}
		if received < 1 {
			return fmt.Errorf("not enough bundled txs received, received %d", received)
		}
		return nil
	})
	if err != nil {
		t.Error(err)
	}
}
