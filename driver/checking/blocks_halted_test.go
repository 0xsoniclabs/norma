package checking

import (
	"testing"

	"github.com/0xsoniclabs/norma/driver/monitoring"
	"go.uber.org/mock/gomock"
)

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

			c := blocksHaltedChecker{monitor: monitor, toleranceSamples: 5}
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

			c := blocksHaltedChecker{monitor: monitor, toleranceSamples: 5}
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

	c := blocksHaltedChecker{monitor: monitor, toleranceSamples: 5}
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

	c := blocksHaltedChecker{monitor: monitor, toleranceSamples: 5}
	if err := c.Check(); err != nil {
		t.Errorf("unexpected error with tolerance 5: %v", err)
	}

	// With tolerance=9, this series is NOT halted (first point=1, last=5)
	configured := c.Configure(CheckerConfig{"tolerance": 9})
	if err := configured.Check(); err == nil {
		t.Errorf("expected error with tolerance 9 but got nil")
	}
}
