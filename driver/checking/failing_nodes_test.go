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
	"strings"
	"testing"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/rpc"
	"github.com/ethereum/go-ethereum/common"
	"go.uber.org/mock/gomock"
)

// healthyRef sets up a non-failing reference node reporting the given height and hash.
func healthyRef(ctrl *gomock.Controller, label, height string, hash *blockHashes) driver.Node {
	client := rpc.NewMockClient(ctrl)
	client.EXPECT().Close()
	client.EXPECT().Call(gomock.Any(), "eth_blockNumber").SetArg(0, height).AnyTimes()
	client.EXPECT().Call(gomock.Any(), "eth_getBlockByNumber", gomock.Any(), false).SetArg(0, hash).AnyTimes()

	node := driver.NewMockNode(ctrl)
	node.EXPECT().IsExpectedFailure().AnyTimes().Return(false)
	node.EXPECT().GetLabel().AnyTimes().Return(label)
	node.EXPECT().DialRpc(gomock.Any()).Return(client, nil)
	return node
}

func TestFailingNodes_NoFailingNodes_ReturnsNil(t *testing.T) {
	ctrl := gomock.NewController(t)

	node := driver.NewMockNode(ctrl)
	node.EXPECT().IsExpectedFailure().AnyTimes().Return(false)
	node.EXPECT().GetLabel().AnyTimes().Return("healthy")

	net := driver.NewMockNetwork(ctrl)
	net.EXPECT().GetActiveNodes().MinTimes(1).Return([]driver.Node{node})

	c := &failingNodesChecker{net: net, slack: defaultSlack}
	if err := c.Check(t.Context()); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestFailingNodes_NoHealthyNodes_ReturnsNil(t *testing.T) {
	ctrl := gomock.NewController(t)

	node := driver.NewMockNode(ctrl)
	node.EXPECT().IsExpectedFailure().AnyTimes().Return(true)
	node.EXPECT().GetLabel().AnyTimes().Return("failing")

	net := driver.NewMockNetwork(ctrl)
	net.EXPECT().GetActiveNodes().MinTimes(1).Return([]driver.Node{node})

	c := &failingNodesChecker{net: net, slack: defaultSlack}
	if err := c.Check(t.Context()); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestFailingNodes_Forked_Deviates(t *testing.T) {
	ctrl := gomock.NewController(t)

	canonical := &blockHashes{Hash: common.Hash{0x11}, StateRoot: common.Hash{0x22}, ReceiptsRoot: common.Hash{0x33}}
	forked := &blockHashes{Hash: common.Hash{0xAA}, StateRoot: common.Hash{0xBB}, ReceiptsRoot: common.Hash{0xCC}}

	ref := healthyRef(ctrl, "healthy", "64", canonical)

	// Same height, divergent hash: a fork.
	failClient := rpc.NewMockClient(ctrl)
	failClient.EXPECT().Close()
	failClient.EXPECT().Call(gomock.Any(), "eth_blockNumber").SetArg(0, "64")
	failClient.EXPECT().Call(gomock.Any(), "eth_getBlockByNumber", gomock.Any(), false).SetArg(0, forked)

	failing := driver.NewMockNode(ctrl)
	failing.EXPECT().IsExpectedFailure().AnyTimes().Return(true)
	failing.EXPECT().IsRunning().AnyTimes().Return(true)
	failing.EXPECT().GetLabel().AnyTimes().Return("forker")
	failing.EXPECT().DialRpc(gomock.Any()).Return(failClient, nil)

	net := driver.NewMockNetwork(ctrl)
	net.EXPECT().GetActiveNodes().MinTimes(1).Return([]driver.Node{ref, failing})

	c := &failingNodesChecker{net: net, slack: defaultSlack}
	if err := c.Check(t.Context()); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestFailingNodes_Behind_Deviates(t *testing.T) {
	ctrl := gomock.NewController(t)

	canonical := &blockHashes{Hash: common.Hash{0x11}}
	ref := healthyRef(ctrl, "healthy", "64", canonical) // block 100

	// Far behind: no hash comparison is reached.
	failClient := rpc.NewMockClient(ctrl)
	failClient.EXPECT().Close()
	failClient.EXPECT().Call(gomock.Any(), "eth_blockNumber").SetArg(0, "a") // block 10

	failing := driver.NewMockNode(ctrl)
	failing.EXPECT().IsExpectedFailure().AnyTimes().Return(true)
	failing.EXPECT().IsRunning().AnyTimes().Return(true)
	failing.EXPECT().GetLabel().AnyTimes().Return("laggard")
	failing.EXPECT().DialRpc(gomock.Any()).Return(failClient, nil)

	net := driver.NewMockNetwork(ctrl)
	net.EXPECT().GetActiveNodes().MinTimes(1).Return([]driver.Node{ref, failing})

	c := &failingNodesChecker{net: net, slack: defaultSlack}
	if err := c.Check(t.Context()); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestFailingNodes_Unreachable_Deviates(t *testing.T) {
	ctrl := gomock.NewController(t)

	canonical := &blockHashes{Hash: common.Hash{0x11}}
	ref := healthyRef(ctrl, "healthy", "64", canonical)

	// Running but its RPC cannot be dialed.
	failing := driver.NewMockNode(ctrl)
	failing.EXPECT().IsExpectedFailure().AnyTimes().Return(true)
	failing.EXPECT().IsRunning().AnyTimes().Return(true)
	failing.EXPECT().GetLabel().AnyTimes().Return("unreachable")
	failing.EXPECT().DialRpc(gomock.Any()).Return(nil, fmt.Errorf("connection refused"))

	net := driver.NewMockNetwork(ctrl)
	net.EXPECT().GetActiveNodes().MinTimes(1).Return([]driver.Node{ref, failing})

	c := &failingNodesChecker{net: net, slack: defaultSlack}
	if err := c.Check(t.Context()); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestFailingNodes_Stopped_Deviates(t *testing.T) {
	ctrl := gomock.NewController(t)

	canonical := &blockHashes{Hash: common.Hash{0x11}}
	ref := healthyRef(ctrl, "healthy", "64", canonical)

	// Stopped node is never dialed.
	failing := driver.NewMockNode(ctrl)
	failing.EXPECT().IsExpectedFailure().AnyTimes().Return(true)
	failing.EXPECT().IsRunning().AnyTimes().Return(false)
	failing.EXPECT().GetLabel().AnyTimes().Return("stopped")

	net := driver.NewMockNetwork(ctrl)
	net.EXPECT().GetActiveNodes().MinTimes(1).Return([]driver.Node{ref, failing})

	c := &failingNodesChecker{net: net, slack: defaultSlack}
	if err := c.Check(t.Context()); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestFailingNodes_StillHealthy_ReturnsError(t *testing.T) {
	ctrl := gomock.NewController(t)

	canonical := &blockHashes{Hash: common.Hash{0x11}, StateRoot: common.Hash{0x22}, ReceiptsRoot: common.Hash{0x33}}
	ref := healthyRef(ctrl, "healthy", "64", canonical) // block 100

	// Same height and hash as canonical: marked failing but actually healthy.
	failClient := rpc.NewMockClient(ctrl)
	failClient.EXPECT().Close()
	failClient.EXPECT().Call(gomock.Any(), "eth_blockNumber").SetArg(0, "64")
	failClient.EXPECT().Call(gomock.Any(), "eth_getBlockByNumber", gomock.Any(), false).SetArg(0, canonical)

	failing := driver.NewMockNode(ctrl)
	failing.EXPECT().IsExpectedFailure().AnyTimes().Return(true)
	failing.EXPECT().IsRunning().AnyTimes().Return(true)
	failing.EXPECT().GetLabel().AnyTimes().Return("impostor")
	failing.EXPECT().DialRpc(gomock.Any()).Return(failClient, nil)

	net := driver.NewMockNetwork(ctrl)
	net.EXPECT().GetActiveNodes().MinTimes(1).Return([]driver.Node{ref, failing})

	c := &failingNodesChecker{net: net, slack: defaultSlack}
	err := c.Check(t.Context())
	if err == nil || !strings.Contains(err.Error(), "still healthy") {
		t.Errorf("expected still-healthy error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "impostor") {
		t.Errorf("expected error to name the offending node, got: %v", err)
	}
}
