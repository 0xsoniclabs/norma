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
	"math/rand"
	"slices"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0xsoniclabs/norma/driver/monitoring"
)

// This file provides an in-memory stand-in for a running network, so that
// checkers observing the network over a window can be unit tested without
// starting any container.
//
// Every simulation runs inside a testing/synctest bubble. The bubble's clock is
// fake and only advances once every goroutine in it is blocked, so a checker
// that waits out a ten second observation window returns instantly, having
// executed exactly the production code path - time.Now and the sleep helper -
// that it runs against a live network.
//
// simulatedNetwork is a MonitoringData implementation with a producer goroutine
// that appends samples to its series as the bubble's clock advances, exactly as
// the real monitor's series grow while a checker waits.
//
// The sampling parameters are randomized from a seed. Running a check over many
// seeds covers the sample alignments, phase offsets and missing samples that a
// hand-written fixed series cannot reproduce, and that are the usual cause of
// flakiness against a live network.

// simulationEpoch is the virtual instant every simulation starts at. A
// synctest bubble always starts its clock here, so no test depends on the wall
// clock and expectations can be written against absolute instants.
var simulationEpoch = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

// production models the block production of one node: one block every
// blockInterval while producing, and a constant height while halted.
type production struct {
	// first is the height at the start of the simulation.
	first uint64
	// blockInterval is the time between two blocks while producing.
	blockInterval time.Duration
	// haltAt is the instant production stops. Zero means it never stops.
	haltAt time.Time
	// resumeAt is the instant production restarts; it must be after haltAt.
	// Zero means it never restarts.
	resumeAt time.Time
}

// heightAt returns the block height the node reports at instant t.
func (p production) heightAt(t time.Time) uint64 {
	if !t.After(simulationEpoch) {
		return p.first
	}
	return p.first + uint64(p.producingFor(t)/p.blockInterval)
}

// producingFor returns how much of the interval between the start of the
// simulation and t the node spent producing blocks.
func (p production) producingFor(t time.Time) time.Duration {
	if p.haltAt.IsZero() || !p.haltAt.Before(t) {
		return t.Sub(simulationEpoch)
	}
	producing := p.haltAt.Sub(simulationEpoch)
	if !p.resumeAt.IsZero() && t.After(p.resumeAt) {
		producing += t.Sub(p.resumeAt)
	}
	return producing
}

// nodeBehavior describes one simulated node: the height it reports over time
// and how faithfully the monitor manages to sample it.
type nodeBehavior struct {
	// height reports the block height the node returns at instant t.
	height func(t time.Time) uint64
	// interval is the nominal spacing between monitoring samples.
	interval time.Duration
	// phase offsets the first sample within the first interval, mirroring the
	// random offset the real monitor applies per node.
	phase time.Duration
	// jitter bounds the deviation of each individual sample instant. It is
	// capped at half an interval so samples stay ordered.
	jitter time.Duration
	// dropRate is the probability that a read fails and appends no sample at
	// all, as the real monitor does on an RPC error.
	dropRate float64
	// unreachableFrom is the instant from which every read fails, modelling a
	// node that was stopped. Zero means the node stays reachable.
	unreachableFrom time.Time
}

// simulatedNode holds the generated series of one node.
type simulatedNode struct {
	behavior nodeBehavior
	series   *monitoring.SyncedSeries[monitoring.Time, monitoring.BlockStatus]
	// next is the virtual instant of the next scheduled read, jitter included.
	next time.Time
	// index counts scheduled reads, used to place samples deterministically.
	index int
}

// simulatedNetwork is a MonitoringData implementation whose series grow as the
// bubble's clock advances. It stands in for a live network in unit tests.
type simulatedNetwork struct {
	mu  sync.Mutex
	rnd *rand.Rand

	order []monitoring.Node
	nodes map[monitoring.Node]*simulatedNode

	gasRates  *monitoring.SyncedSeries[monitoring.BlockNumber, float64]
	gasRateOf func(block uint64) float64
	// lastGasBlock is the highest block already recorded in gasRates.
	lastGasBlock int64

	// added wakes the producer when a node joins after it went to sleep.
	added chan struct{}
}

// newSimulation creates an empty simulation and starts its sample producer.
// All randomness is drawn from seed, so a failing seed reproduces exactly.
func newSimulation(t *testing.T, seed int64) *simulatedNetwork {
	t.Helper()

	// A simulation places its samples at absolute instants counted from
	// simulationEpoch, so it has to start when the bubble's clock is still
	// there. Two simulations cannot share one bubble.
	if now := time.Now(); !now.Equal(simulationEpoch) {
		t.Fatalf(
			"a simulation must start at %v in a fresh synctest bubble, "+
				"but the clock reads %v", simulationEpoch, now,
		)
	}

	net := &simulatedNetwork{
		rnd:          rand.New(rand.NewSource(seed)),
		nodes:        make(map[monitoring.Node]*simulatedNode),
		gasRates:     &monitoring.SyncedSeries[monitoring.BlockNumber, float64]{},
		gasRateOf:    func(uint64) float64 { return 0 },
		lastGasBlock: -1,
		added:        make(chan struct{}, 1),
	}

	// The producer has to be stopped before the bubble ends, or synctest would
	// wait for it forever. Cleanup functions run inside the bubble.
	done := make(chan struct{})
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		net.produce(done)
	}()
	t.Cleanup(func() {
		close(done)
		<-stopped
	})

	return net
}

// addNode registers a node with the given behavior.
func (n *simulatedNetwork) addNode(label monitoring.Node, behavior nodeBehavior) {
	n.mu.Lock()

	if behavior.interval <= 0 {
		behavior.interval = time.Second
	}
	// Cap the jitter at just under half an interval: that keeps consecutive
	// sample instants strictly increasing, which the series requires.
	if limit := behavior.interval/2 - time.Nanosecond; behavior.jitter > limit {
		behavior.jitter = limit
	}

	n.order = append(n.order, label)
	node := &simulatedNode{
		behavior: behavior,
		series:   &monitoring.SyncedSeries[monitoring.Time, monitoring.BlockStatus]{},
	}
	n.nodes[label] = node
	n.schedule(node)
	n.mu.Unlock()

	// The producer sleeps until the earliest scheduled read, so it has to
	// reconsider now that there is one more.
	select {
	case n.added <- struct{}{}:
	default:
	}
}

// randomSampling returns a nodeBehavior for the given height function with
// randomized sampling: a random phase within the first interval, jitter up to
// half an interval, and up to 30% of reads dropped.
func (n *simulatedNetwork) randomSampling(height func(time.Time) uint64) nodeBehavior {
	n.mu.Lock()
	defer n.mu.Unlock()

	interval := time.Second
	return nodeBehavior{
		height:   height,
		interval: interval,
		phase:    time.Duration(n.rnd.Int63n(int64(interval))),
		jitter:   time.Duration(n.rnd.Int63n(int64(interval / 2))),
		dropRate: n.rnd.Float64() * 0.3,
	}
}

// withGasRates makes the simulation record a network-wide gas rate for every
// block produced, as the real monitor does from the node logs.
func (n *simulatedNetwork) withGasRates(rateOf func(block uint64) float64) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.gasRateOf = rateOf
}

// run lets the simulation advance by d without any checker involved, to build
// up the history that precedes a check. It returns once every sample due by
// then has been appended.
func (n *simulatedNetwork) run(d time.Duration) {
	time.Sleep(d)
	n.settle()
}

// settle waits until the producer has appended every sample due at the current
// instant. Without it a sample scheduled for exactly now may still be pending,
// because synctest only orders wake-ups that are strictly apart in time.
func (n *simulatedNetwork) settle() {
	synctest.Wait()
}

// produce appends samples as the bubble's clock reaches their instants, until
// done is closed. It is the only goroutine drawing from n.rnd, which keeps a
// seed reproducible even though it runs concurrently with the checker.
func (n *simulatedNetwork) produce(done <-chan struct{}) {
	for {
		next, scheduled := n.nextRead()
		if !scheduled {
			// No node to sample yet; wait for one to be added.
			select {
			case <-done:
				return
			case <-n.added:
				continue
			}
		}

		if wait := time.Until(next); wait > 0 {
			timer := time.NewTimer(wait)
			select {
			case <-done:
				timer.Stop()
				return
			case <-n.added:
				timer.Stop()
				continue
			case <-timer.C:
			}
		}
		n.collect()
	}
}

// nextRead returns the earliest instant any node is scheduled to be read at.
func (n *simulatedNetwork) nextRead() (time.Time, bool) {
	n.mu.Lock()
	defer n.mu.Unlock()

	var earliest time.Time
	for _, label := range n.order {
		if next := n.nodes[label].next; earliest.IsZero() || next.Before(earliest) {
			earliest = next
		}
	}
	return earliest, !earliest.IsZero()
}

// collect appends the samples the monitor would have taken by now.
func (n *simulatedNetwork) collect() {
	n.mu.Lock()
	defer n.mu.Unlock()

	now := time.Now()
	for _, label := range n.order {
		node := n.nodes[label]
		for !node.next.After(now) {
			n.sample(node)
			n.schedule(node)
		}
	}
	n.recordGasRates(now)
}

// schedule places the next read of the given node, jitter included. Sample
// instants are counted from the epoch rather than from the previous sample, so
// a node's cadence does not drift.
func (n *simulatedNetwork) schedule(node *simulatedNode) {
	behavior := node.behavior

	next := simulationEpoch.Add(
		behavior.phase + time.Duration(node.index)*behavior.interval,
	)
	if behavior.jitter > 0 {
		next = next.Add(time.Duration(
			n.rnd.Int63n(int64(2*behavior.jitter)) - int64(behavior.jitter),
		))
	}
	// The jitter cap keeps instants apart, but the first one can land before
	// the epoch; the series requires strictly increasing positions.
	if floor := node.lastRead(); !next.After(floor) {
		next = floor.Add(time.Nanosecond)
	}

	node.next = next
	node.index++
}

// lastRead returns the instant the node was last scheduled to be read at, or
// the epoch for a node that has not been read yet.
func (s *simulatedNode) lastRead() time.Time {
	if s.index == 0 {
		return simulationEpoch.Add(-time.Nanosecond)
	}
	return s.next
}

// sample records one monitoring read of the given node, or nothing when the
// read fails.
func (n *simulatedNetwork) sample(node *simulatedNode) {
	behavior := node.behavior
	at := node.next

	if !behavior.unreachableFrom.IsZero() &&
		!at.Before(behavior.unreachableFrom) {
		return // the node is gone; the read fails and appends nothing
	}
	if behavior.dropRate > 0 && n.rnd.Float64() < behavior.dropRate {
		return // a failed read leaves a hole in the series
	}

	status := monitoring.BlockStatus{BlockHeight: behavior.height(at)}
	if err := node.series.Append(monitoring.NewTime(at), status); err != nil {
		// Out-of-order appends mean the harness generated an invalid
		// schedule; the jitter cap is supposed to prevent that.
		panic("simulated sample out of order: " + err.Error())
	}
}

// recordGasRates appends a gas rate for every block that became visible on any
// node up to instant t.
func (n *simulatedNetwork) recordGasRates(t time.Time) {
	highest := int64(-1)
	for _, node := range n.nodes {
		if height := int64(node.behavior.height(t)); height > highest {
			highest = height
		}
	}
	for block := n.lastGasBlock + 1; block <= highest; block++ {
		if err := n.gasRates.Append(
			monitoring.BlockNumber(block), n.gasRateOf(uint64(block)),
		); err != nil {
			panic("simulated gas rate out of order: " + err.Error())
		}
		n.lastGasBlock = block
	}
}

func (n *simulatedNetwork) GetNodes() []monitoring.Node {
	n.mu.Lock()
	defer n.mu.Unlock()
	return slices.Clone(n.order)
}

func (n *simulatedNetwork) GetBlockStatus(
	node monitoring.Node,
) monitoring.Series[monitoring.Time, monitoring.BlockStatus] {
	n.mu.Lock()
	defer n.mu.Unlock()
	if simulated, found := n.nodes[node]; found {
		return simulated.series
	}
	return &monitoring.SyncedSeries[monitoring.Time, monitoring.BlockStatus]{}
}

func (n *simulatedNetwork) GetBlockGasRate() monitoring.Series[monitoring.BlockNumber, float64] {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.gasRates
}

// heightsIn returns the block heights sampled for a node within the given
// half-open interval, for assertions about what a check could observe. It
// settles the simulation first, so the result does not depend on whether the
// producer got to run before the reading goroutine.
func (n *simulatedNetwork) heightsIn(
	node monitoring.Node, from, to time.Time,
) []uint64 {
	n.settle()

	points := n.GetBlockStatus(node).GetRange(
		monitoring.NewTime(from), monitoring.NewTime(to),
	)
	heights := make([]uint64, 0, len(points))
	for _, point := range points {
		heights = append(heights, point.Value.BlockHeight)
	}
	return heights
}

// seeds returns the seeds used by the randomized checker tests.
func seeds() []int64 {
	all := make([]int64, 0, 64)
	for seed := int64(1); seed <= 64; seed++ {
		all = append(all, seed)
	}
	return all
}

// eachSeed runs body once per seed, each in its own synctest bubble so that
// every seed starts again from simulationEpoch with a fresh clock.
func eachSeed(t *testing.T, body func(t *testing.T, seed int64)) {
	t.Helper()
	for _, seed := range seeds() {
		synctest.Test(t, func(t *testing.T) { body(t, seed) })
	}
}
