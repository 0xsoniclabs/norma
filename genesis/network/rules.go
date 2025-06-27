package network

import (
	"fmt"
	"log/slog"
	"math/big"
	"time"

	"github.com/0xsoniclabs/norma/driver/rpc"
	"github.com/0xsoniclabs/norma/genesistools/genesis"
	"github.com/0xsoniclabs/sonic/evmcore"
	"github.com/0xsoniclabs/sonic/gossip/contract/driverauth100"
	"github.com/0xsoniclabs/sonic/gossip/contract/sfc100"
	"github.com/0xsoniclabs/sonic/opera"
	"github.com/0xsoniclabs/sonic/opera/contracts/driverauth"
	"github.com/0xsoniclabs/sonic/opera/contracts/sfc"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
)

// ApplyNetworkRules updates the network rules on the network.
func ApplyNetworkRules(backend ContractBackend, rules genesis.NetworkRules) error {
	// Bind contract to update network rules
	contract, err := driverauth100.NewContract(driverauth.ContractAddress, backend)
	if err != nil {
		return fmt.Errorf("failed to get driver auth contract representation; %v", err)
	}

	originalRules := opera.FakeNetRules(opera.GetSonicUpgrades())
	diff, err := genesis.GenerateJsonNetworkRulesUpdates(originalRules, rules)
	if err != nil {
		return fmt.Errorf("failed to generate network rules updates; %v", err)
	}

	// Use Fake ID for the network
	// Driver owner is the first validator from the list i.e., index 1 (defined in genesis export in genesis.GenerateJsonGenesis)
	txOpts, err := bind.NewKeyedTransactorWithChainID(evmcore.FakeKey(1), big.NewInt(int64(originalRules.NetworkID)))
	if err != nil {
		return fmt.Errorf("failed to create txOpts; %v", err)
	}

	tx, err := contract.UpdateNetworkRules(txOpts, []byte(diff))
	if err != nil {
		return fmt.Errorf("failed to update network rules; %v", err)
	}

	slog.Info("requested to update network rules", "nonce", tx.Nonce())

	rec, err := backend.WaitTransactionReceipt(tx.Hash())
	if err != nil {
		return fmt.Errorf("failed to get receipt; %v", err)
	}

	if rec.Status != types.ReceiptStatusSuccessful {
		return fmt.Errorf("failed to update network rules: status: %v", rec.Status)
	}

	return nil
}

// AdvanceEpoch triggers the sealing of an epoch advancing it by the given number.
// The function blocks until the final epoch has been reached
func AdvanceEpoch(backend rpc.Client, epochIncrement int) error {
	contract, err := driverauth100.NewContract(driverauth.ContractAddress, backend)
	if err != nil {
		return fmt.Errorf("failed to get driver auth contract representation; %v", err)
	}

	var currentEpoch hexutil.Uint64
	if err := backend.Client().Call(&currentEpoch, "eth_currentEpoch"); err != nil {
		return fmt.Errorf("failed to get current epoch: %w", err)
	}

	originalRules := opera.FakeNetRules(opera.GetSonicUpgrades())

	// Use Fake ID for the network
	// Driver owner is the first validator from the list i.e., index 1 (defined in genesis export in genesis.GenerateJsonGenesis)
	txOpts, err := bind.NewKeyedTransactorWithChainID(evmcore.FakeKey(1), big.NewInt(int64(originalRules.NetworkID)))
	if err != nil {
		return fmt.Errorf("failed to create txOpts; %v", err)
	}

	tx, err := contract.AdvanceEpochs(txOpts, big.NewInt(int64(epochIncrement)))
	if err != nil {
		return fmt.Errorf("failed to advance epoch; %v", err)
	}

	slog.Info("requested to advance epoch", "current_epoch", currentEpoch, "nonce", tx.Nonce())

	rec, err := backend.WaitTransactionReceipt(tx.Hash())
	if err != nil {
		return fmt.Errorf("failed to get receipt; %v", err)
	}

	if rec.Status != types.ReceiptStatusSuccessful {
		return fmt.Errorf("failed to advance epoch; receipt status: %v", rec.Status)
	}

	// wait until the epoch is advanced
	start := time.Now()
	for time.Since(start) < 10*time.Second {
		var newEpoch hexutil.Uint64
		if err := backend.Client().Call(&newEpoch, "eth_currentEpoch"); err != nil {
			return fmt.Errorf("failed to get current epoch: %w", err)
		}
		if newEpoch >= currentEpoch+hexutil.Uint64(epochIncrement) {
			logEpochSummary(backend)
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}

	return fmt.Errorf("failed to advance epoch: waited too long for the epoch to be advanced")
}

func logEpochSummary(client rpc.Client) {
	// get a representation of the deployed contract
	SFCContract, err := sfc100.NewContract(sfc.ContractAddress, client)
	if err != nil {
		slog.Error("Failed to get SFC contract representation", "error", err)
		return
	}

	epoch, err := SFCContract.CurrentEpoch(nil)
	if err != nil {
		slog.Error("Failed to get current epoch", "error", err)
		return
	}

	validators, err := SFCContract.GetEpochValidatorIDs(nil, epoch)
	if err != nil {
		slog.Error("Failed to get epoch validator IDs", "epoch", epoch, "error", err)
		return
	}

	slog.Info("Epoch summary",
		"current_epoch", epoch,
		"active_validators", validators,
	)
}
