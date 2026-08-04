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
	"errors"
	"strings"
	"testing"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/rpc"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"go.uber.org/mock/gomock"
)

// chainNode describes one node of a simulated chain for the block hashes check.
type chainNode struct {
	label string
	// height is the chain head the node reports.
	height uint64
	// failing marks a node that is expected to fail.
	failing bool
	// dialErr makes dialling the node fail.
	dialErr bool
	// heightErr makes the head query fail.
	heightErr bool
	// fork is the first block from which this node reports different hashes;
	// zero means it agrees with the canonical chain everywhere.
	fork uint64
	// forkField selects which hash differs from the canonical chain at and
	// above fork. Empty means the state root.
	forkField string
	// missing is a block the node claims not to have even though it reported a
	// higher head. Zero means no such gap.
	missing uint64
}

// canonicalHashes returns the hashes every honest node reports for a block.
func canonicalHashes(block uint64) blockHashes {
	return blockHashes{
		Hash:         common.Hash{0x11, byte(block)},
		StateRoot:    common.Hash{0x22, byte(block)},
		ReceiptsRoot: common.Hash{0x33, byte(block)},
	}
}

// chainNetwork builds a mocked network answering head and block queries
// according to the given node descriptions.
func chainNetwork(t *testing.T, nodes ...chainNode) driver.Network {
	t.Helper()
	ctrl := gomock.NewController(t)

	mocked := make([]driver.Node, 0, len(nodes))
	for _, spec := range nodes {
		spec := spec

		node := driver.NewMockNode(ctrl)
		node.EXPECT().GetLabel().AnyTimes().Return(spec.label)
		node.EXPECT().IsExpectedFailure().AnyTimes().Return(spec.failing)

		if spec.dialErr {
			node.EXPECT().DialRpc(gomock.Any()).AnyTimes().
				Return(nil, errors.New("connection refused"))
			mocked = append(mocked, node)
			continue
		}

		client := rpc.NewMockClient(ctrl)
		client.EXPECT().Close().AnyTimes()
		if spec.heightErr {
			client.EXPECT().BlockNumber(gomock.Any()).AnyTimes().
				Return(uint64(0), errors.New("no head"))
		} else {
			client.EXPECT().BlockNumber(gomock.Any()).AnyTimes().
				Return(spec.height, nil)
		}

		client.EXPECT().Call(
			gomock.Any(), "eth_getBlockByNumber", gomock.Any(), false,
		).AnyTimes().DoAndReturn(func(
			result any, _ string, args ...any,
		) error {
			block, err := hexutil.DecodeUint64(args[0].(string))
			if err != nil {
				return err
			}
			target := result.(**blockHashes)
			if block > spec.height ||
				(spec.missing > 0 && block == spec.missing) {
				*target = nil // the node does not have this block
				return nil
			}
			hashes := canonicalHashes(block)
			if spec.fork > 0 && block >= spec.fork {
				diverged := common.Hash{0xFF, byte(block)}
				switch spec.forkField {
				case "receiptsRoot":
					hashes.ReceiptsRoot = diverged
				case "hash":
					hashes.Hash = diverged
				default:
					hashes.StateRoot = diverged
				}
			}
			*target = &hashes
			return nil
		})

		node.EXPECT().DialRpc(gomock.Any()).AnyTimes().Return(client, nil)
		mocked = append(mocked, node)
	}

	net := driver.NewMockNetwork(ctrl)
	net.EXPECT().GetActiveNodes().AnyTimes().Return(mocked)
	return net
}

func TestBlockHashes_PassesWhenAllNodesAgree(t *testing.T) {
	net := chainNetwork(t,
		chainNode{label: "node1", height: 10},
		chainNode{label: "node2", height: 10},
	)

	c := blocksHashesChecker{net: net}
	if err := c.Check(t.Context()); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestBlockHashes_PassesWhenNodesAreAtDifferentHeights(t *testing.T) {
	// The blocks a node has not received yet are not evidence of disagreement;
	// they are simply outside the range that is settled everywhere.
	net := chainNetwork(t,
		chainNode{label: "ahead", height: 30},
		chainNode{label: "behind", height: 10},
	)

	c := blocksHashesChecker{net: net}
	if err := c.Check(t.Context()); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestBlockHashes_FailsOnDivergingHashes(t *testing.T) {
	net := chainNetwork(t,
		chainNode{label: "node1", height: 10},
		chainNode{label: "node2", height: 10, fork: 4},
	)

	c := blocksHashesChecker{net: net}
	err := c.Check(t.Context())
	if err == nil {
		t.Fatalf("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "stateRoot of the block 4 does not match") {
		t.Errorf("unexpected error: %v", err)
	}
	// The error must say which nodes disagree.
	if !strings.Contains(err.Error(), "node1") ||
		!strings.Contains(err.Error(), "node2") {
		t.Errorf("error does not name the disagreeing nodes: %v", err)
	}
}

func TestBlockHashes_ReportsWhicheverHashDiverges(t *testing.T) {
	tests := map[string]string{
		"stateRoot of the block 4 does not match":    "stateRoot",
		"receiptsRoot of the block 4 does not match": "receiptsRoot",
		"hash of the block 4 does not match":         "hash",
	}
	for wantMessage, field := range tests {
		t.Run(field, func(t *testing.T) {
			net := chainNetwork(t,
				chainNode{label: "node1", height: 10},
				chainNode{
					label: "node2", height: 10,
					fork: 4, forkField: field,
				},
			)

			c := blocksHashesChecker{net: net}
			err := c.Check(t.Context())
			if err == nil || !strings.Contains(err.Error(), wantMessage) {
				t.Errorf("got %v, want an error containing %q", err, wantMessage)
			}
		})
	}
}

func TestBlockHashes_FailsWhenAHealthyNodeLacksASettledBlock(t *testing.T) {
	// The node reports a head of 10 but cannot produce block 4. That is not a
	// timing artefact of a growing chain; the block should be there.
	net := chainNetwork(t,
		chainNode{label: "node1", height: 10},
		chainNode{label: "gappy", height: 10, missing: 4},
	)

	c := blocksHashesChecker{net: net}
	err := c.Check(t.Context())
	if err == nil || !strings.Contains(err.Error(), "does not have block 4") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestBlockHashes_IgnoresDivergenceAboveTheCommonHead(t *testing.T) {
	// node2 forks at block 20, which is above the head node2 has in common
	// with node1. Those blocks are not settled everywhere yet, so a check run
	// now must not judge them.
	net := chainNetwork(t,
		chainNode{label: "node1", height: 10},
		chainNode{label: "node2", height: 30, fork: 20},
	)

	c := blocksHashesChecker{net: net}
	if err := c.Check(t.Context()); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestBlockHashes_DivergingFailingNodeIsExpected(t *testing.T) {
	net := chainNetwork(t,
		chainNode{label: "healthy", height: 10},
		chainNode{label: "forked", height: 10, failing: true, fork: 4},
	)

	c := blocksHashesChecker{net: net}
	if err := c.Check(t.Context()); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestBlockHashes_UnreachableFailingNodeIsExpected(t *testing.T) {
	// A node expected to fail may have crashed outright. That used to abort the
	// whole check with a dial error.
	net := chainNetwork(t,
		chainNode{label: "healthy", height: 10},
		chainNode{label: "crashed", failing: true, dialErr: true},
	)

	c := blocksHashesChecker{net: net}
	if err := c.Check(t.Context()); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestBlockHashes_FailingNodeThatAgreesIsReported(t *testing.T) {
	net := chainNetwork(t,
		chainNode{label: "healthy", height: 10},
		chainNode{label: "supposedly-forked", height: 10, failing: true},
	)

	c := blocksHashesChecker{net: net}
	err := c.Check(t.Context())
	if err == nil || !strings.Contains(err.Error(),
		"unexpected failure set to provide the block hashes") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestBlockHashes_ShortFailingNodeIsExpected(t *testing.T) {
	// A node expected to fail that is stuck well below the rest never reaches
	// the compared blocks, which counts as the expected failure.
	net := chainNetwork(t,
		chainNode{label: "healthy1", height: 10},
		chainNode{label: "healthy2", height: 10},
		chainNode{label: "stuck", height: 3, failing: true},
	)

	c := blocksHashesChecker{net: net}
	if err := c.Check(t.Context()); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestBlockHashes_PassesWhenEveryNodeIsExpectedToFail(t *testing.T) {
	net := chainNetwork(t,
		chainNode{label: "node1", height: 10, failing: true},
		chainNode{label: "node2", height: 10, failing: true, fork: 2},
	)

	c := blocksHashesChecker{net: net}
	if err := c.Check(t.Context()); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestBlockHashes_FailsWhenAHealthyNodeIsUnreachable(t *testing.T) {
	net := chainNetwork(t,
		chainNode{label: "healthy", height: 10},
		chainNode{label: "down", dialErr: true},
	)

	c := blocksHashesChecker{net: net}
	err := c.Check(t.Context())
	if err == nil || !strings.Contains(err.Error(), "failed to dial RPC") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestBlockHashes_FailsWhenTheHeadCannotBeRead(t *testing.T) {
	net := chainNetwork(t,
		chainNode{label: "healthy", height: 10},
		chainNode{label: "mute", height: 10, heightErr: true},
	)

	c := blocksHashesChecker{net: net}
	err := c.Check(t.Context())
	if err == nil || !strings.Contains(err.Error(), "failed to get block height") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestBlockHashes_FailsWhenTheChainIsTooShort(t *testing.T) {
	net := chainNetwork(t,
		chainNode{label: "node1", height: 1},
		chainNode{label: "node2", height: 1},
	)

	c := blocksHashesChecker{net: net}
	err := c.Check(t.Context())
	if err == nil || !strings.Contains(err.Error(), "only at block 1") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestBlockHashes_ComparisonNeedsAReferenceNode(t *testing.T) {
	// Check establishes that at least one healthy node is reachable before it
	// compares anything. The guard keeps a violation of that an error rather
	// than a nil dereference.
	ctrl := gomock.NewController(t)
	node := driver.NewMockNode(ctrl)
	node.EXPECT().GetLabel().AnyTimes().Return("failing")
	node.EXPECT().IsExpectedFailure().AnyTimes().Return(true)

	_, err := compareHealthyNodes(
		[]driver.Node{node}, []rpc.Client{rpc.NewMockClient(ctrl)}, 1,
	)
	if err == nil || !strings.Contains(err.Error(), "no reachable healthy node") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestBlockHashes_ConfigureReturnsItself(t *testing.T) {
	c := &blocksHashesChecker{}
	if got := c.Configure(CheckerConfig{"failing": true}); got != c {
		t.Errorf("Configure should return the same checker")
	}
}
