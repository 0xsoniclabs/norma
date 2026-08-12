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

// compositionSource exposes what each application contributed to a block, either
// counted in transactions or in the gas they used. Summed over the applications
// of a block it is that block's own total, so the series describe how the
// applications shared the blocks they were competing for.
//
// The transactions of no application are reported under OtherTransactions.
type compositionSource struct {
	metric  mon.Metric[mon.App, mon.Series[mon.BlockNumber, int]]
	value   func(BlockContribution) int
	tracker *Tracker
	monitor *mon.Monitor
}

func newCompositionSource(
	metric mon.Metric[mon.App, mon.Series[mon.BlockNumber, int]],
	value func(BlockContribution) int,
	monitor *mon.Monitor,
) mon.Source[mon.App, mon.Series[mon.BlockNumber, int]] {
	return &compositionSource{
		metric:  metric,
		value:   value,
		tracker: tracker(monitor),
		monitor: monitor,
	}
}

func (s *compositionSource) GetMetric() mon.Metric[mon.App, mon.Series[mon.BlockNumber, int]] {
	return s.metric
}

func (s *compositionSource) GetSubjects() []mon.App {
	contributors := s.tracker.Contributors()
	subjects := make([]mon.App, 0, len(contributors))
	for _, contributor := range contributors {
		subjects = append(subjects, mon.App(contributor))
	}
	return subjects
}

func (s *compositionSource) GetData(app mon.App) (mon.Series[mon.BlockNumber, int], bool) {
	contributions := s.tracker.BlockContributions(string(app))
	if len(contributions) == 0 {
		return nil, false
	}
	return &blockSeries[BlockContribution]{
		items: contributions,
		block: func(c BlockContribution) int { return c.Block },
		value: s.value,
	}, true
}

func (s *compositionSource) ForEachRecord(consume func(mon.Record)) {
	for _, app := range s.GetSubjects() {
		for _, contribution := range s.tracker.BlockContributions(string(app)) {
			record := mon.Record{}
			record.SetSubject(app)
			record.SetPosition(mon.BlockNumber(contribution.Block))
			record.SetValue(s.value(contribution))
			consume(record)
		}
	}
}

func (s *compositionSource) Shutdown() error {
	stopTracker(s.monitor)
	return nil
}
