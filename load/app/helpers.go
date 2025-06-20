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
	"fmt"
	"github.com/holiman/uint256"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

func createTx(from *Account, toAddress common.Address, value *big.Int, data []byte, gasLimit uint64) (*types.Transaction, error) {
	tx := types.NewTx(&types.DynamicFeeTx{
		Nonce:     from.getNextNonce(),
		GasFeeCap: new(big.Int).Mul(big.NewInt(10_000), big.NewInt(1e9)),
		GasTipCap: big.NewInt(0),
		Gas:       gasLimit,
		To:        &toAddress,
		Value:     value,
		Data:      data,
	})
	return types.SignTx(tx, types.NewLondonSigner(from.chainID), from.privateKey)
}

func createSetCodeTx(from *Account, toAddress common.Address, value *uint256.Int, data []byte, gasLimit uint64, authAccounts []*Account, codeAddr common.Address) (*types.Transaction, error) {
	authList := make([]types.SetCodeAuthorization, 0, len(authAccounts))
	for _, authAccount := range authAccounts {
		auth := types.SetCodeAuthorization{
			ChainID: *uint256.MustFromBig(authAccount.chainID),
			Address: codeAddr,
			Nonce:   authAccount.getNextNonce(),
		}
		auth, err := types.SignSetCode(authAccount.privateKey, auth)
		if err != nil {
			return nil, fmt.Errorf("failed to sign SetCodeAuthorization; %w", err)
		}
		authList = append(authList, auth)
	}

	tx := types.NewTx(&types.SetCodeTx{
		Nonce:     from.getNextNonce(),
		GasFeeCap: new(uint256.Int).Mul(uint256.NewInt(10_000), uint256.NewInt(1e9)),
		GasTipCap: uint256.NewInt(0),
		Gas:       gasLimit,
		To:        toAddress,
		Value:     value,
		Data:      data,
		AuthList:  authList,
	})
	return types.SignTx(tx, types.NewPragueSigner(from.chainID), from.privateKey)
}

func reverseAddresses(in []common.Address) []common.Address {
	out := make([]common.Address, len(in))
	for i := 0; i < len(in); i++ {
		out[i] = in[len(in)-1-i]
	}
	return out
}
