package checking

import (
	"fmt"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/monitoring"
)

const defaultToleranceSamples int = 10

func init() {
	RegisterNetworkCheck("blocks_rolling", func(net driver.Network, monitor *monitoring.Monitor) Checker {
		return &blocksRollingChecker{monitor: &monitoringDataAdapter{monitor}, toleranceSamples: defaultToleranceSamples}
	})
	RegisterNetworkCheck("blocks_rolling_position", func(_ driver.Network, monitor *monitoring.Monitor) Checker {
		return &blocksRollingPositionChecker{monitor: &monitoringDataAdapter{monitor}}
	})
}

// blocksRollingChecker is a Checker checking if all nodes keeps producing blocks.
type blocksRollingChecker struct {
	monitor          MonitoringData
	toleranceSamples int

	// blocksRollingPositionChecker: on Check(), populate the current position of each node
	nodeStartPositions map[monitoring.Node]monitoring.Time
}

// Configure returns a deep copy of the original checker.
// If the config doesn't provide any replacement value, copy from the value of the original.
// If the config is invalid, return error instead.
// If the config is nil, return original checker.
func (c *blocksRollingChecker) Configure(config CheckerConfig) (Checker, error) {
	if config == nil {
		return c, nil
	}

	if val, exist := config["error"]; exist {
		emsg, ok := val.(string)
		if !ok {
			return nil, fmt.Errorf("failed to convert error; %v", val)
		}
		checker, err := &errorChecker{c, emsg}.Configure(config)
		if err != nil {
			return nil, err
		}
		return &errorChecker{checker, emsg}, nil
	}

	tolerance := c.toleranceSamples
	if val, exist := config["tolerance"]; exist {
		t, ok := val.(int)
		if !ok {
			return nil, fmt.Errorf("failed to convert tolerance; %v", val)
		}
		tolerance = t
	}

	return &blocksRollingChecker{
		monitor:          c.monitor,
		toleranceSamples: tolerance,
	}, nil
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

		var first monitoring.Time = 0
		if c.nodeStartPositions != nil {
			if pos, exist := c.nodeStartPositions[node]; exist {
				first = pos
			}
		}
		last := series.GetLatest()
		if last == nil {
			nodeFunctional = false //node produced no blocks
			continue
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

// blocksRollingPositionChecker is a helper Checker that gets the current position for blocks_rolling
type blocksRollingPositionChecker struct {
	monitor MonitoringData
	checker *blocksRollingChecker
}

// Check embeds position into the configured blocksRollingChecker
func (c *blocksRollingPositionChecker) Check() error {
	if c.checker == nil {
		return nil
	}

	nodeStartPositions := make(map[monitoring.Node]monitoring.Time)
	for _, node := range c.monitor.GetNodes() {
		nodeStartPositions[node] = c.monitor.GetBlockStatus(node).GetLatest().Position
	}
	c.checker.nodeStartPositions = nodeStartPositions
	return nil
}

// Configure sets the target blocksRollingChecker
func (c *blocksRollingPositionChecker) Configure(config CheckerConfig) (Checker, error) {
	if config == nil {
		return c, nil
	}

	val, exist := config["target"]
	if !exist {
		return c, nil
	}

	brc, ok := val.(*blocksRollingChecker)
	if !ok {
		return nil, fmt.Errorf("failed to convert blocksRollingChecker; %v", val)
	}

	return &blocksRollingPositionChecker{monitor: c.monitor, checker: brc}, nil
}
