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
	"fmt"
	"math/big"
	"sync/atomic"

	"github.com/0xsoniclabs/norma/driver/rpc"
	contract "github.com/0xsoniclabs/norma/load/contracts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// NewClzApplication deploys a ClzCounter contract and returns an Application
// that exercises the CLZ opcode (EIP-7939) by verifying count-leading-zeros
// results on random 256-bit inputs.
func NewClzApplication(ctxt AppContext, feederId, appId uint32) (Application, error) {
	client := ctxt.GetClient()
	chainId, err := client.ChainID(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to get chain ID; %w", err)
	}

	_, receipt, err := DeployContract(ctxt, func(opts *bind.TransactOpts, backend bind.ContractBackend) (common.Address, *types.Transaction, *contract.ClzCounter, error) {
		return contract.DeployClzCounter(opts, backend)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to deploy ClzCounter contract; %w", err)
	}

	accountFactory, err := NewAccountFactory(chainId, feederId, appId)
	if err != nil {
		return nil, err
	}

	parsedAbi, err := contract.ClzCounterMetaData.GetAbi()
	if err != nil {
		return nil, err
	}

	return &ClzApplication{
		abi:             parsedAbi,
		contractAddress: receipt.ContractAddress,
		accountFactory:  accountFactory,
	}, nil
}

// ClzApplication represents a deployed ClzCounter contract.
type ClzApplication struct {
	abi             *abi.ABI
	contractAddress common.Address
	accountFactory  *AccountFactory
}

func (f *ClzApplication) CreateUsers(appContext AppContext, numUsers int) ([]User, error) {
	users := make([]User, numUsers)
	addresses := make([]common.Address, numUsers)
	for i := 0; i < numUsers; i++ {
		workerAccount, err := f.accountFactory.CreateAccount(appContext.GetClient())
		if err != nil {
			return nil, err
		}
		users[i] = &ClzUser{
			abi:      f.abi,
			sender:   workerAccount,
			contract: f.contractAddress,
		}
		addresses[i] = workerAccount.address
	}

	fundsPerUser := new(big.Int).Mul(big.NewInt(1_000), big.NewInt(1_000_000_000_000_000_000))
	return users, appContext.FundAccounts(addresses, fundsPerUser)
}

func (f *ClzApplication) GetReceivedTransactions(rpcClient rpc.Client) (uint64, error) {
	c, err := contract.NewClzCounter(f.contractAddress, rpcClient)
	if err != nil {
		return 0, fmt.Errorf("failed to get ClzCounter contract; %w", err)
	}
	count, err := c.GetCount(nil)
	if err != nil {
		return 0, err
	}
	return count.Uint64(), nil
}

// ClzUser sends incrementCounter transactions to the ClzCounter contract.
type ClzUser struct {
	abi      *abi.ABI
	sender   *Account
	contract common.Address
	sentTxs  atomic.Uint64
}

func (g *ClzUser) GenerateTx() (*types.Transaction, error) {
	// Generate a random 256-bit value.
	var buf [32]byte
	if _, err := crand.Read(buf[:]); err != nil {
		return nil, fmt.Errorf("failed to generate random value; %w", err)
	}
	value := new(big.Int).SetBytes(buf[:])

	// Compute the expected CLZ: number of leading zero bits in a 256-bit word.
	expectedClz := big.NewInt(int64(clz256(value)))

	data, err := g.abi.Pack("incrementCounter", value, expectedClz)
	if err != nil {
		return nil, fmt.Errorf("failed to pack incrementCounter calldata; %w", err)
	}

	const gasLimit = 30_000
	tx, err := createTx(g.sender, g.contract, big.NewInt(0), data, gasLimit)
	if err == nil {
		g.sentTxs.Add(1)
	}
	return tx, err
}

func (g *ClzUser) GetSentTransactions() uint64 {
	return g.sentTxs.Load()
}

// clz256 returns the number of leading zero bits in the 256-bit representation
// of v, matching the semantics of the CLZ EVM opcode (EIP-7939).
// Returns 256 for v == 0.
func clz256(v *big.Int) int {
	if v.Sign() == 0 {
		return 256
	}
	return 256 - v.BitLen()
}