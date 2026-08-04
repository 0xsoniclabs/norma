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
	"errors"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/rpc"
	"go.uber.org/mock/gomock"
)

func TestBlockHeightCheckerValid(t *testing.T) {
	tests := []struct {
		name         string
		blockHeight1 string
		blockHeight2 string
		slack        int
		config       CheckerConfig
	}{
		{name: "within-tolerance-big-asc", blockHeight1: "0x42", blockHeight2: "0x52", slack: 16},
		{name: "within-tolerance-big-desc", blockHeight1: "0x52", blockHeight2: "0x42", slack: 16},
		{name: "within-tolerance", blockHeight1: "0x42", blockHeight2: "0x43", slack: 1},
		{name: "constant", blockHeight1: "0x42", blockHeight2: "0x42", slack: 0},
		{name: "within-tolerance-big-asc-configured", blockHeight1: "0x42", blockHeight2: "0x52", slack: 1, config: CheckerConfig{"slack": 16}},
		{name: "within-tolerance-big-desc-configured", blockHeight1: "0x52", blockHeight2: "0x42", slack: 1, config: CheckerConfig{"slack": 16}},
		{name: "empty-config", blockHeight1: "0x52", blockHeight2: "0x42", slack: 16, config: CheckerConfig{}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			net := driver.NewMockNetwork(ctrl)
			node1 := driver.NewMockNode(ctrl)
			node2 := driver.NewMockNode(ctrl)
			rpc1 := rpc.NewMockClient(ctrl)
			rpc2 := rpc.NewMockClient(ctrl)
			net.EXPECT().GetActiveNodes().MinTimes(1).Return([]driver.Node{node1, node2})
			node1.EXPECT().DialRpc(gomock.Any()).MinTimes(1).Return(rpc1, nil)
			node1.EXPECT().IsExpectedFailure().AnyTimes()
			node2.EXPECT().DialRpc(gomock.Any()).MinTimes(1).Return(rpc2, nil)
			node2.EXPECT().IsExpectedFailure().AnyTimes()
			node1.EXPECT().GetLabel().AnyTimes().Return("node1")
			node2.EXPECT().GetLabel().AnyTimes().Return("node2")

			rpc1.EXPECT().Call(gomock.Any(), "eth_blockNumber").SetArg(0, test.blockHeight1)
			rpc2.EXPECT().Call(gomock.Any(), "eth_blockNumber").SetArg(0, test.blockHeight2)
			rpc1.EXPECT().Close()
			rpc2.EXPECT().Close()

			checker := blockHeightChecker{net: net, slack: test.slack}
			if err := checker.Configure(test.config).Check(t.Context()); err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestBlockHeightCheckerInvalid_WithSlack(t *testing.T) {
	tests := []struct {
		name         string
		blockHeight1 string
		blockHeight2 string
		slack        int
		config       CheckerConfig
	}{
		{name: "should-reject-asc", blockHeight1: "0x42", blockHeight2: "0x1234", slack: 5},
		{name: "should-reject-desc", blockHeight1: "0x1234", blockHeight2: "0x42", slack: 5},
		{name: "no-slack", blockHeight1: "0x42", blockHeight2: "0x43", slack: 0},
		{name: "should-reject-asc-configured", blockHeight1: "0x42", blockHeight2: "0x52", slack: 255, config: CheckerConfig{"slack": 5}},
		{name: "should-reject-desc-configured", blockHeight1: "0x52", blockHeight2: "0x42", slack: 255, config: CheckerConfig{"slack": 5}},
		{name: "no-slack-configured", blockHeight1: "0x42", blockHeight2: "0x43", slack: 255, config: CheckerConfig{"slack": 0}},
		{name: "empty-config", blockHeight1: "0x42", blockHeight2: "0x1234", slack: 5, config: CheckerConfig{}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			net := driver.NewMockNetwork(ctrl)
			node1 := driver.NewMockNode(ctrl)
			node2 := driver.NewMockNode(ctrl)
			rpc1 := rpc.NewMockClient(ctrl)
			rpc2 := rpc.NewMockClient(ctrl)
			net.EXPECT().GetActiveNodes().MinTimes(1).Return([]driver.Node{node1, node2})
			node1.EXPECT().DialRpc(gomock.Any()).MinTimes(1).Return(rpc1, nil)
			node1.EXPECT().IsExpectedFailure().AnyTimes()
			node2.EXPECT().DialRpc(gomock.Any()).MinTimes(1).Return(rpc2, nil)
			node2.EXPECT().IsExpectedFailure().AnyTimes()
			node1.EXPECT().GetLabel().AnyTimes().Return("node1")
			node2.EXPECT().GetLabel().AnyTimes().Return("node2")

			rpc1.EXPECT().Call(gomock.Any(), "eth_blockNumber").SetArg(0, test.blockHeight1)
			rpc2.EXPECT().Call(gomock.Any(), "eth_blockNumber").SetArg(0, test.blockHeight2)
			rpc1.EXPECT().Close()
			rpc2.EXPECT().Close()

			checker := blockHeightChecker{net: net, slack: test.slack}
			if err := checker.Configure(test.config).Check(t.Context()); err == nil || !strings.Contains(err.Error(), "reports too old block") {
				t.Errorf("Block Height check should failed, got: %v", err)
			}
		})
	}
}

func TestBlockHeight_ExpectedFailingNode(t *testing.T) {
	ctrl := gomock.NewController(t)
	rpc1 := rpc.NewMockClient(ctrl)
	rpc2 := rpc.NewMockClient(ctrl)

	node1 := driver.NewMockNode(ctrl)
	node1.EXPECT().IsExpectedFailure().AnyTimes().Return(false)
	node1.EXPECT().DialRpc(gomock.Any()).Return(rpc1, nil)
	node1.EXPECT().GetLabel().AnyTimes().Return("node1")

	node2 := driver.NewMockNode(ctrl)
	node2.EXPECT().IsExpectedFailure().AnyTimes().Return(true)
	node2.EXPECT().DialRpc(gomock.Any()).Return(rpc2, nil)
	node2.EXPECT().GetLabel().AnyTimes().Return("node2")

	net := driver.NewMockNetwork(ctrl)
	net.EXPECT().GetActiveNodes().MinTimes(1).Return([]driver.Node{node1, node2})

	rpc1.EXPECT().Call(gomock.Any(), "eth_blockNumber").SetArg(0, "1000")
	rpc1.EXPECT().Close()
	rpc2.EXPECT().Call(gomock.Any(), "eth_blockNumber").SetArg(0, "10") // block is late
	rpc2.EXPECT().Close()

	c := blockHeightChecker{net: net}
	if err := c.Check(t.Context()); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestBlockHeight_NoFailure_When_Expected(t *testing.T) {
	ctrl := gomock.NewController(t)
	rpc1 := rpc.NewMockClient(ctrl)
	rpc2 := rpc.NewMockClient(ctrl)

	node1 := driver.NewMockNode(ctrl)
	node1.EXPECT().IsExpectedFailure().AnyTimes().Return(false)
	node1.EXPECT().DialRpc(gomock.Any()).Return(rpc1, nil)
	node1.EXPECT().GetLabel().AnyTimes().Return("node1")

	node2 := driver.NewMockNode(ctrl)
	node2.EXPECT().IsExpectedFailure().AnyTimes().Return(true)
	node2.EXPECT().DialRpc(gomock.Any()).Return(rpc2, nil)
	node2.EXPECT().GetLabel().AnyTimes().Return("node2")

	net := driver.NewMockNetwork(ctrl)
	net.EXPECT().GetActiveNodes().MinTimes(1).Return([]driver.Node{node1, node2})

	rpc1.EXPECT().Call(gomock.Any(), "eth_blockNumber").SetArg(0, "1000")
	rpc1.EXPECT().Close()
	rpc2.EXPECT().Call(gomock.Any(), "eth_blockNumber").SetArg(0, "1000")
	rpc2.EXPECT().Close()

	c := blockHeightChecker{net: net}
	if err := c.Check(t.Context()); err == nil || !strings.Contains(err.Error(), "unexpected failure set to provide the block height") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestBlockHeight_GenesisWithinSlackDoesNotFail(t *testing.T) {
	ctrl := gomock.NewController(t)
	rpc1 := rpc.NewMockClient(ctrl)
	rpc2 := rpc.NewMockClient(ctrl)

	node1 := driver.NewMockNode(ctrl)
	node1.EXPECT().IsExpectedFailure().AnyTimes().Return(false)
	node1.EXPECT().DialRpc(gomock.Any()).Return(rpc1, nil)
	node1.EXPECT().GetLabel().AnyTimes().Return("node1")

	node2 := driver.NewMockNode(ctrl)
	node2.EXPECT().IsExpectedFailure().AnyTimes().Return(false)
	node2.EXPECT().DialRpc(gomock.Any()).Return(rpc2, nil)
	node2.EXPECT().GetLabel().AnyTimes().Return("node2")

	net := driver.NewMockNetwork(ctrl)
	net.EXPECT().GetActiveNodes().MinTimes(1).Return([]driver.Node{node1, node2})

	rpc1.EXPECT().Call(gomock.Any(), "eth_blockNumber").SetArg(0, "1") // genesis
	rpc1.EXPECT().Close()
	rpc2.EXPECT().Call(gomock.Any(), "eth_blockNumber").SetArg(0, "3")
	rpc2.EXPECT().Close()

	c := blockHeightChecker{net: net, slack: defaultSlack}
	if err := c.Check(t.Context()); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// convergingNetwork builds a two-node network where the second node reports the
// given heights on successive reads, so a test can let it catch up over time.
func convergingNetwork(
	t *testing.T, leaderHeight string, followerHeights ...string,
) driver.Network {
	t.Helper()
	ctrl := gomock.NewController(t)

	leaderRpc := rpc.NewMockClient(ctrl)
	leaderRpc.EXPECT().Call(gomock.Any(), "eth_blockNumber").
		SetArg(0, leaderHeight).AnyTimes()
	leaderRpc.EXPECT().Close().AnyTimes()

	followerRpc := rpc.NewMockClient(ctrl)
	reads := make([]any, 0, len(followerHeights))
	for i, height := range followerHeights {
		call := followerRpc.EXPECT().
			Call(gomock.Any(), "eth_blockNumber").SetArg(0, height)
		if i == len(followerHeights)-1 {
			call.AnyTimes() // the last answer is the node's final state
		}
		reads = append(reads, call)
	}
	gomock.InOrder(reads...)
	followerRpc.EXPECT().Close().AnyTimes()

	leader := driver.NewMockNode(ctrl)
	leader.EXPECT().IsExpectedFailure().AnyTimes().Return(false)
	leader.EXPECT().DialRpc(gomock.Any()).AnyTimes().Return(leaderRpc, nil)
	leader.EXPECT().GetLabel().AnyTimes().Return("leader")

	follower := driver.NewMockNode(ctrl)
	follower.EXPECT().IsExpectedFailure().AnyTimes().Return(false)
	follower.EXPECT().DialRpc(gomock.Any()).AnyTimes().Return(followerRpc, nil)
	follower.EXPECT().GetLabel().AnyTimes().Return("follower")

	net := driver.NewMockNetwork(ctrl)
	net.EXPECT().GetActiveNodes().AnyTimes().
		Return([]driver.Node{leader, follower})
	return net
}

func TestBlockHeight_WaitsForALaggingNodeToCatchUp(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// A node that is behind right after a transition - a fresh epoch seal, a
		// rejoining validator - must be given the chance to catch up.
		net := convergingNetwork(t, "0x1000", "0x10", "0x900", "0x1000")

		c := &blockHeightChecker{
			net: net, slack: defaultSlack,
			timeout: 30 * time.Second,
		}
		if err := c.Check(t.Context()); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

func TestBlockHeight_FailsWhenNodesNeverConverge(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		net := convergingNetwork(t, "0x1000", "0x10")

		start := time.Now()
		c := &blockHeightChecker{
			net: net, slack: defaultSlack,
			timeout: 30 * time.Second,
		}

		err := c.Check(t.Context())
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}
		if !strings.Contains(err.Error(), "did not agree on a block height") {
			t.Errorf("unexpected error: %v", err)
		}
		// The underlying reason must survive the wrapping.
		if !strings.Contains(err.Error(), "reports too old block") {
			t.Errorf("error does not explain the disagreement: %v", err)
		}
		if waited := time.Since(start); waited < 30*time.Second {
			t.Errorf("gave up after %v, want at least the 30s timeout", waited)
		}
	})
}

func TestBlockHeight_RetriesWhileANodeIsUnreachable(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctrl := gomock.NewController(t)

		healthyRpc := rpc.NewMockClient(ctrl)
		healthyRpc.EXPECT().Call(gomock.Any(), "eth_blockNumber").
			SetArg(0, "0x1000").AnyTimes()
		healthyRpc.EXPECT().Close().AnyTimes()

		restartedRpc := rpc.NewMockClient(ctrl)
		restartedRpc.EXPECT().Call(gomock.Any(), "eth_blockNumber").
			SetArg(0, "0x1000").AnyTimes()
		restartedRpc.EXPECT().Close().AnyTimes()

		healthy := driver.NewMockNode(ctrl)
		healthy.EXPECT().IsExpectedFailure().AnyTimes().Return(false)
		healthy.EXPECT().DialRpc(gomock.Any()).AnyTimes().Return(healthyRpc, nil)
		healthy.EXPECT().GetLabel().AnyTimes().Return("healthy")

		restarted := driver.NewMockNode(ctrl)
		restarted.EXPECT().IsExpectedFailure().AnyTimes().Return(false)
		restarted.EXPECT().GetLabel().AnyTimes().Return("restarted")
		gomock.InOrder(
			restarted.EXPECT().DialRpc(gomock.Any()).
				Return(nil, errors.New("connection refused")),
			restarted.EXPECT().DialRpc(gomock.Any()).
				AnyTimes().Return(restartedRpc, nil),
		)

		net := driver.NewMockNetwork(ctrl)
		net.EXPECT().GetActiveNodes().AnyTimes().
			Return([]driver.Node{healthy, restarted})

		c := &blockHeightChecker{
			net: net, slack: defaultSlack,
			timeout: 30 * time.Second,
		}
		if err := c.Check(t.Context()); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

func TestBlockHeight_ReturnsContextError(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		net := convergingNetwork(t, "0x1000", "0x10")

		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		c := &blockHeightChecker{
			net: net, slack: defaultSlack,
			timeout: 30 * time.Second,
		}
		if err := c.Check(ctx); !errors.Is(err, context.Canceled) {
			t.Errorf("expected a context error, got %v", err)
		}
	})
}

func TestBlockHeight_ZeroTimeoutChecksOnce(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		net := convergingNetwork(t, "0x1000", "0x10", "0x1000")

		start := time.Now()
		c := &blockHeightChecker{net: net, slack: defaultSlack}

		if err := c.Check(t.Context()); err == nil {
			t.Errorf("expected the first disagreement to be reported")
		}
		if elapsed := time.Since(start); elapsed != 0 {
			t.Errorf("a single attempt should not wait, but took %v", elapsed)
		}
	})
}

func TestBlockHeight_Configure(t *testing.T) {
	orig := &blockHeightChecker{slack: 5, timeout: 20 * time.Second}

	if got := orig.Configure(nil); got != orig {
		t.Errorf("nil config should return the original checker")
	}

	empty := orig.Configure(CheckerConfig{}).(*blockHeightChecker)
	if empty.slack != 5 || empty.timeout != 20*time.Second {
		t.Errorf("empty config should copy original values, got %+v", empty)
	}

	set := orig.Configure(CheckerConfig{
		"slack": 7, "duration": int64(40 * time.Second),
	}).(*blockHeightChecker)
	if set.slack != 7 || set.timeout != 40*time.Second {
		t.Errorf("config values not applied, got %+v", set)
	}
}
