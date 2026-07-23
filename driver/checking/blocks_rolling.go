package checking

import (
	"context"
	"fmt"
	"time"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/monitoring"
)

const defaultToleranceSamples int = 10

// blockSampleInterval converts a tolerance expressed in samples into an
// observation duration. Var so tests can shorten it.
var blockSampleInterval = time.Second

func init() {
	RegisterNetworkCheck("blocksRolling", func(net driver.Network, monitor *monitoring.Monitor) Checker {
		return &blocksRollingChecker{monitor: &monitoringDataAdapter{monitor}, toleranceSamples: defaultToleranceSamples}
	})
}

// blocksRollingChecker verifies the network is still producing blocks by
// observing for a window and requiring at least one node to advance its
// block height during it.
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
	if c.toleranceSamples <= 0 {
		return fmt.Errorf("tolerance must be > 0, got %d", c.toleranceSamples)
	}

	window := c.duration
	if window <= 0 {
		window = time.Duration(c.toleranceSamples) * blockSampleInterval
	}

	// Observing forward in time distinguishes a live network from one halted
	// at check time and ignores history recorded before the check.
	observationStart := monitoring.Time(time.Now().UnixNano())
	timer := time.NewTimer(window)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
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
		if items[0].Value.BlockHeight < last.Value.BlockHeight {
			return nil
		}
	}

	return fmt.Errorf("network is down, nodes stopped producing blocks")
}
