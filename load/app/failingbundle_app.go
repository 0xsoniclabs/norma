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
	"sync"
	"sync/atomic"

	"github.com/0xsoniclabs/norma/driver/rpc"
	contract "github.com/0xsoniclabs/norma/load/contracts/abi"
	"github.com/0xsoniclabs/sonic/gossip/blockproc/bundle"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

const (
	allOfFailureProbability uint8 = 5  // probability of bundle success: 0.95*0.95 = 90%
	oneOfFailureProbability uint8 = 30 // probability of bundle success: 1-(0.3*0.3) = 91%
)

// FailingBundleApplication generates bundle transactions using the ProbabilisticFailing
// contract. Each bundle randomly chooses between:
//   - AllOf with 2x incrementCounter(10): both calls must succeed (10% individual fail rate)
//   - OneOf with 2x incrementCounter(40): first success wins (40% individual fail rate)
type FailingBundleApplication struct {
	contract        *contract.ProbabilisticFailing
	contractAddress common.Address
	contractAbi     *abi.ABI
	accountFactory  *AccountFactory
}

func NewFailingBundleApplication(appContext AppContext, feederId, appId uint32) (Application, error) {
	rpcClient := appContext.GetClient()

	txOpts, err := appContext.GetTransactOptions(appContext.GetTreasure())
	if err != nil {
		return nil, fmt.Errorf("failed to get tx opts for contract deploy: %w", err)
	}
	contractAddress, deployTx, pfContract, err := contract.DeployProbabilisticFailing(txOpts, rpcClient)
	if err != nil {
		return nil, fmt.Errorf("failed to deploy ProbabilisticFailing contract: %w", err)
	}

	accountFactory, err := NewAccountFactory(appContext.GetTreasure().chainID, feederId, appId)
	if err != nil {
		return nil, err
	}

	contractAbi, err := contract.ProbabilisticFailingMetaData.GetAbi()
	if err != nil {
		return nil, fmt.Errorf("failed to parse ProbabilisticFailing ABI: %w", err)
	}

	deployReceipt, err := appContext.GetReceipt(deployTx.Hash())
	if err != nil || deployReceipt.Status != types.ReceiptStatusSuccessful {
		return nil, errors.Join(fmt.Errorf("ProbabilisticFailing deploy transaction failed"), err)
	}

	return &FailingBundleApplication{
		contract:        pfContract,
		contractAddress: contractAddress,
		contractAbi:     contractAbi,
		accountFactory:  accountFactory,
	}, nil
}

// CreateUsers creates numUsers sender pairs. Each pair shares the single
// ProbabilisticFailing contract deployed during application initialisation.
func (a *FailingBundleApplication) CreateUsers(appContext AppContext, numUsers int) ([]User, error) {
	users := make([]User, numUsers)
	senderAddresses := make([]common.Address, 0, numUsers*2)

	for i := range users {
		senderA, err := a.accountFactory.CreateAccount(appContext.GetClient())
		if err != nil {
			return nil, err
		}
		senderB, err := a.accountFactory.CreateAccount(appContext.GetClient())
		if err != nil {
			return nil, err
		}
		users[i] = &FailingBundleUser{
			contractAddress: a.contractAddress,
			contractAbi:     a.contractAbi,
			senderA:         senderA,
			senderB:         senderB,
			signer:          types.LatestSignerForChainID(senderA.chainID),
			client:          appContext.GetClient(),
		}
		senderAddresses = append(senderAddresses, senderA.address, senderB.address)
	}

	fundsPerAccount := new(big.Int).Mul(big.NewInt(1_000), big.NewInt(1e18))
	if err := appContext.FundAccounts(senderAddresses, fundsPerAccount); err != nil {
		return nil, fmt.Errorf("failed to fund accounts: %w", err)
	}

	return users, nil
}

// GetReceivedTransactions returns the total number of successful incrementCounter
// calls on the shared contract across all users and bundle types.
func (a *FailingBundleApplication) GetReceivedTransactions(rpcClient rpc.Client) (uint64, error) {
	c, err := contract.NewProbabilisticFailing(a.contractAddress, rpcClient)
	if err != nil {
		return 0, fmt.Errorf("failed to bind ProbabilisticFailing contract: %w", err)
	}
	count, err := c.GetCount(nil)
	if err != nil {
		return 0, err
	}
	if count.Sign() < 0 {
		return 0, nil
	}
	return count.Uint64(), nil
}

// FailingBundleUser represents one (senderA, senderB) pair. Each GenerateTx call
// produces one bundle envelope randomly chosen between:
//
//  1. AllOf: senderA.incrementCounter(10), senderB.incrementCounter(10)
//  2. OneOf: senderA.incrementCounter(40), senderB.incrementCounter(40)
//
// Nonces are fetched from the RPC on every call.
type FailingBundleUser struct {
	contractAddress common.Address
	contractAbi     *abi.ABI
	senderA         *Account
	senderB         *Account
	signer          types.Signer
	client          rpc.Client
	sentTxs         atomic.Uint64
	gasOnce         sync.Once
}

func (u *FailingBundleUser) GenerateTx() (*types.Transaction, error) {
	ctx := context.Background()
	useAllOf := rand.Intn(2) == 0
	var failureProbability uint8
	if useAllOf {
		failureProbability = allOfFailureProbability
	} else {
		failureProbability = oneOfFailureProbability
	}

	callData, err := u.contractAbi.Pack("incrementCounter", failureProbability)
	if err != nil {
		return nil, fmt.Errorf("failed to pack incrementCounter: %w", err)
	}

	nonceA, err := u.client.PendingNonceAt(ctx, u.senderA.address)
	if err != nil {
		return nil, fmt.Errorf("failed to get nonce for senderA: %w", err)
	}
	nonceB, err := u.client.PendingNonceAt(ctx, u.senderB.address)
	if err != nil {
		return nil, fmt.Errorf("failed to get nonce for senderB: %w", err)
	}

	currentBlock, err := u.client.BlockNumber(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get current block number: %w", err)
	}

	stepA := bundle.Step(u.senderA.privateKey, &types.DynamicFeeTx{
		Nonce:     nonceA,
		Gas:       30_000,
		GasFeeCap: gasFeeCap,
		GasTipCap: gasTipCap,
		To:        &u.contractAddress,
		Data:      callData,
	})
	stepB := bundle.Step(u.senderB.privateKey, &types.DynamicFeeTx{
		Nonce:     nonceB,
		Gas:       30_000,
		GasFeeCap: gasFeeCap,
		GasTipCap: gasTipCap,
		To:        &u.contractAddress,
		Data:      callData,
	})

	builder := bundle.NewBuilder().WithSigner(u.signer).SetEarliest(currentBlock)
	if useAllOf {
		builder = builder.AllOf(stepA, stepB)
	} else {
		builder = builder.OneOf(stepA, stepB)
	}

	u.sentTxs.Add(1)
	return builder.Build(), nil
}

func (u *FailingBundleUser) GetSentTransactions() uint64 {
	return u.sentTxs.Load()
}
