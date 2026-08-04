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
	"github.com/ethereum/go-ethereum/common"
)

// oneWei accompanies every call to a self-destruct factory: the factory forwards
// it to the child contract and gets it back when the child destroys itself.
var oneWei = big.NewInt(1)

// newSelfDestructOldContractGenerator creates a generator whose transactions
// alternately deploy and destroy a child contract through a factory, exercising
// the destruction of a contract that outlived the transaction creating it.
func newSelfDestructOldContractGenerator(feederId, appId uint32) Generator {
	return newSelfDestructGenerator(feederId, appId, selfDestructVariant{
		deploy: func(ctxt AppContext) (common.Address, *abi.ABI, error) {
			_, receipt, err := DeployContractWithValue(ctxt, contract.DeploySelfDestructOldContractFactory, oneWei)
			if err != nil {
				return common.Address{}, nil, fmt.Errorf("failed to deploy the SelfDestructOldContractFactory contract; %w", err)
			}
			parsedAbi, err := contract.SelfDestructOldContractFactoryMetaData.GetAbi()
			return receipt.ContractAddress, parsedAbi, err
		},
		method: "destructAndDeploy",
	})
}

// newSelfDestructNewContractGenerator creates a generator whose every transaction
// deploys a child contract and destroys it again within the same transaction. On
// Cancun and later such a contract is truly removed from the state.
func newSelfDestructNewContractGenerator(feederId, appId uint32) Generator {
	return newSelfDestructGenerator(feederId, appId, selfDestructVariant{
		deploy: func(ctxt AppContext) (common.Address, *abi.ABI, error) {
			_, receipt, err := DeployContract(ctxt, contract.DeploySelfDestructNewContractFactory)
			if err != nil {
				return common.Address{}, nil, fmt.Errorf("failed to deploy the SelfDestructNewContractFactory contract; %w", err)
			}
			parsedAbi, err := contract.SelfDestructNewContractFactoryMetaData.GetAbi()
			return receipt.ContractAddress, parsedAbi, err
		},
		method: "deployAndDestruct",
	})
}

// selfDestructVariant is what distinguishes the two self-destruct generators: the
// factory they install and the method their transactions call on it.
type selfDestructVariant struct {
	deploy func(ctxt AppContext) (common.Address, *abi.ABI, error)
	method string
}

func newSelfDestructGenerator(feederId, appId uint32, variant selfDestructVariant) Generator {
	g := &selfDestructGenerator{
		txGenerator: newTxGenerator(feederId, appId),
		variant:     variant,
	}
	g.onDeploy = g.deploy
	g.onSuccess = g.callFactory
	g.onRevert = g.callUnknownFunction
	return g
}

type selfDestructGenerator struct {
	txGenerator
	variant selfDestructVariant
	abi     *abi.ABI
	address common.Address
}

func (g *selfDestructGenerator) deploy(ctxt AppContext) error {
	address, parsedAbi, err := g.variant.deploy(ctxt)
	if err != nil {
		return err
	}
	g.address, g.abi = address, parsedAbi
	return nil
}

func (g *selfDestructGenerator) callFactory(int, *Account) (txPayload, error) {
	data, err := g.abi.Pack(g.variant.method)
	if err != nil {
		return txPayload{}, fmt.Errorf("failed to pack %s; %w", g.variant.method, err)
	}
	return txPayload{to: &g.address, data: data, value: oneWei, gasLimit: 100_000}, nil
}

func (g *selfDestructGenerator) callUnknownFunction(int, *Account) (txPayload, error) {
	return unknownFunctionPayload(g.address), nil
}
