// Copyright 2024 Fantom Foundation
// This file is part of Norma System Testing Infrastructure for Sonic.
//
// Norma is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// Norma is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with Norma. If not, see <http://www.gnu.org/licenses/>.

package checking

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/monitoring"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

// defaultPeerCountTimeout bounds how long Check waits for the node's peer
// count to enter the accepted interval. Peer connections are established
// asynchronously, so a count that is still short right after a (re)start is
// not yet a failure.
const defaultPeerCountTimeout = 30 * time.Second

// peerCountPollInterval is the delay between convergence polls. Var so
// tests can shorten it.
var peerCountPollInterval = 500 * time.Millisecond

func init() {
	RegisterNetworkCheck("peerCount",
		func(net driver.Network, _ *monitoring.Monitor) Checker {
			return &peerCountChecker{
				net:     net,
				timeout: defaultPeerCountTimeout,
			}
		})
}

// peerCountChecker verifies that the number of p2p peers of a single node,
// as reported by net_peerCount, lies within the configured interval. It is
// the sharpest observable for peer wiring experiments: a min bound proves a
// node was found by (or found) the network, a max of zero proves it is
// isolated.
type peerCountChecker struct {
	net  driver.Network
	node string
	min  *int
	max  *int
	// timeout bounds convergence polling. Zero means a single attempt.
	timeout time.Duration
}

func (c *peerCountChecker) Configure(config CheckerConfig) Checker {
	configured := *c
	if v, ok := config["node"].(string); ok {
		configured.node = v
	}
	if v, ok := config["min"].(int); ok {
		configured.min = &v
	}
	if v, ok := config["max"].(int); ok {
		configured.max = &v
	}
	return &configured
}

func (c *peerCountChecker) Check(ctx context.Context) error {
	if c.node == "" {
		return fmt.Errorf("peerCount check requires a node")
	}
	if c.min == nil && c.max == nil {
		return fmt.Errorf("peerCount check requires at least one of min and max")
	}
	node, err := findNodeByLabel(c.net, c.node)
	if err != nil {
		return err
	}

	deadline := time.Now().Add(c.timeout)
	ticker := time.NewTicker(peerCountPollInterval)
	defer ticker.Stop()

	for {
		count, err := getPeerCount(ctx, node)
		if err == nil {
			err = c.verifyBounds(count)
			if err == nil {
				slog.Info("peer count in bounds", "node", c.node, "peers", count)
				return nil
			}
		}
		// A non-positive timeout leaves the deadline in the past, which makes
		// this the single attempt such a checker is configured for.
		if !time.Now().Before(deadline) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// verifyBounds fails when the given peer count lies outside the configured
// interval.
func (c *peerCountChecker) verifyBounds(count uint64) error {
	if c.min != nil && count < uint64(*c.min) {
		return fmt.Errorf(
			"node %s has %d peers, expected at least %d", c.node, count, *c.min)
	}
	if c.max != nil && count > uint64(*c.max) {
		return fmt.Errorf(
			"node %s has %d peers, expected at most %d", c.node, count, *c.max)
	}
	return nil
}

// findNodeByLabel returns the active node with the given label.
func findNodeByLabel(net driver.Network, label string) (driver.Node, error) {
	for _, n := range net.GetActiveNodes() {
		if n.GetLabel() == label {
			return n, nil
		}
	}
	return nil, fmt.Errorf("node %q not found among active nodes", label)
}

// getPeerCount reads the node's current p2p peer count via net_peerCount.
func getPeerCount(ctx context.Context, node driver.Node) (uint64, error) {
	client, err := node.DialRpc(ctx)
	if err != nil {
		return 0, err
	}
	defer client.Close()
	var count hexutil.Uint64
	if err := client.Call(&count, "net_peerCount"); err != nil {
		return 0, err
	}
	return uint64(count), nil
}
