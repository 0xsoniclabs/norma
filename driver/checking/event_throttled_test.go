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
	"math/big"
	"testing"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/rpc"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestVerifyThrottling_Passes_WhenRatioBelowCeiling(t *testing.T) {
	cases := map[string]struct {
		counts  map[uint64]int
		ceiling int
	}{
		"throttled well below ceiling": {
			counts: map[uint64]int{
				1: 100, // dominant
				2: 80,  // unthrottled non-dominant
				3: 10,  // throttled non-dominant (12.5%)
			},
			ceiling: 50,
		},
		"throttled at ceiling boundary": {
			counts: map[uint64]int{
				1: 200,
				2: 100,
				3: 50, // exactly 50%
			},
			ceiling: 50,
		},
		"custom ceiling": {
			counts: map[uint64]int{
				1: 500,
				2: 100,
				3: 20, // 20%
			},
			ceiling: 25,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			labels := map[int]string{
				1: "node-1",
				2: "node-2",
				3: "node-3",
			}
			err := verifyThrottling(tc.counts, labels, tc.ceiling)
			require.NoError(t, err)
		})
	}
}

func TestVerifyThrottling_Fails_WhenRatioExceedsCeiling(t *testing.T) {
	cases := map[string]struct {
		counts  map[uint64]int
		ceiling int
		errMsg  string
	}{
		"ratio exceeds ceiling": {
			counts: map[uint64]int{
				1: 100,
				2: 80,
				3: 60, // 75% of 80 → exceeds 50%
			},
			ceiling: 50,
			errMsg:  "expected at most 50%",
		},
		"equal non-dominant emissions": {
			counts: map[uint64]int{
				1: 200,
				2: 50,
				3: 50, // 100% → exceeds any ceiling < 100
			},
			ceiling: 50,
			errMsg:  "expected at most 50%",
		},
		"fewer than 2 non-dominant validators": {
			counts: map[uint64]int{
				1: 100,
				2: 50,
			},
			ceiling: 50,
			errMsg:  "need at least 2 non-dominant validators",
		},
		"single validator": {
			counts: map[uint64]int{
				1: 100,
			},
			ceiling: 50,
			errMsg:  "need at least 2 non-dominant validators",
		},
		"max non-dominant emitted zero": {
			counts: map[uint64]int{
				1: 100,
				2: 0,
				3: 0,
			},
			ceiling: 50,
			errMsg:  "max non-dominant validator emitted 0 events",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			labels := map[int]string{
				1: "node-1",
				2: "node-2",
				3: "node-3",
			}
			err := verifyThrottling(tc.counts, labels, tc.ceiling)
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.errMsg)
		})
	}
}

func TestEventThrottledChecker_AppliesCeiling_WhenConfigured(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)

	original := &eventThrottledChecker{net: net, ceiling: 30}

	t.Run("custom ceiling", func(t *testing.T) {
		configured := original.Configure(CheckerConfig{
			"ceiling": 75,
		})
		c := configured.(*eventThrottledChecker)
		require.Equal(t, 75, c.ceiling)
	})

	t.Run("empty config uses default", func(t *testing.T) {
		configured := original.Configure(CheckerConfig{})
		c := configured.(*eventThrottledChecker)
		require.Equal(t, defaultThrottleCeiling, c.ceiling)
	})
}

func TestEventThrottledChecker_ReturnsError_WhenNoActiveNodes(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)
	net.EXPECT().GetActiveNodes().Return(nil)

	checker := &eventThrottledChecker{net: net, ceiling: 50}
	err := checker.Check(t.Context())
	require.Error(t, err)
	require.Contains(t, err.Error(), "no active nodes")
}

func TestEventThrottledChecker_ReturnsError_WhenNoReachableNode(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)
	node := driver.NewMockNode(ctrl)

	net.EXPECT().GetActiveNodes().Return([]driver.Node{node})
	node.EXPECT().IsExpectedFailure().Return(true)

	checker := &eventThrottledChecker{net: net, ceiling: 50}
	err := checker.Check(t.Context())
	require.Error(t, err)
	require.Contains(t, err.Error(), "no reachable node")
}

func TestEventThrottledChecker_Fails_WhenNonDominantRatioExceedsCeiling(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)
	node1 := driver.NewMockNode(ctrl)
	node2 := driver.NewMockNode(ctrl)
	node3 := driver.NewMockNode(ctrl)
	rpcClient := rpc.NewMockClient(ctrl)

	validatorId1 := 1
	validatorId2 := 2
	validatorId3 := 3

	net.EXPECT().GetActiveNodes().Return(
		[]driver.Node{node1, node2, node3},
	)
	node1.EXPECT().IsExpectedFailure().Return(false)
	node1.EXPECT().DialRpc(gomock.Any()).Return(rpcClient, nil)
	node1.EXPECT().GetValidatorId().Return(&validatorId1)
	node1.EXPECT().GetLabel().Return("dominant")
	node2.EXPECT().GetValidatorId().Return(&validatorId2)
	node2.EXPECT().GetLabel().Return("unthrottled")
	node3.EXPECT().GetValidatorId().Return(&validatorId3)
	node3.EXPECT().GetLabel().Return("throttled")

	// Mock eth_currentEpoch → epoch 5
	rpcClient.EXPECT().
		Call(gomock.Any(), "eth_currentEpoch").
		SetArg(0, hexutil.Uint64(5))

	// Mock dag_getHeads → two heads
	head1 := common.HexToHash("0x1111")
	head2 := common.HexToHash("0x2222")
	rpcClient.EXPECT().
		Call(gomock.Any(), "dag_getHeads", "0x5").
		SetArg(0, []string{head1.Hex(), head2.Hex()})

	// Event 1 (head1): creator=1 (dominant), no parents
	rpcClient.EXPECT().
		Call(gomock.Any(), "dag_getEvent", head1.Hex()).
		SetArg(0, map[string]any{
			"creator": "0x1",
			"parents": []any{},
		})

	// Event 2 (head2): creator=2 (unthrottled), parent=event3
	event3Hash := common.HexToHash("0x3333")
	rpcClient.EXPECT().
		Call(gomock.Any(), "dag_getEvent", head2.Hex()).
		SetArg(0, map[string]any{
			"creator": "0x2",
			"parents": []any{event3Hash.Hex()},
		})

	// Event 3: creator=2 (unthrottled), parent=event4
	event4Hash := common.HexToHash("0x4444")
	rpcClient.EXPECT().
		Call(gomock.Any(), "dag_getEvent", event3Hash.Hex()).
		SetArg(0, map[string]any{
			"creator": "0x2",
			"parents": []any{event4Hash.Hex()},
		})

	// Event 4: creator=3 (throttled), no parents
	rpcClient.EXPECT().
		Call(gomock.Any(), "dag_getEvent", event4Hash.Hex()).
		SetArg(0, map[string]any{
			"creator": "0x3",
			"parents": []any{},
		})

	rpcClient.EXPECT().Close()

	// counts: creator1=1, creator2=2, creator3=1
	// dominant=creator2 (2 events)
	// non-dominant: creator1=1, creator3=1 → ratio=100%
	// With ceiling=50, this should fail.
	checker := &eventThrottledChecker{net: net, ceiling: 50}
	err := checker.Check(t.Context())

	// Both non-dominant have 1 event each → ratio is 100% > 50%
	require.Error(t, err)
	require.Contains(t, err.Error(), "expected at most 50%")
}

func TestEventThrottledChecker_Passes_WhenThrottledEmitsFewerEvents(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)
	node1 := driver.NewMockNode(ctrl)
	node2 := driver.NewMockNode(ctrl)
	node3 := driver.NewMockNode(ctrl)
	rpcClient := rpc.NewMockClient(ctrl)

	validatorId1 := 1
	validatorId2 := 2
	validatorId3 := 3

	net.EXPECT().GetActiveNodes().Return(
		[]driver.Node{node1, node2, node3},
	)
	node1.EXPECT().IsExpectedFailure().Return(false)
	node1.EXPECT().DialRpc(gomock.Any()).Return(rpcClient, nil)
	node1.EXPECT().GetValidatorId().Return(&validatorId1)
	node1.EXPECT().GetLabel().Return("dominant")
	node2.EXPECT().GetValidatorId().Return(&validatorId2)
	node2.EXPECT().GetLabel().Return("unthrottled")
	node3.EXPECT().GetValidatorId().Return(&validatorId3)
	node3.EXPECT().GetLabel().Return("throttled")

	// Mock eth_currentEpoch
	rpcClient.EXPECT().
		Call(gomock.Any(), "eth_currentEpoch").
		SetArg(0, hexutil.Uint64(1))

	// Build a DAG where:
	//   creator 1 (dominant): 10 events
	//   creator 2 (unthrottled): 5 events
	//   creator 3 (throttled): 1 event  → ratio = 1/5 = 20% < 50%

	// Generate event hashes
	events := make([]common.Hash, 16)
	for i := range events {
		events[i] = common.BigToHash(big.NewInt(int64(i + 1)))
	}

	// heads point to: events[0..9]=creator1, events[10..14]=creator2, events[15]=creator3
	heads := make([]string, len(events))
	for i, e := range events {
		heads[i] = e.Hex()
	}
	rpcClient.EXPECT().
		Call(gomock.Any(), "dag_getHeads", "0x1").
		SetArg(0, heads)

	// All events have no parents (flat DAG for simplicity)
	for i, e := range events {
		var creator string
		switch {
		case i < 10:
			creator = "0x1" // dominant
		case i < 15:
			creator = "0x2" // unthrottled
		default:
			creator = "0x3" // throttled
		}
		rpcClient.EXPECT().
			Call(gomock.Any(), "dag_getEvent", e.Hex()).
			SetArg(0, map[string]any{
				"creator": creator,
				"parents": []any{},
			})
	}

	rpcClient.EXPECT().Close()

	checker := &eventThrottledChecker{net: net, ceiling: 50}
	err := checker.Check(t.Context())
	require.NoError(t, err)
}
