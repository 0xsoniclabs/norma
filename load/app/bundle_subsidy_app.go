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
	"github.com/0xsoniclabs/norma/load/app/bundling"
	contract "github.com/0xsoniclabs/norma/load/contracts/abi"
	"github.com/0xsoniclabs/sonic/gossip/blockproc/subsidies/registry"
	ethabi "github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// BundleSubsidyApplication generates bundled transactions using the Sonic bundling
// mechanism (Brio hard-fork). Each bundle contains two transactions:
//  1. subsidiesRegistry.Sponsor(fundId) — the sponsor step, which tops up the
//     subsidy pool for the counter contract in the same block.
//  2. counterContract.IncrementCounter() — runs with GasFeeCap=0, covered by
//     the subsidy deposited in step 1.
//
// The envelope transaction wrapping both steps is sent via eth_sendRawTransaction
// and is accepted by the node as a regular transaction.
type BundleSubsidyApplication struct {
	counterContract *contract.Counter
	counterAddress  common.Address
	registryAddress common.Address
	registryAbi     *ethabi.ABI
	counterAbi      *ethabi.ABI
	fundId          [32]byte
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

	// Deploy the Counter contract whose increments we will track.
	counterContract, receipt, err := DeployContract(appContext, contract.DeployCounter)
	if err != nil || receipt.Status != types.ReceiptStatusSuccessful {
		return nil, errors.Join(fmt.Errorf("failed to deploy Counter contract"), err)
	}
	counterAddress := receipt.ContractAddress

	// Bind the subsidies registry and resolve the fund ID for our counter.
	registryAddress := registry.GetAddress()
	subsidiesRegistry, err := registry.NewRegistry(registryAddress, client)
	if err != nil {
		return nil, fmt.Errorf("failed to bind subsidies registry; %w", err)
	}
	_, fundId, err := subsidiesRegistry.ContractSponsorshipFundId(nil, counterAddress)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve subsidy fund ID; %w", err)
	}

	registryAbi, err := registry.RegistryMetaData.GetAbi()
	if err != nil {
		return nil, fmt.Errorf("failed to parse registry ABI; %w", err)
	}
	counterAbi, err := contract.CounterMetaData.GetAbi()
	if err != nil {
		return nil, fmt.Errorf("failed to parse counter ABI; %w", err)
	}

	return &BundleSubsidyApplication{
		counterContract: counterContract,
		counterAddress:  counterAddress,
		registryAddress: registryAddress,
		registryAbi:     registryAbi,
		counterAbi:      counterAbi,
		fundId:          fundId,
		accountFactory:  accountFactory,
		chainId:         chainId,
	}, nil
}

func (f *BundleSubsidyApplication) CreateUsers(appContext AppContext, numUsers int) ([]User, error) {
	users := make([]User, numUsers)
	sponsorAddresses := make([]common.Address, numUsers)

	for i := range users {
		// Each user has a dedicated sponsor account (has balance) and a
		// dedicated counter account (zero balance, runs at GasFeeCap=0).
		sponsorAccount, err := f.accountFactory.CreateAccount(appContext.GetClient())
		if err != nil {
			return nil, err
		}
		counterAccount, err := f.accountFactory.CreateAccount(appContext.GetClient())
		if err != nil {
			return nil, err
		}
		users[i] = &BundleSubsidyUser{
			client:          appContext.GetClient(),
			sponsorAccount:  sponsorAccount,
			counterAccount:  counterAccount,
			registryAddress: f.registryAddress,
			registryAbi:     f.registryAbi,
			counterAddress:  f.counterAddress,
			counterAbi:      f.counterAbi,
			fundId:          f.fundId,
			chainId:         f.chainId,
		}
		sponsorAddresses[i] = sponsorAccount.address
	}

	// Fund each sponsor: 1 ETH is enough for many (Sponsor value + envelope gas) cycles.
	fundsPerUser := new(big.Int).Mul(big.NewInt(1_000), big.NewInt(1e15))
	if err := appContext.FundAccounts(sponsorAddresses, fundsPerUser); err != nil {
		return nil, fmt.Errorf("failed to fund sponsor accounts; %w", err)
	}
	return users, nil
}

func (f *BundleSubsidyApplication) GetReceivedTransactions(rpcClient rpc.Client) (uint64, error) {
	count, err := f.counterContract.GetCount(nil)
	if err != nil {
		return 0, err
	}
	return count.Uint64(), nil
}

// sponsorGasLimit is the gas limit for the Sponsor registry call.
const sponsorGasLimit = 50_000

// counterGasLimit is the gas limit for the IncrementCounter call.
const counterGasLimit = 28_036

// sponsorValuePerBundle is the ETH added to the subsidy pool per bundle.
// Must cover the effective gas cost of one IncrementCounter execution.
var sponsorValuePerBundle = big.NewInt(1e14) // 0.0001 ETH

// BundleSubsidyUser produces one bundle envelope per GenerateTx call. The envelope
// is a regular *types.Transaction addressed to bundling.BundleProcessor and
// can be forwarded to the network via eth_sendRawTransaction.
type BundleSubsidyUser struct {
	client          rpc.Client
	sponsorAccount  *Account
	counterAccount  *Account
	registryAddress common.Address
	registryAbi     *ethabi.ABI
	counterAddress  common.Address
	counterAbi      *ethabi.ABI
	fundId          [32]byte
	chainId         *big.Int
	sentTxs         atomic.Uint64
}

func (u *BundleSubsidyUser) GenerateTx() (*types.Transaction, error) {
	// Anchor the bundle validity window to the current block so the bundle is
	// always accepted within the next bundling.MaxBlockRange blocks.
	var blockNumberHex string
	if err := u.client.Call(&blockNumberHex, "eth_blockNumber"); err != nil {
		return nil, fmt.Errorf("failed to get block number; %w", err)
	}
	blockNumber, ok := new(big.Int).SetString(blockNumberHex[2:], 16)
	if !ok {
		return nil, fmt.Errorf("failed to parse block number %q", blockNumberHex)
	}

	sponsorData, err := u.registryAbi.Pack("sponsor", u.fundId)
	if err != nil {
		return nil, fmt.Errorf("failed to pack sponsor calldata; %w", err)
	}
	counterData, err := u.counterAbi.Pack("incrementCounter")
	if err != nil {
		return nil, fmt.Errorf("failed to pack incrementCounter calldata; %w", err)
	}

	registryAddr := u.registryAddress
	counterAddr := u.counterAddress

	// Step 1: top up the subsidy pool for the counter contract.
	sponsorStep := bundling.Step(u.sponsorAccount.privateKey, &types.DynamicFeeTx{
		ChainID:   u.chainId,
		Nonce:     u.sponsorAccount.getNextNonce(),
		GasFeeCap: new(big.Int).Mul(big.NewInt(10_000), big.NewInt(1e9)),
		GasTipCap: big.NewInt(0),
		Gas:       sponsorGasLimit,
		To:        &registryAddr,
		Value:     new(big.Int).Set(sponsorValuePerBundle),
		Data:      sponsorData,
	})

	// Step 2: increment the counter with GasFeeCap=0, relying on the subsidy
	// deposited by step 1 in the same block.
	counterStep := bundling.Step(u.counterAccount.privateKey, &types.DynamicFeeTx{
		ChainID:   u.chainId,
		Nonce:     u.counterAccount.getNextNonce(),
		GasFeeCap: big.NewInt(0),
		GasTipCap: big.NewInt(0),
		Gas:       counterGasLimit,
		To:        &counterAddr,
		Value:     big.NewInt(0),
		Data:      counterData,
	})

	signer := types.NewLondonSigner(u.chainId)
	earliest := blockNumber.Uint64()
	envelope, err := bundling.NewBuilder(signer).
		SetEarliest(earliest).
		SetLatest(earliest+bundling.MaxBlockRange-1).
		With(sponsorStep, counterStep).
		Build()
	if err != nil {
		return nil, fmt.Errorf("failed to build bundle; %w", err)
	}

	u.sentTxs.Add(1)
	return envelope, nil
}

func (u *BundleSubsidyUser) GetSentTransactions() uint64 {
	return u.sentTxs.Load()
}
