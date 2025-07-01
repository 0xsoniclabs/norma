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
	"fmt"
	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/monitoring"
	"maps"
	"strconv"
	"strings"
)

// allow block height to fall short by this amount
// slack of 5 means that block 95-99 is also accepted when max block height = 100
const defaultSlack = 5

func init() {
	RegisterNetworkCheck("block_height", func(net driver.Network, monitor *monitoring.Monitor) Checker {
		return &blockHeightChecker{net: net, slack: defaultSlack}
	})
}

// blockHeightChecker is a Checker checking if all Opera nodes achieved the same block height.
type blockHeightChecker struct {
	net   driver.Network
	slack uint8
}

// Configure returns a deep copy of the original checker.
// If the config doesn't provide any replacement value, copy from the value of the original.
// If the config is invalid, return error instead.
// If the config is nil, return original checker.
func (c *blockHeightChecker) Configure(config map[string]string) (Checker, error) {
	if config == nil {
		return c, nil
	}

	slack := c.slack
	sString, exist := config["slack"]
	if exist {
		s, err := strconv.Atoi(sString)
		if err != nil {
			return nil, fmt.Errorf("failed to convert slack; %v", err)
		}
		if s < 0 || s > 255 {
			return nil, fmt.Errorf("invalid slack; 0 < %d < 255", s)
		}
		slack = uint8(s)
	}

	return &blockHeightChecker{net: c.net, slack: slack}, nil
}

func (c *blockHeightChecker) Check() error {
	nodes := c.net.GetActiveNodes()
	fmt.Printf("checking block heights for %d nodes\n", len(nodes))
	heights := make([]int64, len(nodes))
	maxHeight := int64(0)
	expectedFailures := make(map[string]struct{})
	for i, n := range nodes {
		if n.IsExpectedFailure() {
			expectedFailures[n.GetLabel()] = struct{}{}
		}

		height, err := getBlockHeight(n)
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

func getBlockHeight(n driver.Node) (int64, error) {
	rpcClient, err := n.DialRpc()
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
