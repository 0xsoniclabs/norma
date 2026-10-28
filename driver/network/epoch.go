package network

import (
	"context"
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
// The function blocks until the final epoch has been reached.
func AdvanceEpoch(ctx context.Context, client rpc.Client, epochIncrement int) error {
	contract, err := driverauth100.NewContract(driverauth.ContractAddress, client)
	if err != nil {
		return fmt.Errorf("failed to get driver auth contract representation; %v", err)
	}

	currentEpoch, err := GetCurrentEpoch(client)
	if err != nil {
		return fmt.Errorf("failed to get current epoch: %w", err)
	}
	slog.Info("advancing epoch", "current", uint64(currentEpoch), "increment", epochIncrement)

	originalRules := opera.FakeNetRules(opera.GetSonicUpgrades())

	// Use Fake ID for the network
	// Driver owner is the first validator from the list i.e., index 1 (defined in genesis export in genesis.GenerateJsonGenesis)
	txOpts, err := bind.NewKeyedTransactorWithChainID(evmcore.FakeKey(1), big.NewInt(int64(originalRules.NetworkID)))
	if err != nil {
		return fmt.Errorf("failed to create txOpts; %v", err)
	}
	txOpts.Context = ctx
	txOpts.GasTipCap = systemTxGasTipCap
	txOpts.GasLimit = systemTxGasLimit

	tx, err := contract.AdvanceEpochs(txOpts, big.NewInt(int64(epochIncrement)))
	if err != nil {
		return fmt.Errorf("failed to advance epoch; %v", err)
	}

	rec, err := client.WaitTransactionReceipt(ctx, tx.Hash())
	if err != nil {
		return fmt.Errorf("failed to get receipt; %v", err)
	}

	if rec.Status != types.ReceiptStatusSuccessful {
		return fmt.Errorf("failed to advance epoch; receipt status: %v", rec.Status)
	}

	// wait until the new epoch has actually started
	start := time.Now()
	for time.Since(start) < 60*time.Second {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("aborted while waiting for epoch to advance: %w", err)
		}
		newEpoch, err := GetCurrentEpoch(client)
		if err != nil {
			return fmt.Errorf("failed to get current epoch after advancing: %w", err)
		}
		if newEpoch >= currentEpoch+hexutil.Uint64(epochIncrement) {
			logEpochSummary(ctx, client)
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("aborted while waiting for epoch to advance: %w", ctx.Err())
		case <-time.After(100 * time.Millisecond):
		}
	}

	return fmt.Errorf("failed to advance epoch: waited too long for the epoch to be advanced")
}

// WaitForEpochChange waits until the current epoch changes. It polls the
// network until a new epoch is observed or a timeout of 60 seconds is reached.
// The network's epoch is the highest the clients report, so a node catching up
// does not pass for a change.
func WaitForEpochChange(ctx context.Context, clients []rpc.Client) error {
	epoch := func() (hexutil.Uint64, error) {
		return MaxOf(ctx, clients, func(_ context.Context, client rpc.Client) (hexutil.Uint64, error) {
			return GetCurrentEpoch(client)
		})
	}
	currentEpoch, err := epoch()
	if err != nil {
		return fmt.Errorf("failed to get current epoch: %w", err)
	}

	start := time.Now()
	for time.Since(start) < 60*time.Second {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("aborted while waiting for epoch to change: %w", err)
		}
		newEpoch, err := epoch()
		if err != nil {
			return fmt.Errorf("failed to get epoch while waiting: %w", err)
		}
		if newEpoch > currentEpoch {
			slog.Info("epoch changed", "from", uint64(currentEpoch), "to", uint64(newEpoch))
			logEpochSummary(ctx, clients[0])
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("aborted while waiting for epoch to change: %w", ctx.Err())
		case <-time.After(100 * time.Millisecond):
		}
	}

	return fmt.Errorf("timed out waiting for epoch to change from %d", uint64(currentEpoch))
}

func GetCurrentEpoch(client rpc.Client) (hexutil.Uint64, error) {
	var currentEpoch hexutil.Uint64
	if err := client.Call(&currentEpoch, "eth_currentEpoch"); err != nil {
		return 0, fmt.Errorf("failed to get current epoch: %w", err)
	}
	return currentEpoch, nil
}

// GetActiveValidatorIDs returns the ids of the validators taking part in
// consensus in the epoch currently being built. SFC validator sets are
// per-epoch: a validator registered mid-epoch is absent from this set until
// that epoch is sealed.
func GetActiveValidatorIDs(ctx context.Context, client rpc.Client) ([]int, error) {
	sfcContract, err := sfc100.NewContract(sfc.ContractAddress, client)
	if err != nil {
		return nil, fmt.Errorf("failed to get SFC contract representation; %v", err)
	}

	epoch, err := sfcContract.CurrentEpoch(&bind.CallOpts{Context: ctx})
	if err != nil {
		return nil, fmt.Errorf("failed to get current epoch: %w", err)
	}

	return getValidatorIDsOfEpoch(ctx, sfcContract, epoch)
}

// getValidatorIDsOfEpoch returns the ids of the validators of the given epoch.
// Callers that already know the epoch use this to avoid reading it twice, which
// would let the two reads straddle a seal and report a mismatched pair.
func getValidatorIDsOfEpoch(
	ctx context.Context, sfcContract *sfc100.Contract, epoch *big.Int,
) ([]int, error) {
	validators, err := sfcContract.GetEpochValidatorIDs(&bind.CallOpts{Context: ctx}, epoch)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to get epoch validator IDs for epoch %v: %w", epoch, err,
		)
	}

	ids := make([]int, 0, len(validators))
	for _, id := range validators {
		ids = append(ids, int(id.Int64()))
	}
	return ids, nil
}

// logEpochSummary logs the epoch and validators the client reports. It is a
// diagnostic, so it gives up on a client that does not answer in time.
func logEpochSummary(ctx context.Context, client rpc.Client) {
	ctx, cancel := context.WithTimeout(ctx, nodeResponseTimeout)
	defer cancel()

	// get a representation of the deployed contract
	sfcContract, err := sfc100.NewContract(sfc.ContractAddress, client)
	if err != nil {
		slog.Error("failed to get SFC contract representation", "error", err)
		return
	}

	epoch, err := sfcContract.CurrentEpoch(&bind.CallOpts{Context: ctx})
	if err != nil {
		slog.Error("failed to get current epoch", "error", err)
		return
	}

	validatorIds, err := getValidatorIDsOfEpoch(ctx, sfcContract, epoch)
	if err != nil {
		slog.Error("failed to get active validators", "error", err)
		return
	}

	slog.Info("epoch summary",
		"current_epoch", epoch,
		"active_validators", validatorIds,
	)
}
