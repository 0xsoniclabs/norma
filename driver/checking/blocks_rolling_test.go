package checking

import (
	"github.com/0xsoniclabs/norma/driver/monitoring"
	"go.uber.org/mock/gomock"
	"testing"
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
			monitor.EXPECT().GetData(gomock.Any()).Return(series)

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
			monitor.EXPECT().GetData(gomock.Any()).Return(series)

			c := blocksRollingChecker{monitor: monitor, toleranceSamples: 5}
			if err := c.Check(); err == nil || err.Error() != "network is down, nodes stopped producing blocks" {
				t.Errorf("unexpected error: %v", err)
			}
		})
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
