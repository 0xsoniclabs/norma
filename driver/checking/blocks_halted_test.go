package checking

import (
	"context"
	"errors"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/monitoring"
	"go.uber.org/mock/gomock"
)

// haltedWindow is the observation window used by these tests. The bubble's fake
// clock makes it free.
const haltedWindow = 10 * time.Second

func TestBlocksHalted_PassesWhenProductionStoppedBeforeTheCheck(t *testing.T) {
	// Whatever the sampling phase, jitter or dropped reads, a network that
	// stopped producing before the check started must be reported as halted:
	// the check must not look at the production that precedes it. A window
	// holding fewer than two samples of a node cannot show a halt; that case is
	// counted rather than asserted, and required to stay rare.
	conclusive := 0
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

		net.run(30 * time.Second)

		start := time.Now()
		c := &blocksHaltedChecker{
			net: activeNetwork(t, net.GetNodes()...), monitor: net,
			duration: haltedWindow,
		}
		err := c.Check(t.Context())

		end := start.Add(haltedWindow + time.Second)
		if len(net.heightsIn("A", start, end)) < 2 || len(net.heightsIn("B", start, end)) < 2 {
			if err == nil || errors.Is(err, ErrAssertion) {
				t.Errorf("seed %d: decided on too few samples: %v", seed, err)
			}
			return
		}
		conclusive++
		if err != nil {
			t.Errorf("seed %d: unexpected error: %v", seed, err)
		}
	})

	if min := len(seeds()) * 9 / 10; conclusive < min {
		t.Errorf("only %d of %d seeds observed enough samples to conclude, "+
			"want at least %d", conclusive, len(seeds()), min)
	}
}

func TestBlocksHalted_SparseSamplesDoNotShowPreHaltProduction(t *testing.T) {
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

		c := &blocksHaltedChecker{
			net: activeNetwork(t, net.GetNodes()...), monitor: net,
			duration: haltedWindow,
		}
		if err := c.Check(t.Context()); errors.Is(err, ErrAssertion) {
			t.Errorf("reported production after the halt: %v", err)
		}
	})
}

func TestBlocksHalted_IgnoresStoppedNodes(t *testing.T) {
	// After a stopNode the series simply stops growing, and the node is no
	// longer active: it cannot produce, so it is not watched.
	synctest.Test(t, func(t *testing.T) {
		net := newSimulation(t, 1)
		stoppedAt := simulationEpoch.Add(20 * time.Second)
		net.addNode("stopped", nodeBehavior{
			height:          production{blockInterval: 300 * time.Millisecond}.heightAt,
			interval:        time.Second,
			unreachableFrom: stoppedAt,
		})
		net.addNode("halted", nodeBehavior{
			height: production{
				blockInterval: 300 * time.Millisecond,
				haltAt:        stoppedAt,
			}.heightAt,
			interval: time.Second,
		})

		net.run(20 * time.Second)

		c := &blocksHaltedChecker{
			net: activeNetwork(t, "halted"), monitor: net,
			duration: haltedWindow,
		}
		if err := c.Check(t.Context()); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

func TestBlocksHalted_IsUndecidedForAnUnreachableActiveNode(t *testing.T) {
	// A running node that stopped answering may still be producing; its
	// silence does not witness a halt.
	synctest.Test(t, func(t *testing.T) {
		net := newSimulation(t, 1)
		net.addNode("A", nodeBehavior{
			height:          production{blockInterval: 300 * time.Millisecond}.heightAt,
			interval:        time.Second,
			unreachableFrom: simulationEpoch.Add(20 * time.Second),
		})

		net.run(20 * time.Second)

		c := &blocksHaltedChecker{
			net: activeNetwork(t, net.GetNodes()...), monitor: net,
			duration: haltedWindow,
		}
		err := c.Check(t.Context())
		if err == nil || !strings.Contains(err.Error(), "a halt was not observed") {
			t.Errorf("expected an error about the unobserved node, got %v", err)
		}
		if errors.Is(err, ErrAssertion) {
			t.Errorf("an unobserved node must not count as producing: %v", err)
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
		c := &blocksHaltedChecker{
			net: activeNetwork(t, net.GetNodes()...), monitor: net,
			duration: haltedWindow,
		}
		err := c.Check(t.Context())

		observed := net.heightsIn("A", start, start.Add(haltedWindow+time.Second))
		if len(observed) < 2 {
			if err == nil || errors.Is(err, ErrAssertion) {
				t.Errorf("seed %d: decided on %d sample(s): %v",
					seed, len(observed), err)
			}
			return
		}
		conclusive++
		if !errors.Is(err, ErrAssertion) {
			t.Errorf("seed %d: expected an assertion failure, got %v (observed heights %v)",
				seed, err, observed)
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

		c := &blocksHaltedChecker{
			net: activeNetwork(t, net.GetNodes()...), monitor: net,
			duration: haltedWindow,
		}
		err := c.Check(t.Context())
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}
		if !strings.HasSuffix(err.Error(), ": alive") {
			t.Errorf("error does not name exactly the producing node: %v", err)
		}
	})
}

func TestBlocksHalted_FailsWhenANodeIsCatchingUp(t *testing.T) {
	// A node still syncing to the halted head is producing blocks of its own;
	// a strict halt check does not wait for it.
	synctest.Test(t, func(t *testing.T) {
		net := newSimulation(t, 1)
		haltAt := simulationEpoch.Add(10 * time.Second)
		net.addNode("halted", nodeBehavior{
			height: production{
				blockInterval: 300 * time.Millisecond,
				haltAt:        haltAt,
			}.heightAt,
			interval: time.Second,
		})
		net.addNode("syncing", nodeBehavior{
			height: production{
				blockInterval: time.Second,
				haltAt:        simulationEpoch.Add(30 * time.Second),
			}.heightAt,
			interval: time.Second,
		})

		net.run(15 * time.Second)

		c := &blocksHaltedChecker{
			net: activeNetwork(t, net.GetNodes()...), monitor: net,
			duration: haltedWindow,
		}
		err := c.Check(t.Context())
		if !errors.Is(err, ErrAssertion) || !strings.Contains(err.Error(), "syncing") {
			t.Errorf("expected the syncing node to be reported, got %v", err)
		}
	})
}

func TestBlocksHalted_FailsOnARiseHiddenByALaterDrop(t *testing.T) {
	// The height rises and then falls back below where the window started, as
	// when a node restarts on an older database; the endpoints alone would
	// look like a halt.
	heights := map[string][]uint64{
		"rises then returns":  {100, 101, 100},
		"drops then recovers": {100, 1, 2},
	}
	for name, sequence := range heights {
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				net := newSimulation(t, 1)
				start := simulationEpoch.Add(10 * time.Second)
				net.addNode("A", nodeBehavior{
					height: func(at time.Time) uint64 {
						i := int(at.Sub(start) / (3 * time.Second))
						return sequence[max(0, min(i, len(sequence)-1))]
					},
					interval: time.Second,
				})

				net.run(10 * time.Second)

				c := &blocksHaltedChecker{
					net: activeNetwork(t, net.GetNodes()...), monitor: net,
					duration: haltedWindow,
				}
				if err := c.Check(t.Context()); !errors.Is(err, ErrAssertion) {
					t.Errorf("expected the rise to be reported, got %v", err)
				}
			})
		})
	}
}

func TestBlocksHalted_IgnoresNodesExpectedToFail(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		net := newSimulation(t, 1)
		net.addNode("halted", nodeBehavior{
			height: production{
				blockInterval: 300 * time.Millisecond,
				haltAt:        simulationEpoch.Add(5 * time.Second),
			}.heightAt,
			interval: time.Second,
		})

		net.run(10 * time.Second)

		ctrl := gomock.NewController(t)
		failing := driver.NewMockNode(ctrl)
		failing.EXPECT().IsExpectedFailure().AnyTimes().Return(true)
		network := driver.NewMockNetwork(ctrl)
		network.EXPECT().GetActiveNodes().Return(append(
			activeNetwork(t, "halted").GetActiveNodes(), failing,
		))

		c := &blocksHaltedChecker{
			net: network, monitor: net, duration: haltedWindow,
		}
		if err := c.Check(t.Context()); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

func TestBlocksHalted_PassesWhenTheNetworkIsGone(t *testing.T) {
	// All validators stopped: nothing is running that could produce a block.
	synctest.Test(t, func(t *testing.T) {
		c := &blocksHaltedChecker{
			net: activeNetwork(t), monitor: newSimulation(t, 1),
			duration: haltedWindow,
		}
		if err := c.Check(t.Context()); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

func TestBlocksHalted_IsUndecidedForSparselySampledNodes(t *testing.T) {
	// A node whose reads keep failing is sampled far more sparsely than once a
	// second - here every three - so a short window holds a single sample,
	// which cannot show whether the height changed. The network below produces
	// a block every 200ms, so reporting a halt on that one sample would be
	// wrong.
	synctest.Test(t, func(t *testing.T) {
		net := newSimulation(t, 1)
		net.addNode("A", nodeBehavior{
			height:   production{blockInterval: 200 * time.Millisecond}.heightAt,
			interval: 3 * time.Second,
		})

		net.run(6 * time.Second)

		start := time.Now()
		c := NewFailingChecker(&blocksHaltedChecker{
			net: activeNetwork(t, net.GetNodes()...), monitor: net,
			duration: 2 * time.Second,
		})
		err := c.Check(t.Context())
		if observed := net.heightsIn("A", start, time.Now()); len(observed) != 1 {
			t.Fatalf("the window held %d samples, want exactly 1", len(observed))
		}
		// Neither a halt nor production was seen, so even a check expected to
		// fail cannot pass.
		if err == nil || !strings.Contains(err.Error(), "could not decide") {
			t.Errorf("expected an undecided check, got %v", err)
		}
	})
}

func TestBlocksHalted_IsUndecidedForNodesWithoutData(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctrl := gomock.NewController(t)
		monitor := NewMockMonitoringData(ctrl)
		monitor.EXPECT().GetBlockStatus(monitoring.Node("A")).Return(
			&monitoring.SyncedSeries[monitoring.Time, monitoring.BlockStatus]{},
		)
		monitor.EXPECT().GetBlockStatus(monitoring.Node("B")).Return(nil)

		c := &blocksHaltedChecker{
			net: activeNetwork(t, "A", "B"), monitor: monitor,
			duration: haltedWindow,
		}
		err := c.Check(t.Context())
		if err == nil || errors.Is(err, ErrAssertion) {
			t.Errorf("expected an undecided check, got %v", err)
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
		c := &blocksHaltedChecker{
			net: activeNetwork(t, net.GetNodes()...), monitor: net,
			toleranceSamples: 6,
		}
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
			net: activeNetwork(t, "A"), monitor: newSimulation(t, 1),
			duration: haltedWindow,
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
