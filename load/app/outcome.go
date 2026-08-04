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
	"math/rand"

	"github.com/0xsoniclabs/sonic/opera"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
)

// outcomeWeights gives the relative frequency of each outcome among the
// outcomes a generator supports. Successful transactions dominate so that the
// load stays representative of a healthy network while still covering the paths
// a node takes for transactions it must reject or abort.
var outcomeWeights = map[Outcome]int{
	Success:  85,
	Reverted: 5,
	Failed:   5,
	Rejected: 5,
}

func pickOutcome(supported []Outcome) Outcome {
	total := 0
	for _, outcome := range supported {
		total += outcomeWeights[outcome]
	}
	if total == 0 {
		return Success
	}
	draw := rand.Intn(total)
	for _, outcome := range supported {
		draw -= outcomeWeights[outcome]
		if draw < 0 {
			return outcome
		}
	}
	return supported[len(supported)-1]
}

// minimumGas returns the smallest gas limit the transaction pool accepts for the
// given payload: the intrinsic cost of the transaction or the EIP-7623 calldata
// floor, whichever is higher. The authorizations a payload carries are part of the
// intrinsic cost, so a payload sent as a set-code transaction needs more than the
// same payload sent as a plain one.
func minimumGas(payload txPayload) (uint64, error) {
	authorizations := make([]types.SetCodeAuthorization, len(payload.delegators))
	intrinsic, err := core.IntrinsicGas(
		payload.data, nil, authorizations, payload.to == nil, true, true, true)
	if err != nil {
		return 0, fmt.Errorf("failed to compute intrinsic gas; %w", err)
	}
	floor, err := core.FloorDataGas(payload.data)
	if err != nil {
		return 0, fmt.Errorf("failed to compute floor data gas; %w", err)
	}
	return max(intrinsic, floor), nil
}

// outOfGasLimit returns a gas limit that covers the transaction's intrinsic cost
// but leaves nothing to execute with. A call sent with it is included in a block
// and aborts with an out-of-gas error, consuming the full limit.
func outOfGasLimit(payload txPayload) (uint64, error) {
	minimum, err := minimumGas(payload)
	if err != nil {
		return 0, err
	}
	return minimum + 1, nil
}

// canRunOutOfGas reports whether the given payload can be made to run out of gas by
// lowering its gas limit, given what the call costs in total.
//
// The EIP-7623 floor makes a transaction pay for its calldata whether it executes
// or not, and whatever that floor charges beyond the intrinsic cost is left for the
// execution to spend. A payload carrying a lot of calldata for a cheap call is
// therefore paid for by its own floor and runs to completion however low its limit
// is set, so such a payload cannot produce a failing transaction.
func canRunOutOfGas(payload txPayload, totalGas uint64) (bool, error) {
	minimum, err := minimumGas(payload)
	if err != nil {
		return false, err
	}
	return minimum < totalGas, nil
}

// authorizationGas is the intrinsic cost of the authorizations a set-code
// transaction carries. Gas estimation covers the call but not the authorizations,
// which are added to a transaction only when it is signed.
func authorizationGas(authorizations int) uint64 {
	return uint64(authorizations) * params.CallNewAccountGas
}

// poolGasLimit is the largest gas limit the transaction pool accepts. It mirrors
// the node's own computation in gossip.EvmStateReader.CurrentMaxGasLimit.
func poolGasLimit(rules opera.Rules) uint64 {
	gas := rules.Economy.Gas
	overhead := gas.EventGas +
		uint64(rules.Dag.MaxParents-rules.Dag.MaxFreeParents)*gas.ParentGas +
		uint64(rules.Dag.MaxExtraData)*gas.ExtraDataGas
	if gas.MaxEventGas < overhead {
		return 0
	}
	return gas.MaxEventGas - overhead
}

// rejectedGasLimit returns a gas limit above the largest one the transaction
// pool accepts, so that no node lets the transaction into a block.
func rejectedGasLimit(rules opera.Rules) uint64 {
	return poolGasLimit(rules) + 1
}
