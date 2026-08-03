package checking

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/monitoring"
)

func init() {
	RegisterNetworkCheck("blocksHalted", func(net driver.Network, monitor *monitoring.Monitor) Checker {
		return &blocksHaltedChecker{
			monitor:          &monitoringDataAdapter{monitor},
			toleranceSamples: defaultToleranceSamples,
		}
	})
}

// blocksHaltedChecker verifies that no node in the network is producing blocks.
// It observes the network forward in time: it waits for an observation window
// and then requires that no node advanced its block height while it waited.
// Only samples collected during that window are considered, so block
// production that happened before the check started - for instance in the
// seconds a stopNode step needs to actually bring the network down - cannot
// make the check fail.
type blocksHaltedChecker struct {
	monitor          MonitoringData
	toleranceSamples int
	// duration overrides the observation window; when 0 it is derived from
	// toleranceSamples.
	duration time.Duration
}

func (c *blocksHaltedChecker) Configure(config CheckerConfig) Checker {
	if config == nil {
		return c
	}

	tolerance := c.toleranceSamples
	if t, exist := config["tolerance"]; exist {
		tolerance = t.(int)
	}

	duration := c.duration
	if d, exist := config["duration"]; exist {
		duration = time.Duration(d.(int64))
	}

	return &blocksHaltedChecker{
		monitor:          c.monitor,
		toleranceSamples: tolerance,
		duration:         duration,
	}
}

func (c *blocksHaltedChecker) Check(ctx context.Context) error {
	window, err := observationWindow(c.duration, c.toleranceSamples)
	if err != nil {
		return err
	}

	observationStart := monitoring.NewTime(time.Now())
	if err := sleep(ctx, window); err != nil {
		return err
	}

	observedNodes := 0
	for _, node := range c.monitor.GetNodes() {
		series := c.monitor.GetBlockStatus(node)
		last := series.GetLatest()
		if last == nil {
			continue
		}

		points := series.GetRange(observationStart, last.Position+1)
		if len(points) == 0 {
			// The node reported nothing while we were observing: it is
			// unreachable or was removed, which is consistent with a halt.
			continue
		}
		observedNodes++

		// A single increase between consecutive samples means the node is
		// still producing blocks.
		for i := 1; i < len(points); i++ {
			if points[i].Value.BlockHeight > points[i-1].Value.BlockHeight {
				return fmt.Errorf(
					"network is still producing blocks: node %s advanced "+
						"from block %d to %d during the %s observation window",
					node,
					points[0].Value.BlockHeight,
					points[len(points)-1].Value.BlockHeight,
					window,
				)
			}
		}
	}

	if observedNodes == 0 {
		// Nothing reported a block height at all. The network is not
		// producing blocks, but we did not actually watch it do so.
		slog.Warn(
			"blocksHalted: no node reported a block height during the "+
				"observation window; the network is considered halted "+
				"because no data was available",
			"window", window,
		)
	}

	return nil
}
