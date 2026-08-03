package checking

import (
	"context"
	"fmt"
	"time"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/monitoring"
)

const defaultToleranceSamples int = 10

// blockSampleInterval is the interval at which the monitor samples the block
// height of each node. It converts an observation window expressed in samples
// into a duration. Var so tests can shorten it.
var blockSampleInterval = time.Second

func init() {
	RegisterNetworkCheck("blocksRolling", func(net driver.Network, monitor *monitoring.Monitor) Checker {
		return &blocksRollingChecker{
			monitor:          &monitoringDataAdapter{monitor},
			toleranceSamples: defaultToleranceSamples,
		}
	})
}

// blocksRollingChecker verifies the network is still producing blocks by
// observing it forward in time for an observation window and requiring at
// least one node to advance its block height while it waits. Progress recorded
// before the check started is ignored, so a network that was alive but halted
// during a transition cannot pass on its own history.
type blocksRollingChecker struct {
	monitor          MonitoringData
	toleranceSamples int
	// duration overrides the observation window; when 0 it is derived from
	// toleranceSamples.
	duration time.Duration
}

// Configure returns a deep copy of the original checker.
// If the config doesn't provide any replacement value, copy from the value of the original.
// If the config is invalid, return error instead.
// If the config is nil, return original checker.
func (c *blocksRollingChecker) Configure(config CheckerConfig) Checker {
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

	return &blocksRollingChecker{
		monitor:          c.monitor,
		toleranceSamples: tolerance,
		duration:         duration,
	}
}

func (c *blocksRollingChecker) Check(ctx context.Context) error {
	window, err := observationWindow(c.duration, c.toleranceSamples)
	if err != nil {
		return err
	}

	// Observing forward in time distinguishes a live network from one halted
	// at check time and ignores history recorded before the check.
	observationStart := monitoring.NewTime(time.Now())
	if err := sleep(ctx, window); err != nil {
		return err
	}

	for _, node := range c.monitor.GetNodes() {
		series := c.monitor.GetBlockStatus(node)
		last := series.GetLatest()
		if last == nil {
			continue
		}
		items := series.GetRange(observationStart, last.Position+1)
		if len(items) == 0 {
			continue
		}
		if items[0].Value.BlockHeight < items[len(items)-1].Value.BlockHeight {
			return nil
		}
	}

	return fmt.Errorf(
		"network is down, no node produced a block during the %s "+
			"observation window", window,
	)
}
