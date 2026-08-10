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
	"slices"
	"sort"
	"time"

	mon "github.com/0xsoniclabs/norma/driver/monitoring"
)

// sampleSource exposes the per-transaction measurements of one phase as a
// metric. Unlike the periodic sources of the monitoring framework it does not
// sample anything itself: the tracker measures a transaction when it reaches a
// phase, and this source only reports what was measured.
//
// The measurements of one application share positions - transactions submitted
// within the same nanosecond, or a resubmission - which is why they are not kept
// in a monitoring.SyncedSeries: that one is an append-only series requiring
// strictly increasing positions.
type sampleSource struct {
	metric  mon.Metric[mon.App, mon.Series[mon.Time, time.Duration]]
	kind    SampleKind
	tracker *Tracker
	monitor *mon.Monitor
}

func newSampleSource(
	metric mon.Metric[mon.App, mon.Series[mon.Time, time.Duration]],
	kind SampleKind,
	monitor *mon.Monitor,
) mon.Source[mon.App, mon.Series[mon.Time, time.Duration]] {
	return &sampleSource{
		metric:  metric,
		kind:    kind,
		tracker: tracker(monitor),
		monitor: monitor,
	}
}

func (s *sampleSource) GetMetric() mon.Metric[mon.App, mon.Series[mon.Time, time.Duration]] {
	return s.metric
}

func (s *sampleSource) GetSubjects() []mon.App {
	apps := s.tracker.Apps()
	slices.Sort(apps)
	subjects := make([]mon.App, 0, len(apps))
	for _, app := range apps {
		subjects = append(subjects, mon.App(app))
	}
	return subjects
}

func (s *sampleSource) GetData(app mon.App) (mon.Series[mon.Time, time.Duration], bool) {
	samples := s.tracker.Samples(string(app), s.kind)
	if len(samples) == 0 {
		return nil, false
	}
	return &sampleSeries{samples: samples}, true
}

func (s *sampleSource) ForEachRecord(consume func(mon.Record)) {
	for _, app := range s.GetSubjects() {
		for _, sample := range s.tracker.Samples(string(app), s.kind) {
			record := mon.Record{}
			record.SetSubject(app)
			record.SetPosition(mon.NewTime(sample.At))
			record.SetValue(sample.Duration)
			consume(record)
		}
	}
}

func (s *sampleSource) Shutdown() error {
	stopTracker(s.monitor)
	return nil
}

// sampleSeries is a read-only view on measurements sorted by position.
type sampleSeries struct {
	samples []Sample
}

func (s *sampleSeries) GetRange(from, to mon.Time) []mon.DataPoint[mon.Time, time.Duration] {
	begin := s.search(from)
	end := s.search(to)
	points := make([]mon.DataPoint[mon.Time, time.Duration], 0, end-begin)
	for _, sample := range s.samples[begin:end] {
		points = append(points, mon.DataPoint[mon.Time, time.Duration]{
			Position: mon.NewTime(sample.At),
			Value:    sample.Duration,
		})
	}
	return points
}

func (s *sampleSeries) GetLatest() *mon.DataPoint[mon.Time, time.Duration] {
	if len(s.samples) == 0 {
		return nil
	}
	last := s.samples[len(s.samples)-1]
	return &mon.DataPoint[mon.Time, time.Duration]{
		Position: mon.NewTime(last.At),
		Value:    last.Duration,
	}
}

// search returns the index of the first sample at or after the given position.
func (s *sampleSeries) search(position mon.Time) int {
	return sort.Search(len(s.samples), func(i int) bool {
		return mon.NewTime(s.samples[i].At) >= position
	})
}
