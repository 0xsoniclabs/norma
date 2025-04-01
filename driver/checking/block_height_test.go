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
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)
	node1 := driver.NewMockNode(ctrl)
	node2 := driver.NewMockNode(ctrl)
	rpc := rpc.NewMockClient(ctrl)
	net.EXPECT().GetActiveNodes().MinTimes(1).Return([]driver.Node{node1, node2})
	node1.EXPECT().DialRpc().MinTimes(1).Return(rpc, nil)
	node1.EXPECT().IsExpectedFailure()
	node2.EXPECT().DialRpc().MinTimes(1).Return(rpc, nil)
	node2.EXPECT().IsExpectedFailure()

	blockHeight := "0x1234"
	rpc.EXPECT().Call(gomock.Any(), "eth_blockNumber").Times(2).SetArg(0, blockHeight)
	rpc.EXPECT().Close().Times(2)

	c := BlockHeightChecker{net: net}
	err := c.Check()
	if err != nil {
		t.Errorf("unexpected error from BlockHeightChecker: %v", err)
	}
}

func TestBlockHeightCheckerInvalid(t *testing.T) {
	tests := []struct {
		name         string
		blockHeight1 string
		blockHeight2 string
	}{
		{name: "ascending", blockHeight1: "0x42", blockHeight2: "0x1234"},
		{name: "descending", blockHeight1: "0x1234", blockHeight2: "0x42"},
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

			c := BlockHeightChecker{net: net}
			err := c.Check()
			if err == nil || !strings.Contains(err.Error(), "reports too old block") {
				t.Errorf("unexpected error from BlockHeightChecker: %v", err)
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

	c := BlockHeightChecker{net: net}
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

	c := BlockHeightChecker{net: net}
	if err := c.Check(); err == nil || !strings.Contains(err.Error(), "unexpected failure set to provide the block height") {
		t.Errorf("unexpected error: %v", err)
	}
}
