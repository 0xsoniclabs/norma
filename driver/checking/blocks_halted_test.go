package checking

import (
	"testing"
	"time"

	"github.com/0xsoniclabs/norma/driver/monitoring"
	"go.uber.org/mock/gomock"
)

// recentNow returns a time close to the test data timestamps (which use
// monitoring.Time(i) i.e. nanoseconds 0,1,2,...). This ensures the staleness
// check does not skip the data.
func recentNow() time.Time {
	return time.Unix(0, 0)
}

func TestBlocksHalted_Passes_WhenConstant(t *testing.T) {
	tests := map[string]struct {
		series []uint64
	}{
		"constant": {
			series: []uint64{5, 5, 5, 5, 5, 5, 5, 5, 5, 5},
		},
		"increased-then-stopped": {
			series: []uint64{1, 2, 3, 4, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5},
		},
		"single-point": {
			series: []uint64{10},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			series := createBlockSeries(t, test.series)
			ctrl := gomock.NewController(t)
			monitor := NewMockMonitoringData(ctrl)
			monitor.EXPECT().GetNodes().Return([]monitoring.Node{"A"})
			monitor.EXPECT().GetBlockStatus(gomock.Any()).Return(series)

			c := blocksHaltedChecker{monitor: monitor, toleranceSamples: 5, now: recentNow}
			if err := c.Check(); err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestBlocksHalted_Fails_WhenIncreasing(t *testing.T) {
	tests := map[string]struct {
		series []uint64
	}{
		"increasing": {
			series: []uint64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
		},
		"increasing-at-tail": {
			series: []uint64{5, 5, 5, 5, 5, 5, 5, 5, 9, 10},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			series := createBlockSeries(t, test.series)
			ctrl := gomock.NewController(t)
			monitor := NewMockMonitoringData(ctrl)
			monitor.EXPECT().GetNodes().Return([]monitoring.Node{"A"})
			monitor.EXPECT().GetBlockStatus(gomock.Any()).Return(series)

			c := blocksHaltedChecker{monitor: monitor, toleranceSamples: 5, now: recentNow}
			if err := c.Check(); err == nil {
				t.Errorf("expected error but got nil")
			}
		})
	}
}

func TestBlocksHalted_Passes_WhenNoData(t *testing.T) {
	series := createBlockSeries(t, []uint64{})
	ctrl := gomock.NewController(t)
	monitor := NewMockMonitoringData(ctrl)
	monitor.EXPECT().GetNodes().Return([]monitoring.Node{"A"})
	monitor.EXPECT().GetBlockStatus(gomock.Any()).Return(series)

	c := blocksHaltedChecker{monitor: monitor, toleranceSamples: 5, now: recentNow}
	if err := c.Check(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestBlocksHalted_Configure(t *testing.T) {
	// With tolerance=5, this series looks halted (last 5 points: 5,5,5,5,5)
	series := createBlockSeries(t, []uint64{1, 2, 3, 4, 5, 5, 5, 5, 5})
	ctrl := gomock.NewController(t)
	monitor := NewMockMonitoringData(ctrl)
	monitor.EXPECT().GetNodes().Return([]monitoring.Node{"A"}).Times(2)
	monitor.EXPECT().GetBlockStatus(gomock.Any()).Return(series).Times(2)

	c := blocksHaltedChecker{monitor: monitor, toleranceSamples: 5, now: recentNow}
	if err := c.Check(); err != nil {
		t.Errorf("unexpected error with tolerance 5: %v", err)
	}

	// With tolerance=9, this series is NOT halted (first point=1, last=5)
	configured := c.Configure(CheckerConfig{"tolerance": 9})
	if err := configured.Check(); err == nil {
		t.Errorf("expected error with tolerance 9 but got nil")
	}
}

func TestBlocksHalted_Passes_WhenStaleData(t *testing.T) {
	// Data shows increasing blocks, but the monitoring is stale (node removed).
	series := createBlockSeries(t, []uint64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10})
	ctrl := gomock.NewController(t)
	monitor := NewMockMonitoringData(ctrl)
	monitor.EXPECT().GetNodes().Return([]monitoring.Node{"A"})
	monitor.EXPECT().GetBlockStatus(gomock.Any()).Return(series)

	// "now" is far in the future relative to the data timestamps,
	// so the data is considered stale and the node is treated as halted.
	farFuture := func() time.Time { return time.Unix(60, 0) }
	c := blocksHaltedChecker{monitor: monitor, toleranceSamples: 5, now: farFuture}
	if err := c.Check(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}