package checking

import (
	"testing"

	"github.com/0xsoniclabs/norma/driver/monitoring"
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
			if err := c.Check(t.Context()); err != nil {
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
			if err := c.Check(t.Context()); err == nil || err.Error() != "network is down, nodes stopped producing blocks" {
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
	if err := checker.Check(t.Context()); err == nil || err.Error() != "network is down, nodes stopped producing blocks" {
		t.Errorf("unexpected error: %v", err)
	}

	configured := checker.Configure(CheckerConfig{"start": int64(5)})
	monitor.EXPECT().GetNodes().Return([]monitoring.Node{"A"})
	monitor.EXPECT().GetBlockStatus(gomock.Any()).Return(series)
	if err := configured.Check(t.Context()); err != nil {
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
	if err := original.Check(t.Context()); err == nil || err.Error() != "network is down, nodes stopped producing blocks" {
		t.Errorf("not caught: network is down; %v", err)
	}

	// emptyOriginal has the same behavior as original
	emptyOriginal := original.Configure(CheckerConfig{})
	if err := emptyOriginal.Check(t.Context()); err == nil || err.Error() != "network is down, nodes stopped producing blocks" {
		t.Errorf("not caught: network is down; %v", err)
	}

	// success will pass because it sees the entire series 1->6
	success := original.Configure(CheckerConfig{"tolerance": 10})
	if err := success.Check(t.Context()); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// emptySuccess has the same behavior as success
	emptySuccess := success.Configure(CheckerConfig{})
	if err := emptySuccess.Check(t.Context()); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestBlocksRolling_Start_UsesFirstVsLastComparison verifies that when a
// non-zero start is set, the checker only compares the first in-range sample
// to the latest sample instead of applying a sliding sub-window. This makes
// the check robust against transient stalls inside the window.
func TestBlocksRolling_Start_UsesFirstVsLastComparison(t *testing.T) {
	// Long-lived stall in the middle, but overall progress at the ends.
	// Sliding window (tolerance=5) would fail on the 6-sample flat span,
	// but the windowed-mode (start > 0) check should pass because the
	// first in-range sample (2) has a lower block height than the last (6).
	blocks := []uint64{1, 2, 2, 2, 2, 2, 2, 3, 4, 5, 6}
	series := createBlockSeries(t, blocks)

	ctrl := gomock.NewController(t)
	monitor := NewMockMonitoringData(ctrl)

	// Sliding-window mode (start = 0) fails as expected.
	monitor.EXPECT().GetNodes().Return([]monitoring.Node{"A"})
	monitor.EXPECT().GetBlockStatus(gomock.Any()).Return(series)
	strict := blocksRollingChecker{monitor: monitor, toleranceSamples: 5}
	if err := strict.Check(t.Context()); err == nil {
		t.Fatalf("expected sliding-window failure")
	}

	// Windowed mode (start = 1) passes because 2 < 6 at the endpoints.
	monitor.EXPECT().GetNodes().Return([]monitoring.Node{"A"})
	monitor.EXPECT().GetBlockStatus(gomock.Any()).Return(series)
	windowed := strict.Configure(CheckerConfig{"start": int64(1)})
	if err := windowed.Check(t.Context()); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestBlocksRolling_Start_FailsWhenWindowStalls verifies that the windowed
// mode still fails if the block height did not advance across the window.
func TestBlocksRolling_Start_FailsWhenWindowStalls(t *testing.T) {
	// Progress in the first half, then a total stall for the last 5 samples.
	blocks := []uint64{1, 2, 3, 4, 5, 5, 5, 5, 5, 5}
	series := createBlockSeries(t, blocks)

	ctrl := gomock.NewController(t)
	monitor := NewMockMonitoringData(ctrl)
	monitor.EXPECT().GetNodes().Return([]monitoring.Node{"A"})
	monitor.EXPECT().GetBlockStatus(gomock.Any()).Return(series)

	checker := (&blocksRollingChecker{monitor: monitor, toleranceSamples: 5}).
		Configure(CheckerConfig{"start": int64(5)})
	if err := checker.Check(t.Context()); err == nil || err.Error() != "network is down, nodes stopped producing blocks" {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestBlocksRolling_ZeroTolerance_ReturnsErrorInsteadOfPanic verifies that
// configuring tolerance=0 does not cause a divide-by-zero panic in the
// sliding-window path; the checker must fail cleanly with a descriptive
// error instead.
func TestBlocksRolling_ZeroTolerance_ReturnsErrorInsteadOfPanic(t *testing.T) {
	checker := &blocksRollingChecker{toleranceSamples: 0}
	err := checker.Check(t.Context())
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if got := err.Error(); got != "tolerance must be > 0, got 0" {
		t.Fatalf("unexpected error: %s", got)
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
