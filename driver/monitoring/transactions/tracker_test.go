// Copyright 2024 Fantom Foundation
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

package txmon

import (
	"crypto/ecdsa"
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/sonic/gossip/blockproc/bundle"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"
)

var (
	chainID = big.NewInt(0xfa3)
	signer  = types.LatestSignerForChainID(chainID)
	epoch   = time.Unix(1_700_000_000, 0)
)

func TestTracker_TransactionMovesThroughItsPhases(t *testing.T) {
	require := require.New(t)
	tracker := NewTracker()
	user := newAccount(t, 1)

	tx := user.transaction(t, 0)
	tracker.OnTransactionSubmitted(source("app", 0), tx, epoch, nil)
	require.Equal(Counts{Pending: 1}, tracker.Counts("app"))

	tracker.MarkEmitted(tx.Hash(), epoch.Add(300*time.Millisecond))
	require.Equal(Counts{Emitted: 1}, tracker.Counts("app"))

	tracker.MarkIncluded(tx.Hash(), epoch.Add(time.Second))
	require.Equal(Counts{Included: 1}, tracker.Counts("app"))

	require.Equal(
		[]Sample{{At: epoch, Duration: 300 * time.Millisecond}},
		tracker.Samples("app", TimeToEmit),
	)
	require.Equal(
		[]Sample{{At: epoch, Duration: time.Second}},
		tracker.Samples("app", TimeToInclude),
	)
	require.Equal(
		[]Sample{{At: epoch, Duration: 700 * time.Millisecond}},
		tracker.Samples("app", TimeEmitToInclude),
	)
}

func TestTracker_RejectedTransactionIsCountedAndNotFollowed(t *testing.T) {
	require := require.New(t)
	tracker := NewTracker()
	user := newAccount(t, 1)

	tx := user.transaction(t, 0)
	tracker.OnTransactionSubmitted(source("app", 0), tx, epoch, errors.New("underpriced"))
	require.Equal(Counts{Rejected: 1}, tracker.Counts("app"))

	// A rejected transaction never reached a pool, so a later report about it
	// cannot be about the same submission.
	tracker.MarkIncluded(tx.Hash(), epoch.Add(time.Second))
	require.Equal(Counts{Rejected: 1}, tracker.Counts("app"))
	require.Empty(tracker.Samples("app", TimeToInclude))
}

func TestTracker_AnUninterruptedNonceRunIsEmittable(t *testing.T) {
	require := require.New(t)
	tracker := NewTracker()
	user := newAccount(t, 1)

	// A validator may carry consecutive nonces of one sender in one event, so
	// all of them are waiting for capacity rather than for each other.
	for nonce := range uint64(3) {
		tracker.OnTransactionSubmitted(source("app", 0), user.transaction(t, nonce), epoch, nil)
	}
	require.Equal(Counts{Pending: 3}, tracker.Counts("app"))
}

func TestTracker_TransactionsBehindAMissingNonceAreStalled(t *testing.T) {
	require := require.New(t)
	tracker := NewTracker()
	user := newAccount(t, 1)

	// The transaction with nonce 1 never reaches the network - it was rejected,
	// lost, or is still being created - so everything above it is stuck.
	tracker.OnTransactionSubmitted(source("app", 0), user.transaction(t, 0), epoch, nil)
	tracker.OnTransactionSubmitted(source("app", 0), user.transaction(t, 2), epoch, nil)
	tracker.OnTransactionSubmitted(source("app", 0), user.transaction(t, 3), epoch, nil)
	require.Equal(Counts{Pending: 1, Stalled: 2}, tracker.Counts("app"))

	// Filling the gap makes the whole run emittable at once.
	tracker.OnTransactionSubmitted(source("app", 0), user.transaction(t, 1), epoch, nil)
	require.Equal(Counts{Pending: 4}, tracker.Counts("app"))
}

func TestTracker_StalledTransactionsStayStalledWhenTheRunBelowThemProgresses(t *testing.T) {
	require := require.New(t)
	tracker := NewTracker()
	user := newAccount(t, 1)

	first, blocked := user.transaction(t, 0), user.transaction(t, 2)
	tracker.OnTransactionSubmitted(source("app", 0), first, epoch, nil)
	tracker.OnTransactionSubmitted(source("app", 0), blocked, epoch, nil)
	require.Equal(Counts{Pending: 1, Stalled: 1}, tracker.Counts("app"))

	tracker.MarkEmitted(first.Hash(), epoch.Add(100*time.Millisecond))
	require.Equal(Counts{Emitted: 1, Stalled: 1}, tracker.Counts("app"))

	// Nonce 1 is still missing, so committing nonce 0 changes nothing for the
	// transaction waiting behind the gap.
	tracker.MarkIncluded(first.Hash(), epoch.Add(time.Second))
	require.Equal(Counts{Included: 1, Stalled: 1}, tracker.Counts("app"))
}

func TestTracker_TransactionsOfDifferentSendersDoNotBlockEachOther(t *testing.T) {
	require := require.New(t)
	tracker := NewTracker()

	for i := range 3 {
		user := newAccount(t, int64(i+1))
		tracker.OnTransactionSubmitted(source("app", i), user.transaction(t, 0), epoch, nil)
	}
	require.Equal(Counts{Pending: 3}, tracker.Counts("app"))
}

func TestTracker_NonceSequenceIsFollowedInAnyReportOrder(t *testing.T) {
	require := require.New(t)
	tracker := NewTracker()
	user := newAccount(t, 1)

	// The transactions are submitted in nonce order, but a network may commit
	// them in one block, reported one after the other.
	txs := []*types.Transaction{
		user.transaction(t, 0), user.transaction(t, 1), user.transaction(t, 2),
	}
	for _, tx := range txs {
		tracker.OnTransactionSubmitted(source("app", 0), tx, epoch, nil)
	}
	require.Equal(Counts{Pending: 3}, tracker.Counts("app"))

	for i, tx := range txs {
		tracker.MarkIncluded(tx.Hash(), epoch.Add(time.Duration(i+1)*time.Second))
	}
	require.Equal(Counts{Included: 3}, tracker.Counts("app"))
}

func TestTracker_TransactionsAreCountedPerApplication(t *testing.T) {
	require := require.New(t)
	tracker := NewTracker()

	fast := newAccount(t, 1).transaction(t, 0)
	ordinary := newAccount(t, 2).transaction(t, 0)
	tracker.OnTransactionSubmitted(source("fast", 0), fast, epoch, nil)
	tracker.OnTransactionSubmitted(source("ordinary", 0), ordinary, epoch, nil)
	tracker.MarkIncluded(fast.Hash(), epoch.Add(time.Second))

	require.Equal(Counts{Included: 1}, tracker.Counts("fast"))
	require.Equal(Counts{Pending: 1}, tracker.Counts("ordinary"))
	require.ElementsMatch([]string{"fast", "ordinary"}, tracker.Apps())
}

func TestTracker_ReportsAboutUnknownTransactionsAreIgnored(t *testing.T) {
	require := require.New(t)
	tracker := NewTracker()

	tracker.MarkEmitted(common.Hash{0x01}, epoch)
	tracker.MarkIncluded(common.Hash{0x02}, epoch)
	require.Empty(tracker.Apps())
}

func TestTracker_RepeatedReportsKeepTheFirstObservation(t *testing.T) {
	require := require.New(t)
	tracker := NewTracker()
	user := newAccount(t, 1)
	tx := user.transaction(t, 0)

	tracker.OnTransactionSubmitted(source("app", 0), tx, epoch, nil)
	tracker.OnTransactionSubmitted(source("app", 0), tx, epoch.Add(time.Second), nil)
	require.Equal(Counts{Pending: 1}, tracker.Counts("app"))

	tracker.MarkEmitted(tx.Hash(), epoch.Add(200*time.Millisecond))
	tracker.MarkEmitted(tx.Hash(), epoch.Add(900*time.Millisecond))
	require.Equal(
		[]Sample{{At: epoch, Duration: 200 * time.Millisecond}},
		tracker.Samples("app", TimeToEmit),
	)

	// The inclusion releases the transaction, so a second report about it finds
	// nothing to count.
	tracker.MarkIncluded(tx.Hash(), epoch.Add(time.Second))
	tracker.MarkIncluded(tx.Hash(), epoch.Add(2*time.Second))
	require.Equal(Counts{Included: 1}, tracker.Counts("app"))
}

func TestTracker_InclusionWithoutAnObservedEmissionIsStillMeasured(t *testing.T) {
	require := require.New(t)
	tracker := NewTracker()
	tx := newAccount(t, 1).transaction(t, 0)

	tracker.OnTransactionSubmitted(source("app", 0), tx, epoch, nil)
	tracker.MarkIncluded(tx.Hash(), epoch.Add(time.Second))

	require.Equal(Counts{Included: 1}, tracker.Counts("app"))
	require.Len(tracker.Samples("app", TimeToInclude), 1)
	require.Empty(tracker.Samples("app", TimeToEmit),
		"the emission was never seen, so its duration is unknown")
	require.Empty(tracker.Samples("app", TimeEmitToInclude))
}

func TestTracker_TimestampsThatContradictThePhaseOrderAreDropped(t *testing.T) {
	require := require.New(t)
	tracker := NewTracker()
	tx := newAccount(t, 1).transaction(t, 0)

	// Emissions are timestamped by the emitting validator, whose clock may be
	// slightly behind the one measuring the submission.
	tracker.OnTransactionSubmitted(source("app", 0), tx, epoch, nil)
	tracker.MarkEmitted(tx.Hash(), epoch.Add(-time.Millisecond))
	require.Empty(tracker.Samples("app", TimeToEmit))
	require.Equal(Counts{Emitted: 1}, tracker.Counts("app"),
		"the emission itself was observed, only its duration is unusable")
}

func TestTracker_TracksAtMostTheConfiguredNumberOfTransactions(t *testing.T) {
	require := require.New(t)
	tracker := NewTracker()

	// Fill the tracker up to its limit with transactions of one sender.
	user := newAccount(t, 1)
	for i := range maxTrackedTransactions {
		tracker.txs[common.BigToHash(big.NewInt(int64(i)))] = &transaction{app: "app"}
	}
	tracker.OnTransactionSubmitted(source("app", 0), user.transaction(t, 0), epoch, nil)

	require.Equal(Counts{}, tracker.Counts("app"))
	require.Equal(1, tracker.untracked)
}

func TestSampleSet_ThinsOutSamplesButKeepsSpanningTheRun(t *testing.T) {
	require := require.New(t)
	set := &sampleSet{}

	const offered = 10 * maxSamplesPerSet
	for i := range offered {
		set.add(epoch.Add(time.Duration(i)*time.Millisecond), time.Duration(i))
	}

	require.LessOrEqual(len(set.samples), maxSamplesPerSet)
	require.GreaterOrEqual(len(set.samples), maxSamplesPerSet/2,
		"thinning out should not empty the set")

	samples := set.sorted()
	first, last := samples[0], samples[len(samples)-1]
	require.Equal(epoch, first.At)
	require.Greater(
		last.At, epoch.Add(time.Duration(offered/2)*time.Millisecond),
		"the retained samples must cover the end of the run, not only its start",
	)
}

func TestSampleSet_SortsSamplesBySubmission(t *testing.T) {
	set := &sampleSet{}
	set.add(epoch.Add(2*time.Second), time.Second)
	set.add(epoch, 2*time.Second)
	set.add(epoch.Add(time.Second), 3*time.Second)

	require.Equal(t, []Sample{
		{At: epoch, Duration: 2 * time.Second},
		{At: epoch.Add(time.Second), Duration: 3 * time.Second},
		{At: epoch.Add(2 * time.Second), Duration: time.Second},
	}, set.sorted())
}

func source(app string, user int) driver.TransactionSource {
	return driver.TransactionSource{App: app, User: user}
}

// account is a signer of test transactions.
type account struct {
	key *ecdsa.PrivateKey
}

func newAccount(t *testing.T, seed int64) account {
	t.Helper()
	key, err := crypto.ToECDSA(common.BigToHash(big.NewInt(seed)).Bytes())
	require.NoError(t, err)
	return account{key: key}
}

// step creates a step of a bundle sending a transaction with the given nonce to
// the address starting with to.
func (a account) step(nonce uint64, to byte) bundle.BuilderStep {
	return bundle.Step(a.key, &types.DynamicFeeTx{
		ChainID: chainID, Nonce: nonce, GasFeeCap: big.NewInt(1e12), Gas: 50_000, To: &common.Address{to},
	})
}

// transaction creates a signed transaction with the given nonce, matching the
// shape the load generators produce.
func (a account) transaction(t *testing.T, nonce uint64) *types.Transaction {
	t.Helper()
	tx, err := types.SignTx(
		types.NewTx(&types.DynamicFeeTx{
			ChainID:   chainID,
			Nonce:     nonce,
			GasFeeCap: big.NewInt(1e12),
			GasTipCap: big.NewInt(0),
			Gas:       21_000,
			To:        &common.Address{0x42},
		}),
		types.NewLondonSigner(chainID),
		a.key,
	)
	require.NoError(t, err)
	return tx
}

func TestTracker_BlockCompositionCountsTheTransactionsAndGasOfEachApplication(t *testing.T) {
	require := require.New(t)
	tracker := NewTracker()

	fast := newAccount(t, 1).transaction(t, 0)
	ordinary := newAccount(t, 2).transaction(t, 0)
	tracker.OnTransactionSubmitted(source("fast", 0), fast, epoch, nil)
	tracker.OnTransactionSubmitted(source("ordinary", 0), ordinary, epoch, nil)

	// The block also carries a transaction of no application, as the ones Norma
	// sends to set a scenario up are.
	setup := common.Hash{0xff}
	tracker.MarkBlock(7, epoch.Add(time.Second), []IncludedTransaction{
		{Hash: fast.Hash(), GasUsed: 21_000},
		{Hash: ordinary.Hash(), GasUsed: 26_000},
		{Hash: setup, GasUsed: 1_000_000},
	})

	require.Equal(
		[]BlockContribution{{Block: 7, Transactions: 1, Gas: 21_000}},
		tracker.BlockContributions("fast"),
	)
	require.Equal(
		[]BlockContribution{{Block: 7, Transactions: 1, Gas: 26_000}},
		tracker.BlockContributions("ordinary"),
	)
	require.Equal(
		[]BlockContribution{{Block: 7, Transactions: 1, Gas: 1_000_000}},
		tracker.BlockContributions(OtherTransactions),
	)
	require.Equal([]string{OtherTransactions, "fast", "ordinary"}, tracker.Contributors())

	// The transactions were included, which is what the block reported.
	require.Equal(Counts{Included: 1}, tracker.Counts("fast"))
	require.Equal(Counts{Included: 1}, tracker.Counts("ordinary"))
}

func TestTracker_BundleIsIncludedOnceWithTheTransactionsItDelivers(t *testing.T) {
	require := require.New(t)
	tracker := NewTracker()
	approver, spender := newAccount(t, 1), newAccount(t, 2)

	allOfEnvelope, allOfBundle, _ := bundle.NewBuilder().WithSigner(signer).
		AllOf(approver.step(0, 0x42), spender.step(0, 0x42)).
		BuildEnvelopeBundleAndPlan()
	oneOfEnvelope, oneOfBundle, _ := bundle.NewBuilder().WithSigner(signer).
		OneOf(approver.step(1, 0x42), spender.step(1, 0x42)).
		BuildEnvelopeBundleAndPlan()
	tracker.OnTransactionSubmitted(source("all-of", 0), allOfEnvelope, epoch, nil)
	tracker.OnTransactionSubmitted(source("one-of", 0), oneOfEnvelope, epoch, nil)
	require.Equal(Counts{Pending: 1}, tracker.Counts("all-of"))

	// Validators carry the envelope in their events, blocks carry what it
	// delivers: all transactions of an all-of bundle, one of a one-of bundle.
	tracker.MarkEmitted(allOfEnvelope.Hash(), epoch.Add(300*time.Millisecond))
	require.Equal(Counts{Emitted: 1}, tracker.Counts("all-of"))
	var included []IncludedTransaction
	for _, tx := range allOfBundle.GetTransactionsInReferencedOrder() {
		included = append(included, IncludedTransaction{Hash: tx.Hash(), GasUsed: 30_000})
	}
	oneOfExecuted := oneOfBundle.GetTransactionsInReferencedOrder()[0]
	included = append(included, IncludedTransaction{Hash: oneOfExecuted.Hash(), GasUsed: 30_000})
	tracker.MarkBlock(7, epoch.Add(time.Second), included)

	require.Equal(Counts{Included: 1}, tracker.Counts("all-of"))
	require.Equal(Counts{Included: 1}, tracker.Counts("one-of"))
	require.Equal(
		[]Sample{{At: epoch, Duration: time.Second}},
		tracker.Samples("all-of", TimeToInclude),
	)
	require.Equal(
		[]BlockContribution{{Block: 7, Transactions: 2, Gas: 60_000}},
		tracker.BlockContributions("all-of"),
	)
	require.Equal(
		[]BlockContribution{{Block: 7, Transactions: 1, Gas: 30_000}},
		tracker.BlockContributions("one-of"),
	)

	// Nothing of either bundle is retained, including the one-of alternative
	// that was not executed.
	require.Empty(tracker.txs)
	require.Empty(tracker.bundled)
	require.Empty(tracker.senders)
}

func TestTracker_ABundleSubmittedTwiceIsFollowedFromItsFirstSubmission(t *testing.T) {
	require := require.New(t)
	tracker := NewTracker()
	user := newAccount(t, 1)

	// Each envelope is signed with a key of its own, but both deliver the same
	// transactions, of which the network executes one.
	first, firstBundle, _ := bundle.NewBuilder().WithSigner(signer).
		AllOf(user.step(0, 0x42)).
		BuildEnvelopeBundleAndPlan()
	second, secondBundle, _ := bundle.NewBuilder().WithSigner(signer).
		AllOf(user.step(0, 0x42)).
		BuildEnvelopeBundleAndPlan()
	require.NotEqual(first.Hash(), second.Hash())
	delivered := firstBundle.GetTransactionsInReferencedOrder()[0]
	require.Equal(delivered.Hash(), secondBundle.GetTransactionsInReferencedOrder()[0].Hash())

	tracker.OnTransactionSubmitted(source("app", 0), first, epoch, nil)
	tracker.OnTransactionSubmitted(source("app", 0), second, epoch.Add(time.Second), nil)
	require.Equal(Counts{Pending: 1}, tracker.Counts("app"))

	tracker.MarkBlock(7, epoch.Add(2*time.Second), []IncludedTransaction{{Hash: delivered.Hash(), GasUsed: 30_000}})
	require.Equal(Counts{Included: 1}, tracker.Counts("app"))
	require.Equal(
		[]Sample{{At: epoch, Duration: 2 * time.Second}},
		tracker.Samples("app", TimeToInclude),
	)
	require.Empty(tracker.txs)
	require.Empty(tracker.bundled)
}

func TestTracker_BundlesSharingNoncesAreFollowedApart(t *testing.T) {
	require := require.New(t)
	tracker := NewTracker()
	user := newAccount(t, 1)

	// Built from the same nonce before it advanced, as a one-of bundle user
	// does; the transactions of a bundle carry the hash of its plan, so the two
	// share none.
	failing, succeeding := byte(0x01), byte(0x02)
	executed, executedBundle, _ := bundle.NewBuilder().WithSigner(signer).
		OneOf(user.step(0, failing), user.step(1, succeeding)).
		BuildEnvelopeBundleAndPlan()
	other, _, _ := bundle.NewBuilder().WithSigner(signer).
		OneOf(user.step(0, succeeding), user.step(0, failing)).
		BuildEnvelopeBundleAndPlan()
	tracker.OnTransactionSubmitted(source("app", 0), executed, epoch, nil)
	tracker.OnTransactionSubmitted(source("app", 0), other, epoch, nil)
	require.Equal(Counts{Pending: 2}, tracker.Counts("app"))

	var included []IncludedTransaction
	for _, tx := range executedBundle.GetTransactionsInReferencedOrder() {
		included = append(included, IncludedTransaction{Hash: tx.Hash(), GasUsed: 30_000})
	}
	tracker.MarkBlock(7, epoch.Add(time.Second), included)

	require.Equal(Counts{Pending: 1, Included: 1}, tracker.Counts("app"))
	require.Contains(tracker.txs, other.Hash())
	require.Len(tracker.bundled, 2, "the transactions of the bundle still in flight")
}

func TestTracker_BlockCompositionIsOrderedByBlockHeight(t *testing.T) {
	require := require.New(t)
	tracker := NewTracker()
	user := newAccount(t, 1)

	// Blocks are announced by every node of a network, so the tracker may hear
	// about them in an order that is not the order they were produced in.
	for _, height := range []int{9, 7, 8} {
		tx := user.transaction(t, uint64(height))
		tracker.OnTransactionSubmitted(source("app", 0), tx, epoch, nil)
		tracker.MarkBlock(height, epoch.Add(time.Second), []IncludedTransaction{
			{Hash: tx.Hash(), GasUsed: 21_000},
		})
	}

	require.Equal([]BlockContribution{
		{Block: 7, Transactions: 1, Gas: 21_000},
		{Block: 8, Transactions: 1, Gas: 21_000},
		{Block: 9, Transactions: 1, Gas: 21_000},
	}, tracker.BlockContributions("app"))
}

func TestTracker_EmptyBlocksAreNotRecorded(t *testing.T) {
	tracker := NewTracker()
	tracker.MarkBlock(7, epoch, nil)
	require.Empty(t, tracker.Contributors())
}

func TestTracker_BlockCompositionAccumulatesTheGasOfAnApplication(t *testing.T) {
	require := require.New(t)
	tracker := NewTracker()
	user := newAccount(t, 1)

	first, second := user.transaction(t, 0), user.transaction(t, 1)
	tracker.OnTransactionSubmitted(source("app", 0), first, epoch, nil)
	tracker.OnTransactionSubmitted(source("app", 0), second, epoch, nil)
	tracker.MarkBlock(3, epoch.Add(time.Second), []IncludedTransaction{
		{Hash: first.Hash(), GasUsed: 21_000},
		{Hash: second.Hash(), GasUsed: 26_000},
	})

	require.Equal(
		[]BlockContribution{{Block: 3, Transactions: 2, Gas: 47_000}},
		tracker.BlockContributions("app"),
	)
}

func TestTracker_BlockGasLimitsAreOrderedByBlockHeight(t *testing.T) {
	require := require.New(t)
	tracker := NewTracker()

	// As for their composition, the blocks may be heard about out of order. A
	// block that carried nothing has a limit too: it is what the network offered,
	// whether or not anything used it.
	for _, height := range []int{9, 7, 8} {
		tracker.MarkBlockLimits(height, BlockLimit{GasLimit: 5_000_000_000})
	}

	require.Equal([]BlockLimit{
		{Block: 7, GasLimit: 5_000_000_000},
		{Block: 8, GasLimit: 5_000_000_000},
		{Block: 9, GasLimit: 5_000_000_000},
	}, tracker.BlockGasLimits())
}

func TestTracker_BlockGasLimitFollowsARuleChange(t *testing.T) {
	require := require.New(t)
	tracker := NewTracker()

	// The limit is a rule of the network, which a scenario may change while it
	// runs, so it is kept per block rather than once.
	tracker.MarkBlockLimits(7, BlockLimit{GasLimit: 5_000_000_000, EventGasLimit: 2_500_000})
	tracker.MarkBlockLimits(8, BlockLimit{GasLimit: 20_500_000_000, EventGasLimit: 100_000_000})

	require.Equal([]BlockLimit{
		{Block: 7, GasLimit: 5_000_000_000, EventGasLimit: 2_500_000},
		{Block: 8, GasLimit: 20_500_000_000, EventGasLimit: 100_000_000},
	}, tracker.BlockGasLimits())
}
