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
	"math/big"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/rpc"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestVerifyThrottled_Passes_WhenGapAboveThreshold(t *testing.T) {
	cases := map[string]struct {
		expected map[uint64]struct{}
		rates    map[uint64]float64
	}{
		"single throttled validator, zero rate": {
			expected: map[uint64]struct{}{2: {}},
			rates:    map[uint64]float64{1: 10.0, 2: 0.0},
		},
		"single throttled validator, low rate": {
			expected: map[uint64]struct{}{2: {}},
			rates:    map[uint64]float64{1: 10.0, 2: 1.0},
		},
		"multiple throttled, all below gap": {
			expected: map[uint64]struct{}{2: {}, 3: {}},
			rates:    map[uint64]float64{1: 10.0, 2: 1.0, 3: 2.0},
		},
		"gap exactly at threshold": {
			expected: map[uint64]struct{}{2: {}},
			rates:    map[uint64]float64{1: 10.0, 2: 5.0},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			require.NoError(t, verifyThrottled(tc.expected, tc.rates))
		})
	}
}

func TestVerifyThrottled_Fails_WhenGapTooNarrow(t *testing.T) {
	cases := map[string]struct {
		expected map[uint64]struct{}
		rates    map[uint64]float64
	}{
		"listed validator emits at full speed": {
			expected: map[uint64]struct{}{2: {}},
			rates:    map[uint64]float64{1: 10.0, 2: 9.5},
		},
		"unlisted validator emits suspiciously slowly": {
			expected: map[uint64]struct{}{2: {}},
			rates:    map[uint64]float64{1: 1.5, 2: 1.0},
		},
		"all rates uniform": {
			expected: map[uint64]struct{}{2: {}, 3: {}},
			rates:    map[uint64]float64{1: 5.0, 2: 5.0, 3: 5.0},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			err := verifyThrottled(tc.expected, tc.rates)
			require.Error(t, err)
			require.Contains(t, err.Error(), "no throttling detected")
		})
	}
}

func TestVerifyThrottled_Fails_WhenAllRatesZero(t *testing.T) {
	err := verifyThrottled(
		map[uint64]struct{}{2: {}},
		map[uint64]float64{1: 0.0, 2: 0.0},
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no validator emitted events")
}

func TestVerifyThrottled_Fails_WhenNoUnlistedValidator(t *testing.T) {
	err := verifyThrottled(
		map[uint64]struct{}{1: {}, 2: {}},
		map[uint64]float64{1: 0.0, 2: 0.0},
	)
	require.Error(t, err)
	require.Contains(t, err.Error(),
		"no unthrottled validator observed for comparison")
}

func TestVerifyThrottled_Fails_WhenNoListedValidator(t *testing.T) {
	err := verifyThrottled(
		map[uint64]struct{}{99: {}},
		map[uint64]float64{1: 10.0, 2: 10.0},
	)
	require.Error(t, err)
	require.Contains(t, err.Error(),
		"no expected-throttled validator observed")
}

func TestResolveLabels_ResolvesLabels_ToValidatorIds(t *testing.T) {
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

	labels, throttled, err := resolveLabels(
		[]driver.Node{nodeA, nodeB, nodeC}, []string{"beta"},
	)
	require.NoError(t, err)
	require.Equal(t, map[uint64]struct{}{2: {}}, throttled)
	require.Equal(t, map[uint64]string{1: "alpha", 2: "beta"}, labels)
}

func TestResolveLabels_ReturnsError_WhenLabelUnknown(t *testing.T) {
	ctrl := gomock.NewController(t)
	nodeA := driver.NewMockNode(ctrl)
	idA := 1
	nodeA.EXPECT().GetValidatorId().Return(&idA)
	nodeA.EXPECT().GetLabel().Return("alpha")

	_, _, err := resolveLabels(
		[]driver.Node{nodeA}, []string{"missing"},
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), `label "missing"`)
}

func TestEventThrottledChecker_AppliesConfiguration_WhenProvided(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)
	original := newEventThrottledChecker(net)

	t.Run("throttled nodes from []any", func(t *testing.T) {
		configured := original.Configure(CheckerConfig{
			"throttledNodes": []any{"node-a", "node-b"},
		})
		c := configured.(*eventThrottledChecker)
		require.Equal(t, []string{"node-a", "node-b"}, c.throttledNodes)
	})

	t.Run("throttled nodes from []string", func(t *testing.T) {
		configured := original.Configure(CheckerConfig{
			"throttledNodes": []string{"node-a"},
		})
		c := configured.(*eventThrottledChecker)
		require.Equal(t, []string{"node-a"}, c.throttledNodes)
	})

	t.Run("empty config leaves throttled nodes unset", func(t *testing.T) {
		configured := original.Configure(CheckerConfig{})
		c := configured.(*eventThrottledChecker)
		require.Empty(t, c.throttledNodes)
	})

	t.Run("unknown keys are silently ignored", func(t *testing.T) {
		configured := original.Configure(CheckerConfig{
			"ceiling":        50,
			"stakeThreshold": 0.75,
			"throttledNodes": []any{"node-a"},
		})
		c := configured.(*eventThrottledChecker)
		require.Equal(t, []string{"node-a"}, c.throttledNodes)
	})

	t.Run("sample window is preserved through Configure", func(t *testing.T) {
		original.sampleWindow = 5 * time.Second
		configured := original.Configure(CheckerConfig{
			"throttledNodes": []any{"node-a"},
		})
		c := configured.(*eventThrottledChecker)
		require.Equal(t, 5*time.Second, c.sampleWindow)
	})
}

func TestEventThrottledChecker_ReturnsError_WhenThrottledNodesEmpty(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)

	checker := newEventThrottledChecker(net)
	err := checker.Check(t.Context())
	require.Error(t, err)
	require.Contains(t, err.Error(), "throttledNodes must not be empty")
}

func TestEventThrottledChecker_ReturnsError_WhenNoActiveNodes(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)
	net.EXPECT().GetActiveNodes().Return(nil)

	checker := newEventThrottledChecker(net)
	checker.throttledNodes = []string{"any"}
	err := checker.Check(t.Context())
	require.Error(t, err)
	require.Contains(t, err.Error(), "no active nodes")
}

func TestEventThrottledChecker_ReturnsError_WhenNoReachableNode(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)
	node := driver.NewMockNode(ctrl)

	validatorId := 1
	net.EXPECT().GetActiveNodes().Return([]driver.Node{node})
	// resolveLabels reads id/label first; then dialFirstReachable
	// sees the failure marker and skips the node.
	node.EXPECT().GetValidatorId().Return(&validatorId)
	node.EXPECT().GetLabel().Return("any")
	node.EXPECT().IsExpectedFailure().Return(true)

	checker := newEventThrottledChecker(net)
	checker.throttledNodes = []string{"any"}
	err := checker.Check(t.Context())
	require.Error(t, err)
	require.Contains(t, err.Error(), "no reachable node")
}

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
	// resolveLabels walks the node list once.
	node1.EXPECT().GetValidatorId().Return(&validatorId1)
	node1.EXPECT().GetLabel().Return("unthrottled")
	node2.EXPECT().GetValidatorId().Return(&validatorId2)
	node2.EXPECT().GetLabel().Return("throttled")
	rpcClient.EXPECT().Close()

	checker := newEventThrottledChecker(net)
	checker.throttledNodes = []string{"throttled"}
	checker.collectRates = func(
		context.Context, rpc.Client,
	) (map[uint64]float64, error) {
		return map[uint64]float64{1: 10.0, 2: 1.0}, nil
	}
	err := checker.Check(t.Context())
	require.NoError(t, err)
}

func TestEventThrottledChecker_Fails_WhenListedNodeIsUnthrottled(t *testing.T) {
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
	node2.EXPECT().GetLabel().Return("also-dominant")
	rpcClient.EXPECT().Close()

	checker := newEventThrottledChecker(net)
	checker.throttledNodes = []string{"also-dominant"}
	checker.collectRates = func(
		context.Context, rpc.Client,
	) (map[uint64]float64, error) {
		return map[uint64]float64{1: 10.0, 2: 10.0}, nil
	}
	err := checker.Check(t.Context())
	require.Error(t, err)
	require.Contains(t, err.Error(), "no throttling detected")
}

func TestCollectEmissionRates_ReturnsDeltaRates_WhenEpochStable(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctrl := gomock.NewController(t)
		rpcClient := rpc.NewMockClient(ctrl)

		// Two snapshots, both in epoch 1.
		// Snapshot 1: 6 events (5 by v1, 1 by v2).
		// Snapshot 2: 18 events (15 by v1, 3 by v2).
		// Delta: creator 1 => 10 events, creator 2 => 2 events.
		s1Heads, s1Events := buildIndependentHeads(
			[]uint64{1, 1, 1, 1, 1, 2},
		)
		// Snapshot 2: 18 events (15 by v1, 3 by v2), continuing.
		s2Creators := make([]uint64, 0, 18)
		for range 15 {
			s2Creators = append(s2Creators, 1)
		}
		for range 3 {
			s2Creators = append(s2Creators, 2)
		}
		s2Heads, s2Events := buildIndependentHeads(s2Creators)

		// Expect first snapshot RPC sequence.
		rpcClient.EXPECT().
			Call(gomock.Any(), "eth_currentEpoch").
			SetArg(0, hexutil.Uint64(1))
		rpcClient.EXPECT().
			Call(gomock.Any(), "dag_getHeads", "0x1").
			SetArg(0, s1Heads)
		for h, ev := range s1Events {
			rpcClient.EXPECT().
				Call(gomock.Any(), "dag_getEvent", h).
				SetArg(0, ev)
		}
		// Expect second snapshot RPC sequence.
		rpcClient.EXPECT().
			Call(gomock.Any(), "eth_currentEpoch").
			SetArg(0, hexutil.Uint64(1))
		rpcClient.EXPECT().
			Call(gomock.Any(), "dag_getHeads", "0x1").
			SetArg(0, s2Heads)
		for h, ev := range s2Events {
			rpcClient.EXPECT().
				Call(gomock.Any(), "dag_getEvent", h).
				SetArg(0, ev)
		}

		checker := sampler(100 * time.Millisecond)
		rates, err := checker.collectEmissionRates(t.Context(), rpcClient)
		require.NoError(t, err)
		// Delta creator 1: 15 - 5 = 10 events over 0.1s => 100/s.
		// Delta creator 2: 3 - 1 = 2 events over 0.1s => 20/s.
		require.InDelta(t, 100.0, rates[1], 0.01)
		require.InDelta(t, 20.0, rates[2], 0.01)
	})
}

func TestSampleEmissionRates_ReturnsError_WhenEpochChanges(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctrl := gomock.NewController(t)
		rpcClient := rpc.NewMockClient(ctrl)

		s1Heads, s1Events := buildIndependentHeads([]uint64{1, 2})
		s2Heads, s2Events := buildIndependentHeads([]uint64{1, 2})

		rpcClient.EXPECT().
			Call(gomock.Any(), "eth_currentEpoch").
			SetArg(0, hexutil.Uint64(1))
		rpcClient.EXPECT().
			Call(gomock.Any(), "dag_getHeads", "0x1").
			SetArg(0, s1Heads)
		for h, ev := range s1Events {
			rpcClient.EXPECT().
				Call(gomock.Any(), "dag_getEvent", h).
				SetArg(0, ev)
		}
		// Second snapshot advertises a different epoch.
		rpcClient.EXPECT().
			Call(gomock.Any(), "eth_currentEpoch").
			SetArg(0, hexutil.Uint64(2))
		rpcClient.EXPECT().
			Call(gomock.Any(), "dag_getHeads", "0x2").
			SetArg(0, s2Heads)
		for h, ev := range s2Events {
			rpcClient.EXPECT().
				Call(gomock.Any(), "dag_getEvent", h).
				SetArg(0, ev)
		}

		_, err := sampler(50*time.Millisecond).
			sampleEmissionRates(t.Context(), rpcClient)
		require.Error(t, err)
		require.Contains(t, err.Error(), "epoch changed during sampling window")
	})
}

// TestCollectEmissionRates_Retries_OnEpochChange verifies that the
// top-level rate collector transparently retries when the first sample
// straddles an epoch boundary and eventually succeeds.
func TestCollectEmissionRates_Retries_OnEpochChange(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctrl := gomock.NewController(t)
		rpcClient := rpc.NewMockClient(ctrl)

		// --- Attempt 1: epoch 1 -> epoch 2, discarded.
		a1s1Heads, a1s1Events := buildIndependentHeads([]uint64{1})
		a1s2Heads, a1s2Events := buildIndependentHeads([]uint64{1})
		rpcClient.EXPECT().
			Call(gomock.Any(), "eth_currentEpoch").
			SetArg(0, hexutil.Uint64(1))
		rpcClient.EXPECT().
			Call(gomock.Any(), "dag_getHeads", "0x1").
			SetArg(0, a1s1Heads)
		for h, ev := range a1s1Events {
			rpcClient.EXPECT().
				Call(gomock.Any(), "dag_getEvent", h).
				SetArg(0, ev)
		}
		rpcClient.EXPECT().
			Call(gomock.Any(), "eth_currentEpoch").
			SetArg(0, hexutil.Uint64(2))
		rpcClient.EXPECT().
			Call(gomock.Any(), "dag_getHeads", "0x2").
			SetArg(0, a1s2Heads)
		for h, ev := range a1s2Events {
			rpcClient.EXPECT().
				Call(gomock.Any(), "dag_getEvent", h).
				SetArg(0, ev)
		}

		// --- Attempt 2: stable in epoch 2, succeeds.
		a2s1Heads, a2s1Events := buildIndependentHeads([]uint64{1})
		a2s2Heads, a2s2Events := buildIndependentHeads([]uint64{1, 1})
		rpcClient.EXPECT().
			Call(gomock.Any(), "eth_currentEpoch").
			SetArg(0, hexutil.Uint64(2))
		rpcClient.EXPECT().
			Call(gomock.Any(), "dag_getHeads", "0x2").
			SetArg(0, a2s1Heads)
		for h, ev := range a2s1Events {
			rpcClient.EXPECT().
				Call(gomock.Any(), "dag_getEvent", h).
				SetArg(0, ev)
		}
		rpcClient.EXPECT().
			Call(gomock.Any(), "eth_currentEpoch").
			SetArg(0, hexutil.Uint64(2))
		rpcClient.EXPECT().
			Call(gomock.Any(), "dag_getHeads", "0x2").
			SetArg(0, a2s2Heads)
		for h, ev := range a2s2Events {
			rpcClient.EXPECT().
				Call(gomock.Any(), "dag_getEvent", h).
				SetArg(0, ev)
		}

		rates, err := sampler(100*time.Millisecond).
			collectEmissionRates(t.Context(), rpcClient)
		require.NoError(t, err)
		// Delta for creator 1: 2 - 1 = 1 event over 0.1s => 10/s.
		require.InDelta(t, 10.0, rates[1], 0.01)
	})
}

// TestCollectEmissionRates_Retries_WhenEpochHasNoHeadsYet reproduces the
// reported failure: right after an epoch rolls over, dag_getHeads returns
// no heads and the first snapshot fails immediately. Because this path
// does not wait out the sampling window, the retry must not be consumed
// instantly — the collector retries and succeeds once heads appear.
func TestCollectEmissionRates_Retries_WhenEpochHasNoHeadsYet(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctrl := gomock.NewController(t)
		rpcClient := rpc.NewMockClient(ctrl)

		// --- Attempt 1: epoch 1 has rolled over but exposes no heads yet.
		rpcClient.EXPECT().
			Call(gomock.Any(), "eth_currentEpoch").
			SetArg(0, hexutil.Uint64(1))
		rpcClient.EXPECT().
			Call(gomock.Any(), "dag_getHeads", "0x1").
			SetArg(0, []string{})

		// --- Attempt 2: same epoch, now populated, succeeds.
		a2s1Heads, a2s1Events := buildIndependentHeads([]uint64{1})
		a2s2Heads, a2s2Events := buildIndependentHeads([]uint64{1, 1})
		rpcClient.EXPECT().
			Call(gomock.Any(), "eth_currentEpoch").
			SetArg(0, hexutil.Uint64(1))
		rpcClient.EXPECT().
			Call(gomock.Any(), "dag_getHeads", "0x1").
			SetArg(0, a2s1Heads)
		for h, ev := range a2s1Events {
			rpcClient.EXPECT().
				Call(gomock.Any(), "dag_getEvent", h).
				SetArg(0, ev)
		}
		rpcClient.EXPECT().
			Call(gomock.Any(), "eth_currentEpoch").
			SetArg(0, hexutil.Uint64(1))
		rpcClient.EXPECT().
			Call(gomock.Any(), "dag_getHeads", "0x1").
			SetArg(0, a2s2Heads)
		for h, ev := range a2s2Events {
			rpcClient.EXPECT().
				Call(gomock.Any(), "dag_getEvent", h).
				SetArg(0, ev)
		}

		rates, err := sampler(100*time.Millisecond).
			collectEmissionRates(t.Context(), rpcClient)
		require.NoError(t, err)
		// Delta for creator 1: 2 - 1 = 1 event over 0.1s => 10/s.
		require.InDelta(t, 10.0, rates[1], 0.01)
	})
}

// TestCollectEmissionRates_StopsRetrying_WhenContextCancelled verifies that
// once the context is cancelled the collector returns promptly and does not
// start another sampling attempt, rather than burning through the remaining
// attempts. A large backoff makes any missed cancellation observable.
func TestCollectEmissionRates_StopsRetrying_WhenContextCancelled(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctrl := gomock.NewController(t)
		rpcClient := rpc.NewMockClient(ctrl)

		ctx, cancel := context.WithCancel(t.Context())

		// First snapshot fails (no heads) and cancels the context. No second
		// snapshot must be attempted; unexpected extra calls fail the mock.
		rpcClient.EXPECT().
			Call(gomock.Any(), "eth_currentEpoch").
			SetArg(0, hexutil.Uint64(1))
		rpcClient.EXPECT().
			Call(gomock.Any(), "dag_getHeads", "0x1").
			DoAndReturn(func(any, string, ...any) error {
				cancel()
				return nil
			})

		// A large backoff makes a missed cancellation observable: the collector
		// would sit out an hour of the bubble's clock before returning.
		start := time.Now()
		checker := &eventThrottledChecker{
			sampleWindow: time.Second,
			retryBackoff: time.Hour,
		}
		_, err := checker.collectEmissionRates(ctx, rpcClient)
		require.ErrorIs(t, err, context.Canceled)
		require.Zero(t, time.Since(start), "waited after cancellation")
	})
}

// sampler returns a checker that measures emission rates over the given
// nominal window. Its callers run in a synctest bubble, whose fake clock makes
// waiting one out free.
func sampler(window time.Duration) *eventThrottledChecker {
	return &eventThrottledChecker{sampleWindow: window}
}

// TestSampleEmissionRates_UsesTheMeasuredInterval checks that rates are
// computed from the interval actually spent between the two snapshots. Walking
// the DAG costs one RPC per event and the DAG grows with the epoch, so the
// snapshots routinely end up further apart than the requested window. Dividing
// by the requested window instead overstates every rate, the more so the
// busier the network.
func TestSampleEmissionRates_UsesTheMeasuredInterval(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctrl := gomock.NewController(t)
		rpcClient := rpc.NewMockClient(ctrl)

		checker := sampler(100 * time.Millisecond)

		s1Heads, s1Events := buildIndependentHeads([]uint64{1})
		s2Heads, s2Events := buildIndependentHeads([]uint64{1, 1})

		rpcClient.EXPECT().Call(gomock.Any(), "eth_currentEpoch").
			SetArg(0, hexutil.Uint64(1)).Times(2)
		gomock.InOrder(
			rpcClient.EXPECT().Call(gomock.Any(), "dag_getHeads", "0x1").
				SetArg(0, s1Heads),
			rpcClient.EXPECT().Call(gomock.Any(), "dag_getHeads", "0x1").
				SetArg(0, s2Heads),
		)

		// Each event fetched costs 100ms, so the first snapshot's walk alone takes
		// as long as the whole requested window.
		events := map[string]*rawEvent{}
		for h, ev := range s1Events {
			events[h] = ev
		}
		for h, ev := range s2Events {
			events[h] = ev
		}
		rpcClient.EXPECT().Call(gomock.Any(), "dag_getEvent", gomock.Any()).
			AnyTimes().DoAndReturn(func(result any, _ string, args ...any) error {
			time.Sleep(100 * time.Millisecond)
			*(result.(**rawEvent)) = events[args[0].(string)]
			return nil
		})

		rates, err := checker.sampleEmissionRates(t.Context(), rpcClient)
		require.NoError(t, err)

		// One new event by creator 1, over the 100ms window plus the 100ms the
		// first walk took: 5/s, not the 10/s the nominal window would suggest.
		require.InDelta(t, 5.0, rates[1], 0.01)
	})
}

// buildIndependentHeads constructs a set of disconnected head events,
// each with no parents, one per entry in `creators`. Returns the head
// hex strings and a map from head hex to the *rawEvent value to hand
// back via SetArg in RPC mocks.
func buildIndependentHeads(
	creators []uint64,
) (heads []string, events map[string]*rawEvent) {
	heads = make([]string, 0, len(creators))
	events = make(map[string]*rawEvent, len(creators))
	for i, c := range creators {
		h := common.BigToHash(big.NewInt(int64(1_000_000 + i))).Hex()
		heads = append(heads, h)
		events[h] = &rawEvent{Creator: hexutil.Uint64(c)}
	}
	return heads, events
}
