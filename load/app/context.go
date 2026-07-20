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

	"github.com/0xsoniclabs/norma/driver/rpc"
	"github.com/0xsoniclabs/norma/genesis"
	contract "github.com/0xsoniclabs/norma/load/contracts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

//go:generate mockgen -source context.go -destination context_mock.go -package app

// AppContext provides a context for the application to interact with the network.
// It includes the network client, the account paying for management tasks, and a helper
// contract used for on-chain operations. It also provides utility functions to interact
// with the network, such as deploying contracts, sending transactions, and waiting for
// receipts.
type AppContext interface {
	GetClient() rpc.Client
	GetTreasure() *Account
	GetNetworkRules() genesis.NetworkRulesPatch
	GetTransactOptions(account *Account) (*bind.TransactOpts, error)
	GetReceipt(txHash common.Hash) (*types.Receipt, error)
	Run(operation func(*bind.TransactOpts) (*types.Transaction, error)) (*types.Receipt, error)
	FundAccounts(accounts []common.Address, value *big.Int) error
	Close()
}

type RpcClientFactory interface {
	DialRandomRpc() (rpc.Client, error)
}

// NewContext initializes an application context bound to a random RPC client,
// treasury account, and the scenario network rules patch. The provided context
// is used for the network operations performed through this app context, so
// cancelling it (for example on a check failure or timeout) aborts pending
// receipt waits and RPC calls.
func NewContext(ctx context.Context, factory RpcClientFactory, treasury *Account, networkRules genesis.NetworkRulesPatch) (AppContext, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	rpcClient, err := factory.DialRandomRpc()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to network: %w", err)
	}

	res := &appContext{
		ctx:          ctx,
		rpcClient:    rpcClient,
		treasury:     treasury,
		networkRules: networkRules,
	}

	return res, nil
}

type appContext struct {
	ctx          context.Context  // < bounds network operations to the scenario lifetime
	rpcClient    rpc.Client       // < access to the network
	treasury     *Account         // < the account paying for management tasks
	helper       *contract.Helper // < a contract used for on-chain operations
	networkRules genesis.NetworkRulesPatch
}

func (c *appContext) Close() {
	c.rpcClient.Close()
}

func (c *appContext) GetClient() rpc.Client {
	return c.rpcClient
}

func (c *appContext) GetTreasure() *Account {
	return c.treasury
}

// GetNetworkRules returns the network rules patch configured for this test run.
func (c *appContext) GetNetworkRules() genesis.NetworkRulesPatch {
	return c.networkRules
}

// GetTransactOptions provides transaction options to be used to send a transaction
// with the given account. The options include the chain ID, a suggested gas price,
// the next free nonce of the given account, and a hard-coded gas limit of 1e6.
// The main purpose of this function is to provide a convenient way to collect all
// the necessary information required to create a transaction in one place.
func (c *appContext) GetTransactOptions(account *Account) (*bind.TransactOpts, error) {
	client := c.rpcClient

	ctxt := c.ctx
	chainId, err := client.ChainID(ctxt)
	if err != nil {
		return nil, fmt.Errorf("failed to get chain ID: %w", err)
	}

	gasPrice, err := client.SuggestGasPrice(ctxt)
	if err != nil {
		return nil, fmt.Errorf("failed to get gas price suggestion: %w", err)
	}

	nonce, err := client.PendingNonceAt(ctxt, account.address)
	if err != nil {
		return nil, fmt.Errorf("failed to get nonce: %w", err)
	}

	txOpts, err := bind.NewKeyedTransactorWithChainID(account.privateKey, chainId)
	if err != nil {
		return nil, fmt.Errorf("failed to create transaction options: %w", err)
	}
	txOpts.GasPrice = new(big.Int).Mul(gasPrice, big.NewInt(2))
	txOpts.Nonce = big.NewInt(int64(nonce))
	return txOpts, nil
}

// GetReceipt blocks until the receipt is available, the context is cancelled,
// or the RPC client times out.
func (c *appContext) GetReceipt(txHash common.Hash) (*types.Receipt, error) {
	return c.rpcClient.WaitTransactionReceipt(c.ctx, txHash)
}

// Apply sends a transaction to the network using the network's validator account
// and waits for the transaction to be processed. The resulting receipt is returned.
func (c *appContext) Run(
	operation func(*bind.TransactOpts) (*types.Transaction, error),
) (*types.Receipt, error) {
	txOpts, err := c.GetTransactOptions(c.treasury)
	if err != nil {
		return nil, fmt.Errorf("failed to get transaction options: %w", err)
	}
	transaction, err := operation(txOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to create transaction: %w", err)
	}
	receipt, err := c.GetReceipt(transaction.Hash())
	if err != nil {
		return nil, err
	}
	if receipt.Status != types.ReceiptStatusSuccessful {
		return receipt, fmt.Errorf("transaction reverted")
	}
	return receipt, nil
}

// FundAccounts transfers the given amount of funds from the treasure to each of the
// given accounts.
func (c *appContext) FundAccounts(accounts []common.Address, value *big.Int) error {

	// Install a helper contract on the network on first use.
	if c.helper == nil {
		helper, _, err := DeployContract(c, contract.DeployHelper)
		if err != nil {
			return fmt.Errorf("failed to deploy helper contract: %w", err)
		}
		c.helper = helper
	}

	// Group funding requests in batches to avoid making individual transactions
	// too big for a single block.
	const batchSize = 128
	batches := make([][]common.Address, 0)
	for i := 0; i < len(accounts); i += batchSize {
		batches = append(batches, accounts[i:min(i+batchSize, len(accounts))])
	}

	// Send one transaction per batch of accounts.
	opts, err := c.GetTransactOptions(c.GetTreasure())
	if err != nil {
		return fmt.Errorf("failed to get transaction options: %w", err)
	}
	txs := make([]*types.Transaction, 0, len(batches))
	for _, batch := range batches {
		opts.Value = new(big.Int).Mul(value, big.NewInt(int64(len(batch))))
		tx, err := c.helper.Distribute(opts, batch)
		if err != nil {
			return fmt.Errorf("failed to distribute funds: %w", err)
		}
		txs = append(txs, tx)

		nonce, err := c.rpcClient.PendingNonceAt(c.ctx, c.GetTreasure().address)
		if err != nil {
			return fmt.Errorf("failed to refresh pending nonce: %w", err)
		}
		opts.Nonce = new(big.Int).SetUint64(nonce)
	}

	// Wait for all the transactions to be completed.
	for _, tx := range txs {
		receipt, err := c.GetReceipt(tx.Hash())
		if err != nil {
			return fmt.Errorf("failed to get receipt: %w", err)
		}
		if receipt.Status != types.ReceiptStatusSuccessful {
			return fmt.Errorf("failed to distribute funds: transaction reverted")
		}
	}
	return nil
}

// DeployContract is a utility function handling the deployment of a contract on the network.
// The contract is deployed with by the network's treasure account. The function returns the
// deployed contract instance and the transaction receipt.
func DeployContract[T any](c AppContext, deploy contractDeployer[T]) (*T, *types.Receipt, error) {
	client := c.GetClient()

	transactOptions, err := c.GetTransactOptions(c.GetTreasure())
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get transaction options: %w", err)
	}

	_, transaction, contract, err := deploy(transactOptions, client)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to deploy contract: %w", err)
	}

	receipt, err := c.GetReceipt(transaction.Hash())
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get receipt: %w", err)
	}
	if receipt.Status != types.ReceiptStatusSuccessful {
		return nil, receipt, fmt.Errorf("contract deployment transaction reverted")
	}
	return contract, receipt, nil
}

// DeployContractWithValue is like DeployContract but sends value (in wei) with the deployment transaction.
func DeployContractWithValue[T any](c AppContext, deploy contractDeployer[T], value *big.Int) (*T, *types.Receipt, error) {
	client := c.GetClient()

	transactOptions, err := c.GetTransactOptions(c.GetTreasure())
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get transaction options: %w", err)
	}
	transactOptions.Value = value

	_, transaction, contract, err := deploy(transactOptions, client)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to deploy contract: %w", err)
	}

	receipt, err := c.GetReceipt(transaction.Hash())
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get receipt: %w", err)
	}
	if receipt.Status != types.ReceiptStatusSuccessful {
		return nil, receipt, fmt.Errorf("contract deployment transaction reverted")
	}
	return contract, receipt, nil
}

// contractDeployer is the type of the deployment functions generated by abigen.
type contractDeployer[T any] func(*bind.TransactOpts, bind.ContractBackend) (common.Address, *types.Transaction, *T, error)
