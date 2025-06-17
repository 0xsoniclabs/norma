package checking

import (
	"fmt"
	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/monitoring"
	nodemon "github.com/0xsoniclabs/norma/driver/monitoring/node"
)

// the longest amount of sampling before considering stagnation
const defaultTolerance = 10

//go:generate mockgen -source blocks_rolling.go -destination blocks_rolling_mock.go -package checking

func NewBlockRollingChecker(toleranceSampleSize uint8) Factory {
	return func(net driver.Network, monitor *monitoring.Monitor) Checker {
		return &blocksRollingChecker{monitor: &monitoringDataAdapter{monitor}, toleranceSamples: 10}
	}
}

// MonitoringData is an interface that defines a method to get monitoring data related to this checker.
type MonitoringData interface {
	// GetNodes returns the nodes that are being monitored.
	GetNodes() []monitoring.Node
	// GetData returns the monitoring data for a specific node.
	GetData(monitoring.Node) monitoring.Series[monitoring.Time, monitoring.BlockStatus]
}

// MonitoringDataAdapter is an adapter that implements the MonitoringData interface
type monitoringDataAdapter struct {
	monitor *monitoring.Monitor
}

func (m *monitoringDataAdapter) GetNodes() []monitoring.Node {
	return monitoring.GetSubjects(m.monitor, nodemon.NodeBlockStatus)
}
func (m *monitoringDataAdapter) GetData(node monitoring.Node) monitoring.Series[monitoring.Time, monitoring.BlockStatus] {
	data, _ := monitoring.GetData(m.monitor, node, nodemon.NodeBlockStatus)
	return data
}

// blocksRollingChecker is a Checker checking if all nodes keeps producing blocks.
type blocksRollingChecker struct {
	monitor          MonitoringData
	toleranceSamples int
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
		series := c.monitor.GetData(node)
		last := series.GetLatest()
		if last == nil {
			nodeFunctional = false //node produced no blocks
			continue
		}
		items := series.GetRange(0, last.Position)
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
