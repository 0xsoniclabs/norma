package checking

import (
	"context"
	"fmt"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/monitoring"
)

const defaultToleranceSamples int = 10

func init() {
	RegisterNetworkCheck("blocksRolling", func(net driver.Network, monitor *monitoring.Monitor) Checker {
		return &blocksRollingChecker{monitor: &monitoringDataAdapter{monitor}, toleranceSamples: defaultToleranceSamples}
	})
}

// blocksRollingChecker is a Checker checking if all nodes keeps producing blocks.
type blocksRollingChecker struct {
	monitor          MonitoringData
	toleranceSamples int
	start            monitoring.Time
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

	start := c.start
	if s, exist := config["start"]; exist {
		start = monitoring.Time(s.(int64))
	}

	return &blocksRollingChecker{
		monitor:          c.monitor,
		toleranceSamples: tolerance,
		start:            start,
	}
}

func (c *blocksRollingChecker) Check(ctx context.Context) error {
	if c.toleranceSamples <= 0 {
		return fmt.Errorf("tolerance must be > 0, got %d", c.toleranceSamples)
	}
	nodes := c.monitor.GetNodes()
	// This function iterates through all nodes in the network and verifies whether their block height increases.
	// A node with a stagnant block height indicates it is not actively participating in block production.
	// If no nodes are found to be producing blocks, the network is deemed non-functional.
	//
	// The test ensures that at least one node is generating blocks, confirming that the network is operational
	// to some extent. It does not verify the functionality of every node, as that is handled by other checks.
	//
	// Two evaluation modes are supported:
	//   * start == 0 (default): a sliding window of size 'toleranceSamples' is
	//     walked across the entire series; the node is functional if every
	//     window shows a strict block-height increase from its oldest to its
	//     newest sample.
	//   * start > 0: the user has already narrowed the window of interest to
	//     samples with Position >= start. The node is considered functional if
	//     the block height at the first in-range sample is strictly less than
	//     the block height of the latest sample. This models the intuitive
	//     question "did the network make progress during the last N seconds?"
	//     and is robust against transient stalls inside that window.
	var networkFunctional bool
	for _, node := range nodes {
		nodeFunctional := true
		series := c.monitor.GetBlockStatus(node)

		last := series.GetLatest()
		if last == nil {
			//node produced no blocks
			continue
		}

		var first monitoring.Time = 0
		found := false
		dataPoints := series.GetRange(0, last.Position+1)
		for _, dp := range dataPoints {
			// >= to account for various tick configuration
			// example: if ticked at 0, 5, 10, ... and c.start = 8,
			// the first tick that is bigger, 10, is selected instead.
			if dp.Position >= c.start {
				first = dp.Position
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("start %d not found", c.start)
		}

		items := append(series.GetRange(first, last.Position), *last)
		if c.start > 0 {
			// Windowed mode: compare the first in-range sample to the latest.
			if items[0].Value.BlockHeight >= last.Value.BlockHeight {
				nodeFunctional = false
			}
		} else {
			// Sliding-window mode over the entire history.
			window := make([]monitoring.BlockStatus, c.toleranceSamples)
			for i, point := range items {
				window[i%c.toleranceSamples] = point.Value
				if i < c.toleranceSamples-1 {
					continue
				}
				prev := (i - c.toleranceSamples + 1) % c.toleranceSamples
				if window[prev].BlockHeight >= point.Value.BlockHeight {
					nodeFunctional = false
					break
				}
			}
		}

		networkFunctional = networkFunctional || nodeFunctional
	}

	var err error
	if !networkFunctional {
		err = fmt.Errorf("network is down, nodes stopped producing blocks")
	}

	return err
}
