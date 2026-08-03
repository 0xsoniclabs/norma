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
	"math"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/monitoring"
	"github.com/0xsoniclabs/norma/driver/rpc"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

// minComparableBlock is the lowest chain head that makes the comparison
// meaningful. Below it the network has barely started and there is nothing to
// disagree about.
const minComparableBlock = 2

func init() {
	RegisterNetworkCheck("blocksHashes",
		func(net driver.Network, monitor *monitoring.Monitor) Checker {
			return &blocksHashesChecker{net: net}
		})
}

// blocksHashesChecker is a Checker checking if all Opera nodes provides the same hashes for all blocks/stateRoots.
//
// The range of blocks to compare is fixed from a snapshot of the node heights
// taken when the check starts, and covers only blocks that every healthy node
// already has. The check therefore never chases a chain that is still growing
// underneath it: it terminates, and its verdict does not depend on which node
// happened to be ahead of which while it was running.
type blocksHashesChecker struct {
	net driver.Network
}

// Configure returns itself since there is nothing to configure
func (c *blocksHashesChecker) Configure(config CheckerConfig) Checker {
	return c
}

func (c *blocksHashesChecker) Check(ctx context.Context) error {
	nodes := c.net.GetActiveNodes()
	slog.Info("checking hashes for nodes", "count", len(nodes))

	clients := make([]rpc.Client, len(nodes))
	defer func() {
		for _, client := range clients {
			if client != nil {
				client.Close()
			}
		}
	}()

	expectedFailures := make(map[string]struct{})
	gotFailures := make(map[string]struct{})
	for i, n := range nodes {
		if n.IsExpectedFailure() {
			expectedFailures[n.GetLabel()] = struct{}{}
		}

		client, err := n.DialRpc(ctx)
		if err != nil {
			// Being unreachable is one of the ways a node that is expected to
			// fail is allowed to fail.
			if n.IsExpectedFailure() {
				gotFailures[n.GetLabel()] = struct{}{}
				continue
			}
			return fmt.Errorf("failed to dial RPC for node %s; %v", n.GetLabel(), err)
		}
		clients[i] = client
	}

	if len(expectedFailures) == len(nodes) {
		return nil // all nodes are expected to fail, cannot get pivot hash, has to only end the test
	}

	limit, err := c.commonHeight(ctx, nodes, clients)
	if err != nil {
		return err
	}
	slog.Info("comparing block hashes", "up_to_block", limit)

	for blockNumber := uint64(0); blockNumber <= limit; blockNumber++ {
		reference, err := compareHealthyNodes(nodes, clients, blockNumber)
		if err != nil {
			return err
		}
		recordFailingNodes(nodes, clients, blockNumber, reference, gotFailures)
	}

	if got, want := gotFailures, expectedFailures; !maps.Equal(got, want) {
		return fmt.Errorf("unexpected failure set to provide the block hashes: got %v, want %v", got, want)
	}

	return nil
}

// commonHeight returns the highest block that every healthy node already
// reports as being part of its chain. Blocks up to it are settled everywhere,
// so comparing them cannot race block production.
func (c *blocksHashesChecker) commonHeight(
	ctx context.Context, nodes []driver.Node, clients []rpc.Client,
) (uint64, error) {
	limit := uint64(math.MaxUint64)
	healthy := 0
	// Read the heights one by one: the result is a minimum, so a height read a
	// moment later can only be larger and the bound only more conservative.
	for i, n := range nodes {
		if clients[i] == nil || n.IsExpectedFailure() {
			continue
		}
		healthy++
		height, err := clients[i].BlockNumber(ctx)
		if err != nil {
			return 0, fmt.Errorf(
				"failed to get block height of node %s; %w", n.GetLabel(), err,
			)
		}
		if height < limit {
			limit = height
		}
	}

	if healthy == 0 {
		return 0, fmt.Errorf("unable to check block hashes: no reachable node")
	}
	if limit < minComparableBlock {
		return 0, fmt.Errorf(
			"unable to check block hashes: the network is only at block %d, "+
				"at least block %d is required",
			limit, minComparableBlock,
		)
	}
	return limit, nil
}

// compareHealthyNodes verifies that all nodes that are not expected to fail
// report the same hashes for the given block, and returns those hashes as the
// reference for the nodes that are.
func compareHealthyNodes(
	nodes []driver.Node, clients []rpc.Client, blockNumber uint64,
) (blockHashes, error) {
	var reference *blockHashes
	var referenceNode string

	for i, n := range nodes {
		if clients[i] == nil || n.IsExpectedFailure() {
			continue
		}

		block, err := getBlockHashes(clients[i], blockNumber)
		if err != nil {
			return blockHashes{}, fmt.Errorf(
				"failed to get block %d detail at node %s; %v",
				blockNumber, n.GetLabel(), err,
			)
		}
		if block == nil {
			return blockHashes{}, fmt.Errorf(
				"node %s does not have block %d, although it reported a "+
					"higher chain head", n.GetLabel(), blockNumber,
			)
		}

		if reference == nil {
			reference, referenceNode = block, n.GetLabel()
			continue
		}
		if err := compareBlockHashes(*reference, *block, blockNumber); err != nil {
			return blockHashes{}, fmt.Errorf(
				"%w (nodes %s and %s)", err, referenceNode, n.GetLabel(),
			)
		}
	}

	return *reference, nil
}

// recordFailingNodes notes which of the nodes expected to fail actually do so
// for the given block, either by not having it or by disagreeing about it.
func recordFailingNodes(
	nodes []driver.Node,
	clients []rpc.Client,
	blockNumber uint64,
	reference blockHashes,
	gotFailures map[string]struct{},
) {
	for i, n := range nodes {
		if clients[i] == nil || !n.IsExpectedFailure() {
			continue
		}
		label := n.GetLabel()
		if _, alreadyFailed := gotFailures[label]; alreadyFailed {
			continue
		}

		block, err := getBlockHashes(clients[i], blockNumber)
		if err != nil || block == nil ||
			compareBlockHashes(reference, *block, blockNumber) != nil {
			gotFailures[label] = struct{}{}
		}
	}
}

// compareBlockHashes reports whether two views of the same block agree.
func compareBlockHashes(
	referenceHashes, block blockHashes, blockNumber uint64,
) error {
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
