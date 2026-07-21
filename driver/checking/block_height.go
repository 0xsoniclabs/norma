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
	"strconv"
	"strings"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/monitoring"
	"github.com/0xsoniclabs/norma/driver/rpc"
)

// allow block height to fall short by this amount
// slack of 5 means that block 95-99 is also accepted when max block height = 100
const defaultSlack = 5

func init() {
	RegisterNetworkCheck("blockHeight", func(net driver.Network, monitor *monitoring.Monitor) Checker {
		return &blockHeightChecker{net: net, slack: defaultSlack}
	})
}

// blockHeightChecker is a Checker checking if all Opera nodes achieved the same block height.
type blockHeightChecker struct {
	net   driver.Network
	slack int
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

	return &blockHeightChecker{net: c.net, slack: slack}
}

func (c *blockHeightChecker) Check(ctx context.Context) error {
	nodes := c.net.GetActiveNodes()
	slog.Info("checking block heights for nodes", "count", len(nodes))
	heights := make([]int64, len(nodes))
	maxHeight := int64(0)
	for i, n := range nodes {
		// Skip nodes expected to fail; they may lag or be unreachable by design.
		if n.IsExpectedFailure() {
			continue
		}

		height, err := getBlockHeight(ctx, n)
		if err != nil {
			return fmt.Errorf("failed to get block height of node %s; %v", n.GetLabel(), err)
		}
		if height == 1 {
			return fmt.Errorf("node %s reports it is at block 1 (only genesis is applied)", n.GetLabel())
		}
		if height < 1 {
			return fmt.Errorf("node %s reports it is at invalid block %d", n.GetLabel(), height)
		}
		if maxHeight < height {
			maxHeight = height
		}
		heights[i] = height
	}

	for i, n := range nodes {
		if n.IsExpectedFailure() {
			continue
		}
		if heights[i] < maxHeight-int64(c.slack) {
			return fmt.Errorf("node %s reports too old block %d (max block is %d, given slack of %d.)", n.GetLabel(), heights[i], maxHeight, c.slack)
		}
	}

	return nil
}

func getBlockHeight(ctx context.Context, n driver.Node) (int64, error) {
	rpcClient, err := n.DialRpc(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to dial node RPC; %v", err)
	}
	defer rpcClient.Close()
	return blockHeightFromClient(rpcClient)
}

// blockHeightFromClient returns the latest block height from a connected RPC client.
func blockHeightFromClient(rpcClient rpc.Client) (int64, error) {
	var blockNumber string
	err := rpcClient.Call(&blockNumber, "eth_blockNumber")
	if err != nil {
		return 0, fmt.Errorf("failed to get block number from RPC; %v", err)
	}
	blockNumber = strings.TrimPrefix(blockNumber, "0x")
	return strconv.ParseInt(blockNumber, 16, 64)
}
