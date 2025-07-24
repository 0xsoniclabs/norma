package checking

import (
	"strings"
	"testing"

	"github.com/0xsoniclabs/norma/driver/monitoring"
	"go.uber.org/mock/gomock"
)

func TestBlocksGasRate_Success(t *testing.T) {
	tests := map[string]struct {
		series []float64
	}{
		"empty": {
			series: []float64{},
		},
		"exact": {
			series: []float64{1, 30},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			series := createGasRateSeries(t, test.series)
			ctrl := gomock.NewController(t)
			monitor := NewMockMonitoringData(ctrl)
			monitor.EXPECT().GetBlockGasRate().Return(series)

			c := blockGasRateChecker{monitor: monitor, ceiling: 30}
			if err := c.Check(); err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestBlocksGasRate_Failure(t *testing.T) {
	tests := map[string]struct {
		series []float64
	}{
		"exceed": {
			series: []float64{30, 31, 32},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			series := createGasRateSeries(t, test.series)
			ctrl := gomock.NewController(t)
			monitor := NewMockMonitoringData(ctrl)
			monitor.EXPECT().GetBlockGasRate().Return(series)

			c := blockGasRateChecker{monitor: monitor, ceiling: 30}
			if err := c.Check(); err == nil || !strings.Contains(err.Error(), "Exceeded gas ceiling") {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestBlocksGasRate_Configure(t *testing.T) {
	series := createGasRateSeries(t, []float64{10, 20, 30, 40})
	ctrl := gomock.NewController(t)
	monitor := NewMockMonitoringData(ctrl)
	monitor.EXPECT().GetBlockGasRate().Return(series).Times(4)

	// original will fail because gas rates exceed 30
	original := blockGasRateChecker{monitor: monitor, ceiling: 30}
	if err := original.Check(); err == nil || !strings.Contains(err.Error(), "Exceeded gas ceiling") {
		t.Errorf("not caught: Exceeded gas ceiling; %v", err)
	}

	// emptyOriginal has the same behavior as original
	emptyOriginal := original.Configure(CheckerConfig{})
	if err := emptyOriginal.Check(); err == nil || !strings.Contains(err.Error(), "Exceeded gas ceiling") {
		t.Errorf("not caught: Exceeded gas ceiling; %v", err)
	}

	// success will pass because ceiling is now 50
	success := original.Configure(CheckerConfig{"ceiling": 50})
	if err := success.Check(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// emptySuccess has the same behavior as success
	emptySuccess := success.Configure(CheckerConfig{})
	if err := emptySuccess.Check(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func createGasRateSeries(t *testing.T, gasRates []float64) monitoring.Series[monitoring.BlockNumber, float64] {
	t.Helper()

	series := monitoring.SyncedSeries[monitoring.BlockNumber, float64]{}
	for block, gasRate := range gasRates {
		if err := series.Append(monitoring.BlockNumber(block), gasRate); err != nil {
			t.Fatalf("failed to append block %d: %v", block, err)
		}
	}
	return &series
}
