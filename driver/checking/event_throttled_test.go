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
		counts       map[uint64]int
		throttledSet map[uint64]struct{}
		ceiling      int
	}{
		"throttled well below ceiling": {
			counts: map[uint64]int{
				1: 100, // unthrottled reference
				2: 80,  // throttled (80% of 100)
				3: 10,  // throttled (10% of 100)
			},
			throttledSet: map[uint64]struct{}{2: {}, 3: {}},
			ceiling:      100,
		},
		"throttled at ceiling boundary": {
			counts: map[uint64]int{
				1: 200, // unthrottled
				2: 100, // throttled (50% of 200)
				3: 50,  // throttled (25% of 200)
			},
			throttledSet: map[uint64]struct{}{2: {}, 3: {}},
			ceiling:      50,
		},
		"custom ceiling": {
			counts: map[uint64]int{
				1: 500, // unthrottled
				2: 100, // throttled (20% of 500)
				3: 20,  // throttled (4% of 500)
			},
			throttledSet: map[uint64]struct{}{2: {}, 3: {}},
			ceiling:      25,
		},
		"multiple unthrottled validators": {
			counts: map[uint64]int{
				1: 100, // unthrottled
				2: 90,  // unthrottled (max reference = 100)
				3: 30,  // throttled (30% of 100)
			},
			throttledSet: map[uint64]struct{}{3: {}},
			ceiling:      50,
		},
		"single throttled validator": {
			counts: map[uint64]int{
				1: 100, // unthrottled
				2: 10,  // throttled (10% of 100)
			},
			throttledSet: map[uint64]struct{}{2: {}},
			ceiling:      50,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			labels := map[int]string{
				1: "node-1",
				2: "node-2",
				3: "node-3",
			}
			err := verifyThrottling(
				tc.counts, tc.throttledSet, labels, tc.ceiling,
			)
			require.NoError(t, err)
		})
	}
}

func TestVerifyThrottling_Fails_WhenRatioExceedsCeiling(t *testing.T) {
	cases := map[string]struct {
		counts       map[uint64]int
		throttledSet map[uint64]struct{}
		ceiling      int
		errMsg       string
	}{
		"ratio exceeds ceiling": {
			counts: map[uint64]int{
				1: 100, // unthrottled
				2: 80,  // throttled (80% of 100)
				3: 60,  // throttled (60% of 100)
			},
			throttledSet: map[uint64]struct{}{2: {}, 3: {}},
			ceiling:      50,
			errMsg:       "expected at most 50%",
		},
		"single throttled above ceiling": {
			counts: map[uint64]int{
				1: 100, // unthrottled
				2: 60,  // throttled (60% of 100)
			},
			throttledSet: map[uint64]struct{}{2: {}},
			ceiling:      50,
			errMsg:       "expected at most 50%",
		},
		"no throttled validator observed": {
			counts: map[uint64]int{
				1: 100,
			},
			throttledSet: map[uint64]struct{}{2: {}},
			ceiling:      50,
			errMsg:       "need at least 1 throttled validator",
		},
		"no unthrottled validator emitted": {
			counts: map[uint64]int{
				2: 50, // throttled only observed
			},
			throttledSet: map[uint64]struct{}{2: {}},
			ceiling:      50,
			errMsg:       "no unthrottled validator emitted events",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			labels := map[int]string{
				1: "node-1",
				2: "node-2",
				3: "node-3",
			}
			err := verifyThrottling(
				tc.counts, tc.throttledSet, labels, tc.ceiling,
			)
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.errMsg)
		})
	}
}

func TestComputeDominantSet_ReturnsExpectedSet_ForVariousStakes(t *testing.T) {
	cases := map[string]struct {
		stakes    map[uint64]*big.Int
		threshold float64
		expected  map[uint64]struct{}
	}{
		"single dominant meets threshold": {
			stakes: map[uint64]*big.Int{
				1: big.NewInt(9_000_000),
				2: big.NewInt(1_000_000),
			},
			threshold: 0.75,
			expected:  map[uint64]struct{}{1: {}},
		},
		"multiple validators needed to reach threshold": {
			stakes: map[uint64]*big.Int{
				1: big.NewInt(40),
				2: big.NewInt(30),
				3: big.NewInt(20),
				4: big.NewInt(10),
			},
			threshold: 0.75,
			// total=100; needed=75; 40+30=70 < 75, add 20 → 90 >= 75
			expected: map[uint64]struct{}{1: {}, 2: {}, 3: {}},
		},
		"threshold not reachable returns all": {
			stakes: map[uint64]*big.Int{
				1: big.NewInt(50),
				2: big.NewInt(50),
			},
			threshold: 0.99,
			expected:  map[uint64]struct{}{1: {}, 2: {}},
		},
		"zero stakes excluded": {
			stakes: map[uint64]*big.Int{
				1: big.NewInt(100),
				2: big.NewInt(0),
			},
			threshold: 0.75,
			expected:  map[uint64]struct{}{1: {}},
		},
		"tie broken by ascending id": {
			stakes: map[uint64]*big.Int{
				1: big.NewInt(50),
				2: big.NewInt(50),
			},
			threshold: 0.5,
			// total=100; needed=50; first (id=1, 50) → 50 >= 50
			expected: map[uint64]struct{}{1: {}},
		},
		"empty stakes returns empty set": {
			stakes:    map[uint64]*big.Int{},
			threshold: 0.75,
			expected:  map[uint64]struct{}{},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := computeDominantSet(tc.stakes, tc.threshold)
			require.Equal(t, tc.expected, got)
		})
	}
}

func TestThrottledSetFromLabels_ResolvesLabels_ToValidatorIds(t *testing.T) {
	ctrl := gomock.NewController(t)
	nodeA := driver.NewMockNode(ctrl)
	nodeB := driver.NewMockNode(ctrl)
	nodeC := driver.NewMockNode(ctrl)

	idA, idB := 1, 2
	nodeA.EXPECT().GetValidatorId().Return(&idA)
	nodeA.EXPECT().GetLabel().Return("alpha")
	nodeB.EXPECT().GetValidatorId().Return(&idB)
	nodeB.EXPECT().GetLabel().Return("beta")
	// nodeC is a non-validator (no ID); must be skipped without error.
	nodeC.EXPECT().GetValidatorId().Return(nil)

	got, err := throttledSetFromLabels(
		[]string{"beta"}, []driver.Node{nodeA, nodeB, nodeC},
	)
	require.NoError(t, err)
	require.Equal(t, map[uint64]struct{}{2: {}}, got)
}

func TestThrottledSetFromLabels_ReturnsError_WhenLabelUnknown(t *testing.T) {
	ctrl := gomock.NewController(t)
	nodeA := driver.NewMockNode(ctrl)
	idA := 1
	nodeA.EXPECT().GetValidatorId().Return(&idA)
	nodeA.EXPECT().GetLabel().Return("alpha")

	_, err := throttledSetFromLabels(
		[]string{"missing"}, []driver.Node{nodeA},
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), `label "missing"`)
}

func TestEventThrottledChecker_AppliesConfiguration_WhenProvided(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)
	original := newEventThrottledChecker(net)

	t.Run("custom ceiling, threshold, and throttled nodes", func(t *testing.T) {
		configured := original.Configure(CheckerConfig{
			"ceiling":        75,
			"stakeThreshold": 0.9,
			"throttledNodes": []any{"node-a", "node-b"},
		})
		c := configured.(*eventThrottledChecker)
		require.Equal(t, 75, c.ceiling)
		require.Equal(t, 0.9, c.stakeThreshold)
		require.Equal(t, []string{"node-a", "node-b"}, c.throttledNodes)
	})

	t.Run("empty config uses defaults", func(t *testing.T) {
		configured := original.Configure(CheckerConfig{})
		c := configured.(*eventThrottledChecker)
		require.Equal(t, defaultThrottleCeiling, c.ceiling)
		require.Equal(t, defaultDominantStakeThreshold, c.stakeThreshold)
		require.Empty(t, c.throttledNodes)
	})
}

func TestEventThrottledChecker_ReturnsError_WhenNoActiveNodes(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)
	net.EXPECT().GetActiveNodes().Return(nil)

	checker := newEventThrottledChecker(net)
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

	checker := newEventThrottledChecker(net)
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
	node2.EXPECT().GetLabel().Return("non-dominant-a")
	node3.EXPECT().GetValidatorId().Return(&validatorId3)
	node3.EXPECT().GetLabel().Return("non-dominant-b")

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

	// Event 2 (head2): creator=2, parent=event3
	event3Hash := common.HexToHash("0x3333")
	rpcClient.EXPECT().
		Call(gomock.Any(), "dag_getEvent", head2.Hex()).
		SetArg(0, map[string]any{
			"creator": "0x2",
			"parents": []any{event3Hash.Hex()},
		})

	// Event 3: creator=2, parent=event4
	event4Hash := common.HexToHash("0x4444")
	rpcClient.EXPECT().
		Call(gomock.Any(), "dag_getEvent", event3Hash.Hex()).
		SetArg(0, map[string]any{
			"creator": "0x2",
			"parents": []any{event4Hash.Hex()},
		})

	// Event 4: creator=3, no parents
	rpcClient.EXPECT().
		Call(gomock.Any(), "dag_getEvent", event4Hash.Hex()).
		SetArg(0, map[string]any{
			"creator": "0x3",
			"parents": []any{},
		})

	rpcClient.EXPECT().Close()

	// Stakes make validator 1 the sole dominant validator.
	stakes := map[uint64]*big.Int{
		1: big.NewInt(9_000_000),
		2: big.NewInt(500_000),
		3: big.NewInt(500_000),
	}

	// counts: creator1=1, creator2=2, creator3=1
	// unthrottled reference (max unthrottled) = counts[1] = 1
	// min throttled = 1 (creator3) → ratio = 100%
	// With ceiling=50, this should fail.
	checker := newEventThrottledChecker(net)
	checker.ceiling = 50
	checker.fetchStakes = func(rpc.Client) (map[uint64]*big.Int, error) {
		return stakes, nil
	}
	err := checker.Check(t.Context())

	require.Error(t, err)
	require.Contains(t, err.Error(), "expected at most 50%")
}

func TestEventThrottledChecker_Passes_WhenThrottledEmitsFewerEvents(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)
	node1 := driver.NewMockNode(ctrl)
	node2 := driver.NewMockNode(ctrl)
	rpcClient := rpc.NewMockClient(ctrl)

	validatorId1 := 1
	validatorId2 := 2

	net.EXPECT().GetActiveNodes().Return(
		[]driver.Node{node1, node2},
	)
	node1.EXPECT().IsExpectedFailure().Return(false)
	node1.EXPECT().DialRpc(gomock.Any()).Return(rpcClient, nil)
	node1.EXPECT().GetValidatorId().Return(&validatorId1)
	node1.EXPECT().GetLabel().Return("dominant")
	node2.EXPECT().GetValidatorId().Return(&validatorId2)
	node2.EXPECT().GetLabel().Return("throttled")

	// Mock eth_currentEpoch
	rpcClient.EXPECT().
		Call(gomock.Any(), "eth_currentEpoch").
		SetArg(0, hexutil.Uint64(1))

	// Build a DAG where:
	//   creator 1 (dominant): 10 events
	//   creator 2 (throttled): 1 event  → ratio = 1/10 = 10% < 50%
	events := make([]common.Hash, 11)
	for i := range events {
		events[i] = common.BigToHash(big.NewInt(int64(i + 1)))
	}

	heads := make([]string, len(events))
	for i, e := range events {
		heads[i] = e.Hex()
	}
	rpcClient.EXPECT().
		Call(gomock.Any(), "dag_getHeads", "0x1").
		SetArg(0, heads)

	for i, e := range events {
		creator := "0x1"
		if i == len(events)-1 {
			creator = "0x2"
		}
		rpcClient.EXPECT().
			Call(gomock.Any(), "dag_getEvent", e.Hex()).
			SetArg(0, map[string]any{
				"creator": creator,
				"parents": []any{},
			})
	}

	rpcClient.EXPECT().Close()

	// Stakes match the throttler_check.yml scenario (90/10 split).
	stakes := map[uint64]*big.Int{
		1: big.NewInt(9_000_000),
		2: big.NewInt(1_000_000),
	}

	checker := newEventThrottledChecker(net)
	checker.ceiling = 50
	checker.fetchStakes = func(rpc.Client) (map[uint64]*big.Int, error) {
		return stakes, nil
	}
	err := checker.Check(t.Context())
	require.NoError(t, err)
}

// TestEventThrottledChecker_Passes_WhenExplicitThrottledNodeMatches verifies
// that specifying `throttledNodes` bypasses the stake-based inference and
// designates the named node as the throttled validator. In this scenario
// stakes are equal (so stake-based inference would fail to find a
// throttled validator), but the explicit configuration lets the check
// succeed.
func TestEventThrottledChecker_Passes_WhenExplicitThrottledNodeMatches(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)
	node1 := driver.NewMockNode(ctrl)
	node2 := driver.NewMockNode(ctrl)
	rpcClient := rpc.NewMockClient(ctrl)

	validatorId1 := 1
	validatorId2 := 2

	net.EXPECT().GetActiveNodes().Return(
		[]driver.Node{node1, node2},
	)
	node1.EXPECT().IsExpectedFailure().Return(false)
	node1.EXPECT().DialRpc(gomock.Any()).Return(rpcClient, nil)
	// throttledSetFromLabels reads GetValidatorId/GetLabel once per node;
	// nodeLabels also reads them once, giving two calls each.
	node1.EXPECT().GetValidatorId().Return(&validatorId1).Times(2)
	node1.EXPECT().GetLabel().Return("unthrottled").Times(2)
	node2.EXPECT().GetValidatorId().Return(&validatorId2).Times(2)
	node2.EXPECT().GetLabel().Return("throttled").Times(2)

	rpcClient.EXPECT().
		Call(gomock.Any(), "eth_currentEpoch").
		SetArg(0, hexutil.Uint64(1))

	events := make([]common.Hash, 11)
	for i := range events {
		events[i] = common.BigToHash(big.NewInt(int64(i + 1)))
	}
	heads := make([]string, len(events))
	for i, e := range events {
		heads[i] = e.Hex()
	}
	rpcClient.EXPECT().
		Call(gomock.Any(), "dag_getHeads", "0x1").
		SetArg(0, heads)

	for i, e := range events {
		creator := "0x1"
		if i == len(events)-1 {
			creator = "0x2"
		}
		rpcClient.EXPECT().
			Call(gomock.Any(), "dag_getEvent", e.Hex()).
			SetArg(0, map[string]any{
				"creator": creator,
				"parents": []any{},
			})
	}

	rpcClient.EXPECT().Close()

	checker := newEventThrottledChecker(net)
	checker.ceiling = 50
	checker.throttledNodes = []string{"throttled"}
	// fetchStakes must not be called when throttledNodes is set.
	checker.fetchStakes = func(rpc.Client) (map[uint64]*big.Int, error) {
		t.Fatal("fetchStakes should not be called when throttledNodes is set")
		return nil, nil
	}
	err := checker.Check(t.Context())
	require.NoError(t, err)
}

// TestEventThrottledChecker_Fails_WhenExplicitThrottledNodeEmitsTooMany
// verifies that if the explicitly named node emits too many events
// relative to the unthrottled reference, the check fails.
func TestEventThrottledChecker_Fails_WhenExplicitThrottledNodeEmitsTooMany(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)
	node1 := driver.NewMockNode(ctrl)
	node2 := driver.NewMockNode(ctrl)
	rpcClient := rpc.NewMockClient(ctrl)

	validatorId1 := 1
	validatorId2 := 2

	net.EXPECT().GetActiveNodes().Return(
		[]driver.Node{node1, node2},
	)
	node1.EXPECT().IsExpectedFailure().Return(false)
	node1.EXPECT().DialRpc(gomock.Any()).Return(rpcClient, nil)
	node1.EXPECT().GetValidatorId().Return(&validatorId1).Times(2)
	node1.EXPECT().GetLabel().Return("unthrottled").Times(2)
	node2.EXPECT().GetValidatorId().Return(&validatorId2).Times(2)
	node2.EXPECT().GetLabel().Return("throttled").Times(2)

	rpcClient.EXPECT().
		Call(gomock.Any(), "eth_currentEpoch").
		SetArg(0, hexutil.Uint64(1))

	// 3 events: unthrottled=2, throttled=1 → ratio 50%
	events := []common.Hash{
		common.BigToHash(big.NewInt(1)),
		common.BigToHash(big.NewInt(2)),
		common.BigToHash(big.NewInt(3)),
	}
	heads := []string{events[0].Hex(), events[1].Hex(), events[2].Hex()}
	rpcClient.EXPECT().
		Call(gomock.Any(), "dag_getHeads", "0x1").
		SetArg(0, heads)

	creators := []string{"0x1", "0x1", "0x2"}
	for i, e := range events {
		rpcClient.EXPECT().
			Call(gomock.Any(), "dag_getEvent", e.Hex()).
			SetArg(0, map[string]any{
				"creator": creators[i],
				"parents": []any{},
			})
	}

	rpcClient.EXPECT().Close()

	checker := newEventThrottledChecker(net)
	checker.ceiling = 25 // 50% > 25%
	checker.throttledNodes = []string{"throttled"}
	checker.fetchStakes = func(rpc.Client) (map[uint64]*big.Int, error) {
		t.Fatal("fetchStakes should not be called when throttledNodes is set")
		return nil, nil
	}
	err := checker.Check(t.Context())
	require.Error(t, err)
	require.Contains(t, err.Error(), "expected at most 25%")
}
