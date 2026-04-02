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
	crand "crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"math/rand"
	"sync/atomic"

	"github.com/0xsoniclabs/norma/driver/rpc"
	"github.com/0xsoniclabs/norma/load/app/bundling"
	contract "github.com/0xsoniclabs/norma/load/contracts/abi"
	"github.com/0xsoniclabs/sonic/gossip/blockproc/subsidies/registry"
	ethabi "github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// BundleSubsidyApplication generates bundled transactions (Brio hard-fork) that
// exercise the approval-based gas subsidy path. Each bundle has three core steps:
//
//  1. subsidiesRegistry.Sponsor(fundId)  — sponsor funds the subsidy pool for the
//     per-user approval operation; sent by sponsorAccount with an ETH value.
//  2. erc20.approve(sponsorAccount, 1)   — run at GasFeeCap=0, covered by step 1's subsidy.
//  3. erc20.transferFrom(userAccount, recipient, 1) — consumes the approval; the
//     token lands in the fixed recipient address that tracks successful bundles.
//
// With 25 % probability a fourth step is appended:
//
//  4. erc20.transferFrom(userAccount, recipient, 1) — always reverts because the
//     allowance granted in step 2 was already spent in step 3. With EF_AllOf
//     semantics the revert cascades to the whole bundle, so the recipient does not
//     receive a token and the bundle counts as failed.
//
// GetReceivedTransactions returns the ERC-20 balance of the recipient address,
// which equals the number of 3-step bundles that executed successfully.
type BundleSubsidyApplication struct {
	erc20Address    common.Address
	recipient       common.Address
	registryAddress common.Address
	registryAbi     *ethabi.ABI
	erc20Abi        *ethabi.ABI
	accountFactory  *AccountFactory
	chainId         *big.Int
}

func NewBundleSubsidyApplication(appContext AppContext, feederId, appId uint32) (Application, error) {
	client := appContext.GetClient()
	chainId, err := client.ChainID(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to get chain ID; %w", err)
	}

	accountFactory, err := NewAccountFactory(chainId, feederId, appId)
	if err != nil {
		return nil, err
	}

	// Deploy the ERC-20 contract whose token balance tracks successful bundles.
	_, receipt, err := DeployContract(appContext, func(opts *bind.TransactOpts, backend bind.ContractBackend) (common.Address, *types.Transaction, *contract.ERC20, error) {
		return contract.DeployERC20(opts, backend, "BundleSubsidy Token", "BST")
	})
	if err != nil || receipt.Status != types.ReceiptStatusSuccessful {
		return nil, errors.Join(fmt.Errorf("failed to deploy ERC20 contract"), err)
	}
	erc20Address := receipt.ContractAddress

	registryAddress := registry.GetAddress()

	registryAbi, err := registry.RegistryMetaData.GetAbi()
	if err != nil {
		return nil, fmt.Errorf("failed to parse registry ABI; %w", err)
	}
	erc20Abi, err := contract.ERC20MetaData.GetAbi()
	if err != nil {
		return nil, fmt.Errorf("failed to parse ERC20 ABI; %w", err)
	}

	// A single fixed address receives 1 token per successful bundle.
	var recipient common.Address
	if _, err := crand.Read(recipient[:]); err != nil {
		return nil, fmt.Errorf("failed to generate recipient address; %w", err)
	}

	return &BundleSubsidyApplication{
		erc20Address:    erc20Address,
		recipient:       recipient,
		registryAddress: registryAddress,
		registryAbi:     registryAbi,
		erc20Abi:        erc20Abi,
		accountFactory:  accountFactory,
		chainId:         chainId,
	}, nil
}

func (f *BundleSubsidyApplication) CreateUsers(appContext AppContext, numUsers int) ([]User, error) {
	users := make([]User, numUsers)
	sponsorAddresses := make([]common.Address, numUsers)
	userAddresses := make([]common.Address, numUsers)

	subsidiesRegistry, err := registry.NewRegistry(f.registryAddress, appContext.GetClient())
	if err != nil {
		return nil, fmt.Errorf("failed to bind subsidies registry; %w", err)
	}

	for i := range users {
		sponsorAccount, err := f.accountFactory.CreateAccount(appContext.GetClient())
		if err != nil {
			return nil, err
		}
		userAccount, err := f.accountFactory.CreateAccount(appContext.GetClient())
		if err != nil {
			return nil, err
		}

		// The subsidy fund ID is derived from the per-user approve call:
		// userAccount approves sponsorAccount to spend 1 token from erc20Address.
		// Because the sender (userAccount) differs per user, so does the fund ID.
		approveCallData, err := f.erc20Abi.Pack("approve", sponsorAccount.address, big.NewInt(1))
		if err != nil {
			return nil, fmt.Errorf("failed to pack approve calldata; %w", err)
		}
		_, fundId, err := subsidiesRegistry.ApprovalSponsorshipFundId(
			nil,
			userAccount.address,
			f.erc20Address,
			approveCallData,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve subsidy fund ID for user %d; %w", i, err)
		}

		users[i] = &BundleSubsidyUser{
			client:          appContext.GetClient(),
			sponsorAccount:  sponsorAccount,
			userAccount:     userAccount,
			registryAddress: f.registryAddress,
			registryAbi:     f.registryAbi,
			erc20Address:    f.erc20Address,
			erc20Abi:        f.erc20Abi,
			fundId:          fundId,
			recipient:       f.recipient,
			chainId:         f.chainId,
		}
		sponsorAddresses[i] = sponsorAccount.address
		userAddresses[i] = userAccount.address
	}

	// Fund each sponsor account: enough ETH for many Sponsor-value + envelope gas cycles.
	fundsPerSponsor := new(big.Int).Mul(big.NewInt(1_000), big.NewInt(1e15))
	if err := appContext.FundAccounts(sponsorAddresses, fundsPerSponsor); err != nil {
		return nil, fmt.Errorf("failed to fund sponsor accounts; %w", err)
	}

	// Mint ERC-20 tokens to each userAccount so it has tokens to approve and transfer.
	erc20Contract, err := contract.NewERC20(f.erc20Address, appContext.GetClient())
	if err != nil {
		return nil, fmt.Errorf("failed to get ERC20 contract; %w", err)
	}
	mintReceipt, err := appContext.Run(func(opts *bind.TransactOpts) (*types.Transaction, error) {
		return erc20Contract.MintForAll(opts, userAddresses, big.NewInt(1_000_000))
	})
	if err != nil || mintReceipt.Status != types.ReceiptStatusSuccessful {
		return nil, errors.Join(fmt.Errorf("failed to mint ERC20 for erc20 accounts"), err)
	}

	return users, nil
}

func (f *BundleSubsidyApplication) GetReceivedTransactions(rpcClient rpc.Client) (uint64, error) {
	erc20Contract, err := contract.NewERC20(f.erc20Address, rpcClient)
	if err != nil {
		return 0, fmt.Errorf("failed to get ERC20 contract; %w", err)
	}
	balance, err := erc20Contract.BalanceOf(nil, f.recipient)
	if err != nil {
		return 0, err
	}
	return balance.Uint64(), nil
}

// Gas limits for each bundle step.
const (
	bundleSponsorGasLimit      = 50_000 // subsidiesRegistry.Sponsor
	bundleApproveGasLimit      = 55_000 // erc20.approve
	bundleTransferFromGasLimit = 65_000 // erc20.transferFrom
)

// bundleSponsorValue is the ETH deposited into the subsidy pool per bundle.
// Must exceed the gas cost of the subsidised approve call.
var bundleSponsorValue = big.NewInt(1e14) // 0.0001 ETH

// bundleGasFeeCap is the gas fee cap used for non-subsidised bundle steps.
var bundleGasFeeCap = new(big.Int).Mul(big.NewInt(10_000), big.NewInt(1e9)) // 10 000 Gwei

// BundleSubsidyUser produces one bundle envelope per GenerateTx call.
type BundleSubsidyUser struct {
	client          rpc.Client
	sponsorAccount  *Account
	userAccount     *Account
	registryAddress common.Address
	registryAbi     *ethabi.ABI
	erc20Address    common.Address
	erc20Abi        *ethabi.ABI
	fundId          [32]byte
	recipient       common.Address
	chainId         *big.Int
	sentTxs         atomic.Uint64
}

func (u *BundleSubsidyUser) GenerateTx() (*types.Transaction, error) {
	// Anchor the validity window to the current block.
	var blockNumberHex string
	if err := u.client.Call(&blockNumberHex, "eth_blockNumber"); err != nil {
		return nil, fmt.Errorf("failed to get block number; %w", err)
	}
	blockNumber, ok := new(big.Int).SetString(blockNumberHex[2:], 16)
	if !ok {
		return nil, fmt.Errorf("failed to parse block number %q", blockNumberHex)
	}

	// Decide upfront whether to add the intentionally failing 4th step so that
	// nonce allocation stays consistent across all GenerateTx calls.
	includeFailing := rand.Intn(4) == 0

	// Allocate nonces: sponsor uses steps 1 + 3 (+ optionally 4), erc20 uses step 2.
	sponsorNonce1 := u.sponsorAccount.getNextNonce()
	erc20Nonce := u.userAccount.getNextNonce()
	sponsorNonce2 := u.sponsorAccount.getNextNonce()
	sponsorNonce3 := uint64(0)
	if includeFailing {
		sponsorNonce3 = u.sponsorAccount.getNextNonce()
	}

	sponsorData, err := u.registryAbi.Pack("sponsor", u.fundId)
	if err != nil {
		return nil, fmt.Errorf("failed to pack sponsor calldata; %w", err)
	}
	approveData, err := u.erc20Abi.Pack("approve", u.sponsorAccount.address, big.NewInt(1))
	if err != nil {
		return nil, fmt.Errorf("failed to pack approve calldata; %w", err)
	}
	transferFromData, err := u.erc20Abi.Pack("transferFrom", u.userAccount.address, u.recipient, big.NewInt(1))
	if err != nil {
		return nil, fmt.Errorf("failed to pack transferFrom calldata; %w", err)
	}

	// Step 1: top up the subsidy pool for the userAccount→erc20 approve operation.
	step1 := bundling.Step(u.sponsorAccount.privateKey, &types.DynamicFeeTx{
		ChainID:   u.chainId,
		Nonce:     sponsorNonce1,
		GasFeeCap: new(big.Int).Set(bundleGasFeeCap),
		GasTipCap: big.NewInt(0),
		Gas:       bundleSponsorGasLimit,
		To:        &u.registryAddress,
		Value:     new(big.Int).Set(bundleSponsorValue),
		Data:      sponsorData,
	})

	// Step 2: approve sponsorAccount to spend 1 token; runs at GasFeeCap=0, subsidised.
	step2 := bundling.Step(u.userAccount.privateKey, &types.DynamicFeeTx{
		ChainID:   u.chainId,
		Nonce:     erc20Nonce,
		GasFeeCap: big.NewInt(0),
		GasTipCap: big.NewInt(0),
		Gas:       bundleApproveGasLimit,
		To:        &u.erc20Address,
		Value:     big.NewInt(0),
		Data:      approveData,
	})

	// Step 3: transferFrom consumes the approval; 1 token reaches the recipient.
	step3 := bundling.Step(u.sponsorAccount.privateKey, &types.DynamicFeeTx{
		ChainID:   u.chainId,
		Nonce:     sponsorNonce2,
		GasFeeCap: new(big.Int).Set(bundleGasFeeCap),
		GasTipCap: big.NewInt(0),
		Gas:       bundleTransferFromGasLimit,
		To:        &u.erc20Address,
		Value:     big.NewInt(0),
		Data:      transferFromData,
	})

	steps := []bundling.BundleStep{step1, step2, step3}

	if includeFailing {
		// Step 4: another transferFrom from the same userAccount. The allowance was
		// already consumed in step 3, so this reverts with "insufficient allowance".
		// EF_AllOf causes the entire bundle to revert; the recipient receives nothing.
		step4 := bundling.Step(u.sponsorAccount.privateKey, &types.DynamicFeeTx{
			ChainID:   u.chainId,
			Nonce:     sponsorNonce3,
			GasFeeCap: new(big.Int).Set(bundleGasFeeCap),
			GasTipCap: big.NewInt(0),
			Gas:       bundleTransferFromGasLimit,
			To:        &u.erc20Address,
			Value:     big.NewInt(0),
			Data:      transferFromData,
		})
		steps = append(steps, step4)
	}

	signer := types.NewLondonSigner(u.chainId)
	earliest := blockNumber.Uint64()
	envelope, err := bundling.NewBuilder(signer).
		SetEarliest(earliest).
		SetLatest(earliest + bundling.MaxBlockRange - 1).
		With(steps...).
		Build()
	if err != nil {
		return nil, fmt.Errorf("failed to build bundle; %w", err)
	}

	if !includeFailing {
		u.sentTxs.Add(1) // only successful bundles count should be compared with GetReceivedTransactions()
	}
	return envelope, nil
}

func (u *BundleSubsidyUser) GetSentTransactions() uint64 {
	return u.sentTxs.Load()
}
