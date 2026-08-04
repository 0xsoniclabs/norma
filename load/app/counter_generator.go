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

	contract "github.com/0xsoniclabs/norma/load/contracts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
)

// newCounterGenerator creates a generator sending transactions to a Counter
// contract, a contract holding a single integer that each call increments. It
// produces the cheapest possible contract call and is the baseline load of the
// mix.
func newCounterGenerator(feederId, appId uint32) Generator {
	g := &counterGenerator{txGenerator: newTxGenerator(feederId, appId)}
	g.onDeploy = g.deploy
	g.onSuccess = g.increment
	g.onRevert = g.callUnknownFunction
	return g
}

type counterGenerator struct {
	txGenerator
	abi     *abi.ABI
	address common.Address
}

func (g *counterGenerator) deploy(ctxt AppContext) error {
	_, receipt, err := DeployContract(ctxt, contract.DeployCounter)
	if err != nil {
		return fmt.Errorf("failed to deploy the Counter contract; %w", err)
	}
	g.address = receipt.ContractAddress
	g.abi, err = contract.CounterMetaData.GetAbi()
	return err
}

func (g *counterGenerator) increment(int, *Account) (txPayload, error) {
	data, err := g.abi.Pack("incrementCounter")
	if err != nil {
		return txPayload{}, fmt.Errorf("failed to pack incrementCounter; %w", err)
	}
	return txPayload{to: &g.address, data: data, gasLimit: 28_036}, nil
}

func (g *counterGenerator) callUnknownFunction(int, *Account) (txPayload, error) {
	return unknownFunctionPayload(g.address), nil
}
