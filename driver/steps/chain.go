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
	"fmt"
	"slices"
	"time"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/network"
	"github.com/0xsoniclabs/norma/driver/rpc"
)

// blockProductionTimeout bounds how long the chain may stand still while
// waiting for blocks, as the network may have halted.
var blockProductionTimeout = 2 * time.Minute

// WaitForBlockProduction waits until the network produces a new block,
// confirming it is actively processing transactions.
func WaitForBlockProduction(ctx context.Context, net driver.Network) error {
	err := WaitForBlocks(ctx, net, 1)
	if errors.Is(err, driver.ErrEmptyNetwork) {
		// Without a node, there is no block production to wait for.
		return nil
	}
	return err
}

// WaitForBlocks waits until the network has grown by count blocks, failing
// once it has not grown for blockProductionTimeout. The network's height is
// the highest of its nodes, so a node catching up does not pass for growth.
func WaitForBlocks(ctx context.Context, net driver.Network, count uint64) error {
	clients, err := dialNodes(ctx, net)
	if err != nil {
		return err
	}
	defer network.CloseAll(clients)
	return waitForBlocks(ctx, func(ctx context.Context) (uint64, error) {
		return network.MaxOf(ctx, clients, readBlockNumber)
	}, count)
}

// waitForBlocks waits until the height read by blockNumber has grown by count
// blocks, failing once it has not grown for blockProductionTimeout.
func waitForBlocks(ctx context.Context, height func(context.Context) (uint64, error), count uint64) error {
	grownAt := time.Now()
	blockNumber := func() (uint64, error) {
		ctx, cancel := context.WithDeadline(ctx, grownAt.Add(blockProductionTimeout))
		defer cancel()
		return height(ctx)
	}

	baseline, err := blockNumber()
	if err != nil {
		return fmt.Errorf("failed to get block number: %w", err)
	}
	for height := baseline; height-baseline < count; {
		if time.Since(grownAt) > blockProductionTimeout {
			return fmt.Errorf("no block produced for %v, at block %d of %d",
				blockProductionTimeout, height-baseline, count)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("context cancelled while waiting for block production: %w", ctx.Err())
		case <-time.After(100 * time.Millisecond):
		}
		block, err := blockNumber()
		if err != nil {
			// A read the stall deadline cut short is reported as the stall.
			if time.Since(grownAt) > blockProductionTimeout && ctx.Err() == nil {
				continue
			}
			return fmt.Errorf("failed to get block number: %w", err)
		}
		if block > height {
			height, grownAt = block, time.Now()
		}
	}
	return nil
}

func readBlockNumber(ctx context.Context, client rpc.Client) (uint64, error) {
	return client.BlockNumber(ctx)
}

// dialNodes connects to the active nodes of the network, failing with
// driver.ErrEmptyNetwork if there are none.
func dialNodes(ctx context.Context, net driver.Network) ([]rpc.Client, error) {
	nodes := net.GetActiveNodes()
	if len(nodes) == 0 {
		return nil, driver.ErrEmptyNetwork
	}
	return network.DialNodes(ctx, nodes)
}

// syncTolerance is how many blocks a node may trail the network by and still
// count as caught up, allowing for blocks that are still reaching it.
const syncTolerance = 2

// WaitForNodeSync waits until the given node is within syncTolerance blocks of
// the most advanced other node of the network, failing if it has not within
// budget. The network's height is read anew on every poll, so on a halted
// network the node is caught up once it has the blocks the others have.
func WaitForNodeSync(ctx context.Context, net driver.Network, node driver.Node, budget time.Duration) error {
	others := slices.DeleteFunc(net.GetActiveNodes(), func(n driver.Node) bool { return n == node })
	if len(others) == 0 {
		return nil
	}
	return withBudget(ctx, budget, func(ctx context.Context) error {
		peers, err := network.DialNodes(ctx, others)
		if err != nil {
			return fmt.Errorf("failed to dial the network: %w", err)
		}
		defer network.CloseAll(peers)
		client, err := node.DialRpc(ctx)
		if err != nil {
			return err
		}
		defer client.Close()

		var block, head uint64
		for {
			// Reads that fail are retried, as the node may still be starting.
			// The network is read first, so the node is never measured against
			// a block produced after its own height was read.
			networkHeight, networkErr := network.MaxOf(ctx, peers, readBlockNumber)
			if networkErr == nil {
				head = networkHeight
			}
			if block, err = client.BlockNumber(ctx); err == nil && networkErr == nil && block+syncTolerance >= head {
				return nil
			}
			select {
			case <-ctx.Done():
				return fmt.Errorf("node at block %d, network at block %d: %w", block, head, ctx.Err())
			case <-time.After(200 * time.Millisecond):
			}
		}
	})
}

// AdvanceEpoch seals the current epoch of the network, failing if the next one
// has not begun within budget.
func AdvanceEpoch(ctx context.Context, net driver.Network, budget time.Duration) error {
	return withBudget(ctx, budget, func(ctx context.Context) error {
		return net.AdvanceEpoch(ctx, 1)
	})
}

// WaitForEpochChange waits until the network begins a new epoch, failing if it
// has not within budget.
func WaitForEpochChange(ctx context.Context, net driver.Network, budget time.Duration) error {
	return withBudget(ctx, budget, net.WaitForEpochChange)
}

// withBudget runs wait with a context bounded by budget, reporting the budget
// if it ran out before the surrounding context did. A wait that completes only
// after its budget fails too.
func withBudget(ctx context.Context, budget time.Duration, wait func(context.Context) error) error {
	waitCtx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	err := wait(waitCtx)
	if err == nil {
		// Done only after the budget, or the surrounding context, ran out.
		err = waitCtx.Err()
	}
	if err != nil && ctx.Err() == nil && waitCtx.Err() != nil {
		return fmt.Errorf("not done within %v: %w", budget, err)
	}
	return err
}
