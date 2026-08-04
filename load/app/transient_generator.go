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

// newTransientGenerator creates a generator exercising the EIP-1153 transient
// storage. Every transaction attempts two increments of the same counter; the
// transient reentrancy guard lets only the first one through, which shows that the
// transient lock survives an external call within one transaction.
func newTransientGenerator(feederId, appId uint32) Generator {
	g := &transientGenerator{txGenerator: newTxGenerator(feederId, appId)}
	g.onDeploy = g.deploy
	g.onSuccess = g.incrementTwice
	g.onRevert = g.callUnknownFunction
	return g
}

type transientGenerator struct {
	txGenerator
	abi     *abi.ABI
	address common.Address
}

func (g *transientGenerator) deploy(ctxt AppContext) error {
	_, receipt, err := DeployContract(ctxt, contract.DeployTransientCounter)
	if err != nil {
		return fmt.Errorf("failed to deploy the TransientCounter contract; %w", err)
	}
	g.address = receipt.ContractAddress
	g.abi, err = contract.TransientCounterMetaData.GetAbi()
	return err
}

func (g *transientGenerator) incrementTwice(int, *Account) (txPayload, error) {
	data, err := g.abi.Pack("incrementCounterTwice")
	if err != nil {
		return txPayload{}, fmt.Errorf("failed to pack incrementCounterTwice; %w", err)
	}
	return txPayload{to: &g.address, data: data, gasLimit: 60_000}, nil
}

func (g *transientGenerator) callUnknownFunction(int, *Account) (txPayload, error) {
	return unknownFunctionPayload(g.address), nil
}
