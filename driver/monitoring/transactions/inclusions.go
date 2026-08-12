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

package txmon

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/0xsoniclabs/norma/driver/monitoring"
	"github.com/0xsoniclabs/norma/driver/rpc"
	"github.com/0xsoniclabs/sonic/opera"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

// pendingBlocks bounds the queue of blocks waiting to have their transactions
// read. Blocks are announced about once per second while a read takes
// milliseconds, so the queue only fills if the network is unreachable, in which
// case the oldest announcements are the least worth waiting for.
const pendingBlocks = 1024

// A block is read from a node that may not have processed it yet, so a failed
// read is retried a few times before the block is given up on.
const (
	blockReadAttempts   = 3
	blockReadRetryDelay = 250 * time.Millisecond
)

// inclusionObserver marks the transactions of every new block as included.
//
// Blocks are announced by the log of the nodes, which is where Norma observes
// block production anyway, and the announcement carries no transactions - so
// their hashes are read from the block afterwards. The moment the announcing
// node logged the block is used as the moment of inclusion: it is the earliest
// point at which the transactions of that block are committed, and it is
// measured on the same clock as the submissions.
//
// The gas limit of the block is read along with them, since it is the ceiling
// those transactions competed for and the block is being read anyway.
type inclusionObserver struct {
	tracker *Tracker
	clients clientSource
	blocks  chan monitoring.Block

	mu     sync.Mutex
	seen   map[int]bool // heights already read, they are announced by every node
	missed int
}

func newInclusionObserver(tracker *Tracker, clients clientSource) *inclusionObserver {
	return &inclusionObserver{
		tracker: tracker,
		clients: clients,
		blocks:  make(chan monitoring.Block, pendingBlocks),
		seen:    map[int]bool{},
	}
}

// OnBlock implements monitoring.LogListener.
func (o *inclusionObserver) OnBlock(_ monitoring.Node, block monitoring.Block) {
	o.mu.Lock()
	if o.seen[block.Height] {
		o.mu.Unlock()
		return
	}
	o.seen[block.Height] = true
	o.mu.Unlock()

	select {
	case o.blocks <- block:
	default:
		o.mu.Lock()
		o.missed++
		o.mu.Unlock()
	}
}

// run reads the transactions of announced blocks until the context is
// cancelled.
func (o *inclusionObserver) run(ctx context.Context) {
	rules := newRulesCache()
	var client rpc.Client
	defer func() {
		if client != nil {
			client.Close()
		}
	}()

	for {
		select {
		case <-ctx.Done():
			o.logSummary()
			return
		case block := <-o.blocks:
			if client == nil {
				var err error
				if client, err = o.clients.dial(ctx); err != nil {
					// The network is gone or not reachable; the remaining
					// blocks are dropped rather than retried forever.
					o.count(1)
					continue
				}
			}
			contents, err := readBlock(ctx, client, block.Height, rules)
			if err != nil {
				o.mu.Lock()
				first := o.missed == 0
				o.mu.Unlock()
				if first {
					slog.Warn("failed to read the contents of a block, inclusions will be missing",
						"block", block.Height, "error", err)
				} else {
					slog.Debug("failed to read contents of block",
						"block", block.Height, "error", err)
				}
				client.Close()
				client = nil
				o.count(1)
				continue
			}
			o.tracker.MarkBlock(block.Height, block.Time, contents.transactions)
			o.tracker.MarkBlockLimits(block.Height, contents.limits)
		}
	}
}

func (o *inclusionObserver) count(missed int) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.missed += missed
}

func (o *inclusionObserver) logSummary() {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.missed > 0 {
		slog.Warn("transactions of some blocks could not be read, their inclusions are missing",
			"blocks", o.missed)
	}
}

// blockContents is what is read from a block: the transactions it carried, with
// the gas each of them used, and the gas ceilings that applied to it.
type blockContents struct {
	transactions []IncludedTransaction
	limits       BlockLimit
}

// readBlock returns the contents of the given block, giving the queried node a
// moment to catch up first if it does not know the block yet. Blocks are
// announced by the node that logged them while they are read from whichever node
// this observer is connected to, which may still be one block behind.
func readBlock(
	ctx context.Context, client rpc.Client, height int, rules *rulesCache,
) (blockContents, error) {
	var err error
	for attempt := range blockReadAttempts {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return blockContents{}, ctx.Err()
			case <-time.After(blockReadRetryDelay):
			}
		}
		var contents blockContents
		if contents, err = blockContent(client, height, rules); err == nil {
			return contents, nil
		}
	}
	return blockContents{}, err
}

// blockContent reads a block once. A block whose receipts are known also has a
// header, so both reads failing and succeeding together is the expected case;
// they are attempted as one so that a node lagging behind is retried for both.
func blockContent(client rpc.Client, height int, rules *rulesCache) (blockContents, error) {
	transactions, err := blockTransactions(client, height)
	if err != nil {
		return blockContents{}, err
	}
	limits, err := blockLimits(client, height, rules)
	if err != nil {
		return blockContents{}, err
	}
	return blockContents{transactions: transactions, limits: limits}, nil
}

// blockTransactions returns the transactions of the given block with the gas
// they used.
//
// The receipts of the whole block are read in one request: they are the only
// place the gas of a single transaction can be read from, and asking for them
// one by one would be one request per transaction.
func blockTransactions(client rpc.Client, height int) ([]IncludedTransaction, error) {
	var receipts *[]struct {
		TransactionHash common.Hash    `json:"transactionHash"`
		GasUsed         hexutil.Uint64 `json:"gasUsed"`
	}
	if err := client.Call(
		&receipts, "eth_getBlockReceipts", hexutil.EncodeUint64(uint64(height)),
	); err != nil {
		return nil, fmt.Errorf("failed to get receipts of block %d: %w", height, err)
	}
	if receipts == nil {
		return nil, fmt.Errorf("block %d not found", height)
	}

	txs := make([]IncludedTransaction, 0, len(*receipts))
	for _, receipt := range *receipts {
		txs = append(txs, IncludedTransaction{
			Hash:    receipt.TransactionHash,
			GasUsed: uint64(receipt.GasUsed),
		})
	}
	return txs, nil
}

// blockLimits returns the gas ceilings that applied to the given block: the limit
// of the block itself, which its header carries, and the limits of the events
// carrying its transactions and of the gas the validators may spend, which are
// rules of the network and therefore read for the epoch the block belongs to.
func blockLimits(client rpc.Client, height int, cache *rulesCache) (BlockLimit, error) {
	var header *struct {
		GasLimit hexutil.Uint64 `json:"gasLimit"`
		Epoch    hexutil.Uint64 `json:"epoch"`
	}
	if err := client.Call(
		&header, "eth_getBlockByNumber", hexutil.EncodeUint64(uint64(height)), false,
	); err != nil {
		return BlockLimit{}, fmt.Errorf("failed to get header of block %d: %w", height, err)
	}
	if header == nil {
		return BlockLimit{}, fmt.Errorf("block %d not found", height)
	}

	rules, err := cache.rulesOf(client, uint64(header.Epoch))
	if err != nil {
		return BlockLimit{}, err
	}

	return BlockLimit{
		Block:          height,
		GasLimit:       uint64(header.GasLimit),
		EventGasLimit:  rules.Economy.Gas.MaxEventGas,
		GasPowerPerSec: rules.Economy.ShortGasPower.AllocPerSec,
	}, nil
}

// rulesCache holds the rules of the epochs seen so far. The rules of a network
// change with an epoch, never within one, so they are read once per epoch rather
// than once per block.
type rulesCache struct {
	epochs map[uint64]opera.Rules
}

func newRulesCache() *rulesCache {
	return &rulesCache{epochs: map[uint64]opera.Rules{}}
}

// rulesOf returns the rules of the given epoch.
//
// eth_getRules is keyed on the epoch, and answers a null for one it does not
// know - which decodes into rules of all zeros without reporting an error. Such
// an answer is rejected here rather than recorded as a network whose gas limits
// are zero.
func (c *rulesCache) rulesOf(client rpc.Client, epoch uint64) (opera.Rules, error) {
	if rules, found := c.epochs[epoch]; found {
		return rules, nil
	}
	rules, err := client.GetNetworkRules(hexutil.EncodeUint64(epoch))
	if err != nil {
		return opera.Rules{}, fmt.Errorf("failed to get the rules of epoch %d: %w", epoch, err)
	}
	if rules.Economy.Gas.MaxEventGas == 0 {
		return opera.Rules{}, fmt.Errorf(
			"the rules of epoch %d are not available: got a gas limit of zero per event", epoch)
	}
	c.epochs[epoch] = rules
	return rules, nil
}
