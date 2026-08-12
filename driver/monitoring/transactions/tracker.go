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

// Package txmon follows the transactions of load generators through the
// network and derives metrics from where they are and how long they took to get
// there.
//
// A transaction becomes observable at three moments, each seen through a
// different window:
//
//   - submitted: it was handed to the RPC interface of a node, which either
//     accepted it into its transaction pool or refused it. The network reports
//     this moment itself, see driver.TransactionObserver.
//   - emitted: a validator put it into an event of the DAG, which is when it
//     starts travelling towards a block. Found by walking the DAG, see
//     emissions.go.
//   - included: it became part of a block and was therefore executed. Read
//     from the transactions of every new block, see inclusions.go.
//
// Between submission and emission a transaction waits in a pool, where it is
// either emittable or parked behind a missing nonce of its own sender. The two
// are counted apart, in the same way a transaction pool separates its pending
// from its queued transactions: a growing number of emittable transactions means
// the network does not keep up with the offered load, while parked ones will not
// be emitted at all until the gap before them is filled - usually because a
// submission was lost. The distinction is derived from the nonces of the
// observed transactions rather than read from the pools, which would mean
// transferring their entire contents on every sample.
//
// Only transactions of load generators are followed. The transactions Norma
// sends to set a scenario up - funding accounts, deploying contracts, changing
// rules - do not pass through the observed submission path.
package txmon

import (
	"cmp"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// CountKind identifies one of the populations of transactions counted per
// application. The first three are disjoint and count the transactions
// currently in the system; the last two accumulate over the run.
type CountKind int

const (
	// Pending counts submitted transactions that could be emitted right away:
	// they form an uninterrupted run of nonces from the one their sender
	// executes next.
	Pending CountKind = iota
	// Stalled counts submitted transactions parked behind a missing nonce of
	// their own sender, which no validator can emit until that gap is filled.
	Stalled
	// Emitted counts transactions carried by an event but not yet by a block.
	Emitted
	// Included counts transactions that made it into a block, over the whole run.
	Included
	// Rejected counts transactions a node refused to accept, over the whole run.
	Rejected
)

// OtherTransactions is the name the composition of a block attributes the
// transactions of no application to. It is not a valid application name, so it
// cannot collide with one.
const OtherTransactions = "(other)"

// SampleKind identifies one of the durations measured per transaction.
type SampleKind int

const (
	// TimeToEmit is the time from the submission of a transaction until a
	// validator put it into an event.
	TimeToEmit SampleKind = iota
	// TimeToInclude is the time from the submission of a transaction until it
	// became part of a block - the latency a user of the network observes.
	TimeToInclude
	// TimeEmitToInclude is the time an emitted transaction needed to reach a
	// block, which is the part of the latency consensus is responsible for.
	TimeEmitToInclude
	numSampleKinds
)

// maxTrackedTransactions bounds the number of transactions in flight the
// tracker retains state for. Records are released on inclusion, so the limit is
// only reached by a backlog of transactions the network never commits; beyond
// it, the counting continues but new transactions are no longer followed
// individually.
const maxTrackedTransactions = 500_000

// Sample is one measured duration of one transaction, positioned at the moment
// the transaction was submitted.
type Sample struct {
	At       time.Time
	Duration time.Duration
}

// IncludedTransaction is a transaction of a block, as read from its receipt.
type IncludedTransaction struct {
	Hash    common.Hash
	GasUsed uint64
}

// BlockContribution is what one application contributed to one block.
type BlockContribution struct {
	Block        int
	Transactions int
	// Gas is the gas the transactions actually used, not the gas they reserved,
	// so the contributions of a block add up to the gas used by that block.
	Gas uint64
}

// BlockLimit are the gas ceilings that applied to one block: what the block
// itself may hold, what one event carrying its transactions may hold, and the gas
// per second the validators were allowed to spend.
type BlockLimit struct {
	Block          int
	GasLimit       uint64 // Blocks.MaxBlockGas, from the block header
	EventGasLimit  uint64 // Economy.Gas.MaxEventGas
	GasPowerPerSec uint64 // Economy.ShortGasPower.AllocPerSec
}

// Counts is a snapshot of the transaction populations of one application.
type Counts struct {
	Pending  int
	Stalled  int
	Emitted  int
	Included int
	Rejected int
}

// Get returns the counter of the given kind.
func (c Counts) Get(kind CountKind) int {
	switch kind {
	case Pending:
		return c.Pending
	case Stalled:
		return c.Stalled
	case Emitted:
		return c.Emitted
	case Included:
		return c.Included
	case Rejected:
		return c.Rejected
	}
	return 0
}

// Tracker maintains the state of the transactions submitted to a network. It is
// safe for concurrent use: submissions, emissions and inclusions are reported
// from independent goroutines.
type Tracker struct {
	mu      sync.Mutex
	txs     map[common.Hash]*transaction
	senders map[common.Address]*sender
	apps    map[string]*application
	// blocks holds, per block, what each application contributed to it - the
	// composition of the block.
	blocks map[int]map[string]*BlockContribution
	// limits holds the gas ceilings that applied to each observed block, which
	// are what those contributions competed for.
	limits map[int]BlockLimit

	// diagnostics, reported when the tracker is stopped
	untracked        int // dropped because maxTrackedTransactions was reached
	unknownSender    int // signature could not be attributed to a sender
	includedNotSeen  int // included before their event was found
	inconsistentTime int // an observed order that time did not agree with
}

// transaction is the state kept for a single transaction in flight. It is
// released once the transaction is included in a block.
type transaction struct {
	app         string
	sender      common.Address
	nonce       uint64
	submittedAt time.Time
	emittedAt   time.Time // zero until the transaction is seen in an event
	blocked     bool      // counted as Stalled rather than Pending
}

// sender is the nonce bookkeeping of one account, which is what decides whether
// a transaction of it can be emitted.
//
// A validator may carry several transactions of one sender in one event, as long
// as they form an uninterrupted run of nonces starting at the one the account
// executes next - which is how a transaction pool separates its executable
// transactions from the ones parked behind a gap. The run is maintained here
// rather than recomputed, so that a submission costs the same whether the sender
// has one or ten thousand transactions waiting.
type sender struct {
	// next is the lowest nonce of this account not known to be included, the one
	// the account executes next.
	next uint64
	// runEnd is the first nonce at or above next that is not in the pool, so
	// every nonce in [next, runEnd) can be emitted and everything above runEnd
	// is stuck behind that gap.
	runEnd uint64
	// inFlight maps the nonces of this sender's tracked transactions to them.
	inFlight map[uint64]common.Hash
}

// executable reports whether a transaction with the given nonce extends the
// emittable run of this sender.
func (s *sender) executable(nonce uint64) bool {
	return nonce <= s.runEnd
}

// extendRun advances the emittable run over the nonces that are in the pool,
// and returns the transactions that stopped being stuck behind a gap.
func (s *sender) extendRun() []common.Hash {
	var unblocked []common.Hash
	for {
		hash, found := s.inFlight[s.runEnd]
		if !found {
			return unblocked
		}
		if s.runEnd > s.next {
			unblocked = append(unblocked, hash)
		}
		s.runEnd++
	}
}

// application accumulates the counters and the samples of one application.
type application struct {
	counts  Counts
	samples [numSampleKinds]*sampleSet
}

// NewTracker creates an empty tracker.
func NewTracker() *Tracker {
	return &Tracker{
		txs:     map[common.Hash]*transaction{},
		senders: map[common.Address]*sender{},
		apps:    map[string]*application{},
		blocks:  map[int]map[string]*BlockContribution{},
		limits:  map[int]BlockLimit{},
	}
}

// OnTransactionSubmitted implements driver.TransactionObserver.
func (t *Tracker) OnTransactionSubmitted(
	source driver.TransactionSource,
	tx *types.Transaction,
	at time.Time,
	err error,
) {
	t.mu.Lock()
	defer t.mu.Unlock()

	app := t.application(source.App)
	if err != nil {
		// The node refused the transaction, so it never reached a pool and will
		// never be seen again.
		app.counts.Rejected++
		return
	}

	if len(t.txs) >= maxTrackedTransactions {
		t.untracked++
		return
	}

	hash := tx.Hash()
	if _, exists := t.txs[hash]; exists {
		// The same transaction was submitted twice; the first submission is the
		// one its latency is measured from.
		return
	}

	from, senderErr := types.Sender(types.LatestSignerForChainID(tx.ChainId()), tx)
	if senderErr != nil {
		t.unknownSender++
	}

	entry := &transaction{
		app:         source.App,
		sender:      from,
		nonce:       tx.Nonce(),
		submittedAt: at,
	}

	account, found := t.senders[from]
	if !found {
		// The lowest nonce seen from an account is assumed to be the one it
		// executes next; the first inclusion corrects this if it was not.
		account = &sender{
			next:     tx.Nonce(),
			runEnd:   tx.Nonce(),
			inFlight: map[uint64]common.Hash{},
		}
		t.senders[from] = account
	}
	account.inFlight[tx.Nonce()] = hash
	entry.blocked = !account.executable(tx.Nonce())

	t.txs[hash] = entry
	if entry.blocked {
		app.counts.Stalled++
	} else {
		app.counts.Pending++
	}

	// This transaction may have filled the gap its successors were waiting
	// behind, which makes all of them emittable at once.
	for _, unblocked := range account.extendRun() {
		t.unblock(unblocked)
	}
}

// unblock reclassifies a transaction that stopped being stuck behind a nonce
// gap. The caller must hold the lock.
func (t *Tracker) unblock(hash common.Hash) {
	entry, found := t.txs[hash]
	if !found || !entry.blocked {
		return
	}
	entry.blocked = false
	if !entry.emittedAt.IsZero() {
		// Already emitted, so it is counted as such rather than as waiting.
		return
	}
	app := t.application(entry.app)
	app.counts.Stalled--
	app.counts.Pending++
}

// MarkEmitted reports that a transaction was seen in an event created at the
// given time. Reports for unknown transactions, and repeated reports for the
// same one, are ignored.
func (t *Tracker) MarkEmitted(hash common.Hash, at time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()

	entry, found := t.txs[hash]
	if !found || !entry.emittedAt.IsZero() {
		return
	}
	entry.emittedAt = at

	app := t.application(entry.app)
	if entry.blocked {
		app.counts.Stalled--
	} else {
		app.counts.Pending--
	}
	app.counts.Emitted++

	// An event may be older than the submission we measured, since the two
	// timestamps come from different clocks; such a sample is dropped rather
	// than reported as a negative duration.
	if delay := at.Sub(entry.submittedAt); delay >= 0 {
		app.sampleSet(TimeToEmit).add(entry.submittedAt, delay)
	} else {
		t.inconsistentTime++
	}
}

// MarkBlock reports the transactions of a block observed at the given time. It
// records which of them belonged to which application, which is the composition
// of that block, and marks them as included.
func (t *Tracker) MarkBlock(height int, at time.Time, txs []IncludedTransaction) {
	t.mu.Lock()
	defer t.mu.Unlock()

	composition := map[string]*BlockContribution{}
	for _, tx := range txs {
		app, tracked := t.markIncluded(tx.Hash, at)
		if !tracked {
			// A transaction of no application: Norma sends those itself, to fund
			// accounts, deploy contracts or change the rules of the network.
			app = OtherTransactions
		}
		contribution, found := composition[app]
		if !found {
			contribution = &BlockContribution{Block: height}
			composition[app] = contribution
		}
		contribution.Transactions++
		contribution.Gas += tx.GasUsed
	}
	if len(composition) > 0 {
		t.blocks[height] = composition
	}
}

// MarkBlockLimits reports the gas ceilings that applied to the given block. They
// are recorded for every observed block, including the ones that carried no
// transaction, so that the limits can be read next to what the blocks used.
func (t *Tracker) MarkBlockLimits(height int, limits BlockLimit) {
	t.mu.Lock()
	defer t.mu.Unlock()
	limits.Block = height
	t.limits[height] = limits
}

// MarkIncluded reports that a transaction became part of a block observed at
// the given time. Reports for unknown transactions are ignored.
func (t *Tracker) MarkIncluded(hash common.Hash, at time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.markIncluded(hash, at)
}

// markIncluded accounts for the inclusion of one transaction and reports which
// application it belonged to, if it was one that is followed. The caller must
// hold the lock.
func (t *Tracker) markIncluded(hash common.Hash, at time.Time) (string, bool) {
	entry, found := t.txs[hash]
	if !found {
		return "", false
	}

	app := t.application(entry.app)
	switch {
	case !entry.emittedAt.IsZero():
		app.counts.Emitted--
	case entry.blocked:
		app.counts.Stalled--
		t.includedNotSeen++
	default:
		app.counts.Pending--
		t.includedNotSeen++
	}
	app.counts.Included++

	if latency := at.Sub(entry.submittedAt); latency >= 0 {
		app.sampleSet(TimeToInclude).add(entry.submittedAt, latency)
	} else {
		t.inconsistentTime++
	}
	if !entry.emittedAt.IsZero() {
		if consensus := at.Sub(entry.emittedAt); consensus >= 0 {
			app.sampleSet(TimeEmitToInclude).add(entry.submittedAt, consensus)
		}
	}

	// The transaction reached its final state; only its sender's nonce
	// bookkeeping outlives it.
	delete(t.txs, hash)
	t.releaseNonce(entry)
	return entry.app, true
}

// releaseNonce advances the sender's next nonce past the included transaction.
// Nothing becomes emittable by this: an uninterrupted run of nonces stays
// uninterrupted when its lowest one leaves. The caller must hold the lock.
func (t *Tracker) releaseNonce(entry *transaction) {
	account, found := t.senders[entry.sender]
	if !found {
		return
	}
	delete(account.inFlight, entry.nonce)
	if entry.nonce < account.next {
		return
	}
	account.next = entry.nonce + 1
	if account.runEnd < account.next {
		account.runEnd = account.next
	}
}

// Counts returns a snapshot of the counters of the given application.
func (t *Tracker) Counts(app string) Counts {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.application(app).counts
}

// Apps returns the names of the applications transactions were seen from.
func (t *Tracker) Apps() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	apps := make([]string, 0, len(t.apps))
	for app := range t.apps {
		apps = append(apps, app)
	}
	return apps
}

// Samples returns the samples of the given kind collected for an application,
// in submission order.
func (t *Tracker) Samples(app string, kind SampleKind) []Sample {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.application(app).sampleSet(kind).sorted()
}

// BlockContributions returns what the given application contributed to each
// block, ordered by block height. Use OtherTransactions for the transactions
// that belong to no application.
func (t *Tracker) BlockContributions(app string) []BlockContribution {
	t.mu.Lock()
	defer t.mu.Unlock()

	contributions := make([]BlockContribution, 0, len(t.blocks))
	for _, composition := range t.blocks {
		if contribution, found := composition[app]; found {
			contributions = append(contributions, *contribution)
		}
	}
	slices.SortFunc(contributions, func(a, b BlockContribution) int {
		return cmp.Compare(a.Block, b.Block)
	})
	return contributions
}

// BlockGasLimits returns the gas ceilings of every observed block, ordered by
// block height.
func (t *Tracker) BlockGasLimits() []BlockLimit {
	t.mu.Lock()
	defer t.mu.Unlock()

	limits := make([]BlockLimit, 0, len(t.limits))
	for _, limit := range t.limits {
		limits = append(limits, limit)
	}
	slices.SortFunc(limits, func(a, b BlockLimit) int {
		return cmp.Compare(a.Block, b.Block)
	})
	return limits
}

// Contributors returns the names of everything that contributed transactions to
// a block, which are the applications plus possibly OtherTransactions.
func (t *Tracker) Contributors() []string {
	t.mu.Lock()
	defer t.mu.Unlock()

	names := map[string]bool{}
	for _, composition := range t.blocks {
		for name := range composition {
			names[name] = true
		}
	}
	contributors := make([]string, 0, len(names))
	for name := range names {
		contributors = append(contributors, name)
	}
	slices.Sort(contributors)
	return contributors
}

// LogSummary reports what the tracker had to leave out, all of which would
// silently distort the metrics derived from it.
func (t *Tracker) LogSummary() {
	t.mu.Lock()
	defer t.mu.Unlock()

	inFlight := len(t.txs)
	if t.untracked > 0 {
		slog.Warn("transactions were not tracked individually, the limit of transactions in flight was reached",
			"transactions", t.untracked, "limit", maxTrackedTransactions)
	}
	if t.unknownSender > 0 {
		slog.Warn("transactions could not be attributed to a sender, they are never counted as stalled",
			"transactions", t.unknownSender)
	}
	if t.includedNotSeen > 0 {
		slog.Info("transactions were included before their event was found, they have no time-to-emit sample",
			"transactions", t.includedNotSeen)
	}
	if t.inconsistentTime > 0 {
		slog.Info("transactions reached a phase before the previous one according to their timestamps, the sample was dropped",
			"transactions", t.inconsistentTime)
	}
	if inFlight > 0 {
		slog.Info("transactions were still in flight when the run ended", "transactions", inFlight)
	}
}

// application returns the state of the given application, creating it on first
// use. The caller must hold the lock.
func (t *Tracker) application(name string) *application {
	app, found := t.apps[name]
	if !found {
		app = &application{}
		t.apps[name] = app
	}
	return app
}

// sampleSet returns the set of the given kind, creating it on first use.
func (a *application) sampleSet(kind SampleKind) *sampleSet {
	if a.samples[kind] == nil {
		a.samples[kind] = &sampleSet{}
	}
	return a.samples[kind]
}
