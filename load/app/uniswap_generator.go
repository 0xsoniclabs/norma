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
	"bytes"
	"fmt"
	"math/big"
	"math/rand"

	contract "github.com/0xsoniclabs/norma/load/contracts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

const (
	tokensInChain = 4
	pairsInChain  = tokensInChain - 1
)

var (
	amountSwapped    = big.NewInt(100)
	userTokenBalance = new(big.Int).Mul(big.NewInt(1_000_000_000), big.NewInt(1e18))
	pairLiquidity    = new(big.Int).Mul(big.NewInt(1_000_000_000_000_000), big.NewInt(1e18))
)

// newUniswapGenerator creates a generator swapping ERC-20 tokens through a chain
// of Uniswap pairs. Each transaction swaps the first token of the chain for the
// last one, or the other way round, traversing every pair in between.
func newUniswapGenerator(feederId, appId uint32) Generator {
	g := &uniswapGenerator{txGenerator: newTxGenerator(feederId, appId)}
	g.onDeploy = g.deploy
	g.onAccounts = g.mintTokens
	g.onSuccess = g.swap
	g.onRevert = g.swapAlongInvalidPath
	return g
}

type uniswapGenerator struct {
	txGenerator
	routerAbi      *abi.ABI
	routerAddress  common.Address
	tokenAddresses []common.Address
	pairAddresses  []common.Address
	tokens         []*contract.ERC20
}

func (g *uniswapGenerator) deploy(ctxt AppContext) error {
	rpcClient := ctxt.GetClient()
	treasure := ctxt.GetTreasure()

	txOpts, err := ctxt.GetTransactOptions(treasure)
	if err != nil {
		return fmt.Errorf("failed to create the transact options for the deployment; %w", err)
	}

	// The deployments and the configuration steps below are sent from the treasure
	// account with consecutive nonces and awaited in bulk afterwards.
	nextNonce := func() {
		txOpts.Nonce = new(big.Int).Add(txOpts.Nonce, big.NewInt(1))
	}

	routerAddress, tx, _, err := contract.DeployUniswapRouter(txOpts, rpcClient)
	if err != nil {
		return fmt.Errorf("failed to deploy the UniswapRouter; %w", err)
	}
	g.routerAddress = routerAddress
	deployments := []*types.Transaction{tx}

	g.tokenAddresses = make([]common.Address, tokensInChain)
	g.tokens = make([]*contract.ERC20, tokensInChain)
	for i := range g.tokens {
		nextNonce()
		g.tokenAddresses[i], tx, g.tokens[i], err = contract.DeployERC20(
			txOpts, rpcClient, fmt.Sprintf("Testing token %d", i), fmt.Sprintf("TOK%d", i))
		if err != nil {
			return fmt.Errorf("failed to deploy the ERC-20 token %d; %w", i, err)
		}
		deployments = append(deployments, tx)
	}

	g.pairAddresses = make([]common.Address, pairsInChain)
	pairs := make([]*contract.UniswapV2Pair, pairsInChain)
	for i := range pairs {
		nextNonce()
		g.pairAddresses[i], tx, pairs[i], err = contract.DeployUniswapV2Pair(txOpts, rpcClient)
		if err != nil {
			return fmt.Errorf("failed to deploy the Uniswap pair %d; %w", i, err)
		}
		deployments = append(deployments, tx)
	}

	if err := awaitReceipts(ctxt, deployments); err != nil {
		return fmt.Errorf("failed to deploy the Uniswap contracts; %w", err)
	}

	// Provide every pair with liquidity in both of its tokens and initialize it.
	configuration := make([]*types.Transaction, 0, 3*pairsInChain+tokensInChain)
	for i := range pairs {
		for _, token := range []*contract.ERC20{g.tokens[i], g.tokens[i+1]} {
			nextNonce()
			tx, err := token.Mint(txOpts, g.pairAddresses[i], pairLiquidity)
			if err != nil {
				return fmt.Errorf("failed to fund the Uniswap pair %d; %w", i, err)
			}
			configuration = append(configuration, tx)
		}

		// The initializer expects the token addresses in ascending order.
		tokenA, tokenB := g.tokenAddresses[i], g.tokenAddresses[i+1]
		if bytes.Compare(tokenA[:], tokenB[:]) > 0 {
			tokenA, tokenB = tokenB, tokenA
		}
		nextNonce()
		tx, err := pairs[i].Initialize(txOpts, tokenA, tokenB)
		if err != nil {
			return fmt.Errorf("failed to initialize the Uniswap pair %d; %w", i, err)
		}
		configuration = append(configuration, tx)
	}

	// Whitelisting the router spares every user from setting an allowance.
	for i, token := range g.tokens {
		nextNonce()
		tx, err := token.WhitelistSpender(txOpts, routerAddress)
		if err != nil {
			return fmt.Errorf("failed to whitelist the router in the ERC-20 token %d; %w", i, err)
		}
		configuration = append(configuration, tx)
	}

	if err := awaitReceipts(ctxt, configuration); err != nil {
		return fmt.Errorf("failed to configure the Uniswap contracts; %w", err)
	}

	g.routerAbi, err = contract.UniswapRouterMetaData.GetAbi()
	return err
}

func (g *uniswapGenerator) mintTokens(ctxt AppContext, accounts []common.Address) error {
	// Users swap in both directions, so they hold the first and the last token.
	for _, token := range []*contract.ERC20{g.tokens[0], g.tokens[tokensInChain-1]} {
		_, err := ctxt.Run(func(opts *bind.TransactOpts) (*types.Transaction, error) {
			return token.MintForAll(opts, accounts, userTokenBalance)
		})
		if err != nil {
			return fmt.Errorf("failed to mint the swapped ERC-20 tokens; %w", err)
		}
	}
	return nil
}

func (g *uniswapGenerator) swap(int, *Account) (txPayload, error) {
	tokens, pairs := g.tokenAddresses, g.pairAddresses
	if rand.Intn(2) == 0 {
		tokens, pairs = reverseAddresses(tokens), reverseAddresses(pairs)
	}
	data, err := g.routerAbi.Pack("swapExactTokensForTokens", amountSwapped, tokens, pairs)
	if err != nil {
		return txPayload{}, fmt.Errorf("failed to pack swapExactTokensForTokens; %w", err)
	}
	// A swap consumes 157571 gas for two tokens plus about 94314 per extra token.
	return txPayload{
		to:       &g.routerAddress,
		data:     data,
		gasLimit: 160_000 + (tokensInChain-2)*95_000,
	}, nil
}

// swapAlongInvalidPath swaps along a path too short to name both ends of a swap,
// which the router rejects before touching any state.
func (g *uniswapGenerator) swapAlongInvalidPath(int, *Account) (txPayload, error) {
	tokens := []common.Address{g.tokenAddresses[0]}
	data, err := g.routerAbi.Pack("swapExactTokensForTokens", amountSwapped, tokens, []common.Address{})
	if err != nil {
		return txPayload{}, fmt.Errorf("failed to pack swapExactTokensForTokens; %w", err)
	}
	return txPayload{
		to:       &g.routerAddress,
		data:     data,
		gasLimit: 40_000,
		reason:   "UniswapV2Library: INVALID_PATH",
	}, nil
}

// awaitReceipts reports the first of the given transactions that did not succeed.
func awaitReceipts(ctxt AppContext, txs []*types.Transaction) error {
	for i, tx := range txs {
		receipt, err := ctxt.GetReceipt(tx.Hash())
		if err != nil {
			return fmt.Errorf("failed to get the receipt of transaction %d; %w", i, err)
		}
		if receipt.Status != types.ReceiptStatusSuccessful {
			return fmt.Errorf("transaction %d was aborted", i)
		}
	}
	return nil
}
