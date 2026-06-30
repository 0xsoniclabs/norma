package app_test

import (
	"context"
	"fmt"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/network"
	"github.com/0xsoniclabs/norma/driver/network/local"
	"github.com/0xsoniclabs/norma/load/app"
	"github.com/0xsoniclabs/sonic/gossip/blockproc/bundle"
	"github.com/ethereum/go-ethereum/core/types"
)

func TestGenerators_Bundles(t *testing.T) {
	rules := getCumulativeUpgrades("UPGRADES_BRIO")
	rules.Upgrades.TransactionBundles = new(true)
	rules.Upgrades.GasSubsidies = new(true)
	net, err := local.NewLocalNetwork(t.Context(), &driver.NetworkConfig{
		Validators:   driver.DefaultValidators(t.Name()),
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
			// DuplicatedBundle intentionally re-sends the same plan after it has been
			// committed, so ErrBundleAlreadyProcessed is expected in that case.
			if strings.Contains(err.Error(), "already been processed") {
				t.Logf("bundle already processed, skipping wait (bundle %d)", i)
				continue
			}
			t.Fatal(err)
		}

		txBundle, err := bundle.OpenEnvelope(signer, tx)
		if err != nil {
			t.Fatalf("failed to open bundle envelope: %v", err)
		}
		planHash := txBundle.Plan.Hash()
		t.Log("Sent bundle", "bundle", i, "plan", planHash)

		// Wait for this bundle to execute before sending the next one so that
		// the pending nonce of the inner-transaction accounts is up to date.
		ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
		info, err := rpcClient.WaitForBundleInfo(ctx, planHash)
		cancel()
		if err != nil {
			t.Fatalf("bundle %d (plan %s) not executed: %v", i, planHash, err)
		}
		t.Log("Bundle executed",
			"bundle", i,
			"plan", planHash,
			"block", info.Block,
			"position", info.Position,
			"count", info.Count)
	}

	err = network.Retry(t.Context(), network.DefaultRetryAttempts, 1*time.Second,
		func(ctx context.Context) error {
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
			t.Logf("eth_sendRawTransaction failed for randomly failing bundle (expected): %v\n", err)
			continue
		}
		t.Logf("Sent bundle %d\n", i)

		// wait for tx to be processed (necessary because of nonce loading in GenerateTx())
		_ = network.Retry(t.Context(), 5, 1*time.Second,
			func(ctx context.Context) error {
				received, err := application.GetReceivedTransactions(rpcClient)
				if err != nil {
					return fmt.Errorf("unable to get amount of received txs; %v", err)
				}
				if received <= lastReceived {
					t.Logf("Waiting for received txs increase before sending next tx (received %d)\n", received)
					return fmt.Errorf("not enough bundled txs received, received %d", received)
				}
				lastReceived = received
				return nil
			})
	}

	err = network.Retry(t.Context(), 10, 1*time.Second,
		func(ctx context.Context) error {
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
