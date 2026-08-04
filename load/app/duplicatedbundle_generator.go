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
	"math/rand"
	"sync"

	"github.com/0xsoniclabs/sonic/gossip/blockproc/bundle"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

// duplicatesPerPlan is how often the same execution plan is submitted, each time
// wrapped in a fresh envelope, before a new plan is built.
const duplicatesPerPlan = 5

// newDuplicatedBundleGenerator creates a generator submitting the same execution
// plan over and over, each time inside a new envelope signed by a different key.
// It covers the network's handling of a plan it has seen before: whatever it does
// with the repeated envelopes, the plan itself must take effect exactly once.
func newDuplicatedBundleGenerator(feederId, appId uint32) Generator {
	g := &duplicatedBundleGenerator{}
	g.bundleGenerator = newBundleGenerator(feederId, appId)
	g.sendersPerUser = 2
	g.onDeploy = g.deploy
	g.onAccounts = g.mintForSenders
	g.onEnvelope = g.buildEnvelope
	return g
}

type duplicatedBundleGenerator struct {
	erc20Bundle
	target common.Address
	// plans holds the plan every user is currently resubmitting.
	plans []plannedBundle
	mutex sync.Mutex
}

type plannedBundle struct {
	envelope    *types.Transaction
	submissions int
}

func (g *duplicatedBundleGenerator) deploy(ctxt AppContext) error {
	if err := g.deployToken(ctxt, "Duplicated Token", "DTOK"); err != nil {
		return err
	}
	target, err := g.accountFactory.CreateAccount(ctxt.GetClient())
	if err != nil {
		return fmt.Errorf("failed to create the target account; %w", err)
	}
	g.target = target.address
	return nil
}

func (g *duplicatedBundleGenerator) mintForSenders(ctxt AppContext, senders [][]*Account) error {
	g.plans = make([]plannedBundle, len(senders))
	addresses := append(accountAddresses(senders, 0), accountAddresses(senders, 1)...)
	return g.mintTokens(ctxt, addresses)
}

func (g *duplicatedBundleGenerator) buildEnvelope(user int, senders []*Account) (*types.Transaction, error) {
	g.mutex.Lock()
	plan := g.plans[user]
	g.mutex.Unlock()

	if plan.envelope == nil || plan.submissions >= duplicatesPerPlan {
		envelope, err := g.buildNewPlan(user, senders)
		if err != nil {
			return nil, err
		}
		g.mutex.Lock()
		g.plans[user] = plannedBundle{envelope: envelope, submissions: 1}
		g.mutex.Unlock()
		return envelope, nil
	}

	duplicate, err := g.rewrapPlan(plan.envelope)
	if err != nil {
		return nil, err
	}
	g.mutex.Lock()
	g.plans[user].submissions++
	g.mutex.Unlock()
	return duplicate, nil
}

func (g *duplicatedBundleGenerator) buildNewPlan(user int, senders []*Account) (*types.Transaction, error) {
	senderA, senderB := senders[0], senders[1]

	toTarget, err := g.pack("transfer", g.target, big.NewInt(1))
	if err != nil {
		return nil, err
	}
	toSenderB, err := g.pack("transfer", senderB.address, big.NewInt(1))
	if err != nil {
		return nil, err
	}

	nonces, err := g.nonces(senders)
	if err != nil {
		return nil, err
	}
	earliest, err := g.currentBlock()
	if err != nil {
		return nil, err
	}

	builder := bundle.NewBuilder().WithSigner(g.signer).SetEarliest(earliest)
	if rand.Intn(2) == 0 {
		builder = builder.OneOf(
			bundle.Step(senderA.privateKey, g.step(nonces[0], toTarget)),
			bundle.Step(senderB.privateKey, g.step(nonces[1], toTarget)),
		)
	} else {
		builder = builder.AllOf(
			bundle.Step(senderA.privateKey, g.step(nonces[0], toSenderB)),
			bundle.Step(senderB.privateKey, g.step(nonces[1], toTarget)),
		)
	}
	return builder.Build(), nil
}

// rewrapPlan puts the plan of an existing envelope into a new envelope signed by a
// freshly generated key. The envelope offers no fee, so the key needs no balance.
func (g *duplicatedBundleGenerator) rewrapPlan(envelope *types.Transaction) (*types.Transaction, error) {
	key, err := crypto.GenerateKey()
	if err != nil {
		return nil, fmt.Errorf("failed to generate an envelope key; %w", err)
	}
	tx, err := types.SignNewTx(key, g.signer, &types.AccessListTx{
		To:   &bundle.BundleProcessor,
		Data: envelope.Data(),
		Gas:  envelope.Gas(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to sign the duplicated envelope; %w", err)
	}
	return tx, nil
}
