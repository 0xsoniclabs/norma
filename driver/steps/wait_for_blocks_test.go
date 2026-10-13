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

package steps

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/rpc"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestWaitForBlocks_ReturnsOnceCountIsProduced(t *testing.T) {
	ctrl := gomock.NewController(t)
	// One read for the baseline, then one per block: the height grows by one
	// per read, so a fourth read means it waited for exactly three blocks.
	net := netWithBlockNumbers(ctrl, growingBlockHeight(), 4)

	step := &WaitForBlocks{Blocks: 3}
	require.NoError(t, step.Run(t.Context(), NewEnv(net, nil, nil)))
}

func TestWaitForBlocks_WaitsPastTheTimeout_WhenBlocksKeepArriving(t *testing.T) {
	ctrl := gomock.NewController(t)
	const count = 8
	net := netWithBlockNumbers(ctrl, growingBlockHeight(), count+1)
	setBlockProductionTimeout(t, 500*time.Millisecond)

	step := &WaitForBlocks{Blocks: count}
	start := time.Now()
	require.NoError(t, step.Run(t.Context(), NewEnv(net, nil, nil)))
	require.Greater(t, time.Since(start), blockProductionTimeout)
}

func TestWaitForBlocks_FailsWithAStall_WhenTheChainStopsGrowing(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := netWithBlockNumbers(ctrl, constantBlockHeight(7), -1)
	// Negative, so the chain counts as stalled from the first poll on.
	setBlockProductionTimeout(t, -1)

	step := &WaitForBlocks{Blocks: 3}
	err := step.Run(t.Context(), NewEnv(net, nil, nil))
	require.ErrorContains(t, err, "no block produced")
}

// A count this large would wrap a baseline+count target below the baseline,
// ending the wait before it began.
func TestWaitForBlocks_KeepsWaiting_WhenTheCountIsTheLargestUint64(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := netWithBlockNumbers(ctrl, constantBlockHeight(7), -1)
	setBlockProductionTimeout(t, -1)

	step := &WaitForBlocks{Blocks: math.MaxUint64}
	err := step.Run(t.Context(), NewEnv(net, nil, nil))
	require.ErrorContains(t, err, "no block produced")
}

// netWithBlockNumbers returns a network whose single RPC client answers
// BlockNumber from heights, expecting reads calls, or any number if negative.
func netWithBlockNumbers(
	ctrl *gomock.Controller,
	heights func(context.Context) (uint64, error),
	reads int,
) driver.Network {
	client := rpc.NewMockClient(ctrl)
	call := client.EXPECT().BlockNumber(gomock.Any()).DoAndReturn(heights)
	if reads < 0 {
		call.AnyTimes()
	} else {
		call.Times(reads)
	}
	client.EXPECT().Close()
	net := driver.NewMockNetwork(ctrl)
	net.EXPECT().DialRandomRpc().Return(client, nil)
	return net
}

func constantBlockHeight(height uint64) func(context.Context) (uint64, error) {
	return func(context.Context) (uint64, error) {
		return height, nil
	}
}

func setBlockProductionTimeout(t *testing.T, timeout time.Duration) {
	original := blockProductionTimeout
	blockProductionTimeout = timeout
	t.Cleanup(func() { blockProductionTimeout = original })
}
