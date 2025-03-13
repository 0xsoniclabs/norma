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

type RpcClient interface {
	bind.ContractBackend
	Call(result interface{}, method string, args ...interface{}) error
	NonceAt(ctx context.Context, account common.Address, blockNumber *big.Int) (uint64, error)
	BalanceAt(ctx context.Context, account common.Address, blockNumber *big.Int) (*big.Int, error)

	// -- ethereum client methods --
	ChainID(ctx context.Context) (*big.Int, error)

	// WaitTransactionReceipt waits for the receipt of the given transaction hash to be available.
	// The function times out after 10 seconds.
	WaitTransactionReceipt(txHash common.Hash) (*types.Receipt, error)

	Close()
}

func WrapRpcClient(rpcClient *rpc.Client) *RpcClientImpl {
	return &RpcClientImpl{
		Client:    ethclient.NewClient(rpcClient),
		RpcClient: rpcClient,
	}
}

type RpcClientImpl struct {
	*ethclient.Client
	RpcClient *rpc.Client
}

func (r RpcClientImpl) Call(result interface{}, method string, args ...interface{}) error {
	return r.RpcClient.Call(result, method, args...)
}

func (r RpcClientImpl) WaitTransactionReceipt(txHash common.Hash) (*types.Receipt, error) {
	// Wait for the response with some exponential backoff.
	const maxDelay = 100 * time.Millisecond
	begin := time.Now()
	delay := time.Millisecond
	for time.Since(begin) < 120000*time.Second {
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
