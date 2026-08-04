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
	"math/big"

	"github.com/0xsoniclabs/sonic/gossip/blockproc/bundle"
	"github.com/ethereum/go-ethereum/core/types"
)

// newAllOfBundleGenerator creates a generator sending bundles in which two
// accounts cooperate atomically: the approver grants the spender an allowance of
// one token and the spender claims it in the very next transaction. The AllOf
// execution plan guarantees both transactions land in the same block or neither
// of them does, which is what makes the allowance safe to grant.
func newAllOfBundleGenerator(feederId, appId uint32) Generator {
	g := &allOfBundleGenerator{}
	g.bundleGenerator = newBundleGenerator(feederId, appId)
	g.sendersPerUser = 2 // < approver and spender
	g.onDeploy = g.deploy
	g.onAccounts = g.mintForApprovers
	g.onEnvelope = g.buildEnvelope
	return g
}

type allOfBundleGenerator struct {
	erc20Bundle
}

func (g *allOfBundleGenerator) deploy(ctxt AppContext) error {
	return g.deployToken(ctxt, "Bundle Token", "BTOK")
}

func (g *allOfBundleGenerator) mintForApprovers(ctxt AppContext, senders [][]*Account) error {
	return g.mintTokens(ctxt, accountAddresses(senders, 0))
}

func (g *allOfBundleGenerator) buildEnvelope(user int, senders []*Account) (*types.Transaction, error) {
	approver, spender := senders[0], senders[1]

	approveData, err := g.pack("approve", spender.address, big.NewInt(1))
	if err != nil {
		return nil, err
	}
	transferData, err := g.pack("transferFrom", approver.address, spender.address, big.NewInt(1))
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

	return bundle.NewBuilder().
		WithSigner(g.signer).
		AllOf(
			bundle.Step(approver.privateKey, g.step(nonces[0], approveData)),
			bundle.Step(spender.privateKey, g.step(nonces[1], transferData)),
		).
		SetEarliest(earliest).
		Build(), nil
}
