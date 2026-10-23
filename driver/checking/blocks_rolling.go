package checking

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/monitoring"
)

const defaultToleranceSamples int = 10

// blockSampleInterval is the interval at which the monitor samples the block
// height of each node. It converts an observation window expressed in samples
// into a duration.
const blockSampleInterval = time.Second

func init() {
	RegisterNetworkCheck("blocksRolling", func(net driver.Network, monitor *monitoring.Monitor) Checker {
		return &blocksRollingChecker{
			net:              net,
			monitor:          &monitoringDataAdapter{monitor},
			toleranceSamples: defaultToleranceSamples,
		}
	})
}

// blocksRollingChecker verifies every node keeps producing blocks by observing
// the network forward in time for an observation window and requiring each
// active node not expected to fail to advance its block height while it waits.
// Progress recorded before the check started is ignored, so a network that was
// alive but halted during a transition cannot pass on its own history, and a
// single stuck node fails the check even while the others progress.
type blocksRollingChecker struct {
	net              driver.Network
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
		net:              c.net,
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

	var nodes []monitoring.Node
	for _, n := range c.net.GetActiveNodes() {
		if !n.IsExpectedFailure() {
			nodes = append(nodes, monitoring.Node(n.GetLabel()))
		}
	}
	if len(nodes) == 0 {
		return errors.New("no active node is expected to produce blocks")
	}

	// Observing forward in time distinguishes a live network from one halted
	// at check time and ignores history recorded before the check.
	observationStart := monitoring.NewTime(time.Now())
	if err := sleep(ctx, window); err != nil {
		return err
	}

	var stalled []string
	for _, node := range nodes {
		if !advanced(c.monitor.GetBlockStatus(node), observationStart) {
			stalled = append(stalled, string(node))
		}
	}
	if len(stalled) > 0 {
		slices.Sort(stalled)
		return assertionf(
			"%d of %d nodes produced no block during the %s observation "+
				"window: %s", len(stalled), len(nodes), window,
			strings.Join(stalled, ", "),
		)
	}
	return nil
}

// advanced reports whether the series shows the block height rising from
// start on.
func advanced(
	series monitoring.Series[monitoring.Time, monitoring.BlockStatus],
	start monitoring.Time,
) bool {
	if series == nil {
		return false
	}
	last := series.GetLatest()
	if last == nil {
		return false
	}
	items := series.GetRange(start, last.Position+1)
	return len(items) > 0 &&
		items[0].Value.BlockHeight < items[len(items)-1].Value.BlockHeight
}
