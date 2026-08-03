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
	"maps"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/monitoring"
)

// allow block height to fall short by this amount
// slack of 5 means that block 95-99 is also accepted when max block height = 100
const defaultSlack = 5

// defaultHeightConvergenceTimeout bounds how long the check waits for the nodes
// to agree on a block height. Nodes seal an epoch at slightly different
// instants, and a node that just rejoined needs a moment to catch up, so a
// disagreement right after a transition is not yet a failure.
const defaultHeightConvergenceTimeout = 30 * time.Second

// heightPollInterval is the delay between convergence polls. Var so tests can
// shorten it.
var heightPollInterval = 500 * time.Millisecond

func init() {
	RegisterNetworkCheck("blockHeight", func(net driver.Network, monitor *monitoring.Monitor) Checker {
		return &blockHeightChecker{
			net:     net,
			slack:   defaultSlack,
			timeout: defaultHeightConvergenceTimeout,
		}
	})
}

// blockHeightChecker is a Checker checking if all Opera nodes achieved the same
// block height. It polls forward in time until they agree, so that a
// disagreement observed while the network is still settling after a step does
// not fail the check; only a disagreement that persists does.
type blockHeightChecker struct {
	net   driver.Network
	slack int
	// timeout bounds convergence polling. Zero means a single attempt.
	timeout time.Duration
}

// Configure returns a deep copy of the original checker.
// If the config doesn't provide any replacement value, copy from the value of the original.
// If the config is invalid, return error instead.
// If the config is nil, return original checker.
func (c *blockHeightChecker) Configure(config CheckerConfig) Checker {
	if config == nil {
		return c
	}

	slack := c.slack
	if val, exist := config["slack"]; exist {
		slack = val.(int)
	}

	timeout := c.timeout
	if val, exist := config["duration"]; exist {
		timeout = time.Duration(val.(int64))
	}

	return &blockHeightChecker{
		net:     c.net,
		slack:   slack,
		timeout: timeout,
	}
}

func (c *blockHeightChecker) Check(ctx context.Context) error {
	if c.timeout <= 0 {
		return c.checkOnce(ctx)
	}

	deadline := time.Now().Add(c.timeout)
	for {
		err := c.checkOnce(ctx)
		if err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf(
				"nodes did not agree on a block height within %s: %w",
				c.timeout, err,
			)
		}
		slog.Info("block heights not settled yet, retrying", "reason", err)
		if err := sleep(ctx, heightPollInterval); err != nil {
			return err
		}
	}
}

func (c *blockHeightChecker) checkOnce(ctx context.Context) error {
	nodes := c.net.GetActiveNodes()
	slog.Info("checking block heights for nodes", "count", len(nodes))

	// Read all heights concurrently so the samples are taken at nearly the
	// same instant; sequential reads let the chain advance between nodes and
	// falsely flag early-read nodes as lagging.
	heights := make([]int64, len(nodes))
	errs := make([]error, len(nodes))
	var wg sync.WaitGroup
	for i, n := range nodes {
		wg.Go(func() {
			heights[i], errs[i] = getBlockHeight(ctx, n)
		})
	}
	wg.Wait()

	maxHeight := int64(0)
	expectedFailures := make(map[string]struct{})
	for i, n := range nodes {
		if n.IsExpectedFailure() {
			expectedFailures[n.GetLabel()] = struct{}{}
		}

		if errs[i] != nil {
			return fmt.Errorf("failed to get block height of node %s; %v", n.GetLabel(), errs[i])
		}
		if heights[i] < 1 {
			return fmt.Errorf("node %s reports it is at invalid block %d", n.GetLabel(), heights[i])
		}
		if maxHeight < heights[i] {
			maxHeight = heights[i]
		}
	}

	gotFailures := make(map[string]struct{})
	for i, n := range nodes {
		if heights[i] < maxHeight-int64(c.slack) {
			if n.IsExpectedFailure() {
				gotFailures[n.GetLabel()] = struct{}{}

			} else {
				return fmt.Errorf("node %s reports too old block %d (max block is %d, given slack of %d.)", n.GetLabel(), heights[i], maxHeight, c.slack)
			}
		}
	}

	if got, want := gotFailures, expectedFailures; !maps.Equal(got, want) {
		return fmt.Errorf("unexpected failure set to provide the block height, got %v, want %v", got, want)
	}

	return nil
}

func getBlockHeight(ctx context.Context, n driver.Node) (int64, error) {
	rpcClient, err := n.DialRpc(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to dial node RPC; %v", err)
	}
	defer rpcClient.Close()
	var blockNumber string
	err = rpcClient.Call(&blockNumber, "eth_blockNumber")
	if err != nil {
		return 0, fmt.Errorf("failed to get block number from RPC; %v", err)
	}
	blockNumber = strings.TrimPrefix(blockNumber, "0x")
	return strconv.ParseInt(blockNumber, 16, 64)
}
