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
	"fmt"
	"log/slog"
	"math/big"
	"sync/atomic"

	"github.com/0xsoniclabs/sonic/opera"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

var defaultFunding = new(big.Int).Mul(big.NewInt(1_000), big.NewInt(1e18))

// txPayload describes the body of a transaction a generator wants to send.
type txPayload struct {
	to       *common.Address // < nil deploys a contract
	data     []byte
	value    *big.Int // < nil sends no value
	gasLimit uint64
	// reason is the revert reason this payload produces. It is only relevant for
	// payloads built to revert; an empty reason accepts any revert reason.
	reason string
	// delegators are the accounts a generator signing set-code transactions
	// delegates along with the payload.
	delegators []*Account
}

// sign turns the payload into a transaction signed by the given account.
func (p txPayload) sign(from *Account, nonce uint64) (*types.Transaction, error) {
	value := p.value
	if value == nil {
		value = big.NewInt(0)
	}
	tx := types.NewTx(&types.DynamicFeeTx{
		Nonce:     nonce,
		GasFeeCap: gasFeeCap,
		GasTipCap: gasTipCap,
		Gas:       p.gasLimit,
		To:        p.to,
		Value:     value,
		Data:      p.data,
	})
	return types.SignTx(tx, types.NewLondonSigner(from.chainID), from.privateKey)
}

// payloadFunc builds the payload of the next transaction of the given user.
type payloadFunc func(user int, from *Account) (txPayload, error)

// unknownFunctionPayload calls a function the given contract does not implement.
// The dispatcher of a contract without a fallback function rejects such a call
// with an empty revert, refunding the gas the call did not use.
func unknownFunctionPayload(to common.Address) txPayload {
	return txPayload{
		to:       &to,
		data:     []byte{0xde, 0xad, 0xbe, 0xef},
		gasLimit: 30_000,
	}
}

// txGenerator implements Generator for generators that send plain transactions
// from a single account per user. A concrete generator embeds it, installs its
// contracts in onDeploy, and describes the transactions it sends in onSuccess
// and - if it can produce reverting calls - onRevert. The Failed and Rejected
// outcomes are derived from onSuccess and the network rules, so a concrete
// generator does not have to describe them.
type txGenerator struct {
	// onDeploy installs the contracts the generator needs on the chain.
	onDeploy func(ctxt AppContext) error
	// onAccounts prepares whatever state the accounts of the generator need, for
	// instance the tokens they are going to transfer. It runs once the accounts
	// have been created and funded.
	onAccounts func(ctxt AppContext, accounts []common.Address) error
	// onSuccess builds a transaction expected to run to completion.
	onSuccess payloadFunc
	// onRevert builds a transaction expected to revert. When nil, the generator
	// does not produce reverting transactions.
	onRevert payloadFunc
	// onSign turns a payload into a signed transaction. When nil, payloads are
	// sent as plain dynamic-fee transactions.
	onSign func(payload txPayload, from *Account, nonce uint64) (*types.Transaction, error)
	// supports reports whether the given rules allow this kind of load. When nil,
	// the generator works under any rules.
	supports func(opera.Rules) bool
	// funding overrides the amount of wei handed to each account.
	funding *big.Int

	accountFactory *AccountFactory
	senders        []*Account
	outcomes       []Outcome
	rejectGasLimit uint64
	rejectCounter  atomic.Uint64
}

// newTxGenerator prepares the account factory shared by all accounts of the
// generator. The concrete generator fills in the remaining fields.
func newTxGenerator(feederId, appId uint32) txGenerator {
	return txGenerator{
		accountFactory: NewAccountFactory(feederId, appId),
	}
}

func (g *txGenerator) Deploy(ctxt AppContext, numUsers int) error {
	rules, err := ctxt.GetRules()
	if err != nil {
		return err
	}
	if g.supports != nil && !g.supports(rules) {
		return ErrUnsupported
	}
	g.rejectGasLimit = rejectedGasLimit(rules)

	if g.onDeploy != nil {
		if err := g.onDeploy(ctxt); err != nil {
			return err
		}
	}

	g.senders = make([]*Account, numUsers)
	addresses := make([]common.Address, numUsers)
	for i := range g.senders {
		account, err := g.accountFactory.CreateAccount(ctxt.GetClient())
		if err != nil {
			return err
		}
		g.senders[i] = account
		addresses[i] = account.address
	}

	funding := g.funding
	if funding == nil {
		funding = defaultFunding
	}
	if err := ctxt.FundAccounts(addresses, funding); err != nil {
		return fmt.Errorf("failed to fund the accounts of the generator; %w", err)
	}

	if g.onAccounts != nil {
		if err := g.onAccounts(ctxt, addresses); err != nil {
			return err
		}
	}

	g.outcomes = []Outcome{Success, Rejected}
	if g.onRevert != nil {
		g.outcomes = append(g.outcomes, Reverted)
	}
	failing, err := g.canProduceFailingCalls(ctxt)
	if err != nil {
		return err
	}
	if failing {
		g.outcomes = append(g.outcomes, Failed)
	} else {
		// The generator being deployed is named in the log line that follows.
		slog.Info("load generator does not produce failing transactions: its calls are already paid for by the floor cost of their own call data")
	}
	return nil
}

// canProduceFailingCalls reports whether lowering the gas limit of the generator's
// call makes it run out of gas. It measures what the call costs on the network the
// generator was deployed to rather than assuming it, because a call carrying a lot
// of call data is paid for by its own floor cost and runs to completion however low
// its limit is set.
func (g *txGenerator) canProduceFailingCalls(ctxt AppContext) (bool, error) {
	if len(g.senders) == 0 {
		// Without a user there is nothing to send and nothing to measure with.
		return false, nil
	}
	from := g.senders[0]
	payload, err := g.onSuccess(0, from)
	if err != nil {
		return false, err
	}

	value := payload.value
	if value == nil {
		value = big.NewInt(0)
	}
	totalGas, err := ctxt.GetClient().EstimateGas(context.Background(), ethereum.CallMsg{
		From:  from.address,
		To:    payload.to,
		Value: value,
		Data:  payload.data,
	})
	if err != nil {
		return false, fmt.Errorf("failed to estimate the gas of the generator's call; %w", err)
	}
	// The estimate covers the call but not the authorizations a set-code
	// transaction adds on top of it.
	totalGas += authorizationGas(len(payload.delegators))

	return canRunOutOfGas(payload, totalGas)
}

func (g *txGenerator) Call(user int) (Call, error) {
	if user < 0 || user >= len(g.senders) {
		return Call{}, fmt.Errorf("no account for user %d, the generator serves %d users", user, len(g.senders))
	}
	switch outcome := pickOutcome(g.outcomes); outcome {
	case Reverted:
		return g.buildCall(user, g.onRevert, Reverted)
	case Failed:
		return g.failedCall(user)
	case Rejected:
		return g.rejectedCall(user)
	default:
		return g.buildCall(user, g.onSuccess, Success)
	}
}

func (g *txGenerator) Check(ctxt AppContext, call Call, receipt *types.Receipt) error {
	return CheckOutcome(ctxt, call, receipt)
}

// sign turns a payload into a signed transaction, using the generator's own
// signing scheme if it defined one.
func (g *txGenerator) sign(payload txPayload, from *Account, nonce uint64) (*types.Transaction, error) {
	if g.onSign != nil {
		return g.onSign(payload, from, nonce)
	}
	return payload.sign(from, nonce)
}

func (g *txGenerator) buildCall(user int, build payloadFunc, outcome Outcome) (Call, error) {
	from := g.senders[user]
	payload, err := build(user, from)
	if err != nil {
		return Call{}, err
	}
	tx, err := g.sign(payload, from, from.getNextNonce())
	if err != nil {
		return Call{}, err
	}
	call := Call{Tx: tx, Outcome: outcome}
	if outcome == Reverted {
		call.Reason = payload.reason
	}
	return call, nil
}

// failedCall sends the generator's regular transaction with a gas limit that
// covers no more than its intrinsic cost, so it is included in a block and runs
// out of gas while executing.
func (g *txGenerator) failedCall(user int) (Call, error) {
	from := g.senders[user]
	payload, err := g.onSuccess(user, from)
	if err != nil {
		return Call{}, err
	}
	payload.gasLimit, err = outOfGasLimit(payload)
	if err != nil {
		return Call{}, err
	}
	tx, err := g.sign(payload, from, from.getNextNonce())
	if err != nil {
		return Call{}, err
	}
	return Call{Tx: tx, Outcome: Failed}, nil
}

// rejectedCall sends the generator's regular transaction with a gas limit above
// the largest one the transaction pool accepts. Because the transaction never
// reaches a block it does not consume a nonce, so it is signed with the sender's
// current nonce without advancing it - the transactions carrying the remaining
// load keep their uninterrupted nonce sequence. The gas limit is varied per call
// so that no two rejected transactions of a generator share a hash.
func (g *txGenerator) rejectedCall(user int) (Call, error) {
	from := g.senders[user]
	payload, err := g.onSuccess(user, from)
	if err != nil {
		return Call{}, err
	}
	payload.gasLimit = g.rejectGasLimit + g.rejectCounter.Add(1)
	tx, err := g.sign(payload, from, from.getCurrentNonce())
	if err != nil {
		return Call{}, err
	}
	return Call{Tx: tx, Outcome: Rejected}, nil
}
