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
	"maps"
	"strconv"
	"strings"

	"github.com/0xsoniclabs/norma/driver"
)

func init() {
	RegisterNetworkChecker("block_height", func(net driver.Network) Checker {
		return &BlockHeightChecker{net: net}
	})
}

// BlockHeightChecker is a Checker checking if all Opera nodes achieved the same block height.
type BlockHeightChecker struct {
	net driver.Network
}

func (c *BlockHeightChecker) Check() error {
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
		if heights[i] < maxHeight-1 {
			if n.IsExpectedFailure() {
				gotFailures[n.GetLabel()] = struct{}{}

			} else {
				return fmt.Errorf("node %s reports too old block %d (max block is %d)", n.GetLabel(), heights[i], maxHeight)
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
