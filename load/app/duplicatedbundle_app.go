// Copyright 2026 Fantom Foundation
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

package app

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"math/rand"
	"sync/atomic"

	"github.com/0xsoniclabs/norma/driver/rpc"
	contract "github.com/0xsoniclabs/norma/load/contracts/abi"
	"github.com/0xsoniclabs/sonic/gossip/blockproc/bundle"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

// duplicatedBundleThreshold is the number of times each ExecutionPlan is
// submitted before new steps are generated. Each submission uses a different
// random envelope key, producing distinct envelope transactions that all carry
// the same inner bundle.
const duplicatedBundleThreshold = 2

// DuplicatedBundleApplication generates bundles where each ExecutionPlan is
// submitted duplicatedBundleThreshold times. Every submission wraps the same
// inner transactions in a new envelope signed by a fresh random key. Steps are
// randomly AllOf or OneOf with two senders. Only one envelope per plan can
// execute, because all duplicates share the same inner-transaction nonces.
type DuplicatedBundleApplication struct {
	erc20Contract  *contract.ERC20
	erc20Address   common.Address
	erc20Abi       *abi.ABI
	accountFactory *AccountFactory
	targetAddress  common.Address
}

func NewDuplicatedBundleApplication(appContext AppContext, feederId, appId uint32) (Application, error) {
	rpcClient := appContext.GetClient()

	txOpts, err := appContext.GetTransactOptions(appContext.GetTreasure())
	if err != nil {
		return nil, fmt.Errorf("failed to get tx opts for contract deploy: %w", err)
	}
	erc20Address, deployTx, erc20Contract, err := contract.DeployERC20(txOpts, rpcClient, "Duplicated Token", "DTOK")
	if err != nil {
		return nil, fmt.Errorf("failed to deploy ERC20 contract: %w", err)
	}

	accountFactory, err := NewAccountFactory(appContext.GetTreasure().chainID, feederId, appId)
	if err != nil {
		return nil, err
	}

	erc20Abi, err := contract.ERC20MetaData.GetAbi()
	if err != nil {
		return nil, fmt.Errorf("failed to parse ERC20 ABI: %w", err)
	}

	deployReceipt, err := appContext.GetReceipt(deployTx.Hash())
	if err != nil || deployReceipt.Status != types.ReceiptStatusSuccessful {
		return nil, errors.Join(fmt.Errorf("ERC20 deploy transaction failed"), err)
	}

	target, err := accountFactory.CreateAccount(rpcClient)
	if err != nil {
		return nil, fmt.Errorf("failed to create target account: %w", err)
	}

	return &DuplicatedBundleApplication{
		erc20Contract:  erc20Contract,
		erc20Address:   erc20Address,
		erc20Abi:       erc20Abi,
		accountFactory: accountFactory,
		targetAddress:  target.address,
	}, nil
}

// CreateUsers creates numUsers (senderA, senderB) pairs. All users share the
// single target address created during application initialisation.
func (a *DuplicatedBundleApplication) CreateUsers(appContext AppContext, numUsers int) ([]User, error) {
	users := make([]User, numUsers)
	senderAAddresses := make([]common.Address, numUsers)
	senderBAddresses := make([]common.Address, numUsers)

	for i := range users {
		senderA, err := a.accountFactory.CreateAccount(appContext.GetClient())
		if err != nil {
			return nil, err
		}
		senderB, err := a.accountFactory.CreateAccount(appContext.GetClient())
		if err != nil {
			return nil, err
		}
		users[i] = &DuplicatedBundleUser{
			erc20Address:  a.erc20Address,
			erc20Abi:      a.erc20Abi,
			senderA:       senderA,
			senderB:       senderB,
			targetAddress: a.targetAddress,
			signer:        types.LatestSignerForChainID(senderA.chainID),
			client:        appContext.GetClient(),
		}
		senderAAddresses[i] = senderA.address
		senderBAddresses[i] = senderB.address
	}

	fundsPerAccount := new(big.Int).Mul(big.NewInt(1_000), big.NewInt(1e18))
	allAddresses := append(senderAAddresses, senderBAddresses...)
	if err := appContext.FundAccounts(allAddresses, fundsPerAccount); err != nil {
		return nil, fmt.Errorf("failed to fund accounts: %w", err)
	}

	tokenAmount := new(big.Int).Mul(big.NewInt(1_000_000), big.NewInt(1e18))
	receipt, err := appContext.Run(func(opts *bind.TransactOpts) (*types.Transaction, error) {
		return a.erc20Contract.MintForAll(opts, allAddresses, tokenAmount)
	})
	if err != nil || receipt.Status != types.ReceiptStatusSuccessful {
		return nil, errors.Join(fmt.Errorf("failed to mint ERC-20 tokens for senders"), err)
	}

	return users, nil
}

// GetReceivedTransactions returns the ERC-20 balance of the shared target
// account. Each successfully executed bundle transfers tokens to the target
// (1 per step for OneOf, 2 per bundle for AllOf).
func (a *DuplicatedBundleApplication) GetReceivedTransactions(rpcClient rpc.Client) (uint64, error) {
	erc20, err := contract.NewERC20(a.erc20Address, rpcClient)
	if err != nil {
		return 0, fmt.Errorf("failed to bind ERC20 contract: %w", err)
	}
	balance, err := erc20.BalanceOf(nil, a.targetAddress)
	if err != nil {
		return 0, err
	}
	return balance.Uint64(), nil
}

// DuplicatedBundleUser represents a (senderA, senderB) pair. On the first use
// of each plan the builder signs the inner transactions and produces the first
// envelope; subsequent uses reuse the encoded payload from that envelope,
// signing only the outer AccessListTx with a fresh random key. Once the plan
// has been submitted duplicatedBundleThreshold times both sender nonces are
// advanced and a new plan is generated. Steps are randomly AllOf or OneOf.
type DuplicatedBundleUser struct {
	erc20Address  common.Address
	erc20Abi      *abi.ABI
	senderA       *Account
	senderB       *Account
	targetAddress common.Address
	signer        types.Signer
	client        rpc.Client
	sentTxs       atomic.Uint64

	usedCount     int
	savedEnvelope *types.Transaction
}

func (u *DuplicatedBundleUser) GenerateTx() (*types.Transaction, error) {
	if u.savedEnvelope == nil || u.usedCount >= duplicatedBundleThreshold {
		tx, err := u.generateNewBundleTx()
		if err != nil {
			return nil, err
		}
		u.savedEnvelope = tx
		u.sentTxs.Add(1)
		u.usedCount = 1
		return tx, nil
	}
	tx, err := u.generateNewEnvelopeForExistingBundle(u.savedEnvelope)
	if err != nil {
		return nil, err
	}
	u.usedCount++
	return tx, nil
}

// generateNewBundleTx builds a fresh bundle tx.
func (u *DuplicatedBundleUser) generateNewBundleTx() (*types.Transaction, error) {
	currentBlock, err := u.client.BlockNumber(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to get current block number: %w", err)
	}
	transferToTargetData, err := u.erc20Abi.Pack("transfer", u.targetAddress, big.NewInt(1))
	if err != nil {
		return nil, fmt.Errorf("failed to pack transfer: %w", err)
	}
	transferToBData, err := u.erc20Abi.Pack("transfer", u.senderB.address, big.NewInt(1))
	if err != nil {
		return nil, fmt.Errorf("failed to pack transfer: %w", err)
	}

	b := bundle.NewBuilder().WithSigner(u.signer).SetEarliest(currentBlock)
	if rand.Intn(2) == 0 {
		b = b.OneOf(bundle.Step(u.senderA.privateKey, types.DynamicFeeTx{
			Nonce: u.senderA.getNextNonce(), Gas: 70_000, GasFeeCap: gasFeeCap, GasTipCap: gasTipCap,
			To: &u.erc20Address, Data: transferToTargetData,
		}), bundle.Step(u.senderB.privateKey, &types.DynamicFeeTx{
			Nonce: u.senderB.getCurrentNonce(), Gas: 70_000, GasFeeCap: gasFeeCap, GasTipCap: gasTipCap,
			To: &u.erc20Address, Data: transferToTargetData,
		}))
	} else {
		b = b.AllOf(bundle.Step(u.senderA.privateKey, types.DynamicFeeTx{
			Nonce: u.senderA.getNextNonce(), Gas: 70_000, GasFeeCap: gasFeeCap, GasTipCap: gasTipCap,
			To: &u.erc20Address, Data: transferToBData,
		}), bundle.Step(u.senderB.privateKey, &types.DynamicFeeTx{
			Nonce: u.senderB.getNextNonce(), Gas: 70_000, GasFeeCap: gasFeeCap, GasTipCap: gasTipCap,
			To: &u.erc20Address, Data: transferToTargetData,
		}))
	}
	return b.Build(), nil
}

// generateNewEnvelopeForExistingBundle reuses the encoded payload of an existing bundle tx, signs it with a new bundler key.
func (u *DuplicatedBundleUser) generateNewEnvelopeForExistingBundle(oldBundle *types.Transaction) (*types.Transaction, error) {
	bundlerKey, err := crypto.GenerateKey()
	if err != nil {
		return nil, fmt.Errorf("failed to generate envelope key: %w", err)
	}
	tx, err := types.SignNewTx(bundlerKey, u.signer, &types.AccessListTx{
		To:   &bundle.BundleProcessor,
		Data: oldBundle.Data(),
		Gas:  oldBundle.Gas(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to sign duplicate envelope: %w", err)
	}
	return tx, nil
}

func (u *DuplicatedBundleUser) GetSentTransactions() uint64 {
	return u.sentTxs.Load()
}
