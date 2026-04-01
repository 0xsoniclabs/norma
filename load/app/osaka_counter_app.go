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

// NewOsakaCounterApplication deploys an OsakaCounter contract to the chain.
// OsakaCounter is functionally identical to Counter but compiled as an EOF
// contract (EIP-7692), exercising the Osaka EVM execution path in Tosca.
func NewOsakaCounterApplication(ctxt AppContext, feederId, appId uint32) (Application, error) {
	client := ctxt.GetClient()
	chainId, err := client.ChainID(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to get chain ID; %w", err)
	}

	_, receipt, err := DeployContract(ctxt, contract.DeployOsakaCounter)
	if err != nil {
		return nil, fmt.Errorf("failed to deploy OsakaCounter contract; %w", err)
	}

	accountFactory, err := NewAccountFactory(chainId, feederId, appId)
	if err != nil {
		return nil, err
	}

	parsedAbi, err := contract.OsakaCounterMetaData.GetAbi()
	if err != nil {
		return nil, err
	}

	return &OsakaCounterApplication{
		abi:             parsedAbi,
		contractAddress: receipt.ContractAddress,
		accountFactory:  accountFactory,
	}, nil
}

type OsakaCounterApplication struct {
	abi             *abi.ABI
	contractAddress common.Address
	accountFactory  *AccountFactory
}

func (f *OsakaCounterApplication) CreateUsers(appContext AppContext, numUsers int) ([]User, error) {
	users := make([]User, numUsers)
	addresses := make([]common.Address, numUsers)
	for i := 0; i < numUsers; i++ {
		workerAccount, err := f.accountFactory.CreateAccount(appContext.GetClient())
		if err != nil {
			return nil, err
		}
		users[i] = &OsakaCounterUser{
			abi:      f.abi,
			sender:   workerAccount,
			contract: f.contractAddress,
		}
		addresses[i] = workerAccount.address
	}

	fundsPerUser := new(big.Int).Mul(big.NewInt(1_000), big.NewInt(1_000_000_000_000_000_000))
	err := appContext.FundAccounts(addresses, fundsPerUser)
	return users, err
}

func (f *OsakaCounterApplication) GetReceivedTransactions(rpcClient rpc.Client) (uint64, error) {
	c, err := contract.NewOsakaCounter(f.contractAddress, rpcClient)
	if err != nil {
		return 0, fmt.Errorf("failed to get OsakaCounter contract representation; %w", err)
	}
	count, err := c.GetCount(nil)
	if err != nil {
		return 0, err
	}
	return count.Uint64(), nil
}

type OsakaCounterUser struct {
	abi      *abi.ABI
	sender   *Account
	contract common.Address
	sentTxs  atomic.Uint64
}

func (g *OsakaCounterUser) GenerateTx() (*types.Transaction, error) {
	data, err := g.abi.Pack("incrementCounter")
	if err != nil || data == nil {
		return nil, fmt.Errorf("failed to prepare tx data; %w", err)
	}

	const gasLimit = 28036
	tx, err := createTx(g.sender, g.contract, big.NewInt(0), data, gasLimit)
	if err == nil {
		g.sentTxs.Add(1)
	}
	return tx, err
}

func (g *OsakaCounterUser) GetSentTransactions() uint64 {
	return g.sentTxs.Load()
}
