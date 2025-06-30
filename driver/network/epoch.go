package network

import (
	"fmt"
	"log/slog"
	big "math/big"
	"time"

	"github.com/0xsoniclabs/norma/driver/rpc"
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

// AdvanceEpoch triggers the sealing of an epoch advancing it by the given number.
// The function blocks until the final epoch has been reached
func AdvanceEpoch(client rpc.Client, epochIncrement int) error {
	contract, err := driverauth100.NewContract(driverauth.ContractAddress, client)
	if err != nil {
		return fmt.Errorf("failed to get driver auth contract representation; %v", err)
	}

	currentEpoch, err := GetCurrentEpoch(client)
	if err != nil {
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

	rec, err := client.WaitTransactionReceipt(tx.Hash())
	if err != nil {
		return fmt.Errorf("failed to get receipt; %v", err)
	}

	if rec.Status != types.ReceiptStatusSuccessful {
		return fmt.Errorf("failed to advance epoch; receipt status: %v", rec.Status)
	}

	// wait until the new epoch has actually started
	start := time.Now()
	for time.Since(start) < 60*time.Second {
		newEpoch, err := GetCurrentEpoch(client)
		if err != nil {
			return fmt.Errorf("failed to get current epoch after advancing: %w", err)
		}
		if newEpoch >= currentEpoch+hexutil.Uint64(epochIncrement) {
			logEpochSummary(client)
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}

	return fmt.Errorf("failed to advance epoch: waited too long for the epoch to be advanced")
}

func GetCurrentEpoch(client rpc.Client) (hexutil.Uint64, error) {
	var currentEpoch hexutil.Uint64
	if err := client.Call(&currentEpoch, "eth_currentEpoch"); err != nil {
		return 0, fmt.Errorf("failed to get current epoch: %w", err)
	}
	return currentEpoch, nil
}

func logEpochSummary(client rpc.Client) {
	// get a representation of the deployed contract
	sfc, err := sfc100.NewContract(sfc.ContractAddress, client)
	if err != nil {
		slog.Error("Failed to get SFC contract representation", "error", err)
		return
	}

	epoch, err := sfc.CurrentEpoch(nil)
	if err != nil {
		slog.Error("Failed to get current epoch", "error", err)
		return
	}

	validators, err := sfc.GetEpochValidatorIDs(nil, epoch)
	if err != nil {
		slog.Error("Failed to get epoch validator IDs", "epoch", epoch, "error", err)
		return
	}

	validatorIds := []int{}
	for _, id := range validators {
		validatorIds = append(validatorIds, int(id.Int64()))
	}

	slog.Info("Epoch summary",
		"current_epoch", epoch,
		"active_validators", validatorIds,
	)
}
