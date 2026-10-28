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
	"time"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/rpc"
)

// blockProductionTimeout bounds how long the chain may stand still while
// waiting for blocks, as the random node polled may have stopped following it.
var blockProductionTimeout = 2 * time.Minute

// WaitForBlockProduction waits until the network produces a new block,
// confirming it is actively processing transactions.
func WaitForBlockProduction(ctx context.Context, net driver.Network) error {
	client, err := net.DialRandomRpc()
	if errors.Is(err, driver.ErrEmptyNetwork) {
		// Without a node, there is no block production to wait for.
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to connect to the network: %w", err)
	}
	defer client.Close()
	return WaitForBlocks(ctx, client, 1)
}

// WaitForBlocks waits until the chain seen by client has grown by count
// blocks, failing once it has not grown for blockProductionTimeout.
func WaitForBlocks(ctx context.Context, client rpc.Client, count uint64) error {
	grownAt := time.Now()
	blockNumber := func() (uint64, error) {
		ctx, cancel := context.WithDeadline(ctx, grownAt.Add(blockProductionTimeout))
		defer cancel()
		return client.BlockNumber(ctx)
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
			return fmt.Errorf("failed to get block number: %w", err)
		}
		if block > height {
			height, grownAt = block, time.Now()
		}
	}
	return nil
}

// NetworkBlockHeight returns the current block height from an existing network node.
func NetworkBlockHeight(ctx context.Context, net driver.Network) (uint64, error) {
	client, err := net.DialRandomRpc()
	if err != nil {
		return 0, err
	}
	defer client.Close()
	return client.BlockNumber(ctx)
}

// WaitForNodeSync waits until the given node has synced to at least the target block height.
func WaitForNodeSync(ctx context.Context, node driver.Node, targetBlock uint64) error {
	client, err := node.DialRpc(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	const syncTimeout = 60 * time.Second
	deadline := time.Now().Add(syncTimeout)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		block, err := client.BlockNumber(ctx)
		if err != nil {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		if block >= targetBlock {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("node did not sync to block %d within %s", targetBlock, syncTimeout)
}
