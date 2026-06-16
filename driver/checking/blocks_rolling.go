package checking

import (
	"fmt"
	"log/slog"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/monitoring"
)

const defaultToleranceSamples int = 10

func init() {
	RegisterNetworkCheck("blocks_rolling", func(net driver.Network, monitor *monitoring.Monitor) Checker {
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
		start = monitoring.Time(s.(int))
	}

	return &blocksRollingChecker{
		monitor:          c.monitor,
		toleranceSamples: tolerance,
		start:            start,
	}
}

func (c *blocksRollingChecker) Check() error {
	nodes := c.monitor.GetNodes()
	// This function iterates through all nodes in the network and verifies whether their block height increases.
	// A node with a stagnant block height indicates it is not actively participating in block production.
	// If no nodes are found to be producing blocks, the network is deemed non-functional.
	//
	// The test ensures that at least one node is generating blocks, confirming that the network is operational
	// to some extent. It does not verify the functionality of every node, as that is handled by other checks.
	//
	// To account for flexibility in block processing, the verification is performed within a sliding window
	// defined by 'toleranceSamples'. Only the block height at the beginning and end of this window are assessed,
	// allowing for scenarios where blocks may not be produced every second.
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
			if dp.Position >= monitoring.Time(c.start) {
				first = dp.Position
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("start %d not found", c.start)
		}

		items := series.GetRange(first, last.Position) // skip last item
		samples := len(items) + 1                      // include last item appended below
		effectiveTolerance := c.toleranceSamples
		if samples < effectiveTolerance {
			effectiveTolerance = samples - 1
			slog.Warn("blocks_rolling range is too short; check is not reliable",
				"node", node,
				"samples", samples,
				"configured_tolerance", c.toleranceSamples,
				"effective_tolerance", effectiveTolerance,
			)
		}
		if effectiveTolerance < 1 {
			nodeFunctional = false
			networkFunctional = networkFunctional || nodeFunctional
			continue
		}

		window := make([]monitoring.BlockStatus, effectiveTolerance)
		for i, point := range append(items, *last) {
			window[i%effectiveTolerance] = point.Value
			if i < effectiveTolerance-1 {
				continue
			}
			prev := (i - effectiveTolerance + 1) % effectiveTolerance
			if window[prev].BlockHeight >= point.Value.BlockHeight {
				nodeFunctional = false
				break
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
