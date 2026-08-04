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
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
)

// storeUpdateSize allocates roughly 1 GB of new state per minute at 1000 Tx/s.
const storeUpdateSize = 260

// newStoreGenerator creates a generator writing to a user-private key/value store,
// the state-heaviest load of the mix.
func newStoreGenerator(feederId, appId uint32) Generator {
	g := &storeGenerator{txGenerator: newTxGenerator(feederId, appId)}
	g.onDeploy = g.deploy
	g.onSuccess = g.fill
	g.onRevert = g.callUnknownFunction
	return g
}

type storeGenerator struct {
	txGenerator
	abi     *abi.ABI
	address common.Address
	// slots gives every call a distinct range of storage slots, so that each of
	// them allocates new state instead of overwriting existing state.
	slots atomic.Int64
}

func (g *storeGenerator) deploy(ctxt AppContext) error {
	_, receipt, err := DeployContract(ctxt, contract.DeployStore)
	if err != nil {
		return fmt.Errorf("failed to deploy the Store contract; %w", err)
	}
	g.address = receipt.ContractAddress
	g.abi, err = contract.StoreMetaData.GetAbi()
	return err
}

func (g *storeGenerator) fill(int, *Account) (txPayload, error) {
	value := g.slots.Add(1)
	from := value * storeUpdateSize
	data, err := g.abi.Pack("fill", big.NewInt(from), big.NewInt(from+storeUpdateSize), big.NewInt(value))
	if err != nil {
		return txPayload{}, fmt.Errorf("failed to pack fill; %w", err)
	}
	return txPayload{
		to:       &g.address,
		data:     data,
		gasLimit: 52_000 + 25_000*storeUpdateSize,
	}, nil
}

func (g *storeGenerator) callUnknownFunction(int, *Account) (txPayload, error) {
	return unknownFunctionPayload(g.address), nil
}
