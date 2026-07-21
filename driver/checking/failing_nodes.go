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
	"github.com/0xsoniclabs/norma/driver/rpc"
)

// defaultFailingNodesTimeout bounds how long Check waits for failing nodes to deviate.
const defaultFailingNodesTimeout = 30 * time.Second

// failingNodesPollInterval is the delay between deviation polls. Var so tests can shorten it.
var failingNodesPollInterval = 500 * time.Millisecond

func init() {
	RegisterNetworkCheck("failingNodes", func(net driver.Network, monitor *monitoring.Monitor) Checker {
		return &failingNodesChecker{net: net, slack: defaultSlack, timeout: defaultFailingNodesTimeout}
	})
}

// failingNodesChecker asserts that every node marked as an expected failure
// actually deviated from the healthy majority.
type failingNodesChecker struct {
	net   driver.Network
	slack int
	// timeout bounds deviation polling; zero means a single attempt.
	timeout time.Duration
}

func (c *failingNodesChecker) Configure(config CheckerConfig) Checker {
	if config == nil {
		return c
	}

	slack := c.slack
	if val, exist := config["tolerance"]; exist {
		slack = val.(int)
	}

	return &failingNodesChecker{net: c.net, slack: slack, timeout: c.timeout}
}

func (c *failingNodesChecker) Check(ctx context.Context) error {
	if c.timeout <= 0 {
		return c.checkOnce(ctx)
	}

	deadline := time.Now().Add(c.timeout)
	ticker := time.NewTicker(failingNodesPollInterval)
	defer ticker.Stop()

	for {
		err := c.checkOnce(ctx)
		if err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (c *failingNodesChecker) checkOnce(ctx context.Context) error {
	nodes := c.net.GetActiveNodes()

	var healthy, failing []driver.Node
	for _, n := range nodes {
		if n.IsExpectedFailure() {
			failing = append(failing, n)
		} else {
			healthy = append(healthy, n)
		}
	}

	if len(failing) == 0 {
		return nil // nothing to assert
	}

	if len(healthy) == 0 {
		// No canonical chain to compare against.
		slog.Warn("cannot verify failing nodes: no healthy nodes to compare against")
		return nil
	}

	refClient, refHeight, err := c.reference(ctx, healthy)
	if err != nil {
		return err
	}
	defer refClient.Close()

	var stillHealthy []string
	for _, n := range failing {
		deviated, reason, err := c.hasDeviated(ctx, n, refClient, refHeight)
		if err != nil {
			return err
		}
		if deviated {
			slog.Info("node marked failing deviated as expected", "node", n.GetLabel(), "reason", reason)
			continue
		}
		stillHealthy = append(stillHealthy, n.GetLabel())
	}

	if len(stillHealthy) > 0 {
		return fmt.Errorf("nodes marked as failing but still healthy (reachable, caught up, and on the canonical chain): %v", stillHealthy)
	}
	return nil
}

// reference returns a client and block height for the first reachable healthy
// node. The caller must Close the returned client.
func (c *failingNodesChecker) reference(ctx context.Context, healthy []driver.Node) (rpc.Client, int64, error) {
	for _, n := range healthy {
		client, err := n.DialRpc(ctx)
		if err != nil {
			continue
		}
		height, err := blockHeightFromClient(client)
		if err != nil {
			client.Close()
			continue
		}
		return client, height, nil
	}
	return nil, 0, fmt.Errorf("cannot verify failing nodes: no healthy node reachable to use as reference")
}

// hasDeviated reports whether a failing node observably differs from the healthy
// chain (stopped, unreachable, behind, or forked) and why. An error is returned
// only for reference-side problems.
func (c *failingNodesChecker) hasDeviated(ctx context.Context, n driver.Node, refClient rpc.Client, refHeight int64) (bool, string, error) {
	if !n.IsRunning() {
		return true, "stopped", nil
	}

	client, err := n.DialRpc(ctx)
	if err != nil {
		return true, fmt.Sprintf("unreachable: %v", err), nil
	}
	defer client.Close()

	height, err := blockHeightFromClient(client)
	if err != nil {
		return true, fmt.Sprintf("unreachable: %v", err), nil
	}

	if height < refHeight-int64(c.slack) {
		return true, fmt.Sprintf("behind the healthy chain (block %d, reference %d, slack %d)", height, refHeight, c.slack), nil
	}

	// Compare hashes at a block both nodes should have.
	compareBlock := height
	if refHeight < compareBlock {
		compareBlock = refHeight
	}

	nodeHash, err := getBlockHashes(client, uint64(compareBlock))
	if err != nil {
		return true, fmt.Sprintf("unreachable: %v", err), nil
	}
	refHash, err := getBlockHashes(refClient, uint64(compareBlock))
	if err != nil {
		return false, "", fmt.Errorf("failed to read reference hash at block %d: %w", compareBlock, err)
	}
	if refHash == nil {
		return false, "", fmt.Errorf("reference node lacks block %d used for comparison", compareBlock)
	}
	if nodeHash == nil {
		return true, fmt.Sprintf("missing block %d present on the healthy chain", compareBlock), nil
	}
	if *nodeHash != *refHash {
		return true, fmt.Sprintf("forked: divergent hash at block %d", compareBlock), nil
	}

	return false, "", nil
}
