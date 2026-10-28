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
	"errors"
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
	client := clientWithBlockNumbers(ctrl, growingBlockHeight(), 4)

	require.NoError(t, WaitForBlocks(t.Context(), client, 3))
}

func TestWaitForBlocks_WaitsPastTheTimeout_WhenBlocksKeepArriving(t *testing.T) {
	ctrl := gomock.NewController(t)
	const count = 8
	client := clientWithBlockNumbers(ctrl, growingBlockHeight(), count+1)
	setBlockProductionTimeout(t, 500*time.Millisecond)

	start := time.Now()
	require.NoError(t, WaitForBlocks(t.Context(), client, count))
	require.Greater(t, time.Since(start), blockProductionTimeout)
}

func TestWaitForBlocks_FailsWithAStall_WhenTheChainStopsGrowing(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := clientWithBlockNumbers(ctrl, constantBlockHeight(7), -1)
	// Negative, so the chain counts as stalled from the first poll on.
	setBlockProductionTimeout(t, -1)

	err := WaitForBlocks(t.Context(), client, 3)
	require.ErrorContains(t, err, "no block produced")
}

// A count this large would wrap a baseline+count target below the baseline,
// ending the wait before it began.
func TestWaitForBlocks_KeepsWaiting_WhenTheCountIsTheLargestUint64(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := clientWithBlockNumbers(ctrl, constantBlockHeight(7), -1)
	setBlockProductionTimeout(t, -1)

	err := WaitForBlocks(t.Context(), client, math.MaxUint64)
	require.ErrorContains(t, err, "no block produced")
}

// A network no node of which answers is not producing blocks, so the wait
// between steps must not pass without having seen one.
func TestWaitForBlockProduction_Fails_WhenNoNodeCanBeDialled(t *testing.T) {
	net := driver.NewMockNetwork(gomock.NewController(t))
	net.EXPECT().DialRandomRpc().Return(nil, errors.New("no reachable node"))

	require.ErrorContains(t, WaitForBlockProduction(t.Context(), net), "no reachable node")
}

func TestWaitForBlockProduction_Passes_OnAnEmptyNetwork(t *testing.T) {
	net := driver.NewMockNetwork(gomock.NewController(t))
	net.EXPECT().DialRandomRpc().Return(nil, driver.ErrEmptyNetwork)

	require.NoError(t, WaitForBlockProduction(t.Context(), net))
}

// clientWithBlockNumbers returns an RPC client answering BlockNumber from
// heights, expecting reads calls, or any number if negative.
func clientWithBlockNumbers(
	ctrl *gomock.Controller,
	heights func(context.Context) (uint64, error),
	reads int,
) rpc.Client {
	client := rpc.NewMockClient(ctrl)
	call := client.EXPECT().BlockNumber(gomock.Any()).DoAndReturn(heights)
	if reads < 0 {
		call.AnyTimes()
	} else {
		call.Times(reads)
	}
	return client
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
