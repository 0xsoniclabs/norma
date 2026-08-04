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
	"fmt"
	"math/big"

	contract "github.com/0xsoniclabs/norma/load/contracts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

var bundleTokensPerAccount = new(big.Int).Mul(big.NewInt(1_000_000), big.NewInt(1e18))

// erc20Bundle installs the ERC-20 token the ERC-20 based bundle generators move
// between the accounts of their bundles.
type erc20Bundle struct {
	bundleGenerator
	token    *contract.ERC20
	address  common.Address
	tokenAbi *abi.ABI
}

func (b *erc20Bundle) deployToken(ctxt AppContext, name, symbol string) error {
	token, receipt, err := DeployContract(ctxt, func(opts *bind.TransactOpts, backend bind.ContractBackend) (common.Address, *types.Transaction, *contract.ERC20, error) {
		return contract.DeployERC20(opts, backend, name, symbol)
	})
	if err != nil {
		return fmt.Errorf("failed to deploy the ERC20 contract; %w", err)
	}
	b.token = token
	b.address = receipt.ContractAddress
	b.tokenAbi, err = contract.ERC20MetaData.GetAbi()
	return err
}

func (b *erc20Bundle) mintTokens(ctxt AppContext, accounts []common.Address) error {
	receipt, err := ctxt.Run(func(opts *bind.TransactOpts) (*types.Transaction, error) {
		return b.token.MintForAll(opts, accounts, bundleTokensPerAccount)
	})
	if err != nil {
		return fmt.Errorf("failed to mint ERC-20 tokens; %w", err)
	}
	if receipt.Status != types.ReceiptStatusSuccessful {
		return fmt.Errorf("the transaction minting the ERC-20 tokens was aborted")
	}
	return nil
}

func (b *erc20Bundle) pack(method string, args ...any) ([]byte, error) {
	data, err := b.tokenAbi.Pack(method, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to pack %s; %w", method, err)
	}
	return data, nil
}

func (b *erc20Bundle) step(nonce uint64, data []byte) *types.DynamicFeeTx {
	return &types.DynamicFeeTx{
		Nonce:     nonce,
		Gas:       bundleStepGas,
		GasFeeCap: gasFeeCap,
		GasTipCap: gasTipCap,
		To:        &b.address,
		Data:      data,
	}
}

func accountAddresses(senders [][]*Account, position int) []common.Address {
	addresses := make([]common.Address, len(senders))
	for i, group := range senders {
		addresses[i] = group[position].address
	}
	return addresses
}
