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
	"errors"
	"fmt"
	"math/big"
	"sync/atomic"

	"github.com/0xsoniclabs/norma/driver/rpc"
	contract "github.com/0xsoniclabs/norma/load/contracts/abi"
	"github.com/0xsoniclabs/sonic/gossip/blockproc/subsidies/registry"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// SubsidiesApplication generates subsidized transactions to increment a counter contract.
// document this
type SubsidiesApplication struct {
	counterContract *contract.Counter
	accountFactory  *AccountFactory
}

func NewSubsidiesApplication(appContext AppContext, feederId, appId uint32) (Application, error) {
	client := appContext.GetClient()
	chainId, err := client.ChainID(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to get chain ID; %w", err)
	}

	accountFactory, err := NewAccountFactory(chainId, feederId, appId)
	if err != nil {
		return nil, err
	}

	// Create an  account paying for all subsidies
	sponsorAccount, err := accountFactory.CreateAccount(appContext.GetClient())
	if err != nil {
		return nil, err
	}

	subsidiesFund := new(big.Int).Mul(big.NewInt(10_000), big.NewInt(1e18))
	err = appContext.FundAccounts([]common.Address{sponsorAccount.address}, subsidiesFund)
	if err != nil {
		return nil, fmt.Errorf("failed to sponsor account; %w", err)
	}

	// Deploy the Counter counterContract to be used by this application.
	counterContract, receipt, err := DeployContract(appContext, contract.DeployCounter)
	if err != nil || receipt.Status != types.ReceiptStatusSuccessful {
		return nil, errors.Join(fmt.Errorf("failed to deploy Counter contract"), err)
	}
	counterContractAddress := receipt.ContractAddress

	// Mount registry contract object from the system address
	subsidiesRegistry, err := registry.NewRegistry(registry.GetAddress(), client)
	if err != nil {
		return nil, fmt.Errorf("failed to bind to subsidies registry contract; %w", err)
	}

	// Fund a subsidy for the counter contract:
	// 1. Get the fund ID for the counter contract
	_, fundId, err := subsidiesRegistry.ContractSponsorshipFundId(nil, counterContractAddress)
	if err != nil {
		return nil, fmt.Errorf("failed to read fundId from contract; %w", err)
	}

	// 2. Get the transact opts for the sponsor account
	opts, err := appContext.GetTransactOptions(sponsorAccount)
	if err != nil {
		return nil, fmt.Errorf("failed to get transact opts for sponsorship; %w", err)
	}
	// 3. Fund with all available balance minus gas for the sponsorship transaction
	opts.Value = new(big.Int).Sub(subsidiesFund,
		new(big.Int).Mul(opts.GasPrice, big.NewInt(150_000)))

	// 4. Call the sponsorship method and check receipt for sanity
	tx, err := subsidiesRegistry.Sponsor(opts, fundId)
	if err != nil {
		return nil, fmt.Errorf("failed to sponsor contract; %w", err)
	}
	receipt, err = appContext.GetReceipt(tx.Hash())
	if err != nil || receipt.Status != types.ReceiptStatusSuccessful {
		return nil, errors.Join(fmt.Errorf("sponsorship transaction failed"), err)
	}

	return &SubsidiesApplication{
		counterContract: counterContract,
		accountFactory:  accountFactory,
	}, nil
}

func (f *SubsidiesApplication) CreateUsers(appContext AppContext, numUsers int) ([]User, error) {
	// Creates a series of accounts to submit transactions
	// none of these accounts have any balance, all gas is paid by the subsidy

	users := make([]User, numUsers)
	addresses := make([]common.Address, numUsers)
	for i := range users {
		// Generate a new account for each worker - avoid account nonces related bottlenecks
		workerAccount, err := f.accountFactory.CreateAccount(appContext.GetClient())
		if err != nil {
			return nil, err
		}
		users[i] = &SubsidiesUser{
			counterContract: f.counterContract,
			Sender:          workerAccount,
		}
		addresses[i] = workerAccount.address
	}

	return users, nil
}

func (f *SubsidiesApplication) GetReceivedTransactions(rpcClient rpc.Client) (uint64, error) {
	count, err := f.counterContract.GetCount(nil)
	if err != nil {
		return 0, err
	}
	return count.Uint64(), nil
}

type SubsidiesUser struct {
	counterContract *contract.Counter
	Sender          *Account
	sentTxs         atomic.Uint64
}

func (u *SubsidiesUser) GenerateTx() (*types.Transaction, error) {
	opts, err := bind.NewKeyedTransactorWithChainID(u.Sender.privateKey, u.Sender.chainID)
	if err != nil {
		return nil, fmt.Errorf("failed to create transactor; %w", err)
	}

	opts.GasLimit = 28036
	opts.GasPrice = big.NewInt(0) // request subsidy
	opts.Nonce = big.NewInt(int64(u.Sender.getNextNonce()) + 1)
	opts.NoSend = true // do not send the tx, norma will send it to the network
	opts.Value = big.NewInt(0)
	tx, err := u.counterContract.IncrementCounter(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to create increment tx; %w", err)
	}

	u.sentTxs.Add(1)
	return tx, nil
}

func (u *SubsidiesUser) GetSentTransactions() uint64 {
	return u.sentTxs.Load()
}
