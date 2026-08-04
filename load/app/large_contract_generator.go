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
	"sync/atomic"

	contract "github.com/0xsoniclabs/norma/load/contracts/abi"
	"github.com/0xsoniclabs/sonic/opera"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
)

// newLargeContractGenerator creates a generator whose every transaction deploys a
// fresh LargeContract of about 48 KiB of runtime bytecode. Both the deployed
// contract and the counter it reports to exceed the standard 24 KiB code size
// limit, so the generator requires the raised limit Sonic Brio introduces.
func newLargeContractGenerator(feederId, appId uint32) Generator {
	g := &largeContractGenerator{txGenerator: newTxGenerator(feederId, appId)}
	g.supports = func(rules opera.Rules) bool { return rules.Upgrades.Brio }
	g.onDeploy = g.deploy
	g.onSuccess = g.deployLargeContract
	return g
}

type largeContractGenerator struct {
	txGenerator
	abi            *abi.ABI
	counterAddress common.Address
	initCodePrefix []byte
	// ids gives every deployed contract a distinct immutable id.
	ids atomic.Uint64
}

func (g *largeContractGenerator) deploy(ctxt AppContext) error {
	_, receipt, err := DeployContract(ctxt, contract.DeployCounter)
	if err != nil {
		return fmt.Errorf("failed to deploy the Counter contract; %w", err)
	}
	g.counterAddress = receipt.ContractAddress
	g.initCodePrefix = common.FromHex(contract.LargeContractMetaData.Bin)
	g.abi, err = contract.LargeContractMetaData.GetAbi()
	return err
}

func (g *largeContractGenerator) deployLargeContract(int, *Account) (txPayload, error) {
	id := new(big.Int).SetUint64(g.ids.Add(1))
	constructorArgs, err := g.abi.Pack("", g.counterAddress, id)
	if err != nil {
		return txPayload{}, fmt.Errorf("failed to pack the constructor arguments; %w", err)
	}
	return txPayload{
		to:       nil, // < deploys a contract
		data:     append(append([]byte{}, g.initCodePrefix...), constructorArgs...),
		gasLimit: 12_000_000,
	}, nil
}
