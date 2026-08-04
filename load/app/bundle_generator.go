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
	"context"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/0xsoniclabs/norma/driver/rpc"
	"github.com/0xsoniclabs/sonic/gossip/blockproc/bundle"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

const (
	// bundleStepGas is the gas limit given to a single transaction inside a bundle.
	// It covers a base cost of 21000 plus a handful of storage writes and an event.
	bundleStepGas = 90_000

	// planExecutionWindow is how long the plan of a bundle is given to be executed
	// before the network is taken to have dropped it.
	planExecutionWindow = 30 * time.Second
)

type envelopeFunc func(user int, senders []*Account) (*types.Transaction, error)

// bundleGenerator implements Generator for the generators sending Brio bundles. A
// bundle wraps several inner transactions of several accounts into one envelope
// that the network either executes as a whole or not at all, so a bundle generator
// owns a group of accounts per user and builds the envelope itself rather than
// letting txGenerator sign a plain transaction.
type bundleGenerator struct {
	// onDeploy installs the contracts the generator needs on the chain.
	onDeploy func(ctxt AppContext) error
	// onAccounts prepares the state the accounts of the generator need once they
	// have been created and funded.
	onAccounts func(ctxt AppContext, accounts [][]*Account) error
	// onEnvelope builds the bundle a user submits next.
	onEnvelope envelopeFunc
	// sendersPerUser is the number of accounts each user needs.
	sendersPerUser int
	// requiresSubsidies marks the generators whose bundles pay for an inner
	// transaction out of a subsidy.
	requiresSubsidies bool

	accountFactory *AccountFactory
	senders        [][]*Account
	signer         types.Signer
	client         rpc.Client
}

func newBundleGenerator(feederId, appId uint32) bundleGenerator {
	return bundleGenerator{
		accountFactory: NewAccountFactory(feederId, appId),
		sendersPerUser: 1,
	}
}

func (g *bundleGenerator) Deploy(ctxt AppContext, numUsers int) error {
	rules, err := ctxt.GetRules()
	if err != nil {
		return err
	}
	if !rules.Upgrades.Brio || !rules.Upgrades.TransactionBundles {
		return ErrUnsupported
	}
	if g.requiresSubsidies && !rules.Upgrades.GasSubsidies {
		return ErrUnsupported
	}

	g.client = ctxt.GetClient()

	if g.onDeploy != nil {
		if err := g.onDeploy(ctxt); err != nil {
			return err
		}
	}

	g.senders = make([][]*Account, numUsers)
	addresses := make([]common.Address, 0, numUsers*g.sendersPerUser)
	for user := range g.senders {
		g.senders[user] = make([]*Account, g.sendersPerUser)
		for i := range g.senders[user] {
			account, err := g.accountFactory.CreateAccount(g.client)
			if err != nil {
				return err
			}
			g.senders[user][i] = account
			addresses = append(addresses, account.address)
		}
	}
	if len(g.senders) > 0 {
		g.signer = types.LatestSignerForChainID(g.senders[0][0].chainID)
	}

	if err := ctxt.FundAccounts(addresses, g.funding()); err != nil {
		return fmt.Errorf("failed to fund the accounts of the generator; %w", err)
	}

	if g.onAccounts != nil {
		return g.onAccounts(ctxt, g.senders)
	}
	return nil
}

// funding is the amount of wei each account of the generator receives. A generator
// paying for subsidies needs enough to cover the subsidized transactions on top of
// its own.
func (g *bundleGenerator) funding() *big.Int {
	if g.requiresSubsidies {
		return new(big.Int).Mul(big.NewInt(100_000), big.NewInt(1e18))
	}
	return defaultFunding
}

// Call produces the next bundle of the given user.
//
// The outcome of a bundle is Indeterminate because an envelope is not a
// transaction of the block it lands in: the network unwraps it and executes the
// inner transactions, and those are the ones the block carries receipts for. The
// envelope hash never turns up in a receipt, so what Check holds the bundle
// against is the fate of the plan it carries.
func (g *bundleGenerator) Call(user int) (Call, error) {
	if user < 0 || user >= len(g.senders) {
		return Call{}, fmt.Errorf("no accounts for user %d, the generator serves %d users", user, len(g.senders))
	}
	tx, err := g.onEnvelope(user, g.senders[user])
	if err != nil {
		return Call{}, err
	}
	return Call{Tx: tx, Outcome: Indeterminate}, nil
}

// Check verifies that the network executed the plan the envelope carries. What the
// plan then does with its inner transactions - run all of them, settle for one of
// them, or let some of them revert - is up to the execution plan the generator
// built and is not part of this check.
func (g *bundleGenerator) Check(ctxt AppContext, call Call, _ *types.Receipt) error {
	_, plan, err := bundle.ValidateEnvelope(g.signer, call.Tx)
	if err != nil {
		return fmt.Errorf("the generated envelope %v is not a valid bundle; %w", call.Tx.Hash(), err)
	}
	planHash := plan.Hash()

	ctx, cancel := context.WithTimeout(context.Background(), planExecutionWindow)
	defer cancel()
	info, err := ctxt.GetClient().WaitForBundleInfo(ctx, planHash)
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("plan %v of envelope %v was not executed within %s",
			planHash, call.Tx.Hash(), planExecutionWindow)
	}
	if err != nil {
		return fmt.Errorf("failed to read the execution info of plan %v; %w", planHash, err)
	}
	if info == nil {
		return fmt.Errorf("plan %v of envelope %v was not executed", planHash, call.Tx.Hash())
	}
	return nil
}

// nonces reads the pending nonces of the given accounts from the chain. The inner
// transactions of a bundle only consume their nonces once the plan is executed, so
// their senders cannot keep a local nonce counter the way a plain sender does.
func (g *bundleGenerator) nonces(accounts []*Account) ([]uint64, error) {
	ctx := context.Background()
	nonces := make([]uint64, len(accounts))
	for i, account := range accounts {
		nonce, err := g.client.PendingNonceAt(ctx, account.address)
		if err != nil {
			return nil, fmt.Errorf("failed to get the nonce of %v; %w", account.address, err)
		}
		nonces[i] = nonce
	}
	return nonces, nil
}

// currentBlock reports the block a bundle becomes eligible from.
func (g *bundleGenerator) currentBlock() (uint64, error) {
	block, err := g.client.BlockNumber(context.Background())
	if err != nil {
		return 0, fmt.Errorf("failed to get the current block number; %w", err)
	}
	return block, nil
}
