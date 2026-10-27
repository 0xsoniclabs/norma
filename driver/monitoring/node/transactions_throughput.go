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

package nodemon

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/0xsoniclabs/norma/driver/monitoring"
	"github.com/0xsoniclabs/norma/driver/monitoring/utils"
)

var (
	// TransactionsThroughput is a metric capturing number of transactions per certain time period, i.e. the throughput
	TransactionsThroughput = monitoring.Metric[monitoring.Node, monitoring.Series[monitoring.BlockNumber, float32]]{
		Name:        "TransactionsThroughput",
		Description: "The number of transactions processed per certain time period by each node",
	}
)

func init() {
	if err := monitoring.RegisterSource(TransactionsThroughput, newTransactionsThroughputSource); err != nil {
		panic(fmt.Sprintf("failed to register metric source: %v", err))
	}
}

// TransactionsThroughputSource is a metric source that captures transaction throughput.
//
// The throughput is measured between the times consensus created the blocks,
// not the times a node processed them: a node catching up processes blocks far
// faster than the network produced them.
type TransactionsThroughputSource struct {
	BlockNodeMetricSource[float32]
	lastHeights map[monitoring.Node]int       // heights of the latest received blocks
	lastTimes   map[monitoring.Node]time.Time // creation times of the latest measured blocks
	carried     map[monitoring.Node]int       // transactions of blocks created at the same time as the latest
}

// NewTransactionsThroughputSource creates a metric capturing transaction throughput.
func NewTransactionsThroughputSource(monitor *monitoring.Monitor) *TransactionsThroughputSource {
	m := &TransactionsThroughputSource{
		SyncedSeriesSource: utils.NewSyncedSeriesSource(TransactionsThroughput),
		monitor:            monitor,
		syncingTracker:     newSyncingTracker(),
		lastHeights:        make(map[monitoring.Node]int, 50),
		lastTimes:          make(map[monitoring.Node]time.Time, 50),
		carried:            make(map[monitoring.Node]int, 50),
	}
	monitor.NodeLogProvider().RegisterLogListener(m)

	return m
}

// newTransactionsThroughputSource is the same as its public counterpart, it only returns the Source interface instead of the struct to be used in factories
func newTransactionsThroughputSource(monitor *monitoring.Monitor) monitoring.Source[monitoring.Node, monitoring.Series[monitoring.BlockNumber, float32]] {
	return NewTransactionsThroughputSource(monitor)
}

func (s *TransactionsThroughputSource) OnBlock(node monitoring.Node, block monitoring.Block) {
	s.markNodeAsSyncing(node)

	if height, exists := s.lastHeights[node]; exists && block.Height <= height {
		// a block replayed by a restarted node, its transactions were counted already
		return
	}
	s.lastHeights[node] = block.Height

	created := block.Time.Add(-block.Age)
	prevTime, exists := s.lastTimes[node]
	if !exists {
		// very first node received - no difference can be computed, but the data series is expected to be created
		s.lastTimes[node] = created
		s.GetOrAddSubject(node)
		return
	}

	timeDiff := created.Sub(prevTime).Nanoseconds()
	// blocks created within the resolution of the log count towards the next one
	s.carried[node] += block.Txs
	if timeDiff <= 0 {
		return
	}
	txs := float64(s.carried[node]) * 1e9 / float64(timeDiff)
	s.lastTimes[node] = created
	s.carried[node] = 0
	series := s.GetOrAddSubject(node)
	if err := series.Append(monitoring.BlockNumber(block.Height), float32(txs)); err != nil {
		if s.shouldSuppressAppendConflict(node, err) {
			return
		}
		slog.Error("error to add to the series", "error", err)
		return
	}
	s.markNodeAsSynced(node)
}
