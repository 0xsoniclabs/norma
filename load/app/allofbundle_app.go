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
	"sync/atomic"

	"github.com/0xsoniclabs/norma/driver/rpc"
	contract "github.com/0xsoniclabs/norma/load/contracts/abi"
	"github.com/0xsoniclabs/sonic/gossip/blockproc/bundle"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// AllOfBundleApplication generates Brio bundle transactions where two accounts
// cooperate atomically: account A grants ERC-20 allowance to account B and
// account B immediately exercises that allowance via transferFrom. The AllOf
// execution plan guarantees both transactions are included in the same block or
// neither is.
type AllOfBundleApplication struct {
	erc20Contract    *contract.ERC20
	erc20Address     common.Address
	erc20Abi         *abi.ABI
	accountFactory   *AccountFactory
	spenderAddresses []common.Address
}

func NewAllOfBundleApplication(appContext AppContext, feederId, appId uint32) (Application, error) {
	rpcClient := appContext.GetClient()

	// Deploy the ERC-20 contract used by all user pairs.
	txOpts, err := appContext.GetTransactOptions(appContext.GetTreasure())
	if err != nil {
		return nil, fmt.Errorf("failed to get tx opts for contract deploy: %w", err)
	}
	erc20Address, deployTx, erc20Contract, err := contract.DeployERC20(txOpts, rpcClient, "Bundle Token", "BTOK")
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

	// wait until the contract will be available on the chain (and will be possible to call CreateGenerator)
	deployReceipt, err := appContext.GetReceipt(deployTx.Hash())
	if err != nil || deployReceipt.Status != types.ReceiptStatusSuccessful {
		return nil, errors.Join(fmt.Errorf("ERC20 deploy transaction failed"), err)
	}

	return &AllOfBundleApplication{
		erc20Contract:  erc20Contract,
		erc20Address:   erc20Address,
		erc20Abi:       erc20Abi,
		accountFactory: accountFactory,
	}, nil
}

// CreateUsers creates numUsers user triples (approver, spender, bundler).
// Each triple shares one AllOfBundleUser that generates one bundle envelope per
// GenerateTx call.
func (a *AllOfBundleApplication) CreateUsers(appContext AppContext, numUsers int) ([]User, error) {
	users := make([]User, numUsers)
	approverAddresses := make([]common.Address, numUsers)
	spenderAddresses := make([]common.Address, numUsers)

	for i := range users {
		approver, err := a.accountFactory.CreateAccount(appContext.GetClient())
		if err != nil {
			return nil, err
		}
		spender, err := a.accountFactory.CreateAccount(appContext.GetClient())
		if err != nil {
			return nil, err
		}
		users[i] = &AllOfBundleUser{
			erc20Address:   a.erc20Address,
			erc20Abi:       a.erc20Abi,
			approver:       approver,
			spender:        spender,
			accountFactory: a.accountFactory,
			signer:         types.LatestSignerForChainID(approver.chainID),
			client:         appContext.GetClient(),
		}
		approverAddresses[i] = approver.address
		spenderAddresses[i] = spender.address
	}

	// Fund all accounts with native currency for gas.
	fundsPerAccount := new(big.Int).Mul(big.NewInt(1_000), big.NewInt(1e18))
	allAddresses := append(approverAddresses, spenderAddresses...)
	if err := appContext.FundAccounts(allAddresses, fundsPerAccount); err != nil {
		return nil, fmt.Errorf("failed to fund accounts: %w", err)
	}

	// Mint ERC-20 tokens to approver accounts so they can grant allowances.
	tokenAmount := new(big.Int).Mul(big.NewInt(1_000_000), big.NewInt(1e18))
	receipt, err := appContext.Run(func(opts *bind.TransactOpts) (*types.Transaction, error) {
		return a.erc20Contract.MintForAll(opts, approverAddresses, tokenAmount)
	})
	if err != nil || receipt.Status != types.ReceiptStatusSuccessful {
		return nil, errors.Join(fmt.Errorf("failed to mint ERC-20 tokens for approvers"), err)
	}

	a.spenderAddresses = append(a.spenderAddresses, spenderAddresses...)
	return users, nil
}

// GetReceivedTransactions returns the total ERC-20 balance across all spender
// accounts, which grows by 1 for each successfully executed bundle.
func (a *AllOfBundleApplication) GetReceivedTransactions(rpcClient rpc.Client) (uint64, error) {
	erc20, err := contract.NewERC20(a.erc20Address, rpcClient)
	if err != nil {
		return 0, fmt.Errorf("failed to bind ERC20 contract: %w", err)
	}
	total := uint64(0)
	for _, addr := range a.spenderAddresses {
		balance, err := erc20.BalanceOf(nil, addr)
		if err != nil {
			return 0, err
		}
		total += balance.Uint64()
	}
	return total, nil
}

// AllOfBundleUser represents one (approver, spender, bundler) triple. Each
// GenerateTx call produces one bundle envelope containing:
//
//  1. approver.approve(spender, 1)               — grants allowance of 1 token
//  2. spender.transferFrom(approver, spender, 1) — claims the token
//
// The envelope is sent from the bundler account. The AllOf execution plan
// ensures both inner transactions run or neither does.
type AllOfBundleUser struct {
	erc20Address   common.Address
	erc20Abi       *abi.ABI
	approver       *Account
	spender        *Account
	accountFactory *AccountFactory
	signer         types.Signer
	client         rpc.Client
	sentTxs        atomic.Uint64
}

func (u *AllOfBundleUser) GenerateTx() (*types.Transaction, error) {

	approveData, err := u.erc20Abi.Pack("approve", u.spender.address, big.NewInt(1))
	if err != nil {
		return nil, fmt.Errorf("failed to pack approve: %w", err)
	}

	transferData, err := u.erc20Abi.Pack("transferFrom", u.approver.address, u.spender.address, big.NewInt(1))
	if err != nil {
		return nil, fmt.Errorf("failed to pack transferFrom: %w", err)
	}

	currentBlock, err := u.client.BlockNumber(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to get current block number: %w", err)
	}

	envelope := bundle.NewBuilder().
		WithSigner(u.signer).
		AllOf(
			bundle.Step(u.approver.privateKey, &types.DynamicFeeTx{
				Nonce:     u.approver.getCurrentNonce(),
				Gas:       70_000, // base (21k) + approve SSTORE (22k) + event
				GasFeeCap: gasFeeCap,
				GasTipCap: gasTipCap,
				To:        &u.erc20Address,
				Data:      approveData,
			}),
			bundle.Step(u.spender.privateKey, &types.DynamicFeeTx{
				Nonce:     u.spender.getCurrentNonce(),
				Gas:       90_000, // base (21k) + 3x SLOAD + 3x SSTORE (22k) + event
				GasFeeCap: gasFeeCap,
				GasTipCap: gasTipCap,
				To:        &u.erc20Address,
				Data:      transferData,
			}),
		).
		SetEarliest(currentBlock).
		Build()

	u.approver.getNextNonce()
	u.spender.getNextNonce()
	u.sentTxs.Add(1)
	return envelope, nil
}

func (u *AllOfBundleUser) GetSentTransactions() uint64 {
	return u.sentTxs.Load()
}
