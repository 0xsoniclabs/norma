package network

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"github.com/0xsoniclabs/sonic/evmcore"
	"github.com/0xsoniclabs/sonic/gossip/blockproc/netrules"
	"github.com/0xsoniclabs/sonic/opera"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/core/types"
)

// HandOverToConsensusChain flips the on-chain NetworkRules useConsensusChain
// flag on, handing block production over to the Sonic consensus engine from the
// next block. It requires the Sonic engine to already be running (in shadow) on
// the nodes, i.e. the RunConsensusChain upgrade enabled and the mesh seeded.
//
// The setter is owner-gated in production but open on a dev/fakenet, so the
// transaction is signed with the same fake key used for the other system
// operations and processed by whichever engine is canonical at the time (the
// legacy engine, pre-hand-over).
func HandOverToConsensusChain(ctx context.Context, backend ContractBackend) error {
	contract, err := netrules.NewNetworkRulesTransactor(netrules.GetAddress(), backend)
	if err != nil {
		return fmt.Errorf("failed to get network rules contract representation; %v", err)
	}

	originalRules := opera.FakeNetRules(opera.GetSonicUpgrades())
	txOpts, err := bind.NewKeyedTransactorWithChainID(evmcore.FakeKey(1), big.NewInt(int64(originalRules.NetworkID)))
	if err != nil {
		return fmt.Errorf("failed to create txOpts; %v", err)
	}
	txOpts.Context = ctx
	txOpts.GasTipCap = systemTxGasTipCap
	txOpts.GasLimit = systemTxGasLimit

	tx, err := contract.SetUseConsensusChain(txOpts, true)
	if err != nil {
		return fmt.Errorf("failed to set useConsensusChain; %v", err)
	}

	rec, err := backend.WaitTransactionReceipt(ctx, tx.Hash())
	if err != nil {
		return fmt.Errorf("failed to get receipt; %v", err)
	}
	if rec.Status != types.ReceiptStatusSuccessful {
		return fmt.Errorf("failed to hand over to consensus chain; receipt status: %v", rec.Status)
	}

	// Wait until the flip is readable. The receipt is served at the block's
	// commit, but a read at "latest" resolves through the asynchronously written
	// archive state, so it can transiently see the pre-flip value. The flag is
	// one-way, so waiting for true is sound.
	caller, err := netrules.NewNetworkRulesCaller(netrules.GetAddress(), backend)
	if err != nil {
		return fmt.Errorf("failed to get network rules caller; %v", err)
	}
	start := time.Now()
	for time.Since(start) < 60*time.Second {
		active, err := caller.GetUseConsensusChain(&bind.CallOpts{Context: ctx})
		if err == nil && active {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("aborted while waiting for hand-over to take effect: %w", ctx.Err())
		case <-time.After(100 * time.Millisecond):
		}
	}
	return fmt.Errorf("timed out waiting for the hand-over flag to become active")
}
