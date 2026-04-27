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
	"github.com/0xsoniclabs/sonic/gossip/blockproc/subsidies/registry"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// SubsidizedBundleApplication generates bundle transactions that combine
// subsidies with atomic execution. Each bundle contains three inner transactions:
//
//  1. sponsor → subsidies registry: fund the approval subsidy for the user's
//     upcoming approve call (gasPrice > 0, normal tx)
//  2. user → ERC-20: approve(sponsor, 1) with gasPrice=0, covered by the
//     subsidy set up in step 1
//  3. sponsor → ERC-20: transferFrom(user, sponsor, 1) to claim the token
//
// The AllOf execution plan guarantees all three run in the same block or none
// does. The sponsor's ERC-20 balance tracks the number of successful bundles.
type SubsidizedBundleApplication struct {
	erc20Contract    *contract.ERC20
	erc20Address     common.Address
	erc20Abi         *abi.ABI
	registryAddress  common.Address
	registryAbi      *abi.ABI
	accountFactory   *AccountFactory
	sponsorAddresses []common.Address
}

func NewSubsidizedBundleApplication(appContext AppContext, feederId, appId uint32) (Application, error) {
	rpcClient := appContext.GetClient()

	txOpts, err := appContext.GetTransactOptions(appContext.GetTreasure())
	if err != nil {
		return nil, fmt.Errorf("failed to get tx opts for contract deploy: %w", err)
	}
	erc20Address, deployTx, erc20Contract, err := contract.DeployERC20(txOpts, rpcClient, "Token", "TOK")
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

	registryAbi, err := registry.RegistryMetaData.GetAbi()
	if err != nil {
		return nil, fmt.Errorf("failed to parse registry ABI: %w", err)
	}

	deployReceipt, err := appContext.GetReceipt(deployTx.Hash())
	if err != nil || deployReceipt.Status != types.ReceiptStatusSuccessful {
		return nil, errors.Join(fmt.Errorf("ERC20 deploy transaction failed"), err)
	}

	return &SubsidizedBundleApplication{
		erc20Contract:   erc20Contract,
		erc20Address:    erc20Address,
		erc20Abi:        erc20Abi,
		registryAddress: registry.GetAddress(),
		registryAbi:     registryAbi,
		accountFactory:  accountFactory,
	}, nil
}

// CreateUsers creates numUsers user/sponsor pairs. The user account holds ERC-20
// tokens and submits subsidized approve txs; the sponsor funds the subsidies and
// collects tokens via transferFrom.
func (a *SubsidizedBundleApplication) CreateUsers(appContext AppContext, numUsers int) ([]User, error) {
	users := make([]User, numUsers)
	userAddresses := make([]common.Address, numUsers)
	sponsorAddresses := make([]common.Address, numUsers)

	subsidiesRegistry, err := registry.NewRegistry(a.registryAddress, appContext.GetClient())
	if err != nil {
		return nil, fmt.Errorf("failed to bind subsidies registry: %w", err)
	}

	for i := range users {
		user, err := a.accountFactory.CreateAccount(appContext.GetClient())
		if err != nil {
			return nil, err
		}
		sponsor, err := a.accountFactory.CreateAccount(appContext.GetClient())
		if err != nil {
			return nil, err
		}

		// Compute approve(sponsor, 1) calldata to derive the approval fund ID.
		fundIdApproveData, err := a.erc20Abi.Pack("approve", sponsor.address, big.NewInt(1))
		if err != nil {
			return nil, fmt.Errorf("failed to pack approve: %w", err)
		}

		// Derive the approval subsidy fund ID for this (user, erc20, sponsor) triple.
		_, approvalFundId, err := subsidiesRegistry.ApprovalSponsorshipFundId(
			nil, user.address, a.erc20Address, fundIdApproveData,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to get approval fund ID: %w", err)
		}

		users[i] = &SubsidizedBundleUser{
			erc20Address:    a.erc20Address,
			erc20Abi:        a.erc20Abi,
			registryAddress: a.registryAddress,
			registryAbi:     a.registryAbi,
			user:            user,
			sponsor:         sponsor,
			accountFactory:  a.accountFactory,
			signer:          types.LatestSignerForChainID(user.chainID),
			client:          appContext.GetClient(),
			approvalFundId:  approvalFundId,
		}
		userAddresses[i] = user.address
		sponsorAddresses[i] = sponsor.address
	}

	// Sponsors pay gas for two txs per bundle plus the subsidy value itself.
	fundsPerSponsor := new(big.Int).Mul(big.NewInt(100_000), big.NewInt(1e18))
	if err := appContext.FundAccounts(sponsorAddresses, fundsPerSponsor); err != nil {
		return nil, fmt.Errorf("failed to fund sponsor accounts: %w", err)
	}

	// Mint ERC-20 tokens to user accounts so they can be transferred later.
	tokenAmount := new(big.Int).Mul(big.NewInt(1_000_000), big.NewInt(1e18))
	receipt, err := appContext.Run(func(opts *bind.TransactOpts) (*types.Transaction, error) {
		return a.erc20Contract.MintForAll(opts, userAddresses, tokenAmount)
	})
	if err != nil || receipt.Status != types.ReceiptStatusSuccessful {
		return nil, errors.Join(fmt.Errorf("failed to mint ERC-20 tokens for users"), err)
	}

	a.sponsorAddresses = append(a.sponsorAddresses, sponsorAddresses...)
	return users, nil
}

// GetReceivedTransactions returns the total ERC-20 balance across all sponsor
// accounts, which grows by 1 for each successfully executed bundle.
func (a *SubsidizedBundleApplication) GetReceivedTransactions(rpcClient rpc.Client) (uint64, error) {
	erc20, err := contract.NewERC20(a.erc20Address, rpcClient)
	if err != nil {
		return 0, fmt.Errorf("failed to bind ERC20 contract: %w", err)
	}
	total := uint64(0)
	for _, addr := range a.sponsorAddresses {
		balance, err := erc20.BalanceOf(nil, addr)
		if err != nil {
			return 0, err
		}
		total += balance.Uint64()
	}
	return total, nil
}

// SubsidizedBundleUser represents one (user, sponsor) pair. Each GenerateTx call
// produces one AllOf bundle envelope containing:
//
//  1. sponsor.sponsor(approvalFundId, value)    — funds the approve subsidy
//  2. user.approve(sponsor, 1)                  — subsidized approve (gasPrice=0)
//  3. sponsor.transferFrom(user, sponsor, 1)    — claims the token
type SubsidizedBundleUser struct {
	erc20Address    common.Address
	erc20Abi        *abi.ABI
	registryAddress common.Address
	registryAbi     *abi.ABI
	user            *Account
	sponsor         *Account
	accountFactory  *AccountFactory
	signer          types.Signer
	client          rpc.Client
	approvalFundId  [32]byte
	sentTxs         atomic.Uint64
}

func (u *SubsidizedBundleUser) GenerateTx() (*types.Transaction, error) {
	// sponsoredValue must cover the user's approve tx gas cost.
	approveGasLimit := big.NewInt(70_000)
	sponsoredValue := new(big.Int).Mul(approveGasLimit, gasFeeCap)

	sponsorData, err := u.registryAbi.Pack("sponsor", u.approvalFundId)
	if err != nil {
		return nil, fmt.Errorf("failed to pack sponsor: %w", err)
	}

	approveData, err := u.erc20Abi.Pack("approve", u.sponsor.address, big.NewInt(1))
	if err != nil {
		return nil, fmt.Errorf("failed to pack approve: %w", err)
	}

	transferData, err := u.erc20Abi.Pack("transferFrom", u.user.address, u.sponsor.address, big.NewInt(1))
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
			bundle.Step(u.sponsor.privateKey, &types.DynamicFeeTx{
				Nonce:     u.sponsor.getCurrentNonce(),
				Gas:       90_000,
				GasFeeCap: gasFeeCap,
				GasTipCap: gasTipCap,
				To:        &u.registryAddress,
				Value:     sponsoredValue,
				Data:      sponsorData,
			}),
			bundle.Step(u.user.privateKey, &types.DynamicFeeTx{
				Nonce:     u.user.getCurrentNonce(),
				Gas:       approveGasLimit.Uint64(),
				GasFeeCap: big.NewInt(0), // gasPrice=0: covered by the subsidy from step 1
				GasTipCap: big.NewInt(0),
				To:        &u.erc20Address,
				Data:      approveData,
			}),
			bundle.Step(u.sponsor.privateKey, &types.DynamicFeeTx{
				Nonce:     u.sponsor.getCurrentNonce() + 1,
				Gas:       90_000,
				GasFeeCap: gasFeeCap,
				GasTipCap: gasTipCap,
				To:        &u.erc20Address,
				Data:      transferData,
			}),
		).
		SetEarliest(currentBlock).
		Build()

	u.sponsor.getNextNonce()
	u.user.getNextNonce()
	u.sponsor.getNextNonce()
	u.sentTxs.Add(1)
	return envelope, nil
}

func (u *SubsidizedBundleUser) GetSentTransactions() uint64 {
	return u.sentTxs.Load()
}
