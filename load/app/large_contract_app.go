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
	"fmt"
	"math/big"
	"sync/atomic"

	"github.com/0xsoniclabs/norma/driver/rpc"
	contract "github.com/0xsoniclabs/norma/load/contracts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// NewLargeContractApplication deploys a LargeContractCounter and returns an
// Application where each transaction is a CREATE transaction deploying a new
// LargeContract (~48 KiB runtime bytecode). This exercises the Sonic Brio
// increased code size limit (48 KiB, up from the standard 24 KiB).
//
// Both LargeContract and LargeContractCounter exceed the standard 24 KiB limit
// and therefore require Sonic Brio or a network with the same raised limits.
func NewLargeContractApplication(ctxt AppContext, feederId, appId uint32) (Application, error) {
	client := ctxt.GetClient()
	chainId, err := client.ChainID(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to get chain ID; %w", err)
	}

	_, receipt, err := DeployContract(ctxt, contract.DeployLargeContractCounter)
	if err != nil {
		return nil, fmt.Errorf("failed to deploy LargeContractCounter; %w", err)
	}

	largeContractAbi, err := contract.LargeContractMetaData.GetAbi()
	if err != nil {
		return nil, err
	}

	accountFactory, err := NewAccountFactory(chainId, feederId, appId)
	if err != nil {
		return nil, err
	}

	return &LargeContractApplication{
		counterAddress:   receipt.ContractAddress,
		largeContractAbi: largeContractAbi,
		initCodePrefix:   common.FromHex(contract.LargeContractMetaData.Bin),
		accountFactory:   accountFactory,
	}, nil
}

// LargeContractApplication represents a deployed LargeContractCounter.
type LargeContractApplication struct {
	counterAddress   common.Address
	largeContractAbi *abi.ABI
	initCodePrefix   []byte // LargeContract bytecode without constructor args
	accountFactory   *AccountFactory
}

func (f *LargeContractApplication) CreateUsers(appContext AppContext, numUsers int) ([]User, error) {
	users := make([]User, numUsers)
	addresses := make([]common.Address, numUsers)
	for i := 0; i < numUsers; i++ {
		workerAccount, err := f.accountFactory.CreateAccount(appContext.GetClient())
		if err != nil {
			return nil, err
		}
		users[i] = &LargeContractUser{
			abi:            f.largeContractAbi,
			initCodePrefix: f.initCodePrefix,
			counterAddress: f.counterAddress,
			sender:         workerAccount,
		}
		addresses[i] = workerAccount.address
	}

	fundsPerUser := new(big.Int).Mul(big.NewInt(1_000), big.NewInt(1_000_000_000_000_000_000))
	return users, appContext.FundAccounts(addresses, fundsPerUser)
}

func (f *LargeContractApplication) GetReceivedTransactions(rpcClient rpc.Client) (uint64, error) {
	c, err := contract.NewLargeContractCounter(f.counterAddress, rpcClient)
	if err != nil {
		return 0, fmt.Errorf("failed to get LargeContractCounter; %w", err)
	}
	count, err := c.GetCount(nil)
	if err != nil {
		return 0, err
	}
	return count.Uint64(), nil
}

// LargeContractUser sends CREATE transactions deploying a new LargeContract each time.
// Each deployment passes a unique _id so the immutable is distinct per contract instance.
type LargeContractUser struct {
	abi            *abi.ABI
	initCodePrefix []byte
	counterAddress common.Address
	sender         *Account
	sentTxs        atomic.Uint64
}

func (g *LargeContractUser) GenerateTx() (*types.Transaction, error) {
	id := new(big.Int).SetUint64(g.sentTxs.Load())
	constructorArgs, err := g.abi.Pack("", g.counterAddress, id)
	if err != nil {
		return nil, fmt.Errorf("failed to pack constructor args; %w", err)
	}

	deployData := make([]byte, len(g.initCodePrefix)+len(constructorArgs))
	copy(deployData, g.initCodePrefix)
	copy(deployData[len(g.initCodePrefix):], constructorArgs)

	const gasLimit = 12_000_000
	tx, err := createDeployTx(g.sender, big.NewInt(0), deployData, gasLimit)
	if err == nil {
		g.sentTxs.Add(1)
	}
	return tx, err
}

func (g *LargeContractUser) GetSentTransactions() uint64 {
	return g.sentTxs.Load()
}
