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

package app

import (
	"context"
	"fmt"
	"math/big"
	"sync/atomic"

	"github.com/0xsoniclabs/norma/driver/rpc"
	contract "github.com/0xsoniclabs/norma/load/contracts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// oneWei is passed as value with every deployOrDestruct / deployAndDestruct call.
var oneWei = big.NewInt(1)

// ── SelfDestructor ───────────────────────────────────────────────────────────

// NewSelfDestructorApplication deploys a SelfDestructorFactory contract.
// Alternating transactions deploy and then destroy a child SelfDestructor contract,
// transferring 1 wei to the child on deploy and receiving it back via selfdestruct.
func NewSelfDestructorApplication(ctxt AppContext, feederId, appId uint32) (Application, error) {
	client := ctxt.GetClient()
	chainId, err := client.ChainID(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to get chain ID; %w", err)
	}

	_, receipt, err := DeployContract(ctxt, contract.DeploySelfDestructorFactory)
	if err != nil {
		return nil, fmt.Errorf("failed to deploy SelfDestructorFactory contract; %w", err)
	}

	accountFactory, err := NewAccountFactory(chainId, feederId, appId)
	if err != nil {
		return nil, err
	}

	parsedAbi, err := contract.SelfDestructorFactoryMetaData.GetAbi()
	if err != nil {
		return nil, err
	}

	return &SelfDestructorApplication{
		abi:             parsedAbi,
		contractAddress: receipt.ContractAddress,
		accountFactory:  accountFactory,
	}, nil
}

type SelfDestructorApplication struct {
	abi             *abi.ABI
	contractAddress common.Address
	accountFactory  *AccountFactory
}

func (f *SelfDestructorApplication) CreateUsers(appContext AppContext, numUsers int) ([]User, error) {
	users := make([]User, numUsers)
	addresses := make([]common.Address, numUsers)
	for i := 0; i < numUsers; i++ {
		workerAccount, err := f.accountFactory.CreateAccount(appContext.GetClient())
		if err != nil {
			return nil, err
		}
		users[i] = &SelfDestructorUser{
			abi:      f.abi,
			sender:   workerAccount,
			contract: f.contractAddress,
		}
		addresses[i] = workerAccount.address
	}

	fundsPerUser := new(big.Int).Mul(big.NewInt(1_000), big.NewInt(1e18))
	err := appContext.FundAccounts(addresses, fundsPerUser)
	return users, err
}

func (f *SelfDestructorApplication) GetReceivedTransactions(rpcClient rpc.Client) (uint64, error) {
	c, err := contract.NewSelfDestructorFactory(f.contractAddress, rpcClient)
	if err != nil {
		return 0, fmt.Errorf("failed to get SelfDestructorFactory contract representation; %w", err)
	}
	count, err := c.GetCount(nil)
	if err != nil {
		return 0, err
	}
	return count.Uint64(), nil
}

// SelfDestructorUser sends deployOrDestruct() transactions.
// The factory contract alternates between deploying and destroying a child contract.
type SelfDestructorUser struct {
	abi      *abi.ABI
	sender   *Account
	contract common.Address
	sentTxs  atomic.Uint64
}

func (g *SelfDestructorUser) GenerateTx() (*types.Transaction, error) {
	data, err := g.abi.Pack("deployOrDestruct")
	if err != nil || data == nil {
		return nil, fmt.Errorf("failed to prepare tx data; %w", err)
	}

	// Deploy path costs more gas (CREATE + SSTORE) than destroy path (CALL + selfdestruct).
	// Use the higher bound to cover both cases.
	const gasLimit = 200_000
	tx, err := createTx(g.sender, g.contract, oneWei, data, gasLimit)
	if err == nil {
		g.sentTxs.Add(1)
	}
	return tx, err
}

func (g *SelfDestructorUser) GetSentTransactions() uint64 {
	return g.sentTxs.Load()
}

// ── InstantSelfDestructor ────────────────────────────────────────────────────

// NewInstantSelfDestructorApplication deploys an InstantSelfDestructorFactory contract.
// Every transaction deploys a child contract and immediately destroys it in the same
// transaction, transferring 1 wei to the child and receiving it back via selfdestruct.
// On Cancun+, contracts created and destroyed in the same transaction are truly removed.
func NewInstantSelfDestructorApplication(ctxt AppContext, feederId, appId uint32) (Application, error) {
	client := ctxt.GetClient()
	chainId, err := client.ChainID(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to get chain ID; %w", err)
	}

	_, receipt, err := DeployContract(ctxt, contract.DeployInstantSelfDestructorFactory)
	if err != nil {
		return nil, fmt.Errorf("failed to deploy InstantSelfDestructorFactory contract; %w", err)
	}

	accountFactory, err := NewAccountFactory(chainId, feederId, appId)
	if err != nil {
		return nil, err
	}

	parsedAbi, err := contract.InstantSelfDestructorFactoryMetaData.GetAbi()
	if err != nil {
		return nil, err
	}

	return &InstantSelfDestructorApplication{
		abi:             parsedAbi,
		contractAddress: receipt.ContractAddress,
		accountFactory:  accountFactory,
	}, nil
}

type InstantSelfDestructorApplication struct {
	abi             *abi.ABI
	contractAddress common.Address
	accountFactory  *AccountFactory
}

func (f *InstantSelfDestructorApplication) CreateUsers(appContext AppContext, numUsers int) ([]User, error) {
	users := make([]User, numUsers)
	addresses := make([]common.Address, numUsers)
	for i := 0; i < numUsers; i++ {
		workerAccount, err := f.accountFactory.CreateAccount(appContext.GetClient())
		if err != nil {
			return nil, err
		}
		users[i] = &InstantSelfDestructorUser{
			abi:      f.abi,
			sender:   workerAccount,
			contract: f.contractAddress,
		}
		addresses[i] = workerAccount.address
	}

	fundsPerUser := new(big.Int).Mul(big.NewInt(1_000), big.NewInt(1e18))
	err := appContext.FundAccounts(addresses, fundsPerUser)
	return users, err
}

func (f *InstantSelfDestructorApplication) GetReceivedTransactions(rpcClient rpc.Client) (uint64, error) {
	c, err := contract.NewInstantSelfDestructorFactory(f.contractAddress, rpcClient)
	if err != nil {
		return 0, fmt.Errorf("failed to get InstantSelfDestructorFactory contract representation; %w", err)
	}
	count, err := c.GetCount(nil)
	if err != nil {
		return 0, err
	}
	return count.Uint64(), nil
}

// InstantSelfDestructorUser sends deployAndDestruct() transactions.
// Each transaction deploys and immediately destroys a child contract.
type InstantSelfDestructorUser struct {
	abi      *abi.ABI
	sender   *Account
	contract common.Address
	sentTxs  atomic.Uint64
}

func (g *InstantSelfDestructorUser) GenerateTx() (*types.Transaction, error) {
	data, err := g.abi.Pack("deployAndDestruct")
	if err != nil || data == nil {
		return nil, fmt.Errorf("failed to prepare tx data; %w", err)
	}

	const gasLimit = 100_000
	tx, err := createTx(g.sender, g.contract, oneWei, data, gasLimit)
	if err == nil {
		g.sentTxs.Add(1)
	}
	return tx, err
}

func (g *InstantSelfDestructorUser) GetSentTransactions() uint64 {
	return g.sentTxs.Load()
}
