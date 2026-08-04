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

package network

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"log/slog"
	"math/big"
	"time"

	"github.com/0xsoniclabs/norma/driver/rpc"
	"github.com/0xsoniclabs/sonic/gossip/contract/sfc100"
	"github.com/0xsoniclabs/sonic/opera"
	"github.com/0xsoniclabs/sonic/opera/contracts/sfc"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

// delegationGasLimit is a fixed gas limit for delegate / undelegate
// transactions executed by external delegator accounts. SFC delegate and
// undelegate calls consume well under this limit. Using a fixed value
// bypasses eth_estimateGas, which is unreliable when a scenario has
// reduced MaxEventGas via updateRules.
const delegationGasLimit uint64 = 1_000_000

// fundingGasLimit is the gas cost of a plain value-transfer between EOAs.
const fundingGasLimit uint64 = 21_000

// DelegateToValidator delegates the given stake (in S) from the delegator
// account to the validator identified by validatorId. The delegator account
// must already hold enough funds to cover stake + gas.
func DelegateToValidator(
	ctx context.Context,
	client rpc.Client,
	validatorId int,
	stake uint64,
	delegator *ecdsa.PrivateKey,
) error {
	if stake == 0 {
		return fmt.Errorf("delegate stake must be greater than zero")
	}

	sfcContract, err := sfc100.NewContract(sfc.ContractAddress, client)
	if err != nil {
		return fmt.Errorf("failed to get SFC contract representation; %w", err)
	}

	txOpts, err := newDelegatorTxOpts(ctx, delegator)
	if err != nil {
		return err
	}
	txOpts.Value = stakeToWei(stake)

	tx, err := sfcContract.Delegate(txOpts, big.NewInt(int64(validatorId)))
	if err != nil {
		return fmt.Errorf("failed to submit delegate tx; %w", err)
	}

	receipt, err := client.WaitTransactionReceipt(ctx, tx.Hash())
	if err != nil {
		return fmt.Errorf("failed to delegate, receipt error: %w", err)
	}
	if receipt.Status != types.ReceiptStatusSuccessful {
		return fmt.Errorf("delegate transaction reverted (validator=%d)", validatorId)
	}

	slog.Info(
		"delegated stake to validator",
		"validator_id", validatorId,
		"stake", stake,
		"delegator", crypto.PubkeyToAddress(delegator.PublicKey).Hex(),
	)
	return nil
}

// UndelegateFromValidator undelegates the given stake (in S) from the
// validator, signing the transaction with the delegator's private key.
// When stake is 0, the current on-chain stake for the delegator on the
// given validator is queried and fully undelegated.
func UndelegateFromValidator(
	ctx context.Context,
	client rpc.Client,
	validatorId int,
	stake uint64,
	delegator *ecdsa.PrivateKey,
) error {
	sfcContract, err := sfc100.NewContract(sfc.ContractAddress, client)
	if err != nil {
		return fmt.Errorf("failed to get SFC contract representation; %w", err)
	}

	txOpts, err := newDelegatorTxOpts(ctx, delegator)
	if err != nil {
		return err
	}

	var stakeWei *big.Int
	if stake == 0 {
		addr := crypto.PubkeyToAddress(delegator.PublicKey)
		stakeWei, err = sfcContract.GetStake(nil, addr, big.NewInt(int64(validatorId)))
		if err != nil {
			return fmt.Errorf("failed to query stake for delegator %s on validator %d; %w",
				addr.Hex(), validatorId, err)
		}
		if stakeWei.Sign() == 0 {
			return fmt.Errorf("delegator %s has no stake on validator %d",
				addr.Hex(), validatorId)
		}
	} else {
		stakeWei = stakeToWei(stake)
	}

	// withdraw ID must be unique per (delegator, validator) pair.
	// We use a timestamp as a best-effort unique value within a scenario run.
	withdrawId := big.NewInt(time.Now().UnixNano())
	tx, err := sfcContract.Undelegate(txOpts, big.NewInt(int64(validatorId)), withdrawId, stakeWei)
	if err != nil {
		return fmt.Errorf("failed to submit undelegate tx; %w", err)
	}

	receipt, err := client.WaitTransactionReceipt(ctx, tx.Hash())
	if err != nil {
		return fmt.Errorf("failed to undelegate, receipt error: %w", err)
	}
	if receipt.Status != types.ReceiptStatusSuccessful {
		return fmt.Errorf("undelegate transaction reverted (validator=%d)", validatorId)
	}

	slog.Info(
		"undelegated stake from validator",
		"validator_id", validatorId,
		"delegator", crypto.PubkeyToAddress(delegator.PublicKey).Hex(),
	)
	return nil
}

// GetDelegatorStake returns the current on-chain stake (in S) held by
// the given delegator on the given validator.
func GetDelegatorStake(
	client rpc.Client,
	delegator common.Address,
	validatorId int,
) (uint64, error) {
	sfcContract, err := sfc100.NewContract(sfc.ContractAddress, client)
	if err != nil {
		return 0, fmt.Errorf("failed to get SFC contract representation; %w", err)
	}
	stakeWei, err := sfcContract.GetStake(nil, delegator, big.NewInt(int64(validatorId)))
	if err != nil {
		return 0, fmt.Errorf("failed to query stake; %w", err)
	}
	return weiToStake(stakeWei), nil
}

// FundAccount transfers `amount` S from the source account to the destination
// address using a plain value-transfer transaction.
func FundAccount(
	ctx context.Context,
	client rpc.Client,
	source *ecdsa.PrivateKey,
	dest common.Address,
	amount uint64,
) error {
	from := crypto.PubkeyToAddress(source.PublicKey)

	nonce, err := client.PendingNonceAt(ctx, from)
	if err != nil {
		return fmt.Errorf("failed to get pending nonce for %s; %w", from.Hex(), err)
	}

	chainId := big.NewInt(int64(opera.FakeNetRules(opera.GetSonicUpgrades()).NetworkID))

	tx := types.NewTx(&types.DynamicFeeTx{
		ChainID:   chainId,
		Nonce:     nonce,
		GasFeeCap: big.NewInt(1e12),
		GasTipCap: big.NewInt(0),
		Gas:       fundingGasLimit,
		To:        &dest,
		Value:     stakeToWei(amount),
	})
	signed, err := types.SignTx(tx, types.NewLondonSigner(chainId), source)
	if err != nil {
		return fmt.Errorf("failed to sign funding tx; %w", err)
	}
	if err := client.SendTransaction(ctx, signed); err != nil {
		return fmt.Errorf("failed to submit funding tx; %w", err)
	}
	receipt, err := client.WaitTransactionReceipt(ctx, signed.Hash())
	if err != nil {
		return fmt.Errorf("failed to fund account, receipt error: %w", err)
	}
	if receipt.Status != types.ReceiptStatusSuccessful {
		return fmt.Errorf("funding transaction reverted (dest=%s)", dest.Hex())
	}
	return nil
}

// weiToStake converts a wei value to the corresponding stake amount (in S).
// Values are rounded down.
func weiToStake(wei *big.Int) uint64 {
	if wei == nil {
		return 0
	}
	stake := new(big.Int).Quo(wei, big.NewInt(1e18))
	if !stake.IsUint64() {
		return 0
	}
	return stake.Uint64()
}

// newDelegatorTxOpts builds a signer TransactOpts for the given delegator key
// with the fixed delegation gas limit and a fresh tip cap.
func newDelegatorTxOpts(ctx context.Context, delegator *ecdsa.PrivateKey) (*bind.TransactOpts, error) {
	chainId := big.NewInt(int64(opera.FakeNetRules(opera.GetSonicUpgrades()).NetworkID))
	txOpts, err := bind.NewKeyedTransactorWithChainID(delegator, chainId)
	if err != nil {
		return nil, fmt.Errorf("failed to create txOpts; %w", err)
	}
	txOpts.Context = ctx
	txOpts.GasTipCap = systemTxGasTipCap
	txOpts.GasLimit = delegationGasLimit
	return txOpts, nil
}
