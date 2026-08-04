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
	crand "crypto/rand"
	"fmt"
	"math/big"
	"math/rand"

	contract "github.com/0xsoniclabs/norma/load/contracts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

var erc20TokensPerUser = big.NewInt(1e18)

// newErc20Generator creates a generator transferring ERC-20 tokens between
// accounts. Its reverting variant transfers more tokens than the sender owns,
// which the token contract rejects with an arithmetic underflow.
func newErc20Generator(feederId, appId uint32) Generator {
	g := &erc20Generator{txGenerator: newTxGenerator(feederId, appId)}
	g.onDeploy = g.deploy
	g.onAccounts = g.mintTokens
	g.onSuccess = g.transfer
	g.onRevert = g.transferMoreThanOwned
	return g
}

type erc20Generator struct {
	txGenerator
	abi        *abi.ABI
	address    common.Address
	token      *contract.ERC20
	recipients []common.Address
}

func (g *erc20Generator) deploy(ctxt AppContext) error {
	token, receipt, err := DeployContract(ctxt, func(opts *bind.TransactOpts, backend bind.ContractBackend) (common.Address, *types.Transaction, *contract.ERC20, error) {
		return contract.DeployERC20(opts, backend, "Testing Token", "TOK")
	})
	if err != nil {
		return fmt.Errorf("failed to deploy the ERC20 contract; %w", err)
	}
	g.token = token
	g.address = receipt.ContractAddress

	if g.abi, err = contract.ERC20MetaData.GetAbi(); err != nil {
		return err
	}

	g.recipients = make([]common.Address, 100)
	for i := range g.recipients {
		if _, err := crand.Read(g.recipients[i][:]); err != nil {
			return fmt.Errorf("failed to generate a recipient address; %w", err)
		}
	}
	return nil
}

func (g *erc20Generator) mintTokens(ctxt AppContext, accounts []common.Address) error {
	_, err := ctxt.Run(func(opts *bind.TransactOpts) (*types.Transaction, error) {
		return g.token.MintForAll(opts, accounts, erc20TokensPerUser)
	})
	if err != nil {
		return fmt.Errorf("failed to mint ERC-20 tokens for the users; %w", err)
	}
	return nil
}

func (g *erc20Generator) transfer(int, *Account) (txPayload, error) {
	recipient := g.recipients[rand.Intn(len(g.recipients))]
	data, err := g.abi.Pack("transfer", recipient, big.NewInt(1))
	if err != nil {
		return txPayload{}, fmt.Errorf("failed to pack transfer; %w", err)
	}
	// A transfer call consumes 51349 gas.
	return txPayload{to: &g.address, data: data, gasLimit: 52_000}, nil
}

func (g *erc20Generator) transferMoreThanOwned(int, *Account) (txPayload, error) {
	recipient := g.recipients[rand.Intn(len(g.recipients))]
	tooMuch := new(big.Int).Mul(erc20TokensPerUser, big.NewInt(1_000_000))
	data, err := g.abi.Pack("transfer", recipient, tooMuch)
	if err != nil {
		return txPayload{}, fmt.Errorf("failed to pack transfer; %w", err)
	}
	return txPayload{
		to:       &g.address,
		data:     data,
		gasLimit: 52_000,
		reason:   "arithmetic underflow or overflow",
	}, nil
}
