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

	"github.com/0xsoniclabs/sonic/gossip/blockproc/bundle"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// newOneOfBundleGenerator creates a generator sending bundles of which only one
// transaction is meant to take effect: the sender offers a transfer it can afford
// and one over its balance, and the OneOf execution plan settles for the first one
// that works. The order of the two is randomized so that both the case of the
// first step succeeding and of it failing are exercised.
func newOneOfBundleGenerator(feederId, appId uint32) Generator {
	g := &oneOfBundleGenerator{}
	g.bundleGenerator = newBundleGenerator(feederId, appId)
	g.onDeploy = g.deploy
	g.onAccounts = g.mintForSenders
	g.onEnvelope = g.buildEnvelope
	return g
}

type oneOfBundleGenerator struct {
	erc20Bundle
	target common.Address
}

func (g *oneOfBundleGenerator) deploy(ctxt AppContext) error {
	if err := g.deployToken(ctxt, "OneOf Token", "OTOK"); err != nil {
		return err
	}
	target, err := g.accountFactory.CreateAccount(ctxt.GetClient())
	if err != nil {
		return fmt.Errorf("failed to create the target account; %w", err)
	}
	g.target = target.address
	return nil
}

func (g *oneOfBundleGenerator) mintForSenders(ctxt AppContext, senders [][]*Account) error {
	return g.mintTokens(ctxt, accountAddresses(senders, 0))
}

func (g *oneOfBundleGenerator) buildEnvelope(user int, senders []*Account) (*types.Transaction, error) {
	sender := senders[0]

	affordable, err := g.pack("transfer", g.target, big.NewInt(1))
	if err != nil {
		return nil, err
	}
	overBalance, err := g.pack("transfer", g.target, new(big.Int).Mul(bundleTokensPerAccount, big.NewInt(2)))
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
	nonce := nonces[0]

	var first, second bundle.BuilderStep
	if rand.Intn(2) == 0 {
		// The affordable transfer comes first and settles the plan, so the second
		// step never runs and its nonce is never consumed.
		first = bundle.Step(sender.privateKey, g.step(nonce, affordable))
		second = bundle.Step(sender.privateKey, g.step(nonce, overBalance))
	} else {
		// The over-balance transfer comes first and fails, so both steps run and
		// both nonces are consumed.
		first = bundle.Step(sender.privateKey, g.step(nonce, overBalance))
		second = bundle.Step(sender.privateKey, g.step(nonce+1, affordable))
	}

	return bundle.NewBuilder().
		WithSigner(g.signer).
		OneOf(first, second).
		SetEarliest(earliest).
		Build(), nil
}
