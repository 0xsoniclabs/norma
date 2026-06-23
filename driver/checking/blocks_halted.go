package checking

import (
	"fmt"
	"time"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/monitoring"
)

func init() {
	RegisterNetworkCheck("blocks_halted", func(net driver.Network, monitor *monitoring.Monitor) Checker {
		return &blocksHaltedChecker{
			monitor:          &monitoringDataAdapter{monitor},
			toleranceSamples: defaultToleranceSamples,
			now:              time.Now,
		}
	})
}

// blocksHaltedChecker verifies that no node in the network is producing blocks.
// It checks only the most recent window of data points (tail), so historical
// data from removed nodes does not interfere.
type blocksHaltedChecker struct {
	monitor          MonitoringData
	toleranceSamples int
	now              func() time.Time
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
		now:              c.now,
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

		// If monitoring has not received new data for longer than the
		// tolerance window, the node is no longer reachable and is
		// considered halted (e.g. after stopNode).
		staleness := c.now().Sub(last.Position.Time())
		if staleness > time.Duration(c.toleranceSamples)*time.Second {
			continue
		}

		// Fetch all data points and examine the tail.
		allPoints := series.GetRange(0, last.Position+1)
		if len(allPoints) < 2 {
			continue
		}

		// Only look at the last toleranceSamples data points.
		start := len(allPoints) - c.toleranceSamples
		if start < 0 {
			start = 0
		}
		points := allPoints[start:]

		// The network is halted only if block height has not increased
		// between any two consecutive samples in the window. A single
		// increase anywhere means blocks are still being produced.
		halted := true
		for i := 1; i < len(points); i++ {
			if points[i].Value.BlockHeight > points[i-1].Value.BlockHeight {
				halted = false
				break
			}
		}
		if !halted {
			return fmt.Errorf("network is still producing blocks: node %s block height increased from %d to %d in the last %d samples", node, points[0].Value.BlockHeight, points[len(points)-1].Value.BlockHeight, len(points))
		}
	}
	return nil
}
