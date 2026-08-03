package checking

import (
	"context"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0xsoniclabs/norma/driver/monitoring"
	"go.uber.org/mock/gomock"
)

// haltedWindow is the observation window used by these tests. The bubble's fake
// clock makes it free.
const haltedWindow = 10 * time.Second

func TestBlocksHalted_PassesWhenProductionStoppedBeforeTheCheck(t *testing.T) {
	// Whatever the sampling phase, jitter or dropped reads, a network that
	// stopped producing before the check started must be reported as halted:
	// the check must not look at the production that precedes it.
	eachSeed(t, func(t *testing.T, seed int64) {
		net := newSimulation(t, seed)
		haltAt := simulationEpoch.Add(30 * time.Second)
		net.addNode("A", net.randomSampling(production{
			blockInterval: 300 * time.Millisecond,
			haltAt:        haltAt,
		}.heightAt))
		net.addNode("B", net.randomSampling(production{
			blockInterval: 700 * time.Millisecond,
			haltAt:        haltAt,
		}.heightAt))

		net.run(30 * time.Second) // a long history of block production

		c := &blocksHaltedChecker{monitor: net, duration: haltedWindow}
		if err := c.Check(t.Context()); err != nil {
			t.Errorf("seed %d: unexpected error: %v", seed, err)
		}
	})
}

func TestBlocksHalted_PassesWhenSamplesAreSparse(t *testing.T) {
	// Failed reads append nothing, so a fixed number of trailing samples can
	// reach far back in time. With a forward window, a node that only manages
	// a read every few seconds cannot drag pre-halt production into view.
	synctest.Test(t, func(t *testing.T) {
		net := newSimulation(t, 1)
		haltAt := simulationEpoch.Add(60 * time.Second)
		net.addNode("A", nodeBehavior{
			height: production{
				blockInterval: 200 * time.Millisecond,
				haltAt:        haltAt,
			}.heightAt,
			interval: time.Second,
			dropRate: 0.8,
		})

		net.run(60 * time.Second)

		c := &blocksHaltedChecker{monitor: net, duration: haltedWindow}
		if err := c.Check(t.Context()); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

func TestBlocksHalted_PassesWhenNodesBecameUnreachable(t *testing.T) {
	// After a stopNode the series simply stops growing. Nothing is observed
	// during the window, so the network counts as halted.
	synctest.Test(t, func(t *testing.T) {
		net := newSimulation(t, 1)
		stoppedAt := simulationEpoch.Add(20 * time.Second)
		net.addNode("A", nodeBehavior{
			height:          production{blockInterval: 300 * time.Millisecond}.heightAt,
			interval:        time.Second,
			unreachableFrom: stoppedAt,
		})

		net.run(20 * time.Second)

		c := &blocksHaltedChecker{monitor: net, duration: haltedWindow}
		if err := c.Check(t.Context()); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

func TestBlocksHalted_FailsWhileTheNetworkKeepsProducing(t *testing.T) {
	// A network that produces blocks during the window must be reported. A
	// window holding fewer than two samples cannot show a change; that case is
	// counted rather than asserted, and required to stay rare.
	conclusive := 0
	eachSeed(t, func(t *testing.T, seed int64) {
		net := newSimulation(t, seed)
		net.addNode("A", net.randomSampling(
			production{blockInterval: 300 * time.Millisecond}.heightAt,
		))

		net.run(15 * time.Second)

		start := time.Now()
		c := &blocksHaltedChecker{monitor: net, duration: haltedWindow}
		err := c.Check(t.Context())

		observed := net.heightsIn("A", start, start.Add(haltedWindow+time.Second))
		if len(observed) < 2 {
			if err != nil {
				t.Errorf("seed %d: reported production from %d sample(s)",
					seed, len(observed))
			}
			return
		}
		conclusive++
		if err == nil {
			t.Errorf("seed %d: expected an error, observed heights %v",
				seed, observed)
		}
	})

	if min := len(seeds()) * 9 / 10; conclusive < min {
		t.Errorf("only %d of %d seeds observed enough samples to conclude, "+
			"want at least %d", conclusive, len(seeds()), min)
	}
}

func TestBlocksHalted_FailsWhenASingleNodeStillProduces(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		net := newSimulation(t, 1)
		haltAt := simulationEpoch.Add(20 * time.Second)
		net.addNode("halted", nodeBehavior{
			height: production{
				blockInterval: 300 * time.Millisecond,
				haltAt:        haltAt,
			}.heightAt,
			interval: time.Second,
		})
		net.addNode("alive", nodeBehavior{
			height:   production{blockInterval: 300 * time.Millisecond}.heightAt,
			interval: time.Second,
		})

		net.run(20 * time.Second)

		c := &blocksHaltedChecker{monitor: net, duration: haltedWindow}
		err := c.Check(t.Context())
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}
		if !strings.Contains(err.Error(), "alive") {
			t.Errorf("error does not name the producing node: %v", err)
		}
	})
}

func TestBlocksHalted_PassesWhenTheNetworkIsGone(t *testing.T) {
	// All validators stopped: no node is monitored at all.
	synctest.Test(t, func(t *testing.T) {
		net := newSimulation(t, 1)
		c := &blocksHaltedChecker{monitor: net, duration: haltedWindow}
		if err := c.Check(t.Context()); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

func TestBlocksHalted_IgnoresNodesWithoutData(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctrl := gomock.NewController(t)
		monitor := NewMockMonitoringData(ctrl)
		monitor.EXPECT().GetNodes().Return([]monitoring.Node{"A"})
		monitor.EXPECT().GetBlockStatus(gomock.Any()).Return(
			&monitoring.SyncedSeries[monitoring.Time, monitoring.BlockStatus]{},
		)

		c := &blocksHaltedChecker{monitor: monitor, duration: haltedWindow}
		if err := c.Check(t.Context()); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

func TestBlocksHalted_DerivesTheWindowFromTolerance(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		net := newSimulation(t, 1)
		net.addNode("A", nodeBehavior{
			height:   production{blockInterval: 300 * time.Millisecond}.heightAt,
			interval: time.Second,
		})

		net.run(5 * time.Second)

		start := time.Now()
		c := &blocksHaltedChecker{monitor: net, toleranceSamples: 6}
		if err := c.Check(t.Context()); err == nil {
			t.Errorf("expected an error for a producing network")
		}
		if got, want := time.Since(start), 6*time.Second; got != want {
			t.Errorf("observed for %v, want %v derived from the tolerance", got, want)
		}
	})
}

func TestBlocksHalted_RejectsUnusableWindows(t *testing.T) {
	tests := map[string]blocksHaltedChecker{
		"zero tolerance":     {toleranceSamples: 0},
		"negative tolerance": {toleranceSamples: -1},
		"window too short":   {duration: time.Millisecond},
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

func TestBlocksHalted_ReturnsContextError(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		start := time.Now()
		c := &blocksHaltedChecker{
			monitor: newSimulation(t, 1), duration: haltedWindow,
		}
		if err := c.Check(ctx); err == nil {
			t.Errorf("expected a context error, got nil")
		}
		// A checker that ignored the cancellation would wait out the window;
		// on a fake clock that is only visible as elapsed virtual time.
		if elapsed := time.Since(start); elapsed != 0 {
			t.Errorf("waited %v after cancellation, want no wait", elapsed)
		}
	})
}

func TestBlocksHalted_Configure(t *testing.T) {
	orig := &blocksHaltedChecker{toleranceSamples: 5, duration: 2 * time.Second}

	if got := orig.Configure(nil); got != orig {
		t.Errorf("nil config should return the original checker")
	}

	empty := orig.Configure(CheckerConfig{}).(*blocksHaltedChecker)
	if empty.toleranceSamples != 5 || empty.duration != 2*time.Second {
		t.Errorf("empty config should copy original values, got %+v", empty)
	}

	set := orig.Configure(CheckerConfig{
		"tolerance": 7, "duration": int64(20 * time.Second),
	}).(*blocksHaltedChecker)
	if set.toleranceSamples != 7 || set.duration != 20*time.Second {
		t.Errorf("config values not applied, got %+v", set)
	}
}
