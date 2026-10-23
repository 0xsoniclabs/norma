package checking

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/monitoring"
)

func init() {
	RegisterNetworkCheck("blocksHalted", func(net driver.Network, monitor *monitoring.Monitor) Checker {
		return &blocksHaltedChecker{
			net:              net,
			monitor:          &monitoringDataAdapter{monitor},
			toleranceSamples: defaultToleranceSamples,
		}
	})
}

// blocksHaltedChecker verifies that no node in the network is producing blocks.
// It observes the network forward in time: it waits for an observation window
// and then requires that every active node not expected to fail was seen
// keeping its block height while it waited. Only samples collected during that
// window are considered, so block production that happened before the check
// started - for instance in the seconds a stopNode step needs to actually bring
// the network down - cannot make the check fail. A node catching up is still
// producing, and fails it.
type blocksHaltedChecker struct {
	net              driver.Network
	monitor          MonitoringData
	toleranceSamples int
	// duration overrides the observation window; when 0 it is derived from
	// toleranceSamples.
	duration time.Duration
}

func (c *blocksHaltedChecker) Configure(config CheckerConfig) Checker {
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

	return &blocksHaltedChecker{
		net:              c.net,
		monitor:          c.monitor,
		toleranceSamples: tolerance,
		duration:         duration,
	}
}

func (c *blocksHaltedChecker) Check(ctx context.Context) error {
	window, err := observationWindow(c.duration, c.toleranceSamples)
	if err != nil {
		return err
	}

	// Stopped nodes cannot produce blocks, so they are not watched; with none
	// running at all, the network is halted.
	var nodes []monitoring.Node
	for _, n := range c.net.GetActiveNodes() {
		if !n.IsExpectedFailure() {
			nodes = append(nodes, monitoring.Node(n.GetLabel()))
		}
	}

	observationStart := monitoring.NewTime(time.Now())
	if err := sleep(ctx, window); err != nil {
		return err
	}

	var producing, unobserved []string
	for _, node := range nodes {
		switch progress(c.monitor.GetBlockStatus(node), observationStart) {
		case advancedProgress:
			producing = append(producing, string(node))
		case unobservedProgress:
			unobserved = append(unobserved, string(node))
		}
	}
	slices.Sort(producing)
	slices.Sort(unobserved)
	if len(producing) > 0 {
		return assertionf(
			"network is still producing blocks: %d of %d nodes advanced "+
				"during the %s observation window: %s", len(producing),
			len(nodes), window, strings.Join(producing, ", "),
		)
	}
	if len(unobserved) > 0 {
		return fmt.Errorf(
			"%d of %d nodes reported fewer than %d block heights during the "+
				"%s observation window, so a halt was not observed: %s",
			len(unobserved), len(nodes), MinObservationSamples, window,
			strings.Join(unobserved, ", "),
		)
	}
	return nil
}
