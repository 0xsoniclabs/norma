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
	"testing"
	"time"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/rpc"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// peerCountNetwork builds a network of one node with the given label that
// reports the given peer count via net_peerCount.
func peerCountNetwork(
	ctrl *gomock.Controller, label string, peers uint64,
) *driver.MockNetwork {
	client := rpc.NewMockClient(ctrl)
	client.EXPECT().Call(gomock.Any(), "net_peerCount").DoAndReturn(
		func(result any, method string, args ...any) error {
			*result.(*hexutil.Uint64) = hexutil.Uint64(peers)
			return nil
		}).AnyTimes()
	client.EXPECT().Close().AnyTimes()

	node := driver.NewMockNode(ctrl)
	node.EXPECT().GetLabel().Return(label).AnyTimes()
	node.EXPECT().DialRpc(gomock.Any()).Return(client, nil).AnyTimes()

	net := driver.NewMockNetwork(ctrl)
	net.EXPECT().GetActiveNodes().Return([]driver.Node{node}).AnyTimes()
	return net
}

func TestPeerCountChecker_Fails_WithoutNode(t *testing.T) {
	ctrl := gomock.NewController(t)
	checker := &peerCountChecker{net: driver.NewMockNetwork(ctrl)}
	err := checker.Check(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "requires a node")
}

func TestPeerCountChecker_Fails_WithoutBounds(t *testing.T) {
	ctrl := gomock.NewController(t)
	checker := &peerCountChecker{
		net:  driver.NewMockNetwork(ctrl),
		node: "validator-1",
	}
	err := checker.Check(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "at least one of min and max")
}

func TestPeerCountChecker_Fails_WhenNodeIsNotActive(t *testing.T) {
	ctrl := gomock.NewController(t)
	min := 1
	checker := &peerCountChecker{
		net:  peerCountNetwork(ctrl, "validator-1", 0),
		node: "validator-2",
		min:  &min,
	}
	err := checker.Check(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), `node "validator-2" not found`)
}

func TestPeerCountChecker_JudgesCountAgainstBounds(t *testing.T) {
	zero, one, three := 0, 1, 3
	tests := map[string]struct {
		peers    uint64
		min, max *int
		want     string // substring of the expected error, empty for success
	}{
		"min satisfied":    {peers: 2, min: &one},
		"max satisfied":    {peers: 0, max: &zero},
		"within interval":  {peers: 2, min: &one, max: &three},
		"below min":        {peers: 0, min: &one, want: "expected at least 1"},
		"above max":        {peers: 2, max: &zero, want: "expected at most 0"},
		"outside interval": {peers: 4, min: &one, max: &three, want: "expected at most 3"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			checker := &peerCountChecker{
				net:  peerCountNetwork(ctrl, "validator-1", test.peers),
				node: "validator-1",
				min:  test.min,
				max:  test.max,
			}
			err := checker.Check(context.Background())
			if test.want == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				require.Contains(t, err.Error(), test.want)
			}
		})
	}
}

func TestPeerCountChecker_Converges_WhenCountEntersBounds(t *testing.T) {
	ctrl := gomock.NewController(t)

	// The first read is short of the bound, the second satisfies it.
	client := rpc.NewMockClient(ctrl)
	peers := uint64(0)
	client.EXPECT().Call(gomock.Any(), "net_peerCount").DoAndReturn(
		func(result any, method string, args ...any) error {
			*result.(*hexutil.Uint64) = hexutil.Uint64(peers)
			peers++
			return nil
		}).Times(2)
	client.EXPECT().Close().Times(2)

	node := driver.NewMockNode(ctrl)
	node.EXPECT().GetLabel().Return("validator-1").AnyTimes()
	node.EXPECT().DialRpc(gomock.Any()).Return(client, nil).AnyTimes()

	net := driver.NewMockNetwork(ctrl)
	net.EXPECT().GetActiveNodes().Return([]driver.Node{node}).AnyTimes()

	oldInterval := peerCountPollInterval
	peerCountPollInterval = time.Millisecond
	t.Cleanup(func() { peerCountPollInterval = oldInterval })

	min := 1
	checker := &peerCountChecker{
		net:     net,
		node:    "validator-1",
		min:     &min,
		timeout: time.Second,
	}
	require.NoError(t, checker.Check(context.Background()))
}

func TestPeerCountChecker_Configure_SetsNodeAndBounds(t *testing.T) {
	ctrl := gomock.NewController(t)
	base := &peerCountChecker{
		net:     driver.NewMockNetwork(ctrl),
		timeout: defaultPeerCountTimeout,
	}

	configured, ok := base.Configure(CheckerConfig{
		"node": "validator-1",
		"min":  1,
		"max":  3,
	}).(*peerCountChecker)
	require.True(t, ok)
	require.Equal(t, "validator-1", configured.node)
	require.NotNil(t, configured.min)
	require.Equal(t, 1, *configured.min)
	require.NotNil(t, configured.max)
	require.Equal(t, 3, *configured.max)
	require.Equal(t, defaultPeerCountTimeout, configured.timeout)

	// The original checker is left untouched.
	require.Empty(t, base.node)
	require.Nil(t, base.min)
	require.Nil(t, base.max)
}
