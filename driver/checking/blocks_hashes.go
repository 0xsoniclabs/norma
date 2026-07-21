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

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/monitoring"
	"github.com/0xsoniclabs/norma/driver/rpc"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

func init() {
	RegisterNetworkCheck("blocksHashes",
		func(net driver.Network, monitor *monitoring.Monitor) Checker {
			return &blocksHashesChecker{net: net}
		})
}

// blocksHashesChecker is a Checker checking if all Opera nodes provides the same hashes for all blocks/stateRoots.
type blocksHashesChecker struct {
	net driver.Network
}

// Configure returns itself since there is nothing to configure
func (c *blocksHashesChecker) Configure(config CheckerConfig) Checker {
	return c
}

func (c *blocksHashesChecker) Check(ctx context.Context) (err error) {
	allNodes := c.net.GetActiveNodes()
	slog.Info("checking hashes for nodes", "count", len(allNodes))

	// Skip nodes expected to fail; they may fork or be unreachable by design.
	nodes := make([]driver.Node, 0, len(allNodes))
	for _, n := range allNodes {
		if n.IsExpectedFailure() {
			continue
		}
		nodes = append(nodes, n)
	}

	if len(nodes) == 0 {
		return nil // no checkable nodes, nothing to compare
	}

	rpcClients := make([]rpc.Client, len(nodes))
	defer func() {
		for _, rpcClient := range rpcClients {
			if rpcClient != nil {
				rpcClient.Close()
			}
		}
	}()

	for i, n := range nodes {
		rpcClients[i], err = n.DialRpc(ctx)
		if err != nil {
			return fmt.Errorf("failed to dial RPC for node %s; %v", n.GetLabel(), err)
		}
	}

	check := func(referenceHashes, block blockHashes, blockNumber uint64) error {
		if referenceHashes.StateRoot != block.StateRoot {
			return fmt.Errorf("stateRoot of the block %d does not match", blockNumber)
		}
		if referenceHashes.ReceiptsRoot != block.ReceiptsRoot {
			return fmt.Errorf("receiptsRoot of the block %d does not match", blockNumber)
		}
		if referenceHashes.Hash != block.Hash {
			return fmt.Errorf("hash of the block %d does not match", blockNumber)
		}

		return nil
	}

	for blockNumber := uint64(0); ; blockNumber++ {
		var nodesLackingTheBlock = 0
		var hashes []*blockHashes
		for i, n := range nodes {
			block, err := getBlockHashes(rpcClients[i], blockNumber)
			if err != nil {
				return fmt.Errorf("failed to get block %d detail at node %s; %v", blockNumber, n.GetLabel(), err)
			}

			if block == nil { // block does not exist on the node
				if blockNumber <= 2 {
					return fmt.Errorf("unable to check block hashes - block %d does not exists at node %s", blockNumber, n.GetLabel())
				}
				nodesLackingTheBlock++
			}

			hashes = append(hashes, block)
		}

		// no node has the last block, i.e. we have reached the end of the chain
		if nodesLackingTheBlock == len(nodes) {
			return nil // finish successfully
		}

		// find a reference hash from the first node that reached this block height
		var referenceHashes blockHashes
		for _, block := range hashes {
			if block != nil {
				referenceHashes = *block
				break
			}
		}

		// check the hashes
		for _, block := range hashes {
			// skip nodes that did not reach this block height
			if block == nil {
				continue // this node does not reach this block
			}
			if err := check(referenceHashes, *block, blockNumber); err != nil {
				return err
			}
		}
	}
}

type blockHashes struct {
	Hash         common.Hash
	StateRoot    common.Hash
	ReceiptsRoot common.Hash
}

func getBlockHashes(rpcClient rpc.Client, blockNumber uint64) (*blockHashes, error) {
	var block *blockHashes
	err := rpcClient.Call(&block, "eth_getBlockByNumber", hexutil.EncodeUint64(blockNumber), false)
	if err != nil {
		return nil, fmt.Errorf("failed to get block state root from RPC; %v", err)
	}
	return block, nil
}
