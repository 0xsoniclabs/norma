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

// NewTransientApplication deploys a TransientCounter contract to the chain.
// The contract uses EIP-1153 transient storage as a reentrancy guard.
// Each transaction calls incrementCounterTwice(), which attempts to increment
// the counter twice in one transaction. Due to the transient nonReentrant guard,
// only the first increment succeeds – demonstrating that the transient lock
// persists across external calls within the same transaction.
func NewTransientApplication(ctxt AppContext, feederId, appId uint32) (Application, error) {
	client := ctxt.GetClient()
	chainId, err := client.ChainID(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to get chain ID; %w", err)
	}

	_, receipt, err := DeployContract(ctxt, contract.DeployTransientCounter)
	if err != nil {
		return nil, fmt.Errorf("failed to deploy TransientCounter contract; %w", err)
	}

	accountFactory, err := NewAccountFactory(chainId, feederId, appId)
	if err != nil {
		return nil, err
	}

	parsedAbi, err := contract.TransientCounterMetaData.GetAbi()
	if err != nil {
		return nil, err
	}

	return &TransientApplication{
		abi:             parsedAbi,
		contractAddress: receipt.ContractAddress,
		accountFactory:  accountFactory,
	}, nil
}

// TransientApplication represents a deployed TransientCounter contract.
// It is thread-safe; each created user should be used in a single thread only.
type TransientApplication struct {
	abi             *abi.ABI
	contractAddress common.Address
	accountFactory  *AccountFactory
}

func (f *TransientApplication) CreateUsers(appContext AppContext, numUsers int) ([]User, error) {
	users := make([]User, numUsers)
	addresses := make([]common.Address, numUsers)
	for i := 0; i < numUsers; i++ {
		workerAccount, err := f.accountFactory.CreateAccount(appContext.GetClient())
		if err != nil {
			return nil, err
		}
		users[i] = &TransientUser{
			abi:      f.abi,
			sender:   workerAccount,
			contract: f.contractAddress,
		}
		addresses[i] = workerAccount.address
	}

	fundsPerUser := new(big.Int).Mul(big.NewInt(1_000), big.NewInt(1e18)) // wei
	err := appContext.FundAccounts(addresses, fundsPerUser)

	return users, err
}

func (f *TransientApplication) GetReceivedTransactions(rpcClient rpc.Client) (uint64, error) {
	c, err := contract.NewTransientCounter(f.contractAddress, rpcClient)
	if err != nil {
		return 0, fmt.Errorf("failed to get TransientCounter contract representation; %w", err)
	}
	count, err := c.GetCount(nil)
	if err != nil {
		return 0, err
	}
	return count.Uint64(), nil
}

// TransientUser sends incrementCounterTwice() transactions.
// Each tx attempts two increments; only the first succeeds due to the transient guard.
type TransientUser struct {
	abi      *abi.ABI
	sender   *Account
	contract common.Address
	sentTxs  atomic.Uint64
}

func (g *TransientUser) GenerateTx() (*types.Transaction, error) {
	data, err := g.abi.Pack("incrementCounterTwice")
	if err != nil || data == nil {
		return nil, fmt.Errorf("failed to prepare tx data; %w", err)
	}

	const gasLimit = 60000
	tx, err := createTx(g.sender, g.contract, big.NewInt(0), data, gasLimit)
	if err == nil {
		g.sentTxs.Add(1)
	}
	return tx, err
}

func (g *TransientUser) GetSentTransactions() uint64 {
	return g.sentTxs.Load()
}
