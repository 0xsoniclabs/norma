package checking

import (
	"context"
	"fmt"
	"time"

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
	// duration, when > 0, switches the checker into live-observation mode:
	// Check blocks for that long and then verifies that at least one node
	// advanced its block height during the observation window.
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

	// Two evaluation modes are supported:
	//   * duration == 0 (default): a sliding window of size 'toleranceSamples'
	//     is walked across the entire recorded series; the node is considered
	//     functional if every window shows a strict block-height increase from
	//     its oldest to its newest sample.
	//   * duration > 0 (live-observation mode): mark the current time,
	//     sleep for the configured duration, then require that at least one
	//     node produced new blocks during the observation window. This models
	//     the intuitive question "is the network making progress right now?"
	//     without depending on samples recorded prior to this check.
	var observationStart monitoring.Time
	if c.duration > 0 {
		observationStart = monitoring.Time(time.Now().UnixNano())
		timer := time.NewTimer(c.duration)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
	}

	// This function iterates through all nodes in the network and verifies whether their block height increases.
	// A node with a stagnant block height indicates it is not actively participating in block production.
	// If no nodes are found to be producing blocks, the network is deemed non-functional.
	//
	// The test ensures that at least one node is generating blocks, confirming that the network is operational
	// to some extent. It does not verify the functionality of every node, as that is handled by other checks.
	var networkFunctional bool
	for _, node := range c.monitor.GetNodes() {
		nodeFunctional := true
		series := c.monitor.GetBlockStatus(node)

		last := series.GetLatest()
		if last == nil {
			//node produced no blocks
			continue
		}

		if c.duration > 0 {
			// Live-observation mode: find the first sample recorded after the
			// observation window started and compare its block height to the
			// latest sample. Missing new samples counts as no progress.
			nodeFunctional = false
			for _, dp := range series.GetRange(0, last.Position+1) {
				if dp.Position >= observationStart {
					if dp.Value.BlockHeight < last.Value.BlockHeight {
						nodeFunctional = true
					}
					break
				}
			}
		} else {
			// Sliding-window mode over the entire history.
			items := series.GetRange(0, last.Position+1)
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

	if !networkFunctional {
		return fmt.Errorf("network is down, nodes stopped producing blocks")
	}
	return nil
}
