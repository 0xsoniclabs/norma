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
)

// OneOfBundleApplication generates bundle transactions where each bundle
// contains a OneOf section with two transfers to a target account: one from a
// richSender (who has ERC-20 tokens) and one from a poorSender (who has none).
// The OneOf execution plan picks the first successful transaction — exactly one
// of the two inner transactions will execute per bundle.
type OneOfBundleApplication struct {
	erc20Contract  *contract.ERC20
	erc20Address   common.Address
	erc20Abi       *abi.ABI
	accountFactory *AccountFactory
	targetAddress  common.Address
}

func NewOneOfBundleApplication(appContext AppContext, feederId, appId uint32) (Application, error) {
	rpcClient := appContext.GetClient()

	txOpts, err := appContext.GetTransactOptions(appContext.GetTreasure())
	if err != nil {
		return nil, fmt.Errorf("failed to get tx opts for contract deploy: %w", err)
	}
	erc20Address, deployTx, erc20Contract, err := contract.DeployERC20(txOpts, rpcClient, "OneOf Token", "OTOK")
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

	return &OneOfBundleApplication{
		erc20Contract:  erc20Contract,
		erc20Address:   erc20Address,
		erc20Abi:       erc20Abi,
		accountFactory: accountFactory,
		targetAddress:  target.address,
	}, nil
}

// CreateUsers creates numUsers user pairs (richSender, poorSender). All users
// share the single target address created during application initialisation.
func (a *OneOfBundleApplication) CreateUsers(appContext AppContext, numUsers int) ([]User, error) {
	users := make([]User, numUsers)
	richSenderAddresses := make([]common.Address, numUsers)
	poorSenderAddresses := make([]common.Address, numUsers)

	for i := range users {
		richSender, err := a.accountFactory.CreateAccount(appContext.GetClient())
		if err != nil {
			return nil, err
		}
		poorSender, err := a.accountFactory.CreateAccount(appContext.GetClient())
		if err != nil {
			return nil, err
		}
		users[i] = &OneOfBundleUser{
			erc20Address:   a.erc20Address,
			erc20Abi:       a.erc20Abi,
			richSender:     richSender,
			poorSender:     poorSender,
			targetAddress:  a.targetAddress,
			accountFactory: a.accountFactory,
			signer:         types.NewLondonSigner(richSender.chainID),
			client:         appContext.GetClient(),
		}
		richSenderAddresses[i] = richSender.address
		poorSenderAddresses[i] = poorSender.address
	}

	// Fund all tx sending accounts with native currency for gas.
	fundsPerAccount := new(big.Int).Mul(big.NewInt(1_000), big.NewInt(1e18))
	fundedAddresses := append(richSenderAddresses, poorSenderAddresses...)
	if err := appContext.FundAccounts(fundedAddresses, fundsPerAccount); err != nil {
		return nil, fmt.Errorf("failed to fund accounts: %w", err)
	}

	// Mint ERC-20 tokens only to richSender accounts; poorSenders receive none.
	tokenAmount := new(big.Int).Mul(big.NewInt(1_000_000), big.NewInt(1e18))
	receipt, err := appContext.Run(func(opts *bind.TransactOpts) (*types.Transaction, error) {
		return a.erc20Contract.MintForAll(opts, richSenderAddresses, tokenAmount)
	})
	if err != nil || receipt.Status != types.ReceiptStatusSuccessful {
		return nil, errors.Join(fmt.Errorf("failed to mint ERC-20 tokens for rich senders"), err)
	}

	return users, nil
}

// GetReceivedTransactions returns the ERC-20 balance of the shared target
// account, which grows by 1 for each successfully executed bundle.
func (a *OneOfBundleApplication) GetReceivedTransactions(rpcClient rpc.Client) (uint64, error) {
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

// OneOfBundleUser represents one (richSender, poorSender, target) triple. Each
// GenerateTx call produces one bundle envelope containing a OneOf section with:
//
//  1. richSender.transfer(target, 1) — succeeds (richSender has tokens)
//  2. poorSender.transfer(target, 1) — fails   (poorSender has no tokens)
//
// The order of the two steps within the OneOf section is randomized on every
// call. The OneOf execution plan picks the first succeeding transaction.
type OneOfBundleUser struct {
	erc20Address   common.Address
	erc20Abi       *abi.ABI
	richSender     *Account
	poorSender     *Account
	targetAddress  common.Address
	accountFactory *AccountFactory
	signer         types.Signer
	client         rpc.Client
	sentTxs        atomic.Uint64
}

func (u *OneOfBundleUser) GenerateTx() (*types.Transaction, error) {
	random := rand.Intn(3)
	shouldFail := random == 0
	successfulFirst := random == 1

	transferAmount := big.NewInt(1)
	if shouldFail {
		transferAmount = new(big.Int).Mul(big.NewInt(1e10), big.NewInt(1e18)) // exceeds the approved allowance, causing the bundle to fail
	}
	transferData, err := u.erc20Abi.Pack("transfer", u.targetAddress, transferAmount)
	if err != nil {
		return nil, fmt.Errorf("failed to pack rich transfer: %w", err)
	}

	currentBlock, err := u.client.BlockNumber(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to get current block number: %w", err)
	}

	successfulStep := bundle.Step(u.richSender.privateKey, &types.DynamicFeeTx{
		Nonce:     u.richSender.getCurrentNonce(),
		Gas:       70_000,
		GasFeeCap: gasFeeCap,
		GasTipCap: gasTipCap,
		To:        &u.erc20Address,
		Data:      transferData,
	})
	failingStep := bundle.Step(u.poorSender.privateKey, &types.DynamicFeeTx{
		Nonce:     u.poorSender.getCurrentNonce(),
		Gas:       70_000,
		GasFeeCap: gasFeeCap,
		GasTipCap: gasTipCap,
		To:        &u.erc20Address,
		Data:      transferData,
	})

	var firstStep, secondStep bundle.BuilderStep
	if successfulFirst {
		firstStep, secondStep = successfulStep, failingStep
	} else {
		firstStep, secondStep = failingStep, successfulStep
	}
	envelope := bundle.NewBuilder().
		WithSigner(u.signer).
		OneOf(firstStep, secondStep).
		SetEarliest(currentBlock).
		Build()

	if !shouldFail {
		if successfulFirst {
			u.richSender.getNextNonce()
			// not incrementing poorSender nonce when second (not executed)
		} else {
			u.poorSender.getNextNonce() // incrementing nonce when first (failed)
			u.richSender.getNextNonce()
		}
		u.sentTxs.Add(1)
	}
	return envelope, nil
}

func (u *OneOfBundleUser) GetSentTransactions() uint64 {
	return u.sentTxs.Load()
}
