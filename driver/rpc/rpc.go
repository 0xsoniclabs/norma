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
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum"

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
		receipt, err := r.transactionReceipt(context.Background(), txHash)
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

func (r Impl) transactionReceipt(ctxt context.Context, txHash common.Hash) (*types.Receipt, error) {
	var result map[string]any
	err := r.Call(&result, "eth_getTransactionReceipt", txHash)
	if err == nil && result == nil {
		return nil, ethereum.NotFound
	}

	// Remove all log.blockTimestamps to provide backward compatibility.
	// This fields was introduced in geth 1.16.1 or 1.16.2, but some versions
	// of Sonic (at least up to 2.1.2) are using 1.16.0. However, the client
	// used in this code is based on 1.16.2 or older, depending on this field
	// to be present and formatted in a specific way to parse the JSON result.
	// We do not need the field in Norma, so we can filter it out.
	if logs, ok := result["logs"].([]any); ok {
		for _, log := range logs {
			if logMap, ok := log.(map[string]any); ok {
				delete(logMap, "blockTimestamp")
			}
		}
	}

	// Re-encode the result to JSON and decode it into a types.Receipt.
	jsonEncoded, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal transaction receipt: %w", err)
	}

	var receipt *types.Receipt
	err = json.Unmarshal(jsonEncoded, &receipt)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal transaction receipt: %w", err)
	}

	return receipt, nil
}
