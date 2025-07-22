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
		config CheckerConfig
	}{
		"empty": {
			series: []float64{},
			config: CheckerConfig{},
		},
		"exact": {
			series: []float64{1, 30},
			config: CheckerConfig{},
		},
		"exceed-catch-error": {
			series: []float64{30, 31, 32},
			config: CheckerConfig{"error": "Exceeded gas ceiling"},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			series := createGasRateSeries(t, test.series)
			ctrl := gomock.NewController(t)
			monitor := NewMockMonitoringData(ctrl)
			monitor.EXPECT().GetBlockGasRate().Return(series)

			c := blockGasRateChecker{monitor: monitor, ceiling: 30}
			configured, err := c.Configure(test.config)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if err := configured.Check(); err != nil {
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
	// success will pass because ceiling is now 50
	success, err := original.Configure(CheckerConfig{"ceiling": 50})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	// emptyOriginal has the same behavior as original
	emptyOriginal, err := original.Configure(CheckerConfig{})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	// emptySuccess has the same behavior as success
	emptySuccess, err := success.Configure(CheckerConfig{})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	// misconfigured will throw an error
	if _, err := original.Configure(CheckerConfig{"ceiling": "abc"}); err == nil || !strings.Contains(err.Error(), "failed to convert ceiling") {
		t.Errorf("not caught: failed to convert ceiling; %v", err)
	}

	if err := original.Check(); err == nil || !strings.Contains(err.Error(), "Exceeded gas ceiling") {
		t.Errorf("not caught: Exceeded gas ceiling; %v", err)
	}
	if err := emptyOriginal.Check(); err == nil || !strings.Contains(err.Error(), "Exceeded gas ceiling") {
		t.Errorf("not caught: Exceeded gas ceiling; %v", err)
	}
	if err := success.Check(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
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

// TestBlocksGasRate_ParsingCeiling checks parsing any to float64
func TestBlocksGasRate_ParsingCeiling(t *testing.T) {
	tests := []any{
		123,        // int
		456.7,      // float
		uint64(89), // uint64
		^uint64(0), // max uint64
	}

	for _, test := range tests {
		ctrl := gomock.NewController(t)
		monitor := NewMockMonitoringData(ctrl)

		original := blockGasRateChecker{monitor: monitor, ceiling: 30}
		_, err := original.Configure(CheckerConfig{"ceiling": test})
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	}
}
