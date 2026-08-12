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

package checking

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"testing/synctest"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

const txGas = 30_000

func TestPriorityOrdering_HoistedBlocksPass(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		source := blocksOf(20, hoisted(10, 10))
		require.NoError(t, checkerFor(source).Check(t.Context()))
	})
}

func TestPriorityOrdering_ScrambledBlocksFail(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// The order a network without priorities produces: the two classes are mixed
		// through the block, so the opening run covers a small share of them.
		source := blocksOf(20, alternating(10, 10))
		err := checkerFor(source).Check(t.Context())
		require.ErrorContains(t, err, "does not look prioritized")
	})
}

func TestPriorityOrdering_ABlockOpeningWithOrdinaryTransactionsFails(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		block := append(ordinary(10), prioritized(10)...)
		err := checkerFor(blocksOf(20, block)).Check(t.Context())
		require.ErrorContains(t, err, "does not look prioritized")
	})
}

func TestPriorityOrdering_RequiresEnoughBlocksCarryingBoth(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Blocks of one class say nothing about the order of the other, so a run
		// where the prioritized load never met ordinary traffic must not pass.
		err := checkerFor(blocksOf(20, prioritized(10))).Check(t.Context())
		require.ErrorContains(t, err, "carried both")
		require.ErrorContains(t, err, "would pass on a network without priorities")
	})
}

func TestPriorityOrdering_JudgesOnlyBlocksProducedAfterItStarted(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// A chain of well ordered blocks that stops producing when the check starts.
		// The check observes forward in time, so it has nothing to judge and must
		// say so rather than pass on the blocks of an earlier step.
		source := blocksOf(0, nil)
		source.existing, source.past = 50, hoisted(10, 10)

		err := checkerFor(source).Check(t.Context())
		require.ErrorContains(t, err, "carried both")
	})
}

func TestPriorityOrdering_WaitsForTheBlocksItNeeds(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// The blocks arrive one every two seconds, well beyond the interval the
		// check polls at, and it collects them rather than giving up.
		source := blocksOf(20, hoisted(10, 10))
		source.blockTime = 2 * time.Second
		require.NoError(t, checkerFor(source).Check(t.Context()))
	})
}

func TestPriorityOrdering_GivesUpAfterItsBudget(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// A network so slow that the check cannot see its evidence: it ends after
		// its budget rather than holding the scenario.
		source := blocksOf(20, hoisted(10, 10))
		source.blockTime = time.Minute

		checker := checkerFor(source).Configure(CheckerConfig{"duration": int64(10 * time.Second)})
		start := time.Now()
		require.ErrorContains(t, checker.Check(t.Context()), "in 10s")
		require.WithinDuration(t, start.Add(10*time.Second), time.Now(), time.Second)
	})
}

func TestPriorityOrdering_HoistingBeyondTheQuotaFails(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// 10 transactions of 30k gas against a quota of 200k: the client may hoist
		// six of them, so a block whose opening run carries all ten is over budget.
		source := blocksOf(20, hoisted(10, 10))
		source.quota = 200_000
		err := checkerFor(source).Check(t.Context())
		require.ErrorContains(t, err, "beyond the gas one entity may have per block")
	})
}

func TestPriorityOrdering_TheQuotaIsCountedPerEntity(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Two entities hoisting 60k each exceed a quota of 100k together but not
		// individually, which is how the client grants it.
		block := append(prioritizedOf(1, 2, 30_000), prioritizedOf(2, 2, 30_000)...)
		source := blocksOf(20, append(block, ordinary(10)...))
		source.quota = 100_000
		require.NoError(t, checkerFor(source).Check(t.Context()))

		// The same gas spent by one entity is over its budget.
		source = blocksOf(20, append(prioritizedOf(1, 4, 30_000), ordinary(10)...))
		source.quota = 100_000
		require.ErrorContains(t, checkerFor(source).Check(t.Context()),
			"beyond the gas one entity may have per block")
	})
}

func TestPriorityOrdering_TransactionsTheQuotaDemotesAreNotAMisordering(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// A lane offering more gas than its quota: the client hoists what fits and
		// leaves the rest among the ordinary transactions. Judged against all
		// prioritized transactions of the block the order would look broken, while it
		// is exactly what the quota prescribes.
		block := append(prioritizedOf(1, 2, 50_000_000), ordinary(10)...)
		block = append(block, prioritizedOf(1, 8, 50_000_000)...)

		source := blocksOf(20, block)
		source.quota = 100_000_000
		require.NoError(t, checkerFor(source).Check(t.Context()))
	})
}

func TestPriorityOrdering_TheQuotaIsNotAssertedWhereTransactionsWereDemoted(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// A block that demoted prioritized transactions does not show where its
		// hoisted prefix ended, so the gas of the opening run beyond the quota cannot
		// be told from the demoted remainder and must not be reported as an overrun.
		block := append(prioritizedOf(1, 3, 50_000_000), ordinary(10)...)
		block = append(block, prioritizedOf(1, 5, 50_000_000)...)

		source := blocksOf(20, block)
		source.quota = 100_000_000
		require.NoError(t, checkerFor(source).Check(t.Context()))
	})
}

func TestPriorityOrdering_TheRequiredCoverageCanBeRelaxed(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Three of the ten transactions the quota admits open the block.
		block := append(prioritized(3), ordinary(10)...)
		block = append(block, prioritized(7)...)

		base := checkerFor(blocksOf(20, block))
		require.ErrorContains(t, base.Check(t.Context()), "does not look prioritized")

		relaxed := checkerFor(blocksOf(20, block)).Configure(CheckerConfig{"minRunCoverage": 0.25})
		require.NoError(t, relaxed.Check(t.Context()))
	})
}

func TestPriorityOrdering_PartialHoistingIsAccepted(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Transactions a nonce gap or the quota left behind stay among the ordinary
		// ones; the check judges the median block rather than demanding every
		// prioritized transaction be hoisted.
		block := append(prioritized(7), ordinary(10)...)
		block = append(block, prioritized(3)...)
		require.NoError(t, checkerFor(blocksOf(20, block)).Check(t.Context()))
	})
}

func TestPriorityOrdering_InternalTransactionsDoNotCount(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// The client inserts its own transactions around the ordered ones, so an
		// epoch seal at the front of a block is not an ordinary transaction that
		// overtook the lane.
		block := append([]blockTransaction{{From: common.Address{0xff}, Gas: txGas, Internal: true}}, hoisted(10, 10)...)
		require.NoError(t, checkerFor(blocksOf(20, block)).Check(t.Context()))
	})
}

func TestPriorityOrdering_ConfigureOverridesTheRequiredEvidence(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		base := checkerFor(blocksOf(4, hoisted(10, 10)))
		require.ErrorContains(t, base.Check(t.Context()), "carried both",
			"four blocks are below the default requirement")

		relaxed := checkerFor(blocksOf(4, hoisted(10, 10))).
			Configure(CheckerConfig{"minMixedBlocks": 4})
		require.NoError(t, relaxed.Check(t.Context()))
	})
}

func TestPriorityOrdering_ReportsAFailingSource(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		source := blocksOf(20, hoisted(10, 10))
		source.err = errors.New("no reachable node")
		require.ErrorContains(t, checkerFor(source).Check(t.Context()), "no reachable node")
	})
}

func checkerFor(source *fakePrioritySource) Checker {
	return &priorityOrderingChecker{
		source:         source,
		minMixedBlocks: defaultMinMixedBlocks,
		minRunCoverage: defaultMinRunCoverage,
		timeout:        defaultPriorityTimeout,
	}
}

// prioritized and ordinary build transactions of senders the fake source
// classifies by their address: the first byte carries the priority, the second
// the entity the sender belongs to.
func prioritized(count int) []blockTransaction {
	return prioritizedOf(1, count, txGas)
}

func prioritizedOf(entity byte, count int, gas uint64) []blockTransaction {
	return transactionsOf(count, common.Address{0x01, entity}, gas)
}

func ordinary(count int) []blockTransaction {
	return transactionsOf(count, common.Address{0x02}, txGas)
}

func transactionsOf(count int, sender common.Address, gas uint64) []blockTransaction {
	txs := make([]blockTransaction, 0, count)
	for range count {
		txs = append(txs, blockTransaction{From: sender, Gas: gas})
	}
	return txs
}

// hoisted is a block as the client forms it: the prioritized transactions first.
func hoisted(prio, plain int) []blockTransaction {
	return append(prioritized(prio), ordinary(plain)...)
}

// alternating is a block whose classes are interleaved, as an order that ignores
// priorities produces.
func alternating(prio, plain int) []blockTransaction {
	var txs []blockTransaction
	for i := 0; i < prio || i < plain; i++ {
		if i < prio {
			txs = append(txs, prioritized(1)...)
		}
		if i < plain {
			txs = append(txs, ordinary(1)...)
		}
	}
	return txs
}

// blocksOf is a network that produces the given number of blocks, all of the
// given content, while the check observes it, and then stops.
func blocksOf(count int, block []blockTransaction) *fakePrioritySource {
	return &fakePrioritySource{produced: count, block: block, quota: 100_000_000}
}

// fakePrioritySource is a chain of existing blocks that produces produced more,
// one per blockTime, from the moment its head is first read.
type fakePrioritySource struct {
	existing  int                // blocks on the chain before the check starts
	produced  int                // blocks produced after that
	blockTime time.Duration      // how long each of those takes
	past      []blockTransaction // what the existing blocks carry
	block     []blockTransaction // what the produced blocks carry
	quota     uint64
	err       error

	firstRead time.Time
}

func (s *fakePrioritySource) LatestBlock(context.Context) (int, error) {
	if s.err != nil {
		return 0, s.err
	}
	if s.firstRead.IsZero() {
		s.firstRead = time.Now()
		return s.existing, nil
	}
	produced := s.produced
	if s.blockTime > 0 {
		produced = min(produced, int(time.Since(s.firstRead)/s.blockTime))
	}
	return s.existing + produced, nil
}

func (s *fakePrioritySource) Block(_ context.Context, height int) ([]blockTransaction, error) {
	if height <= s.existing {
		return s.past, s.err
	}
	return s.block, s.err
}

func (s *fakePrioritySource) Prioritized(_ context.Context, sender common.Address) (priorityClass, error) {
	if sender[0] != 0x01 {
		return priorityClass{}, s.err
	}
	return priorityClass{Prioritized: true, Entity: fmt.Sprintf("%d", sender[1])}, s.err
}

func (s *fakePrioritySource) GasQuota(context.Context) (uint64, error) {
	return s.quota, s.err
}
