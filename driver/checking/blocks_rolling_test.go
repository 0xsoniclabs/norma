package checking

import (
	"strings"
	"testing"

	"github.com/0xsoniclabs/norma/driver/monitoring"
	"github.com/0xsoniclabs/norma/driver/parser"
	"go.uber.org/mock/gomock"
)

func TestBlocksRolling_Blocks_Processed(t *testing.T) {
	tests := map[string]struct {
		series []uint64
	}{
		"one": {
			series: []uint64{1},
		},
		"monotonic-increasing": {
			series: []uint64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
		},
		"monotonic-non-decreasing": {
			series: []uint64{1, 2, 3, 4, 5, 5, 5, 5, 6, 7, 8, 9},
		},
		"monotonic-non-decreasing-towards-beginning": {
			series: []uint64{5, 5, 5, 5, 6, 7, 8, 9, 10, 11, 12, 13},
		},
		"monotonic-non-decreasing-towards-end": {
			series: []uint64{1, 2, 3, 4, 5, 6, 7, 8, 8, 8, 8},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			series := createBlockSeries(t, test.series)
			ctrl := gomock.NewController(t)
			monitor := NewMockMonitoringData(ctrl)
			monitor.EXPECT().GetNodes().Return([]monitoring.Node{"A"})
			monitor.EXPECT().GetBlockStatus(gomock.Any()).Return(series)

			c := blocksRollingChecker{monitor: monitor, toleranceSamples: 5}
			if err := c.Check(); err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestBlocksRolling_Blocks_Failure(t *testing.T) {
	tests := map[string]struct {
		series []uint64
	}{
		"empty": {
			series: []uint64{},
		},
		"monotonic-decreasing": {
			series: []uint64{10, 9, 8, 7, 6, 5, 4, 3, 2, 1},
		},
		"monotonic-non-increasing": {
			series: []uint64{10, 9, 8, 7, 6, 6, 6, 6, 5, 4, 3, 2},
		},
		"non-monotonic-towards-end": {
			series: []uint64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 5},
		},
		"non-monotonic-towards-beginning": {
			series: []uint64{10, 1, 9, 8, 7, 6, 5, 4, 3, 2, 1},
		},
		"monotonic-non-decreasing-long": {
			series: []uint64{1, 2, 3, 4, 5, 5, 5, 5, 5, 6, 7, 8, 9},
		},
		"constant": {
			series: []uint64{5, 5, 5, 5, 5, 5, 5, 5, 5, 5},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			series := createBlockSeries(t, test.series)
			ctrl := gomock.NewController(t)
			monitor := NewMockMonitoringData(ctrl)
			monitor.EXPECT().GetNodes().Return([]monitoring.Node{"A"})
			monitor.EXPECT().GetBlockStatus(gomock.Any()).Return(series)

			c := blocksRollingChecker{monitor: monitor, toleranceSamples: 5}
			if err := c.Check(); err == nil || err.Error() != "network is down, nodes stopped producing blocks" {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestBlocksRolling_Blocks_WithStarts(t *testing.T) {
	series := createBlockSeries(t, []uint64{1, 1, 1, 1, 1, 2, 3, 4, 5, 6})

	ctrl := gomock.NewController(t)
	monitor := NewMockMonitoringData(ctrl)

	// this fails because of 1, 1, 1, 1, 1
	checker := blocksRollingChecker{monitor: monitor, toleranceSamples: 5}
	monitor.EXPECT().GetNodes().Return([]monitoring.Node{"A"})
	monitor.EXPECT().GetBlockStatus(gomock.Any()).Return(series)
	if err := checker.Check(); err == nil || err.Error() != "network is down, nodes stopped producing blocks" {
		t.Errorf("unexpected error: %v", err)
	}

	configured, err := checker.Configure(parser.CheckerConfig{"start": 5})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	monitor.EXPECT().GetNodes().Return([]monitoring.Node{"A"})
	monitor.EXPECT().GetBlockStatus(gomock.Any()).Return(series)
	if err := configured.Check(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestBlocksRolling_Configure(t *testing.T) {
	series := createBlockSeries(t, []uint64{1, 1, 1, 1, 1, 2, 3, 4, 5, 6})
	ctrl := gomock.NewController(t)
	monitor := NewMockMonitoringData(ctrl)
	monitor.EXPECT().GetNodes().Return([]monitoring.Node{"A"}).Times(4)
	monitor.EXPECT().GetBlockStatus(gomock.Any()).Return(series).Times(4)

	// original returns error because it sees 1, 1, 1, 1, 1
	original := blocksRollingChecker{monitor: monitor, toleranceSamples: 5}
	// success will pass because it sees the entire series 1->6
	success, err := original.Configure(parser.CheckerConfig{"tolerance": 10})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	// emptyOriginal has the same behavior as original
	emptyOriginal, err := original.Configure(parser.CheckerConfig{})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	// emptySuccess has the same behavior as success
	emptySuccess, err := success.Configure(parser.CheckerConfig{})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	// misconfigured will throw an error
	if _, err := original.Configure(parser.CheckerConfig{"tolerance": "abc"}); err == nil || !strings.Contains(err.Error(), "failed to convert tolerance") {
		t.Errorf("not caught: failed to convert tolerance; %v", err)
	}

	if err := original.Check(); err == nil || err.Error() != "network is down, nodes stopped producing blocks" {
		t.Errorf("not caught: network is down; %v", err)
	}

	if err := emptyOriginal.Check(); err == nil || err.Error() != "network is down, nodes stopped producing blocks" {
		t.Errorf("not caught: network is down; %v", err)
	}

	if err := success.Check(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if err := emptySuccess.Check(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func createBlockSeries(t *testing.T, blocks []uint64) monitoring.Series[monitoring.Time, monitoring.BlockStatus] {
	t.Helper()

	series := monitoring.SyncedSeries[monitoring.Time, monitoring.BlockStatus]{}
	for i, block := range blocks {
		if err := series.Append(monitoring.Time(i), monitoring.BlockStatus{BlockHeight: block}); err != nil {
			t.Fatalf("failed to append block %d: %v", block, err)
		}
	}
	return &series
}
