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
	"github.com/0xsoniclabs/sonic/gossip/blockproc/bundle"
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
	numTransactions := 5
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
			if receipt, err := ctxt.GetReceipt(innerTx.Hash()); err != nil {
				t.Errorf("inner step tx receipt not available: %v", err)
			} else if receipt.Status != types.ReceiptStatusSuccessful {
				t.Errorf("tx %s failed with status %d", innerTx.Hash(), receipt.Status)
			} else {
				fmt.Printf("tx receipt %s OK\n", innerTx.Hash())
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
