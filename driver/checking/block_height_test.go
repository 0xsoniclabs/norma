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
	"strings"
	"testing"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/rpc"
	"go.uber.org/mock/gomock"
)

func TestBlockHeightCheckerValid(t *testing.T) {
	tests := []struct {
		name         string
		blockHeight1 string
		blockHeight2 string
		slack        uint8
		config       map[string]string
	}{
		{name: "within-tolerance-big-asc", blockHeight1: "0x42", blockHeight2: "0x52", slack: 16},
		{name: "within-tolerance-big-desc", blockHeight1: "0x52", blockHeight2: "0x42", slack: 16},
		{name: "within-tolerance", blockHeight1: "0x42", blockHeight2: "0x43", slack: 1},
		{name: "constant", blockHeight1: "0x42", blockHeight2: "0x42", slack: 0},
		{name: "within-tolerance-big-asc-configured", blockHeight1: "0x42", blockHeight2: "0x52", slack: 1, config: map[string]string{"slack": "16"}},
		{name: "within-tolerance-big-desc-configured", blockHeight1: "0x52", blockHeight2: "0x42", slack: 1, config: map[string]string{"slack": "16"}},
		{name: "empty-config", blockHeight1: "0x52", blockHeight2: "0x42", slack: 16, config: map[string]string{}},
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
			node1.EXPECT().DialRpc().MinTimes(1).Return(rpc1, nil)
			node1.EXPECT().IsExpectedFailure().AnyTimes()
			node2.EXPECT().DialRpc().MinTimes(1).Return(rpc2, nil)
			node2.EXPECT().IsExpectedFailure().AnyTimes()
			node1.EXPECT().GetLabel().AnyTimes().Return("node1")
			node2.EXPECT().GetLabel().AnyTimes().Return("node2")

			rpc1.EXPECT().Call(gomock.Any(), "eth_blockNumber").SetArg(0, test.blockHeight1)
			rpc2.EXPECT().Call(gomock.Any(), "eth_blockNumber").SetArg(0, test.blockHeight2)
			rpc1.EXPECT().Close()
			rpc2.EXPECT().Close()

			c := blockHeightChecker{net: net, slack: test.slack}
			configured, err := c.Configure(test.config)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if err := configured.Check(); err != nil {
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
		slack        uint8
		config       map[string]string
	}{
		{name: "should-reject-asc", blockHeight1: "0x42", blockHeight2: "0x1234", slack: 5},
		{name: "should-reject-desc", blockHeight1: "0x1234", blockHeight2: "0x42", slack: 5},
		{name: "no-slack", blockHeight1: "0x42", blockHeight2: "0x43", slack: 0},
		{name: "should-reject-asc", blockHeight1: "0x42", blockHeight2: "0x52", slack: 255, config: map[string]string{"slack": "5"}},
		{name: "should-reject-desc", blockHeight1: "0x52", blockHeight2: "0x42", slack: 255, config: map[string]string{"slack": "5"}},
		{name: "no-slack", blockHeight1: "0x42", blockHeight2: "0x43", slack: 255, config: map[string]string{"slack": "0"}},
		{name: "empty-config", blockHeight1: "0x42", blockHeight2: "0x1234", slack: 5, config: map[string]string{}},
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
			node1.EXPECT().DialRpc().MinTimes(1).Return(rpc1, nil)
			node1.EXPECT().IsExpectedFailure().AnyTimes()
			node2.EXPECT().DialRpc().MinTimes(1).Return(rpc2, nil)
			node2.EXPECT().IsExpectedFailure().AnyTimes()
			node1.EXPECT().GetLabel().AnyTimes().Return("node1")
			node2.EXPECT().GetLabel().AnyTimes().Return("node2")

			rpc1.EXPECT().Call(gomock.Any(), "eth_blockNumber").SetArg(0, test.blockHeight1)
			rpc2.EXPECT().Call(gomock.Any(), "eth_blockNumber").SetArg(0, test.blockHeight2)
			rpc1.EXPECT().Close()
			rpc2.EXPECT().Close()

			c := blockHeightChecker{net: net, slack: test.slack}
			configured, err := c.Configure(test.config)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if err := configured.Check(); err == nil || !strings.Contains(err.Error(), "reports too old block") {
				t.Errorf("Block Height check should failed, got: %v", err)
			}
		})
	}
}

func TestBlockHeightChecker_ConfigureInvalid(t *testing.T) {
	tests := []struct {
		name   string
		config map[string]string
		err    string
	}{
		{name: "test1", config: map[string]string{"slack": "abc"}, err: "failed to convert slack"},
		{name: "test2", config: map[string]string{"slack": "-1"}, err: "invalid slack"},
		{name: "test3", config: map[string]string{"slack": "256"}, err: "invalid slack"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			net := driver.NewMockNetwork(ctrl)

			c := blockHeightChecker{net: net, slack: 123}
			if _, err := c.Configure(test.config); err == nil || !strings.Contains(err.Error(), test.err) {
				t.Errorf("not caught: %s; %v", test.err, err)
			}
		})
	}
}

func TestBlockHeight_ExpectedFailingNode(t *testing.T) {
	ctrl := gomock.NewController(t)
	rpc := rpc.NewMockClient(ctrl)
	rpc.EXPECT().Close().Times(2)

	node1 := driver.NewMockNode(ctrl)
	node1.EXPECT().IsExpectedFailure().AnyTimes().Return(false)
	node1.EXPECT().DialRpc().Return(rpc, nil)
	node1.EXPECT().GetLabel().AnyTimes().Return("node1")

	node2 := driver.NewMockNode(ctrl)
	node2.EXPECT().IsExpectedFailure().AnyTimes().Return(true)
	node2.EXPECT().DialRpc().Return(rpc, nil)
	node2.EXPECT().GetLabel().AnyTimes().Return("node2")

	net := driver.NewMockNetwork(ctrl)
	net.EXPECT().GetActiveNodes().MinTimes(1).Return([]driver.Node{node1, node2})

	gomock.InOrder(
		rpc.EXPECT().Call(gomock.Any(), "eth_blockNumber").SetArg(0, "1000"),
		rpc.EXPECT().Call(gomock.Any(), "eth_blockNumber").SetArg(0, "10"), // block is late
	)

	c := blockHeightChecker{net: net}
	if err := c.Check(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestBlockHeight_NoFailure_When_Expected(t *testing.T) {
	ctrl := gomock.NewController(t)
	rpc := rpc.NewMockClient(ctrl)
	rpc.EXPECT().Close().Times(2)

	node1 := driver.NewMockNode(ctrl)
	node1.EXPECT().IsExpectedFailure().AnyTimes().Return(false)
	node1.EXPECT().DialRpc().Return(rpc, nil)
	node1.EXPECT().GetLabel().AnyTimes().Return("node1")

	node2 := driver.NewMockNode(ctrl)
	node2.EXPECT().IsExpectedFailure().AnyTimes().Return(true)
	node2.EXPECT().DialRpc().Return(rpc, nil)
	node2.EXPECT().GetLabel().AnyTimes().Return("node2")

	net := driver.NewMockNetwork(ctrl)
	net.EXPECT().GetActiveNodes().MinTimes(1).Return([]driver.Node{node1, node2})

	gomock.InOrder(
		rpc.EXPECT().Call(gomock.Any(), "eth_blockNumber").SetArg(0, "1000"),
		rpc.EXPECT().Call(gomock.Any(), "eth_blockNumber").SetArg(0, "1000"),
	)

	c := blockHeightChecker{net: net}
	if err := c.Check(); err == nil || !strings.Contains(err.Error(), "unexpected failure set to provide the block height") {
		t.Errorf("unexpected error: %v", err)
	}
}
