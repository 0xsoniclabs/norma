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

	require.NoError(t, waitForBlocks(t.Context(), client.BlockNumber, 3))
}

func TestWaitForBlocks_WaitsPastTheTimeout_WhenBlocksKeepArriving(t *testing.T) {
	ctrl := gomock.NewController(t)
	const count = 8
	client := clientWithBlockNumbers(ctrl, growingBlockHeight(), count+1)
	setBlockProductionTimeout(t, 500*time.Millisecond)

	start := time.Now()
	require.NoError(t, waitForBlocks(t.Context(), client.BlockNumber, count))
	require.Greater(t, time.Since(start), blockProductionTimeout)
}

func TestWaitForBlocks_FailsWithAStall_WhenTheChainStopsGrowing(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := clientWithBlockNumbers(ctrl, constantBlockHeight(7), -1)
	// Negative, so the chain counts as stalled from the first poll on.
	setBlockProductionTimeout(t, -1)

	err := waitForBlocks(t.Context(), client.BlockNumber, 3)
	require.ErrorContains(t, err, "no block produced")
}

// A count this large would wrap a baseline+count target below the baseline,
// ending the wait before it began.
func TestWaitForBlocks_KeepsWaiting_WhenTheCountIsTheLargestUint64(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := clientWithBlockNumbers(ctrl, constantBlockHeight(7), -1)
	setBlockProductionTimeout(t, -1)

	err := waitForBlocks(t.Context(), client.BlockNumber, math.MaxUint64)
	require.ErrorContains(t, err, "no block produced")
}

// A network no node of which answers is not producing blocks, so the wait
// between steps must not pass without having seen one.
func TestWaitForBlockProduction_Fails_WhenNoNodeCanBeDialled(t *testing.T) {
	ctrl := gomock.NewController(t)
	node := driver.NewMockNode(ctrl)
	node.EXPECT().IsExpectedFailure().Return(false).AnyTimes()
	node.EXPECT().DialRpc(gomock.Any()).Return(nil, errors.New("no reachable node"))
	net := driver.NewMockNetwork(ctrl)
	net.EXPECT().GetActiveNodes().Return([]driver.Node{node})

	require.ErrorContains(t, WaitForBlockProduction(t.Context(), net), "no reachable node")
}

func TestWaitForBlockProduction_Passes_OnAnEmptyNetwork(t *testing.T) {
	net := driver.NewMockNetwork(gomock.NewController(t))
	net.EXPECT().GetActiveNodes().Return(nil)

	require.NoError(t, WaitForBlockProduction(t.Context(), net))
}

// A node catching up shows its height growing while the network is halted;
// the network's height is that of its most advanced node.
func TestNetworkBlockHeight_IsTheHighestOfTheNodes(t *testing.T) {
	ctrl := gomock.NewController(t)
	var nodes []driver.Node
	for _, height := range []uint64{7, 12, 9} {
		client := rpc.NewMockClient(ctrl)
		client.EXPECT().BlockNumber(gomock.Any()).Return(height, nil)
		client.EXPECT().Close()
		nodes = append(nodes, nodeServing(ctrl, client))
	}
	down := driver.NewMockNode(ctrl)
	down.EXPECT().IsExpectedFailure().Return(false).AnyTimes()
	down.EXPECT().DialRpc(gomock.Any()).Return(nil, errors.New("down"))
	net := driver.NewMockNetwork(ctrl)
	net.EXPECT().GetActiveNodes().Return(append(nodes, down))

	height, err := NetworkBlockHeight(t.Context(), net)
	require.NoError(t, err)
	require.Equal(t, uint64(12), height)
}

// A node that does not reach its target within the budget fails the wait,
// rather than holding up the scenario until its own deadline.
func TestWaitForNodeSync_Fails_WhenTheBudgetRunsOut(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := rpc.NewMockClient(ctrl)
	client.EXPECT().BlockNumber(gomock.Any()).Return(uint64(5), nil).AnyTimes()
	client.EXPECT().Close()

	err := WaitForNodeSync(t.Context(), nodeServing(ctrl, client), 10, 50*time.Millisecond)
	require.ErrorContains(t, err, "not done within 50ms")
	require.ErrorContains(t, err, "at block 5")
}

func TestWaitForNodeSync_ReturnsOnceTheTargetIsReached(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := rpc.NewMockClient(ctrl)
	client.EXPECT().BlockNumber(gomock.Any()).DoAndReturn(growingBlockHeight()).Times(3)
	client.EXPECT().Close()

	require.NoError(t, WaitForNodeSync(t.Context(), nodeServing(ctrl, client), 3, time.Minute))
}

// A wait that only completes after its budget is too late, even if it reports
// success: the budget is what the step asserts.
func TestWithBudget_Fails_WhenTheWaitCompletesAfterTheBudget(t *testing.T) {
	err := withBudget(t.Context(), time.Millisecond, func(ctx context.Context) error {
		<-ctx.Done()
		return nil
	})
	require.ErrorContains(t, err, "not done within 1ms")
}

// A node catching up keeps growing while the network is halted; the wait
// follows the most advanced node, so it neither passes nor resets its stall
// timeout on the growth of one that is behind.
func TestWaitForBlocks_FailsWithAStall_WhenOnlyANodeBehindIsGrowing(t *testing.T) {
	ctrl := gomock.NewController(t)
	halted := rpc.NewMockClient(ctrl)
	halted.EXPECT().BlockNumber(gomock.Any()).Return(uint64(100), nil).AnyTimes()
	halted.EXPECT().Close()
	catchingUp := rpc.NewMockClient(ctrl)
	catchingUp.EXPECT().BlockNumber(gomock.Any()).DoAndReturn(growingBlockHeight()).AnyTimes()
	catchingUp.EXPECT().Close()
	net := driver.NewMockNetwork(ctrl)
	net.EXPECT().GetActiveNodes().Return([]driver.Node{nodeServing(ctrl, halted), nodeServing(ctrl, catchingUp)})
	setBlockProductionTimeout(t, 300*time.Millisecond)

	err := WaitForBlocks(t.Context(), net, 1)
	require.ErrorContains(t, err, "no block produced")
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

// nodeServing returns a node, not expected to fail, whose RPC is client.
func nodeServing(ctrl *gomock.Controller, client rpc.Client) driver.Node {
	node := driver.NewMockNode(ctrl)
	node.EXPECT().IsExpectedFailure().Return(false).AnyTimes()
	node.EXPECT().DialRpc(gomock.Any()).Return(client, nil).AnyTimes()
	return node
}
