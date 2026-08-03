package checking

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0xsoniclabs/norma/driver/monitoring"
	"go.uber.org/mock/gomock"
)

// rollingWindow is the observation window used by these tests. The bubble's
// fake clock makes it free.
const rollingWindow = 10 * time.Second

func TestBlocksRolling_PassesWhileTheNetworkProduces(t *testing.T) {
	// Whatever the sampling phase, jitter or dropped reads, a network that
	// keeps producing during the window must be reported as alive. A window
	// holding fewer than two samples cannot show progress; that case is
	// counted rather than asserted, and required to stay rare.
	conclusive := 0
	eachSeed(t, func(t *testing.T, seed int64) {
		net := newSimulation(t, seed)
		net.addNode("A", net.randomSampling(
			production{blockInterval: 300 * time.Millisecond}.heightAt,
		))

		net.run(15 * time.Second)

		start := time.Now()
		c := &blocksRollingChecker{
			monitor: net, duration: rollingWindow,
		}
		err := c.Check(t.Context())

		observed := net.heightsIn("A", start, start.Add(rollingWindow+time.Second))
		if len(observed) < 2 {
			if err == nil {
				t.Errorf("seed %d: reported progress from %d sample(s)",
					seed, len(observed))
			}
			return
		}
		conclusive++
		if err != nil {
			t.Errorf("seed %d: unexpected error: %v (observed heights %v)",
				seed, err, observed)
		}
	})

	if min := len(seeds()) * 9 / 10; conclusive < min {
		t.Errorf("only %d of %d seeds observed enough samples to conclude, "+
			"want at least %d", conclusive, len(seeds()), min)
	}
}

func TestBlocksRolling_FailsWhenProductionStoppedBeforeTheCheck(t *testing.T) {
	// A network that produced blocks right up to the start of the check, and
	// then halted, must not pass on that history.
	eachSeed(t, func(t *testing.T, seed int64) {
		net := newSimulation(t, seed)
		haltAt := simulationEpoch.Add(30 * time.Second)
		net.addNode("A", net.randomSampling(production{
			blockInterval: 300 * time.Millisecond,
			haltAt:        haltAt,
		}.heightAt))

		net.run(30 * time.Second)

		c := &blocksRollingChecker{
			monitor: net, duration: rollingWindow,
		}
		if err := c.Check(t.Context()); err == nil {
			t.Errorf("seed %d: expected an error, got nil", seed)
		}
	})
}

func TestBlocksRolling_PassesWhenProductionResumesDuringTheWindow(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		net := newSimulation(t, 1)
		net.addNode("A", nodeBehavior{
			height: production{
				blockInterval: 300 * time.Millisecond,
				haltAt:        simulationEpoch.Add(10 * time.Second),
				resumeAt:      simulationEpoch.Add(22 * time.Second),
			}.heightAt,
			interval: time.Second,
		})

		net.run(20 * time.Second)

		c := &blocksRollingChecker{
			monitor: net, duration: rollingWindow,
		}
		if err := c.Check(t.Context()); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

func TestBlocksRolling_PassesWhenAnyNodeProduces(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		net := newSimulation(t, 1)
		net.addNode("halted", nodeBehavior{
			height: production{
				blockInterval: 300 * time.Millisecond,
				haltAt:        simulationEpoch.Add(5 * time.Second),
			}.heightAt,
			interval: time.Second,
		})
		net.addNode("alive", nodeBehavior{
			height:   production{blockInterval: 300 * time.Millisecond}.heightAt,
			interval: time.Second,
		})

		net.run(10 * time.Second)

		c := &blocksRollingChecker{
			monitor: net, duration: rollingWindow,
		}
		if err := c.Check(t.Context()); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

func TestBlocksRolling_FailsWhenAllNodesAreHalted(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		net := newSimulation(t, 1)
		haltAt := simulationEpoch.Add(5 * time.Second)
		for _, label := range []monitoring.Node{"A", "B"} {
			net.addNode(label, nodeBehavior{
				height: production{
					blockInterval: 300 * time.Millisecond,
					haltAt:        haltAt,
				}.heightAt,
				interval: time.Second,
			})
		}

		net.run(10 * time.Second)

		c := &blocksRollingChecker{
			monitor: net, duration: rollingWindow,
		}
		if err := c.Check(t.Context()); err == nil {
			t.Errorf("expected an error, got nil")
		}
	})
}

func TestBlocksRolling_FailsWhenNodesStoppedReporting(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		net := newSimulation(t, 1)
		net.addNode("A", nodeBehavior{
			height:          production{blockInterval: 300 * time.Millisecond}.heightAt,
			interval:        time.Second,
			unreachableFrom: simulationEpoch.Add(10 * time.Second),
		})

		net.run(10 * time.Second)

		c := &blocksRollingChecker{
			monitor: net, duration: rollingWindow,
		}
		if err := c.Check(t.Context()); err == nil {
			t.Errorf("expected an error, got nil")
		}
	})
}

func TestBlocksRolling_FailsWithoutAnyMonitoredNode(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := &blocksRollingChecker{
			monitor: newSimulation(t, 1), duration: rollingWindow,
		}
		if err := c.Check(t.Context()); err == nil {
			t.Errorf("expected an error, got nil")
		}
	})
}

func TestBlocksRolling_IgnoresNodesWithoutData(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctrl := gomock.NewController(t)
		monitor := NewMockMonitoringData(ctrl)
		monitor.EXPECT().GetNodes().Return([]monitoring.Node{"A"})
		monitor.EXPECT().GetBlockStatus(gomock.Any()).Return(
			&monitoring.SyncedSeries[monitoring.Time, monitoring.BlockStatus]{},
		)

		c := &blocksRollingChecker{
			monitor: monitor, duration: rollingWindow,
		}
		if err := c.Check(t.Context()); err == nil {
			t.Errorf("expected an error, got nil")
		}
	})
}

func TestBlocksRolling_DerivesTheWindowFromTolerance(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		net := newSimulation(t, 1)
		net.addNode("A", nodeBehavior{
			height:   production{blockInterval: 300 * time.Millisecond}.heightAt,
			interval: time.Second,
		})

		start := time.Now()
		c := &blocksRollingChecker{
			monitor: net, toleranceSamples: 6,
		}
		if err := c.Check(t.Context()); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if got, want := time.Since(start), 6*time.Second; got != want {
			t.Errorf("observed for %v, want %v derived from the tolerance", got, want)
		}
	})
}

func TestBlocksRolling_RejectsUnusableWindows(t *testing.T) {
	tests := map[string]blocksRollingChecker{
		"zero tolerance":     {toleranceSamples: 0},
		"negative tolerance": {toleranceSamples: -1},
		// A window shorter than two sample intervals could never observe a
		// change and would report every network as down.
		"window too short": {duration: time.Millisecond},
	}
	for name, checker := range tests {
		// No bubble needed: an unusable window is rejected before any wait.
		t.Run(name, func(t *testing.T) {
			if err := checker.Check(t.Context()); err == nil {
				t.Errorf("expected an error, got nil")
			}
		})
	}
}

func TestBlocksRolling_ReturnsContextError(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		start := time.Now()
		c := &blocksRollingChecker{
			monitor: newSimulation(t, 1), duration: rollingWindow,
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

func TestBlocksRolling_Configure(t *testing.T) {
	orig := &blocksRollingChecker{toleranceSamples: 5, duration: 2 * time.Second}

	if got := orig.Configure(nil); got != orig {
		t.Errorf("nil config should return the original checker")
	}

	empty := orig.Configure(CheckerConfig{}).(*blocksRollingChecker)
	if empty.toleranceSamples != 5 || empty.duration != 2*time.Second {
		t.Errorf("empty config should copy original values, got %+v", empty)
	}

	set := orig.Configure(CheckerConfig{
		"tolerance": 7, "duration": int64(20 * time.Second),
	}).(*blocksRollingChecker)
	if set.toleranceSamples != 7 || set.duration != 20*time.Second {
		t.Errorf("config values not applied, got %+v", set)
	}
}
