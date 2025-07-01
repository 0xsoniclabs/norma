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

package network

import (
	"encoding/hex"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

//go:generate mockgen -source backend.go -destination backend_mock.go -package network

// ContractBackend is an interface for a client to interact with the network.
type ContractBackend interface {
	bind.ContractBackend
	// WaitTransactionReceipt waits for the receipt of the given transaction hash to be available.
	// The function times out after 10 seconds.
	WaitTransactionReceipt(txHash common.Hash) (*types.Receipt, error)
}

// convertContractBytecode converts a contract hex string to bytecode.
func convertContractBytecode(contractHex string) ([]byte, error) {
	contractHex = strings.TrimPrefix(contractHex, "0x")

	bytecode, err := hex.DecodeString(contractHex)
	if err != nil {
		return nil, err
	}
	return bytecode, nil
}
