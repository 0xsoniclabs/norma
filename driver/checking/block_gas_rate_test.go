package checking

import (
	"context"
	"errors"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0xsoniclabs/norma/driver/monitoring"
	"go.uber.org/mock/gomock"
)

// gasRateWindow is the observation window used by these tests. The bubble's
// fake clock makes it free.
const gasRateWindow = 10 * time.Second

// gasRateSimulation returns a producing network that records gasRate for every
// block, together with a switch that changes the rate recorded from then on.
func gasRateSimulation(
	t *testing.T, seed int64, before, after float64,
) (*simulatedNetwork, func()) {
	t.Helper()

	net := newSimulation(t, seed)
	net.addNode("A", net.randomSampling(
		production{blockInterval: 300 * time.Millisecond}.heightAt,
	))

	rate := before
	net.withGasRates(func(uint64) float64 { return rate })
	return net, func() { rate = after }
}

func TestBlocksGasRate_IgnoresBlocksProducedBeforeTheCheck(t *testing.T) {
	// A gas rate spike that happened before the check started must not fail
	// it - nor every later check in the same scenario.
	eachSeed(t, func(t *testing.T, seed int64) {
		net, switchToLowRate := gasRateSimulation(t, seed, 500, 10)

		net.run(20 * time.Second) // history recorded well above the ceiling
		switchToLowRate()

		c := &blockGasRateChecker{
			monitor: net, ceiling: 30,
			duration: gasRateWindow,
		}
		if err := c.Check(t.Context()); err != nil {
			t.Errorf("seed %d: unexpected error: %v", seed, err)
		}
	})
}

func TestBlocksGasRate_FailsOnBlocksProducedDuringTheWindow(t *testing.T) {
	eachSeed(t, func(t *testing.T, seed int64) {
		net, switchToHighRate := gasRateSimulation(t, seed, 10, 500)

		net.run(20 * time.Second)
		switchToHighRate()

		c := &blockGasRateChecker{
			monitor: net, ceiling: 30,
			duration: gasRateWindow,
		}
		err := c.Check(t.Context())
		if err == nil {
			t.Errorf("seed %d: expected an error, got nil", seed)
			return
		}
		if !strings.Contains(err.Error(), "exceeded gas ceiling") {
			t.Errorf("seed %d: unexpected error: %v", seed, err)
		}
	})
}

func TestBlocksGasRate_PassesWhenBlocksStayBelowTheCeiling(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		net, _ := gasRateSimulation(t, 1, 30, 30) // exactly at the ceiling

		net.run(10 * time.Second)

		c := &blockGasRateChecker{
			monitor: net, ceiling: 30, duration: gasRateWindow,
		}
		if err := c.Check(t.Context()); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

func TestBlocksGasRate_PassesWhenNoBlockIsProduced(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		net := newSimulation(t, 1)
		net.addNode("A", nodeBehavior{
			height: production{
				blockInterval: 300 * time.Millisecond,
				haltAt:        simulationEpoch.Add(10 * time.Second),
			}.heightAt,
			interval: time.Second,
		})
		net.withGasRates(func(uint64) float64 { return 500 })

		net.run(10 * time.Second)

		// The network is halted, so the window contains no block at all and there
		// is nothing to compare against the ceiling.
		c := &blockGasRateChecker{
			monitor: net, ceiling: 30, duration: gasRateWindow,
		}
		if err := c.Check(t.Context()); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

func TestBlocksGasRate_PassesOnAnEmptySeries(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctrl := gomock.NewController(t)
		monitor := NewMockMonitoringData(ctrl)
		monitor.EXPECT().GetBlockGasRate().Return(
			&monitoring.SyncedSeries[monitoring.BlockNumber, float64]{},
		).AnyTimes()

		c := &blockGasRateChecker{
			monitor: monitor, ceiling: 30,
			duration: gasRateWindow,
		}
		if err := c.Check(t.Context()); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

func TestBlocksGasRate_RejectsUnusableWindows(t *testing.T) {
	tests := map[string]blockGasRateChecker{
		"zero tolerance":     {toleranceSamples: 0},
		"negative tolerance": {toleranceSamples: -1},
		"window too short":   {duration: time.Millisecond},
	}
	for name, checker := range tests {
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				checker.monitor = newSimulation(t, 1)
				if err := checker.Check(t.Context()); err == nil {
					t.Errorf("expected an error, got nil")
				}
			})
		})
	}
}

func TestBlocksGasRate_ReturnsContextError(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		start := time.Now()
		c := &blockGasRateChecker{
			monitor: newSimulation(t, 1), ceiling: 30,
			duration: gasRateWindow,
		}
		if err := c.Check(ctx); !errors.Is(err, context.Canceled) {
			t.Errorf("expected a context error, got %v", err)
		}
		// A checker that ignored the cancellation would wait out the window;
		// on a fake clock that is only visible as elapsed virtual time.
		if elapsed := time.Since(start); elapsed != 0 {
			t.Errorf("waited %v after cancellation, want no wait", elapsed)
		}
	})
}

func TestBlocksGasRate_Configure(t *testing.T) {
	orig := &blockGasRateChecker{
		ceiling: 30, toleranceSamples: 5, duration: 20 * time.Second,
	}

	if got := orig.Configure(nil); got != orig {
		t.Errorf("nil config should return the original checker")
	}

	empty := orig.Configure(CheckerConfig{}).(*blockGasRateChecker)
	if empty.ceiling != 30 || empty.toleranceSamples != 5 ||
		empty.duration != 20*time.Second {
		t.Errorf("empty config should copy original values, got %+v", empty)
	}

	set := orig.Configure(CheckerConfig{
		"ceiling": 50, "tolerance": 7, "duration": int64(30 * time.Second),
	}).(*blockGasRateChecker)
	if set.ceiling != 50 || set.toleranceSamples != 7 ||
		set.duration != 30*time.Second {
		t.Errorf("config values not applied, got %+v", set)
	}
}

func TestBlocksGasRate_ConfiguredCeilingIsApplied(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		net, switchRate := gasRateSimulation(t, 1, 10, 40)
		net.run(10 * time.Second)
		switchRate()

		strict := &blockGasRateChecker{
			monitor: net, ceiling: 30, duration: gasRateWindow,
		}
		relaxed := strict.Configure(CheckerConfig{"ceiling": 50})

		// The relaxed copy observes its own window, after the strict one; the
		// simulated network keeps producing blocks at the same rate throughout.
		if err := strict.Check(t.Context()); err == nil {
			t.Errorf("expected an error for a ceiling of 30")
		}
		if err := relaxed.Check(t.Context()); err != nil {
			t.Errorf("unexpected error for a ceiling of 50: %v", err)
		}
	})
}
