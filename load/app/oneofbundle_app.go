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
// contains a OneOf section with two transfers to a target account, both signed
// by the same sender. One step transfers 1 token (succeeds) and the other
// attempts to transfer more tokens than the sender holds (fails). The OneOf
// execution plan picks the first successful transaction — exactly one of the
// two inner transactions will execute per bundle.
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

// CreateUsers creates numUsers users. All users share the single target address
// created during application initialisation.
func (a *OneOfBundleApplication) CreateUsers(appContext AppContext, numUsers int) ([]User, error) {
	users := make([]User, numUsers)
	senderAddresses := make([]common.Address, numUsers)

	for i := range users {
		sender, err := a.accountFactory.CreateAccount(appContext.GetClient())
		if err != nil {
			return nil, err
		}
		users[i] = &OneOfBundleUser{
			erc20Address:   a.erc20Address,
			erc20Abi:       a.erc20Abi,
			sender:         sender,
			targetAddress:  a.targetAddress,
			accountFactory: a.accountFactory,
			signer:         types.LatestSignerForChainID(sender.chainID),
			client:         appContext.GetClient(),
		}
		senderAddresses[i] = sender.address
	}

	// Fund all tx sending accounts with native currency for gas.
	fundsPerAccount := new(big.Int).Mul(big.NewInt(1_000), big.NewInt(1e18))
	if err := appContext.FundAccounts(senderAddresses, fundsPerAccount); err != nil {
		return nil, fmt.Errorf("failed to fund accounts: %w", err)
	}

	// Mint ERC-20 tokens to sender accounts.
	tokenAmount := new(big.Int).Mul(big.NewInt(1_000_000), big.NewInt(1e18))
	receipt, err := appContext.Run(func(opts *bind.TransactOpts) (*types.Transaction, error) {
		return a.erc20Contract.MintForAll(opts, senderAddresses, tokenAmount)
	})
	if err != nil || receipt.Status != types.ReceiptStatusSuccessful {
		return nil, errors.Join(fmt.Errorf("failed to mint ERC-20 tokens for senders"), err)
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

// OneOfBundleUser holds a single sender account. Each GenerateTx call produces
// one bundle envelope containing a OneOf section with:
//
//  1. sender.transfer(target, 1)               — succeeds (sender has tokens)
//  2. sender.transfer(target, overBalance)     — fails   (amount exceeds balance)
//
// Both steps share the same nonce. The order of the two steps within the OneOf
// section is randomized on every call. The OneOf execution plan picks the first
// succeeding transaction — exactly one execution per bundle.
type OneOfBundleUser struct {
	erc20Address   common.Address
	erc20Abi       *abi.ABI
	sender         *Account
	targetAddress  common.Address
	accountFactory *AccountFactory
	signer         types.Signer
	client         rpc.Client
	sentTxs        atomic.Uint64
}

func (u *OneOfBundleUser) GenerateTx() (*types.Transaction, error) {
	successfulFirst := rand.Intn(2) == 0

	transferSuccessfulData, err := u.erc20Abi.Pack("transfer", u.targetAddress, big.NewInt(1))
	if err != nil {
		return nil, fmt.Errorf("failed to pack transfer data: %w", err)
	}

	// Exceeds the minted token supply (1_000_000 * 1e18), so this step always fails.
	overBalanceAmount := new(big.Int).Mul(big.NewInt(2_000_000), big.NewInt(1e18))
	transferFailingData, err := u.erc20Abi.Pack("transfer", u.targetAddress, overBalanceAmount)
	if err != nil {
		return nil, fmt.Errorf("failed to pack over-balance transfer data: %w", err)
	}

	currentBlock, err := u.client.BlockNumber(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to get current block number: %w", err)
	}

	var firstStep, secondStep bundle.BuilderStep
	if successfulFirst {
		firstStep = bundle.Step(u.sender.privateKey, &types.DynamicFeeTx{
			Nonce:     u.sender.getNextNonce(),
			Gas:       70_000,
			GasFeeCap: gasFeeCap,
			GasTipCap: gasTipCap,
			To:        &u.erc20Address,
			Data:      transferSuccessfulData,
		})
		secondStep = bundle.Step(u.sender.privateKey, &types.DynamicFeeTx{
			Nonce:     u.sender.getCurrentNonce(), // will not run, nonce not consumed
			Gas:       70_000,
			GasFeeCap: gasFeeCap,
			GasTipCap: gasTipCap,
			To:        &u.erc20Address,
			Data:      transferFailingData,
		})
	} else {
		firstStep = bundle.Step(u.sender.privateKey, &types.DynamicFeeTx{
			Nonce:     u.sender.getNextNonce(),
			Gas:       70_000,
			GasFeeCap: gasFeeCap,
			GasTipCap: gasTipCap,
			To:        &u.erc20Address,
			Data:      transferFailingData,
		})
		secondStep = bundle.Step(u.sender.privateKey, &types.DynamicFeeTx{
			Nonce:     u.sender.getNextNonce(), // both will run, both nonces will be consumed
			Gas:       70_000,
			GasFeeCap: gasFeeCap,
			GasTipCap: gasTipCap,
			To:        &u.erc20Address,
			Data:      transferSuccessfulData,
		})
	}
	envelope := bundle.NewBuilder().
		WithSigner(u.signer).
		OneOf(firstStep, secondStep).
		SetEarliest(currentBlock).
		Build()

	u.sentTxs.Add(1)
	return envelope, nil
}

func (u *OneOfBundleUser) GetSentTransactions() uint64 {
	return u.sentTxs.Load()
}
