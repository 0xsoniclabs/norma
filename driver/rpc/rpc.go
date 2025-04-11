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

package rpc

import (
	"context"
	"errors"
	"fmt"
	"github.com/ethereum/go-ethereum"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
)

//go:generate mockgen -source rpc.go -destination rpc_mock.go -package rpc

// Client is an interface that provides a subset of the Ethereum client and RPC client interfaces.
type Client interface {
	ethRpcClient
	rpcClient

	// WaitTransactionReceipt waits for the transaction receipt of the given transaction hash.
	// It returns an error if the receipt could not be obtained within a certain time frame.
	// This method retries with exponential backoff to fetch the transaction receipt,
	//  until a certain timeout is reached.
	WaitTransactionReceipt(txHash common.Hash) (*types.Receipt, error)
}

func WrapRpcClient(rpcClient *rpc.Client) *Impl {
	return &Impl{
		ethRpcClient:     ethclient.NewClient(rpcClient),
		rpcClient:        rpcClient,
		txReceiptTimeout: 600 * time.Second,
	}
}

// ethRpcClient is a subset of the Ethereum client interface that is used by the application.
type ethRpcClient interface {
	bind.ContractBackend
	NonceAt(ctx context.Context, account common.Address, blockNumber *big.Int) (uint64, error)
	BalanceAt(ctx context.Context, account common.Address, blockNumber *big.Int) (*big.Int, error)
	ChainID(ctx context.Context) (*big.Int, error)
	TransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error)

	Close()
}

// rpcClient is a subset of the RPC client interface that is used by the application.
type rpcClient interface {
	Call(result interface{}, method string, args ...interface{}) error
}

type Impl struct {
	ethRpcClient
	rpcClient
	txReceiptTimeout time.Duration
}

func (r Impl) Call(result interface{}, method string, args ...interface{}) error {
	return r.rpcClient.Call(result, method, args...)
}

func (r Impl) WaitTransactionReceipt(txHash common.Hash) (*types.Receipt, error) {
	// Wait for the response with some exponential backoff.
	const maxDelay = 5 * time.Second
	begin := time.Now()
	delay := time.Millisecond
	for time.Since(begin) < r.txReceiptTimeout {
		receipt, err := r.TransactionReceipt(context.Background(), txHash)
		if errors.Is(err, ethereum.NotFound) {
			time.Sleep(delay)
			delay = 2 * delay
			if delay > maxDelay {
				delay = maxDelay
			}
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("failed to get transaction receipt: %w", err)
		}
		return receipt, nil
	}
	return nil, fmt.Errorf("failed to get transaction receipt: timeout")
}
