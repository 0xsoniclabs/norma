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
	"sync"
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
type TransactionsThroughputSource struct {
	BlockNodeMetricSource[float32]
	lastTimes    map[monitoring.Node]time.Time // timestamps of the latest received blocks
	syncingNodes map[monitoring.Node]bool
	syncingMutex sync.Mutex
}

// NewTransactionsThroughputSource creates a metric capturing transaction throughput.
func NewTransactionsThroughputSource(monitor *monitoring.Monitor) *TransactionsThroughputSource {
	m := &TransactionsThroughputSource{
		BlockNodeMetricSource: BlockNodeMetricSource[float32]{
			SyncedSeriesSource: utils.NewSyncedSeriesSource(TransactionsThroughput),
			monitor:            monitor,
		},
		lastTimes:    make(map[monitoring.Node]time.Time, 50),
		syncingNodes: make(map[monitoring.Node]bool, 50),
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

	prevTime, exists := s.lastTimes[node]
	s.lastTimes[node] = block.Time
	if !exists {
		// very first node received - no difference can be computed, but the data series is expected to be created
		s.GetOrAddSubject(node)
		return
	}

	timeDiff := block.Time.Sub(prevTime).Nanoseconds()
	// prevent NaN or Inf: when the time difference is bellow measured value, skip the block.
	if timeDiff != 0 {
		txs := float64(block.Txs) * 1e9 / float64(timeDiff)
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
}

func (s *TransactionsThroughputSource) markNodeAsSyncing(node monitoring.Node) {
	s.syncingMutex.Lock()
	defer s.syncingMutex.Unlock()
	if _, exists := s.syncingNodes[node]; !exists {
		s.syncingNodes[node] = true
	}
}

func (s *TransactionsThroughputSource) markNodeAsSynced(node monitoring.Node) {
	s.syncingMutex.Lock()
	defer s.syncingMutex.Unlock()
	s.syncingNodes[node] = false
}

func (s *TransactionsThroughputSource) isNodeSyncing(node monitoring.Node) bool {
	s.syncingMutex.Lock()
	defer s.syncingMutex.Unlock()
	syncing, exists := s.syncingNodes[node]
	if !exists {
		return true
	}
	return syncing
}

func (s *TransactionsThroughputSource) shouldSuppressAppendConflict(node monitoring.Node, err error) bool {
	if !monitoring.IsOutOfOrderAppendError(err) {
		return false
	}
	return s.isNodeSyncing(node)
}
