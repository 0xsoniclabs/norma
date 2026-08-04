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

	"github.com/0xsoniclabs/sonic/gossip/blockproc/bundle"
	"github.com/0xsoniclabs/sonic/gossip/blockproc/subsidies/registry"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// subsidizedApproveGas is the gas limit of the subsidized approve call and the
// amount the sponsorship has to cover.
const subsidizedApproveGas = 70_000

// newSubsidizedBundleGenerator creates a generator combining subsidies with atomic
// execution. Each bundle funds the subsidy of an approve call, has the user make
// that call without paying for it, and lets the sponsor claim the approved token -
// all three within one AllOf plan, so the sponsor never funds a subsidy it does not
// get to use.
func newSubsidizedBundleGenerator(feederId, appId uint32) Generator {
	g := &subsidizedBundleGenerator{}
	g.bundleGenerator = newBundleGenerator(feederId, appId)
	g.sendersPerUser = 2 // < user and sponsor
	g.requiresSubsidies = true
	g.onDeploy = g.deploy
	g.onAccounts = g.prepareUsers
	g.onEnvelope = g.buildEnvelope
	return g
}

type subsidizedBundleGenerator struct {
	erc20Bundle
	registryAddress common.Address
	registryAbi     *abi.ABI
	// approvalFunds holds the subsidy fund id of the approve call of every user.
	approvalFunds [][32]byte
}

func (g *subsidizedBundleGenerator) deploy(ctxt AppContext) error {
	if err := g.deployToken(ctxt, "Token", "TOK"); err != nil {
		return err
	}
	g.registryAddress = registry.GetAddress()
	registryAbi, err := registry.RegistryMetaData.GetAbi()
	if err != nil {
		return fmt.Errorf("failed to parse the subsidies registry ABI; %w", err)
	}
	g.registryAbi = registryAbi
	return nil
}

// prepareUsers mints the tokens the users approve and looks up the subsidy fund
// each of their approve calls is paid from.
func (g *subsidizedBundleGenerator) prepareUsers(ctxt AppContext, senders [][]*Account) error {
	if err := g.mintTokens(ctxt, accountAddresses(senders, 0)); err != nil {
		return err
	}

	subsidiesRegistry, err := registry.NewRegistry(g.registryAddress, ctxt.GetClient())
	if err != nil {
		return fmt.Errorf("failed to bind the subsidies registry; %w", err)
	}

	g.approvalFunds = make([][32]byte, len(senders))
	for i, group := range senders {
		user, sponsor := group[0], group[1]
		approveData, err := g.pack("approve", sponsor.address, big.NewInt(1))
		if err != nil {
			return err
		}
		// The fund id identifies the sponsorship of exactly this call by exactly
		// this user on exactly this contract.
		_, fundId, err := subsidiesRegistry.ApprovalSponsorshipFundId(nil, user.address, g.address, approveData)
		if err != nil {
			return fmt.Errorf("failed to get the approval fund id of user %d; %w", i, err)
		}
		g.approvalFunds[i] = fundId
	}
	return nil
}

func (g *subsidizedBundleGenerator) buildEnvelope(user int, senders []*Account) (*types.Transaction, error) {
	sender, sponsor := senders[0], senders[1]

	sponsorData, err := g.registryAbi.Pack("sponsor", g.approvalFunds[user])
	if err != nil {
		return nil, fmt.Errorf("failed to pack sponsor; %w", err)
	}
	approveData, err := g.pack("approve", sponsor.address, big.NewInt(1))
	if err != nil {
		return nil, err
	}
	transferData, err := g.pack("transferFrom", sender.address, sponsor.address, big.NewInt(1))
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
	nonceUser, nonceSponsor := nonces[0], nonces[1]

	// The sponsored value has to cover the gas of the user's approve call.
	sponsoredValue := new(big.Int).Mul(big.NewInt(subsidizedApproveGas), gasFeeCap)

	return bundle.NewBuilder().
		WithSigner(g.signer).
		AllOf(
			// 1. The sponsor funds the subsidy of the approve call.
			bundle.Step(sponsor.privateKey, &types.DynamicFeeTx{
				Nonce:     nonceSponsor,
				Gas:       bundleStepGas,
				GasFeeCap: gasFeeCap,
				GasTipCap: gasTipCap,
				To:        &g.registryAddress,
				Value:     sponsoredValue,
				Data:      sponsorData,
			}),
			// 2. The user approves without offering a fee, which the subsidy pays.
			bundle.Step(sender.privateKey, &types.DynamicFeeTx{
				Nonce:     nonceUser,
				Gas:       subsidizedApproveGas,
				GasFeeCap: big.NewInt(0),
				GasTipCap: big.NewInt(0),
				To:        &g.address,
				Data:      approveData,
			}),
			// 3. The sponsor claims the approved token.
			bundle.Step(sponsor.privateKey, g.step(nonceSponsor+1, transferData)),
		).
		SetEarliest(earliest).
		Build(), nil
}
