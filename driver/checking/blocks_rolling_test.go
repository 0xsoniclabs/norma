package checking

import (
	"testing"
	"time"

	"github.com/0xsoniclabs/norma/driver/monitoring"
	"go.uber.org/mock/gomock"
)

const testObservation = time.Millisecond

const networkDown = "network is down, nodes stopped producing blocks"

func TestBlocksRolling_ProducingNetworkPasses(t *testing.T) {
	tests := map[string][]uint64{
		"monotonic-increasing": {1, 2, 3, 4, 5},
		"flat-then-increase":   {5, 5, 5, 6, 7},
		"increase-then-flat":   {1, 2, 3, 3, 3},
		"single-late-increase": {4, 4, 4, 4, 5},
	}
	for name, blocks := range tests {
		t.Run(name, func(t *testing.T) {
			series := futureSeries(t, blocks)
			ctrl := gomock.NewController(t)
			monitor := NewMockMonitoringData(ctrl)
			monitor.EXPECT().GetNodes().Return([]monitoring.Node{"A"})
			monitor.EXPECT().GetBlockStatus(gomock.Any()).Return(series)

			c := blocksRollingChecker{monitor: monitor, toleranceSamples: 5, duration: testObservation}
			if err := c.Check(t.Context()); err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestBlocksRolling_HaltedNetworkFails(t *testing.T) {
	tests := map[string][]uint64{
		"empty":         {},
		"single-sample": {7},
		"constant":      {5, 5, 5, 5, 5},
	}
	for name, blocks := range tests {
		t.Run(name, func(t *testing.T) {
			series := futureSeries(t, blocks)
			ctrl := gomock.NewController(t)
			monitor := NewMockMonitoringData(ctrl)
			monitor.EXPECT().GetNodes().Return([]monitoring.Node{"A"})
			monitor.EXPECT().GetBlockStatus(gomock.Any()).Return(series)

			c := blocksRollingChecker{monitor: monitor, toleranceSamples: 5, duration: testObservation}
			if err := c.Check(t.Context()); err == nil || err.Error() != networkDown {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestBlocksRolling_NoSamplesInWindowFails(t *testing.T) {
	series := createBlockSeries(t, []uint64{1, 2, 3, 4, 5})
	ctrl := gomock.NewController(t)
	monitor := NewMockMonitoringData(ctrl)
	monitor.EXPECT().GetNodes().Return([]monitoring.Node{"A"})
	monitor.EXPECT().GetBlockStatus(gomock.Any()).Return(series)

	c := blocksRollingChecker{monitor: monitor, toleranceSamples: 5, duration: testObservation}
	if err := c.Check(t.Context()); err == nil || err.Error() != networkDown {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestBlocksRolling_IgnoresPastProduction(t *testing.T) {
	series := mixedSeries(t, []uint64{1, 2, 3}, []uint64{3, 3, 3})
	ctrl := gomock.NewController(t)
	monitor := NewMockMonitoringData(ctrl)
	monitor.EXPECT().GetNodes().Return([]monitoring.Node{"A"})
	monitor.EXPECT().GetBlockStatus(gomock.Any()).Return(series)

	c := blocksRollingChecker{monitor: monitor, toleranceSamples: 5, duration: testObservation}
	if err := c.Check(t.Context()); err == nil || err.Error() != networkDown {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestBlocksRolling_DetectsProductionAfterPastStall(t *testing.T) {
	series := mixedSeries(t, []uint64{5, 5, 5}, []uint64{5, 6, 7})
	ctrl := gomock.NewController(t)
	monitor := NewMockMonitoringData(ctrl)
	monitor.EXPECT().GetNodes().Return([]monitoring.Node{"A"})
	monitor.EXPECT().GetBlockStatus(gomock.Any()).Return(series)

	c := blocksRollingChecker{monitor: monitor, toleranceSamples: 5, duration: testObservation}
	if err := c.Check(t.Context()); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestBlocksRolling_PassesWhenAnyNodeProduces(t *testing.T) {
	ctrl := gomock.NewController(t)
	monitor := NewMockMonitoringData(ctrl)
	monitor.EXPECT().GetNodes().Return([]monitoring.Node{"A", "B"})
	monitor.EXPECT().GetBlockStatus(monitoring.Node("A")).Return(futureSeries(t, []uint64{5, 5, 5}))
	monitor.EXPECT().GetBlockStatus(monitoring.Node("B")).Return(futureSeries(t, []uint64{1, 2, 3}))

	c := blocksRollingChecker{monitor: monitor, toleranceSamples: 5, duration: testObservation}
	if err := c.Check(t.Context()); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestBlocksRolling_FailsWhenAllNodesHalted(t *testing.T) {
	ctrl := gomock.NewController(t)
	monitor := NewMockMonitoringData(ctrl)
	monitor.EXPECT().GetNodes().Return([]monitoring.Node{"A", "B"})
	monitor.EXPECT().GetBlockStatus(monitoring.Node("A")).Return(futureSeries(t, []uint64{5, 5, 5}))
	monitor.EXPECT().GetBlockStatus(monitoring.Node("B")).Return(futureSeries(t, []uint64{9, 9, 9}))

	c := blocksRollingChecker{monitor: monitor, toleranceSamples: 5, duration: testObservation}
	if err := c.Check(t.Context()); err == nil || err.Error() != networkDown {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestBlocksRolling_DerivesWindowFromTolerance(t *testing.T) {
	prev := blockSampleInterval
	blockSampleInterval = time.Millisecond
	defer func() { blockSampleInterval = prev }()

	series := futureSeries(t, []uint64{1, 2, 3})
	ctrl := gomock.NewController(t)
	monitor := NewMockMonitoringData(ctrl)
	monitor.EXPECT().GetNodes().Return([]monitoring.Node{"A"})
	monitor.EXPECT().GetBlockStatus(gomock.Any()).Return(series)

	c := blocksRollingChecker{monitor: monitor, toleranceSamples: 3}
	if err := c.Check(t.Context()); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestBlocksRolling_Configure(t *testing.T) {
	orig := &blocksRollingChecker{toleranceSamples: 5, duration: 2 * time.Second}

	if got := orig.Configure(nil); got != orig {
		t.Errorf("nil config should return the original checker")
	}

	empty := orig.Configure(CheckerConfig{}).(*blocksRollingChecker)
	if empty.toleranceSamples != 5 || empty.duration != 2*time.Second {
		t.Errorf("empty config should copy original values, got %+v", empty)
	}

	set := orig.Configure(CheckerConfig{"tolerance": 7, "duration": int64(time.Millisecond)}).(*blocksRollingChecker)
	if set.toleranceSamples != 7 || set.duration != time.Millisecond {
		t.Errorf("config values not applied, got %+v", set)
	}
}

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
	return createBlockSeriesAt(t, 0, blocks)
}

func futureSeries(t *testing.T, blocks []uint64) monitoring.Series[monitoring.Time, monitoring.BlockStatus] {
	t.Helper()
	base := monitoring.Time(time.Now().UnixNano() + int64(time.Hour))
	return createBlockSeriesAt(t, base, blocks)
}

func createBlockSeriesAt(t *testing.T, base monitoring.Time, blocks []uint64) monitoring.Series[monitoring.Time, monitoring.BlockStatus] {
	t.Helper()

	series := monitoring.SyncedSeries[monitoring.Time, monitoring.BlockStatus]{}
	for i, block := range blocks {
		pos := base + monitoring.Time(i)
		if err := series.Append(pos, monitoring.BlockStatus{BlockHeight: block}); err != nil {
			t.Fatalf("failed to append block %d: %v", block, err)
		}
	}
	return &series
}

func mixedSeries(t *testing.T, past, future []uint64) monitoring.Series[monitoring.Time, monitoring.BlockStatus] {
	t.Helper()

	series := monitoring.SyncedSeries[monitoring.Time, monitoring.BlockStatus]{}
	base := monitoring.Time(time.Now().UnixNano() + int64(time.Hour))
	appendAt := func(offset monitoring.Time, blocks []uint64) {
		for i, block := range blocks {
			if err := series.Append(offset+monitoring.Time(i), monitoring.BlockStatus{BlockHeight: block}); err != nil {
				t.Fatalf("failed to append block %d: %v", block, err)
			}
		}
	}
	appendAt(0, past)
	appendAt(base, future)
	return &series
}
