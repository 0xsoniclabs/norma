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
	"testing"

	"github.com/0xsoniclabs/norma/driver"
	mon "github.com/0xsoniclabs/norma/driver/monitoring"
	"go.uber.org/mock/gomock"
)

func TestNodeEpochStatus_ForEachRecord_EmitsEpochValues(t *testing.T) {
	ctrl := gomock.NewController(t)

	net := driver.NewMockNetwork(ctrl)
	net.EXPECT().RegisterListener(gomock.Any()).AnyTimes()
	net.EXPECT().GetActiveNodes().Return([]driver.Node{}).AnyTimes()

	monitor, err := mon.NewMonitor(net, mon.MonitorConfig{OutputDir: t.TempDir()})
	if err != nil {
		t.Fatalf("failed to create monitor: %v", err)
	}

	// Install a fake NodeBlockStatus source with known data.
	fakeSource := &fakeBlockStatusSource{
		data: map[mon.Node]*mon.SyncedSeries[mon.Time, mon.BlockStatus]{
			"node-A": {},
		},
	}
	// Populate with block status entries across two epochs.
	entries := []struct {
		time  mon.Time
		epoch uint64
		block uint64
	}{
		{time: 1, epoch: 1, block: 10},
		{time: 2, epoch: 1, block: 20},
		{time: 3, epoch: 2, block: 30},
		{time: 4, epoch: 2, block: 40},
	}
	for _, e := range entries {
		if err := fakeSource.data["node-A"].Append(e.time, mon.BlockStatus{Epoch: e.epoch, BlockHeight: e.block}); err != nil {
			t.Fatalf("failed to append test data: %v", err)
		}
	}

	// Manually install the fake source under the NodeBlockStatus metric name.
	if err := mon.InstallSource(monitor, fakeSource); err != nil {
		t.Fatalf("failed to install fake source: %v", err)
	}

	// Create the epoch source under test.
	epochSource := newNodeEpochStatusSource(monitor)

	// Verify GetSubjects delegates correctly.
	subjects := epochSource.GetSubjects()
	if len(subjects) != 1 || subjects[0] != "node-A" {
		t.Fatalf("expected [node-A], got %v", subjects)
	}

	// Verify GetData returns nil (not implemented).
	_, exists := epochSource.GetData("node-A")
	if exists {
		t.Fatal("expected GetData to return false")
	}

	// Collect records from ForEachRecord.
	var records []mon.Record
	epochSource.ForEachRecord(func(r mon.Record) {
		records = append(records, r)
	})

	// We expect one record per data point in GetRange + the latest point.
	// GetRange(0, latest.Position) is half-open [0, 4), so it returns times 1,2,3.
	// Then latest (time=4) is appended separately.
	// Total: 4 records for node-A.
	if len(records) != 4 {
		t.Fatalf("expected 4 records, got %d", len(records))
	}

	// Verify epoch values are correct.
	expectedEpochs := []string{"1", "1", "2", "2"}
	for i, r := range records {
		if r.Value != expectedEpochs[i] {
			t.Errorf("record %d: expected epoch %s, got %s", i, expectedEpochs[i], r.Value)
		}
	}
}

func TestNodeEpochStatus_ForEachRecord_NoDataNoRecords(t *testing.T) {
	ctrl := gomock.NewController(t)

	net := driver.NewMockNetwork(ctrl)
	net.EXPECT().RegisterListener(gomock.Any()).AnyTimes()
	net.EXPECT().GetActiveNodes().Return([]driver.Node{}).AnyTimes()

	monitor, err := mon.NewMonitor(net, mon.MonitorConfig{OutputDir: t.TempDir()})
	if err != nil {
		t.Fatalf("failed to create monitor: %v", err)
	}

	// Install an empty fake source.
	fakeSource := &fakeBlockStatusSource{
		data: map[mon.Node]*mon.SyncedSeries[mon.Time, mon.BlockStatus]{},
	}
	if err := mon.InstallSource(monitor, fakeSource); err != nil {
		t.Fatalf("failed to install fake source: %v", err)
	}

	epochSource := newNodeEpochStatusSource(monitor)

	var records []mon.Record
	epochSource.ForEachRecord(func(r mon.Record) {
		records = append(records, r)
	})

	if len(records) != 0 {
		t.Fatalf("expected 0 records, got %d", len(records))
	}
}

func TestNodeEpochStatus_ForEachRecord_SkipsNodeWithNoSeries(t *testing.T) {
	ctrl := gomock.NewController(t)

	net := driver.NewMockNetwork(ctrl)
	net.EXPECT().RegisterListener(gomock.Any()).AnyTimes()
	net.EXPECT().GetActiveNodes().Return([]driver.Node{}).AnyTimes()

	monitor, err := mon.NewMonitor(net, mon.MonitorConfig{OutputDir: t.TempDir()})
	if err != nil {
		t.Fatalf("failed to create monitor: %v", err)
	}

	// Subject exists but series has no data points.
	fakeSource := &fakeBlockStatusSource{
		data: map[mon.Node]*mon.SyncedSeries[mon.Time, mon.BlockStatus]{
			"node-B": {}, // empty series — GetLatest returns nil
		},
	}
	if err := mon.InstallSource(monitor, fakeSource); err != nil {
		t.Fatalf("failed to install fake source: %v", err)
	}

	epochSource := newNodeEpochStatusSource(monitor)

	var records []mon.Record
	epochSource.ForEachRecord(func(r mon.Record) {
		records = append(records, r)
	})

	if len(records) != 0 {
		t.Fatalf("expected 0 records for empty series, got %d", len(records))
	}
}

// fakeBlockStatusSource is a test helper implementing Source for NodeBlockStatus.
type fakeBlockStatusSource struct {
	data map[mon.Node]*mon.SyncedSeries[mon.Time, mon.BlockStatus]
}

func (f *fakeBlockStatusSource) GetMetric() mon.Metric[mon.Node, mon.Series[mon.Time, mon.BlockStatus]] {
	return NodeBlockStatus
}

func (f *fakeBlockStatusSource) CreateSource(_ *mon.Monitor) mon.Source[mon.Node, mon.Series[mon.Time, mon.BlockStatus]] {
	return f
}

func (f *fakeBlockStatusSource) GetSubjects() []mon.Node {
	subjects := make([]mon.Node, 0, len(f.data))
	for k := range f.data {
		subjects = append(subjects, k)
	}
	return subjects
}

func (f *fakeBlockStatusSource) GetData(node mon.Node) (mon.Series[mon.Time, mon.BlockStatus], bool) {
	s, ok := f.data[node]
	if !ok {
		return nil, false
	}
	return s, true
}

func (f *fakeBlockStatusSource) Shutdown() error { return nil }

func (f *fakeBlockStatusSource) ForEachRecord(consumer func(r mon.Record)) {
	for node, series := range f.data {
		latest := series.GetLatest()
		if latest == nil {
			continue
		}
		var first mon.Time
		allData := series.GetRange(first, latest.Position)
		r := mon.Record{}
		r.SetSubject(node)
		for _, point := range allData {
			r.SetPosition(point.Position)
			r.Value = fmt.Sprintf("%v", point.Value)
			consumer(r)
		}
		r.SetPosition(latest.Position)
		r.Value = fmt.Sprintf("%v", latest.Value)
		consumer(r)
	}
}
