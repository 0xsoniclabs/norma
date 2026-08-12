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
	mon "github.com/0xsoniclabs/norma/driver/monitoring"
)

// gasLimitSource exposes the gas limit of each block, which is the ceiling the
// gas of that block's transactions had to fit into. It is read from the header of
// the blocks whose transactions are read anyway, see inclusions.go, and reported
// as a property of the network rather than of a node: every node of a network
// agrees on the limit of a block.
type gasLimitSource struct {
	metric  mon.Metric[mon.Network, mon.Series[mon.BlockNumber, int]]
	value   func(BlockLimit) int
	tracker *Tracker
	monitor *mon.Monitor
}

func newGasLimitSource(
	metric mon.Metric[mon.Network, mon.Series[mon.BlockNumber, int]],
	value func(BlockLimit) int,
	monitor *mon.Monitor,
) mon.Source[mon.Network, mon.Series[mon.BlockNumber, int]] {
	return &gasLimitSource{
		metric:  metric,
		value:   value,
		tracker: tracker(monitor),
		monitor: monitor,
	}
}

func (s *gasLimitSource) GetMetric() mon.Metric[mon.Network, mon.Series[mon.BlockNumber, int]] {
	return s.metric
}

func (s *gasLimitSource) GetSubjects() []mon.Network {
	return []mon.Network{{}}
}

func (s *gasLimitSource) GetData(mon.Network) (mon.Series[mon.BlockNumber, int], bool) {
	limits := s.tracker.BlockGasLimits()
	if len(limits) == 0 {
		return nil, false
	}
	return &blockSeries[BlockLimit]{
		items: limits,
		block: func(l BlockLimit) int { return l.Block },
		value: s.value,
	}, true
}

func (s *gasLimitSource) ForEachRecord(consume func(mon.Record)) {
	for _, limit := range s.tracker.BlockGasLimits() {
		record := mon.Record{}
		record.SetSubject(mon.Network{})
		record.SetPosition(mon.BlockNumber(limit.Block))
		record.SetValue(s.value(limit))
		consume(record)
	}
}

func (s *gasLimitSource) Shutdown() error {
	stopTracker(s.monitor)
	return nil
}
