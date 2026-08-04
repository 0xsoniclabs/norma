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
	"time"

	contract "github.com/0xsoniclabs/norma/load/contracts/abi"
	"github.com/0xsoniclabs/sonic/gossip/blockproc/bundle"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

const (
	// allOfFailureProbability leaves an AllOf bundle of two steps a 90% chance of
	// going through as a whole (0.95 * 0.95).
	allOfFailureProbability uint8 = 5
	// oneOfFailureProbability leaves a OneOf bundle of two steps a 91% chance of
	// having one of them go through (1 - 0.3 * 0.3).
	oneOfFailureProbability uint8 = 30
	failingBundleStepGas = 30_000
)

// newFailingBundleGenerator creates a generator whose bundle steps fail
// unpredictably: the contract they call reverts on a share of the calls that
// depends on state the caller cannot compute up front. Each bundle is either an
// AllOf of two rarely failing steps or a OneOf of two often failing steps, so both
// the plan that needs every step and the plan that needs any step meet failures.
func newFailingBundleGenerator(feederId, appId uint32) Generator {
	g := &failingBundleGenerator{}
	g.bundleGenerator = newBundleGenerator(feederId, appId)
	g.sendersPerUser = 2
	g.onDeploy = g.deploy
	g.onEnvelope = g.buildEnvelope
	return g
}

type failingBundleGenerator struct {
	bundleGenerator
	abi     *abi.ABI
	address common.Address
}

// Check accepts both a plan the network executed and one it dropped. Unlike the
// other bundle generators, this one is built so that its steps fail unpredictably,
// and a plan whose steps fail is meant to be dropped - roughly one in ten of them
// is. Pinning it to either outcome would be asserting the outcome of a coin toss.
// What is still verified is that every envelope it produced is a bundle the network
// could make sense of.
func (g *failingBundleGenerator) Check(_ AppContext, call Call, _ *types.Receipt) error {
	if _, _, err := bundle.ValidateEnvelope(g.signer, call.Tx); err != nil {
		return fmt.Errorf("the generated envelope %v is not a valid bundle; %w", call.Tx.Hash(), err)
	}
	return nil
}

func (g *failingBundleGenerator) deploy(ctxt AppContext) error {
	_, receipt, err := DeployContract(ctxt, contract.DeployProbabilisticFailing)
	if err != nil {
		return fmt.Errorf("failed to deploy the ProbabilisticFailing contract; %w", err)
	}
	g.address = receipt.ContractAddress
	g.abi, err = contract.ProbabilisticFailingMetaData.GetAbi()
	return err
}

func (g *failingBundleGenerator) buildEnvelope(user int, senders []*Account) (*types.Transaction, error) {
	useAllOf := rand.Intn(2) == 0
	failureProbability := oneOfFailureProbability
	if useAllOf {
		failureProbability = allOfFailureProbability
	}

	// The seed keeps the outcome unpredictable from one bundle to the next.
	data, err := g.abi.Pack("incrementCounter", failureProbability, uint32(time.Now().Nanosecond()))
	if err != nil {
		return nil, fmt.Errorf("failed to pack incrementCounter; %w", err)
	}

	nonces, err := g.nonces(senders)
	if err != nil {
		return nil, err
	}
	earliest, err := g.currentBlock()
	if err != nil {
		return nil, err
	}

	steps := make([]bundle.BuilderStep, len(senders))
	for i, sender := range senders {
		steps[i] = bundle.Step(sender.privateKey, &types.DynamicFeeTx{
			Nonce:     nonces[i],
			Gas:       failingBundleStepGas,
			GasFeeCap: gasFeeCap,
			GasTipCap: gasTipCap,
			To:        &g.address,
			Data:      data,
		})
	}

	builder := bundle.NewBuilder().WithSigner(g.signer).SetEarliest(earliest)
	if useAllOf {
		builder = builder.AllOf(steps[0], steps[1])
	} else {
		builder = builder.OneOf(steps[0], steps[1])
	}
	return builder.Build(), nil
}
