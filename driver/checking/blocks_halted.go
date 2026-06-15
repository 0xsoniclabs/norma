package checking

import (
	"fmt"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/monitoring"
)

func init() {
	RegisterNetworkCheck("blocks_halted", func(net driver.Network, monitor *monitoring.Monitor) Checker {
		return &blocksHaltedChecker{monitor: &monitoringDataAdapter{monitor}, toleranceSamples: defaultToleranceSamples}
	})
}

// blocksHaltedChecker verifies that no node in the network is producing blocks.
// It checks only the most recent window of data points (tail), so historical
// data from removed nodes does not interfere.
type blocksHaltedChecker struct {
	monitor          MonitoringData
	toleranceSamples int
}

func (c *blocksHaltedChecker) Configure(config CheckerConfig) Checker {
	if config == nil {
		return c
	}

	tolerance := c.toleranceSamples
	if t, exist := config["tolerance"]; exist {
		tolerance = t.(int)
	}

	return &blocksHaltedChecker{
		monitor:          c.monitor,
		toleranceSamples: tolerance,
	}
}

func (c *blocksHaltedChecker) Check() error {
	nodes := c.monitor.GetNodes()
	for _, node := range nodes {
		series := c.monitor.GetBlockStatus(node)

		last := series.GetLatest()
		if last == nil {
			continue
		}

		// Only look at the tail: the last toleranceSamples data points.
		var from monitoring.Time
		if last.Position >= monitoring.Time(c.toleranceSamples) {
			from = last.Position - monitoring.Time(c.toleranceSamples) + 1
		}
		points := series.GetRange(from, last.Position+1)
		if len(points) < 2 {
			continue
		}

		first := points[0].Value.BlockHeight
		latest := points[len(points)-1].Value.BlockHeight
		if latest > first {
			return fmt.Errorf("network is still producing blocks: node %s block height increased from %d to %d in the last %d samples", node, first, latest, len(points))
		}
	}
	return nil
}
