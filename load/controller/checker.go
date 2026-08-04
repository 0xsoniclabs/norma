// Copyright 2026 Fantom Foundation
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

package controller

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/rpc"
	"github.com/0xsoniclabs/norma/load/app"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

const (
	blockPollInterval = 200 * time.Millisecond

	// settleWindow is how long a submitted transaction may stay invisible - not in
	// a block and not in the transaction pool - before the checker concludes the
	// network is not going to include it. Transactions still waiting in the pool
	// are given as long as they need.
	settleWindow = 20 * time.Second

	// verifiers verify outcomes in parallel, because verifying may take requests of
	// its own - reading the reason a call reverted, or waiting for the plan of a
	// bundle to be executed - which must not hold up following the chain.
	verifiers = 16

	// verificationQueueSize bounds the backlog of transactions to verify. Once it
	// is full, following the chain slows down rather than checks being skipped.
	verificationQueueSize = 1024

	maxReportedFailures = 20
)

// checker verifies that the transactions a load produced reached the outcomes
// their generators created them for.
//
// Rather than waiting for each transaction on its own, the checker follows the
// chain block by block and matches the receipts of every block against the
// transactions it is still waiting for. This keeps the cost of checking at one
// request per block instead of one per transaction, which is what makes checking
// affordable at load-generating rates.
type checker struct {
	ctxt   app.AppContext
	client rpc.Client
	// heldByAnyNode reports whether any node of the network still holds the
	// transaction in its pool. A pool is node-local, so a transaction waiting on
	// the node it was sent to is invisible to every other node.
	heldByAnyNode func(common.Hash) (bool, error)

	mutex   sync.Mutex
	pending map[common.Hash]pendingCall

	verifications chan verification
	verifiers     sync.WaitGroup

	confirmed atomic.Uint64
	failed    atomic.Uint64

	failuresMutex sync.Mutex
	failures      []error
}

type pendingCall struct {
	call      app.Call
	generator app.Generator
	name      string
	// waitingSince is when the transaction was last seen to be legitimately
	// waiting for inclusion.
	waitingSince time.Time // < reset while the pool still holds it
}

type verification struct {
	pending pendingCall
	receipt *types.Receipt // < nil if the transaction never reached a block
	// refusal is why no node accepted the transaction, when that is the reason it
	// never reached a block.
	refusal error
}

func newChecker(ctxt app.AppContext, network driver.Network) *checker {
	c := &checker{
		ctxt:          ctxt,
		client:        ctxt.GetClient(),
		pending:       map[common.Hash]pendingCall{},
		verifications: make(chan verification, verificationQueueSize),
	}
	c.heldByAnyNode = func(hash common.Hash) (bool, error) {
		return heldByAnyNode(network, hash)
	}
	return c
}

// heldByAnyNode asks every node of the network whether it still holds the given
// transaction in its pool. Transactions are spread across the nodes as they are
// sent, and a pool only knows the transactions offered to its own node, so all of
// them have to be asked before concluding that the network let a transaction go.
func heldByAnyNode(network driver.Network, hash common.Hash) (bool, error) {
	var errs []error
	for _, node := range network.GetActiveNodes() {
		client, err := node.DialRpc(context.Background())
		if err != nil {
			errs = append(errs, err)
			continue
		}
		status, err := client.GetTransactionPoolStatus(hash)
		client.Close()
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if status != rpc.TxPoolStatusNotPresent {
			return true, nil
		}
	}
	// Only report an error if no node gave an answer: as long as one did, the
	// transaction is known not to be waiting there.
	if len(errs) > 0 && len(errs) == len(network.GetActiveNodes()) {
		return false, errors.Join(errs...)
	}
	return false, nil
}

func (c *checker) submit(name string, generator app.Generator, call app.Call) {
	pending := pendingCall{
		call:         call,
		generator:    generator,
		name:         name,
		waitingSince: time.Now(),
	}

	// A transaction of indeterminate outcome has no receipt to wait for - a bundle
	// envelope, for instance, is unwrapped by the network and never appears in a
	// block itself. Its generator verifies the effect by other means, so it goes
	// straight to the verifiers.
	if call.Outcome == app.Indeterminate {
		c.verifications <- verification{pending: pending}
		return
	}

	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.pending[call.Tx.Hash()] = pending
}

// reportSent takes the result of offering a submitted transaction to a node. A
// refusal settles the transaction right away: it will never reach a block, which
// is what a rejected transaction is created for and a failure for any other.
func (c *checker) reportSent(hash common.Hash, err error) {
	if err == nil {
		return
	}
	c.settleRefused(hash, err)
}

// run follows the chain until the context is cancelled, verifying the outcome of
// every transaction it finds, and then gives the transactions still in flight a
// last chance to settle. It returns once every submitted transaction has been
// accounted for.
func (c *checker) run(ctx context.Context) {
	for i := 0; i < verifiers; i++ {
		c.verifiers.Add(1)
		go func() {
			defer c.verifiers.Done()
			for verification := range c.verifications {
				c.verify(verification)
			}
		}()
	}
	defer func() {
		close(c.verifications)
		c.verifiers.Wait()
	}()

	next, err := c.client.BlockNumber(ctx)
	if err != nil {
		slog.Error("transaction checks disabled: failed to read the current block", "error", err)
		return
	}

	ticker := time.NewTicker(blockPollInterval)
	defer ticker.Stop()
	for {
		next = c.scanBlocks(ctx, next)
		c.retireOverdue(ctx)

		select {
		case <-ticker.C:
		case <-ctx.Done():
			c.drain(next)
			return
		}
	}
}

// scanBlocks returns the number of the first block it has not seen yet.
func (c *checker) scanBlocks(ctx context.Context, next uint64) uint64 {
	head, err := c.client.BlockNumber(ctx)
	if err != nil {
		slog.Warn("failed to read the current block number", "error", err)
		return next
	}
	for ; next <= head; next++ {
		receipts, err := c.client.GetBlockReceipts(next)
		if err != nil {
			slog.Warn("failed to read the receipts of a block", "block", next, "error", err)
			return next
		}
		for _, receipt := range receipts {
			c.settle(receipt.TxHash, receipt)
		}
	}
	return next
}

// retireOverdue resolves the transactions that have not turned up in a block for
// longer than the settle window. A transaction the pool still holds is given more
// time however long it takes: under load the pool is exactly where transactions
// are meant to be waiting. One the pool does not hold either has been dropped by
// the network, which is the outcome a rejected transaction is checked against.
func (c *checker) retireOverdue(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}

	now := time.Now()
	c.mutex.Lock()
	overdue := make([]common.Hash, 0, len(c.pending))
	for hash, pending := range c.pending {
		if now.Sub(pending.waitingSince) >= settleWindow {
			overdue = append(overdue, hash)
		}
	}
	c.mutex.Unlock()

	for _, hash := range overdue {
		held, err := c.heldByAnyNode(hash)
		if err != nil {
			slog.Warn("failed to read the transaction pool status", "tx", hash, "error", err)
			continue
		}
		if held {
			c.keepWaiting(hash)
			continue
		}

		// Not in the pool. Ask for the receipt directly rather than relying on the
		// block scan, which may not have caught up with the head of the chain.
		receipt, err := c.client.GetTransactionReceipt(hash)
		if err != nil {
			slog.Warn("failed to read a transaction receipt", "tx", hash, "error", err)
			continue
		}
		c.settle(hash, receipt)
	}
}

func (c *checker) keepWaiting(hash common.Hash) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if pending, ok := c.pending[hash]; ok {
		pending.waitingSince = time.Now()
		c.pending[hash] = pending
	}
}

// drain resolves everything still pending once the load has stopped, giving the
// last transactions the settle window to make it into a block.
func (c *checker) drain(next uint64) {
	ctx, cancel := context.WithTimeout(context.Background(), settleWindow)
	defer cancel()

	ticker := time.NewTicker(blockPollInterval)
	defer ticker.Stop()
	for {
		next = c.scanBlocks(ctx, next)
		if c.numPending() == 0 {
			return
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			// Whatever has not been included by now never will be.
			for _, hash := range c.pendingHashes() {
				c.settle(hash, nil)
			}
			return
		}
	}
}

func (c *checker) settle(hash common.Hash, receipt *types.Receipt) {
	c.enqueue(hash, receipt, nil)
}

// settleRefused records why no node accepted the transaction, so that the report
// can name the reason instead of only the missing receipt.
func (c *checker) settleRefused(hash common.Hash, refusal error) {
	c.enqueue(hash, nil, refusal)
}

func (c *checker) enqueue(hash common.Hash, receipt *types.Receipt, refusal error) {
	c.mutex.Lock()
	pending, ok := c.pending[hash]
	if ok {
		delete(c.pending, hash)
	}
	c.mutex.Unlock()
	if !ok {
		// Not ours: the blocks also carry the transactions of other loads.
		return
	}
	c.verifications <- verification{pending: pending, receipt: receipt, refusal: refusal}
}

func (c *checker) verify(v verification) {
	err := v.pending.generator.Check(c.ctxt, v.pending.call, v.receipt)
	if err == nil {
		c.confirmed.Add(1)
		return
	}
	if v.refusal != nil {
		err = fmt.Errorf("%w; no node accepted the transaction: %v", err, v.refusal)
	}
	c.recordFailure(fmt.Errorf("%s: %w", v.pending.name, err))
}

func (c *checker) recordFailure(err error) {
	c.failed.Add(1)
	c.failuresMutex.Lock()
	if len(c.failures) < maxReportedFailures {
		c.failures = append(c.failures, err)
	}
	c.failuresMutex.Unlock()
	slog.Error("transaction check failed", "error", err)
}

func (c *checker) numPending() int {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return len(c.pending)
}

func (c *checker) pendingHashes() []common.Hash {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	hashes := make([]common.Hash, 0, len(c.pending))
	for hash := range c.pending {
		hashes = append(hashes, hash)
	}
	return hashes
}

func (c *checker) getConfirmedTransactions() uint64 {
	return c.confirmed.Load()
}

// result names how many checks failed in total and describes the first of them.
func (c *checker) result() error {
	failed := c.failed.Load()
	if failed == 0 {
		return nil
	}
	c.failuresMutex.Lock()
	reported := make([]error, len(c.failures))
	copy(reported, c.failures)
	c.failuresMutex.Unlock()

	summary := fmt.Errorf("%d transaction(s) did not reach the expected outcome", failed)
	if uint64(len(reported)) < failed {
		summary = fmt.Errorf("%w, the first %d of them", summary, len(reported))
	}
	return errors.Join(append([]error{summary}, reported...)...)
}
