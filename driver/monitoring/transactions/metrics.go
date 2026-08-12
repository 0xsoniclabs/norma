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
	"fmt"
	"time"

	"github.com/0xsoniclabs/norma/driver"
	mon "github.com/0xsoniclabs/norma/driver/monitoring"
	appmon "github.com/0xsoniclabs/norma/driver/monitoring/app"
	"github.com/0xsoniclabs/norma/driver/monitoring/utils"
)

// The number of transactions of an application in each phase of their life,
// sampled once per second. The first three are disjoint and add up to the
// transactions of the application currently in the system.
var (
	TransactionsPending = mon.Metric[mon.App, mon.Series[mon.Time, int]]{
		Name:        "TransactionsPending",
		Description: "The number of submitted transactions the network could emit right away, waiting for capacity",
	}

	TransactionsStalled = mon.Metric[mon.App, mon.Series[mon.Time, int]]{
		Name:        "TransactionsStalled",
		Description: "The number of submitted transactions stalling in a pool behind a missing nonce of their own sender",
	}

	TransactionsEmitted = mon.Metric[mon.App, mon.Series[mon.Time, int]]{
		Name:        "TransactionsEmitted",
		Description: "The number of transactions carried by an event of the DAG but not yet by a block",
	}

	TransactionsIncluded = mon.Metric[mon.App, mon.Series[mon.Time, int]]{
		Name:        "TransactionsIncluded",
		Description: "The total number of transactions that became part of a block",
	}

	TransactionsRejected = mon.Metric[mon.App, mon.Series[mon.Time, int]]{
		Name:        "TransactionsRejected",
		Description: "The total number of transactions a node refused to accept",
	}
)

// How the applications shared each block, counted in transactions and in the gas
// they used. Per block the values add up to that block's own totals.
var (
	BlockTransactionsPerApp = mon.Metric[mon.App, mon.Series[mon.BlockNumber, int]]{
		Name:        "BlockTransactionsPerApp",
		Description: "The number of transactions of an application in a block, the transactions of none of them under " + OtherTransactions,
	}

	BlockGasPerApp = mon.Metric[mon.App, mon.Series[mon.BlockNumber, int]]{
		Name:        "BlockGasPerApp",
		Description: "The gas used in a block by the transactions of an application, the transactions of none of them under " + OtherTransactions,
	}
)

// The gas ceilings that applied to a block, recorded per block so they can be
// read next to the gas the block used. BlockGasLimit is the technical hard limit
// of the block itself, which a network is not normally expected to reach; what a
// block actually carries is governed by the other two.
var (
	BlockGasLimit = mon.Metric[mon.Network, mon.Series[mon.BlockNumber, int]]{
		Name:        "BlockGasLimit",
		Description: "The gas limit a block was formed under, as reported by its header",
	}

	EventGasLimit = mon.Metric[mon.Network, mon.Series[mon.BlockNumber, int]]{
		Name:        "EventGasLimit",
		Description: "The gas one event carrying the transactions of a block may hold, the MaxEventGas rule",
	}

	GasPowerAllocPerSec = mon.Metric[mon.Network, mon.Series[mon.BlockNumber, int]]{
		Name:        "GasPowerAllocPerSec",
		Description: "The gas per second the validators of the network may spend together, the ShortGasPower.AllocPerSec rule",
	}
)

// The durations transactions needed to pass through the phases of their life.
// Each data point is one transaction, positioned at the moment it was submitted;
// there are far too many of them to read individually, they describe a
// distribution to be aggregated.
var (
	TransactionTimeToEmit = mon.Metric[mon.App, mon.Series[mon.Time, time.Duration]]{
		Name:        "TransactionTimeToEmit",
		Description: "The time transactions spent between their submission and being emitted in an event",
	}

	TransactionTimeToInclude = mon.Metric[mon.App, mon.Series[mon.Time, time.Duration]]{
		Name:        "TransactionTimeToInclude",
		Description: "The time transactions spent between their submission and becoming part of a block",
	}

	TransactionTimeEmitToInclude = mon.Metric[mon.App, mon.Series[mon.Time, time.Duration]]{
		Name:        "TransactionTimeEmitToInclude",
		Description: "The time emitted transactions needed to reach a block",
	}
)

func init() {
	counts := []struct {
		metric mon.Metric[mon.App, mon.Series[mon.Time, int]]
		kind   CountKind
	}{
		{TransactionsPending, Pending},
		{TransactionsStalled, Stalled},
		{TransactionsEmitted, Emitted},
		{TransactionsIncluded, Included},
		{TransactionsRejected, Rejected},
	}
	for _, count := range counts {
		kind := count.kind
		if err := mon.RegisterSource(count.metric, func(monitor *mon.Monitor) mon.Source[mon.App, mon.Series[mon.Time, int]] {
			return newCountSource(count.metric, kind, monitor)
		}); err != nil {
			panic(fmt.Sprintf("failed to register metric source: %v", err))
		}
	}

	compositions := []struct {
		metric mon.Metric[mon.App, mon.Series[mon.BlockNumber, int]]
		value  func(BlockContribution) int
	}{
		{BlockTransactionsPerApp, func(c BlockContribution) int { return c.Transactions }},
		{BlockGasPerApp, func(c BlockContribution) int { return int(c.Gas) }},
	}
	for _, composition := range compositions {
		value := composition.value
		if err := mon.RegisterSource(composition.metric, func(monitor *mon.Monitor) mon.Source[mon.App, mon.Series[mon.BlockNumber, int]] {
			return newCompositionSource(composition.metric, value, monitor)
		}); err != nil {
			panic(fmt.Sprintf("failed to register metric source: %v", err))
		}
	}

	limits := []struct {
		metric mon.Metric[mon.Network, mon.Series[mon.BlockNumber, int]]
		value  func(BlockLimit) int
	}{
		{BlockGasLimit, func(l BlockLimit) int { return int(l.GasLimit) }},
		{EventGasLimit, func(l BlockLimit) int { return int(l.EventGasLimit) }},
		{GasPowerAllocPerSec, func(l BlockLimit) int { return int(l.GasPowerPerSec) }},
	}
	for _, limit := range limits {
		value := limit.value
		if err := mon.RegisterSource(limit.metric, func(monitor *mon.Monitor) mon.Source[mon.Network, mon.Series[mon.BlockNumber, int]] {
			return newGasLimitSource(limit.metric, value, monitor)
		}); err != nil {
			panic(fmt.Sprintf("failed to register metric source: %v", err))
		}
	}

	durations := []struct {
		metric mon.Metric[mon.App, mon.Series[mon.Time, time.Duration]]
		kind   SampleKind
	}{
		{TransactionTimeToEmit, TimeToEmit},
		{TransactionTimeToInclude, TimeToInclude},
		{TransactionTimeEmitToInclude, TimeEmitToInclude},
	}
	for _, duration := range durations {
		kind := duration.kind
		if err := mon.RegisterSource(duration.metric, func(monitor *mon.Monitor) mon.Source[mon.App, mon.Series[mon.Time, time.Duration]] {
			return newSampleSource(duration.metric, kind, monitor)
		}); err != nil {
			panic(fmt.Sprintf("failed to register metric source: %v", err))
		}
	}
}

// newCountSource creates a source sampling one of the tracker's counters once
// per second, for every application of the network.
func newCountSource(
	metric mon.Metric[mon.App, mon.Series[mon.Time, int]],
	kind CountKind,
	monitor *mon.Monitor,
) mon.Source[mon.App, mon.Series[mon.Time, int]] {
	return &countSource{
		Source:  appmon.NewPeriodicAppDataSource(metric, monitor, countSensorFactory{tracker(monitor), kind}),
		monitor: monitor,
	}
}

// countSource is a periodic per-application source that releases the tracker
// shared by all transaction metrics when it is shut down.
type countSource struct {
	mon.Source[mon.App, mon.Series[mon.Time, int]]
	monitor *mon.Monitor
}

func (s *countSource) Shutdown() error {
	stopTracker(s.monitor)
	return s.Source.Shutdown()
}

type countSensorFactory struct {
	tracker *Tracker
	kind    CountKind
}

func (f countSensorFactory) CreateSensor(app driver.Application) (utils.Sensor[int], error) {
	return countSensor{tracker: f.tracker, app: app.Config().Name, kind: f.kind}, nil
}

type countSensor struct {
	tracker *Tracker
	app     string
	kind    CountKind
}

func (s countSensor) ReadValue() (int, error) {
	return s.tracker.Counts(s.app).Get(s.kind), nil
}
