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
	"github.com/0xsoniclabs/norma/driver/rpc"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"maps"
)

// BlocksHashesChecker is a Checker checking if all Opera nodes provides the same hashes for all blocks/stateRoots.
type BlocksHashesChecker struct {
}

func (*BlocksHashesChecker) Check(net driver.Network) (err error) {
	nodes := net.GetActiveNodes()
	fmt.Printf("checking hashes for %d nodes\n", len(nodes))

	rpcClients := make([]rpc.RpcClient, len(nodes))
	defer func() {
		for _, rpcClient := range rpcClients {
			if rpcClient != nil {
				rpcClient.Close()
			}
		}
	}()

	expectedFailures := make(map[string]struct{})
	for i, n := range nodes {
		if n.IsExpectedFailure() {
			expectedFailures[n.GetLabel()] = struct{}{}
		}
		rpcClients[i], err = n.DialRpc()
		if err != nil {
			return fmt.Errorf("failed to dial RPC for node %s; %v", n.GetLabel(), err)
		}
	}

	if len(expectedFailures) == len(nodes) {
		return nil // all nodes are expected to fail, cannot get pivot hash, has to only end the test
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

	gotFailures := make(map[string]struct{})
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
			if got, want := gotFailures, expectedFailures; !maps.Equal(got, want) {
				return fmt.Errorf("unexpected failure set to provide the block hashes: got %v, want %v", got, want)
			}

			return nil // finish successfully
		}

		// find a reference hash from a non-failing nodes, and only nodes that reached this block height
		var referenceHashes blockHashes
		for i, block := range hashes {
			// use only hash from a block that reached this block height and it is not expected to fail
			if block != nil && !nodes[i].IsExpectedFailure() {
				referenceHashes = *block
				break
			}
		}

		// check the hashes
		for i, block := range hashes {
			n := nodes[i]
			// skip nodes that did not reach this block height, and potentially mark expected failed nodes
			if block == nil {
				if n.IsExpectedFailure() {
					gotFailures[n.GetLabel()] = struct{}{}
				}
				continue // this node does not reach this block
			}
			if err := check(referenceHashes, *block, blockNumber); err != nil {
				if n.IsExpectedFailure() {
					gotFailures[n.GetLabel()] = struct{}{}
				} else {
					return err
				}
			}
		}
	}
}

type blockHashes struct {
	Hash         common.Hash
	StateRoot    common.Hash
	ReceiptsRoot common.Hash
}

func getBlockHashes(rpcClient rpc.RpcClient, blockNumber uint64) (*blockHashes, error) {
	var block *blockHashes
	err := rpcClient.Call(&block, "eth_getBlockByNumber", hexutil.EncodeUint64(blockNumber), false)
	if err != nil {
		return nil, fmt.Errorf("failed to get block state root from RPC; %v", err)
	}
	return block, nil
}
