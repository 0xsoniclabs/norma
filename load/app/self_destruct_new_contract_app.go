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

// NewSelfDestructNewContractApplication deploys an SelfDestructNewContractFactory contract.
// Every transaction deploys a child contract and immediately destroys it in the same
// transaction, transferring 1 wei to the child and receiving it back via selfdestruct.
// On Cancun+, contracts created and destroyed in the same transaction are truly removed.
func NewSelfDestructNewContractApplication(ctxt AppContext, feederId, appId uint32) (Application, error) {
	client := ctxt.GetClient()
	chainId, err := client.ChainID(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to get chain ID; %w", err)
	}

	_, receipt, err := DeployContract(ctxt, contract.DeploySelfDestructNewContractFactory)
	if err != nil {
		return nil, fmt.Errorf("failed to deploy SelfDestructNewContractFactory contract; %w", err)
	}

	accountFactory, err := NewAccountFactory(chainId, feederId, appId)
	if err != nil {
		return nil, err
	}

	parsedAbi, err := contract.SelfDestructNewContractFactoryMetaData.GetAbi()
	if err != nil {
		return nil, err
	}

	return &SelfDestructNewContractApplication{
		abi:             parsedAbi,
		contractAddress: receipt.ContractAddress,
		accountFactory:  accountFactory,
	}, nil
}

type SelfDestructNewContractApplication struct {
	abi             *abi.ABI
	contractAddress common.Address
	accountFactory  *AccountFactory
}

func (f *SelfDestructNewContractApplication) CreateUsers(appContext AppContext, numUsers int) ([]User, error) {
	users := make([]User, numUsers)
	addresses := make([]common.Address, numUsers)
	for i := 0; i < numUsers; i++ {
		workerAccount, err := f.accountFactory.CreateAccount(appContext.GetClient())
		if err != nil {
			return nil, err
		}
		users[i] = &SelfDestructNewContractUser{
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

func (f *SelfDestructNewContractApplication) GetReceivedTransactions(rpcClient rpc.Client) (uint64, error) {
	c, err := contract.NewSelfDestructNewContractFactory(f.contractAddress, rpcClient)
	if err != nil {
		return 0, fmt.Errorf("failed to get SelfDestructNewContractFactory contract representation; %w", err)
	}
	count, err := c.GetCount(nil)
	if err != nil {
		return 0, err
	}
	return count.Uint64(), nil
}

// SelfDestructNewContractUser sends deployAndDestruct() transactions.
// Each transaction deploys and immediately destroys a child contract.
type SelfDestructNewContractUser struct {
	abi      *abi.ABI
	sender   *Account
	contract common.Address
	sentTxs  atomic.Uint64
}

func (g *SelfDestructNewContractUser) GenerateTx() (*types.Transaction, error) {
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

func (g *SelfDestructNewContractUser) GetSentTransactions() uint64 {
	return g.sentTxs.Load()
}
