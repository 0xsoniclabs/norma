package checking

import (
	"fmt"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/monitoring"
	"github.com/0xsoniclabs/norma/driver/parser"
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
	start            *monitoring.Time // default to 0 if nil
}

// Configure returns a deep copy of the original checker.
// If the config doesn't provide any replacement value, copy from the value of the original.
// If the config is invalid, return error instead.
// If the config is nil, return original checker.
func (c *blocksRollingChecker) Configure(config parser.CheckerConfig) Checker {
	if config == nil {
		return c
	}

	tolerance := c.toleranceSamples
	if t, exist := config["tolerance"]; exist {
		tolerance = t.(int)
	}

	start := c.start
	if s, exist := config["start"]; exist {
		time := monitoring.Time(s.(int))
		start = &time
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
			nodeFunctional = false //node produced no blocks
			continue
		}
		var first monitoring.Time = 0
		if c.start != nil {
			first = *c.start
		}

		items := series.GetRange(first, last.Position)
		window := make([]monitoring.BlockStatus, c.toleranceSamples)
		for i, point := range append(items, *last) {
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
		networkFunctional = networkFunctional || nodeFunctional
	}

	var err error
	if !networkFunctional {
		err = fmt.Errorf("network is down, nodes stopped producing blocks")
	}

	return err
}
