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
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"slices"
	"time"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/monitoring"
	"github.com/0xsoniclabs/norma/driver/rpc"
	priority_registry "github.com/0xsoniclabs/sonic/gossip/blockproc/priorities/registry"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

// The check observes the network forward in time: it notes the head of the chain
// when it starts and judges only blocks produced after that, so it reports on the
// load the scenario has running rather than on anything an earlier step left
// behind. It stops once it has seen enough blocks carrying both prioritized and
// ordinary transactions, or when one of the two bounds below is reached.
const (
	// maxBlocksObserved bounds the blocks read, so that a fast chain cannot turn
	// one check into thousands of requests.
	maxBlocksObserved = 500

	// defaultPriorityTimeout is how long the check waits for the blocks it needs. It has
	// to outlast a lane that meets ordinary traffic only occasionally, and it has
	// to end a run in which the two never meet.
	defaultPriorityTimeout = 2 * time.Minute

	// priorityPollInterval is how long the check waits before asking again for
	// the head of the chain.
	priorityPollInterval = 500 * time.Millisecond
)

// defaultMinMixedBlocks is how many blocks must carry transactions of both
// classes for the observation to mean anything. Without a floor the check would
// pass on a network where nothing was prioritized at all.
const defaultMinMixedBlocks = 10

// defaultMinRunCoverage is the share of the prioritized transactions a block
// could hoist that its opening run must cover, as a median over the observed
// blocks. Hoisting puts all of them in that run, so the value is near one; an
// order that ignores priorities leaves runs of the length chance produces, which
// is a small fraction of a block carrying hundreds of transactions. Scenarios
// that congest the lane can lower it, see the minRunCoverage parameter.
const defaultMinRunCoverage = 0.5

func init() {
	RegisterNetworkCheck("prioritizedTransactionsFirst",
		func(net driver.Network, _ *monitoring.Monitor) Checker {
			return newPriorityOrderingChecker(net)
		})
}

// priorityOrderingChecker verifies that block formation hoists the transactions
// of registered senders: a block carrying both classes opens with a run of
// prioritized transactions, and that run stays within the gas the registry
// grants one entity per block.
//
// The ordering is a pure function of the block's transactions, the registry and
// the account nonces, so unlike a comparison of latencies it does not depend on
// rates, timing or the machine the network runs on.
//
// What a block does not show is which of its transactions were hoisted: a
// prioritized transaction the quota or a nonce gap demoted is appended among the
// ordinary ones and is indistinguishable from one. The check therefore measures
// the opening run against the transactions the quota could admit rather than
// against all of them, judges the median block, and asserts the quota only where
// no demotion is visible.
type priorityOrderingChecker struct {
	source         prioritySource
	minMixedBlocks int
	minRunCoverage float64
	timeout        time.Duration
}

func newPriorityOrderingChecker(net driver.Network) *priorityOrderingChecker {
	return &priorityOrderingChecker{
		source:         &networkPrioritySource{net: net},
		minMixedBlocks: defaultMinMixedBlocks,
		minRunCoverage: defaultMinRunCoverage,
		timeout:        defaultPriorityTimeout,
	}
}

// prioritySource provides what the check reads: the chain's blocks in order, the
// priority of a sender and the gas one entity may have hoisted per block.
type prioritySource interface {
	LatestBlock(ctx context.Context) (int, error)
	Block(ctx context.Context, height int) ([]blockTransaction, error)
	Prioritized(ctx context.Context, sender common.Address) (priorityClass, error)
	GasQuota(ctx context.Context) (uint64, error)
}

// priorityClass is what the registry says about a sender: whether its
// transactions are prioritized and which entity's per-block gas budget they draw
// from. The entity is compared and reported, never interpreted.
type priorityClass struct {
	Prioritized bool
	Entity      string
}

// blockTransaction is one transaction of a block, in the order the block carries
// it.
type blockTransaction struct {
	From common.Address
	Gas  uint64 // the gas reserved, which is what the quota counts
	// Internal marks a transaction the client itself inserted, such as sealing an
	// epoch. Those are placed around the transactions of users rather than
	// ordered among them.
	Internal bool
}

func (c *priorityOrderingChecker) Configure(config CheckerConfig) Checker {
	if config == nil {
		return c
	}
	relaxed := *c
	if v, exists := config["minMixedBlocks"]; exists {
		relaxed.minMixedBlocks = v.(int)
	}
	if v, exists := config["minRunCoverage"]; exists {
		relaxed.minRunCoverage = v.(float64)
	}
	if v, exists := config["duration"]; exists {
		relaxed.timeout = time.Duration(v.(int64))
	}
	return &relaxed
}

func (c *priorityOrderingChecker) Check(ctx context.Context) error {
	quota, err := c.source.GasQuota(ctx)
	if err != nil {
		return fmt.Errorf("failed to read the priority configuration: %w", err)
	}
	start, err := c.source.LatestBlock(ctx)
	if err != nil {
		return fmt.Errorf("failed to read the head of the chain: %w", err)
	}

	classes := map[common.Address]priorityClass{}
	classOf := func(sender common.Address) (priorityClass, error) {
		if known, found := classes[sender]; found {
			return known, nil
		}
		known, err := c.source.Prioritized(ctx, sender)
		if err != nil {
			return priorityClass{}, err
		}
		classes[sender] = known
		return known, nil
	}

	seen := observation{}
	deadline := time.Now().Add(c.timeout)
	next := start + 1
	waited := false

	for seen.mixed < c.minMixedBlocks && seen.blocks < maxBlocksObserved {
		head, err := c.source.LatestBlock(ctx)
		if err != nil {
			return fmt.Errorf("failed to read the head of the chain: %w", err)
		}
		if head < next {
			// The block is not there yet: wait for the network to produce it, as
			// long as the budget lasts.
			if time.Now().After(deadline) {
				waited = true
				break
			}
			if err := sleep(ctx, priorityPollInterval); err != nil {
				return err
			}
			continue
		}

		for ; next <= head && seen.mixed < c.minMixedBlocks && seen.blocks < maxBlocksObserved; next++ {
			txs, err := c.source.Block(ctx, next)
			if err != nil {
				return fmt.Errorf("failed to read block %d: %w", next, err)
			}

			classified := make([]classifiedTransaction, 0, len(txs))
			for _, tx := range txs {
				if tx.Internal {
					continue
				}
				known, err := classOf(tx.From)
				if err != nil {
					return fmt.Errorf("failed to read the priority of %v: %w", tx.From, err)
				}
				classified = append(classified, classifiedTransaction{class: known, gas: tx.Gas})
			}
			seen.add(next, measureOrder(classified, quota), quota)
		}
	}

	slog.Info("checked the order of prioritized transactions",
		"observedBlocks", seen.blocks,
		"firstBlock", start+1,
		"mixedBlocks", seen.mixed,
		"medianRunCoverage", medianOf(seen.coverages),
		"minRunCoverage", c.minRunCoverage,
		"blocksOverQuota", len(seen.overQuota),
		"blocksWithDemotedTransactions", seen.demoted,
		"gasQuota", quota)

	var issues []error
	if seen.mixed < c.minMixedBlocks {
		reason := fmt.Sprintf("in the %d blocks produced since the check started", seen.blocks)
		if waited {
			reason = fmt.Sprintf("in the %d blocks the network produced in %s", seen.blocks, c.timeout)
		}
		issues = append(issues, fmt.Errorf(
			"only %d blocks carried both prioritized and ordinary transactions %s, "+
				"%d are required: the ordering of an absent load cannot be observed, "+
				"so this check would pass on a network without priorities",
			seen.mixed, reason, c.minMixedBlocks))
	}
	if len(seen.overQuota) > 0 {
		issues = append(issues, fmt.Errorf(
			"prioritized transactions were hoisted beyond the gas one entity may "+
				"have per block: %v", seen.overQuota))
	}
	if median := medianOf(seen.coverages); seen.mixed >= c.minMixedBlocks && median < c.minRunCoverage {
		issues = append(issues, fmt.Errorf(
			"blocks carrying both do not open with their prioritized transactions: "+
				"the opening run covers a median %.2f of the transactions the gas "+
				"quota could admit, %.2f is required - the block order does not look "+
				"prioritized at all",
			median, c.minRunCoverage))
	}
	return errors.Join(issues...)
}

// observation accumulates what the blocks read so far say about the order.
type observation struct {
	blocks    int       // blocks read
	mixed     int       // of those, the ones carrying both classes
	demoted   int       // of those, the ones that also left prioritized ones behind
	coverages []float64 // the coverage of each mixed block
	overQuota []string  // the blocks whose hoisted run exceeded the quota
}

func (o *observation) add(height int, order blockOrder, quota uint64) {
	o.blocks++
	if !order.mixed || order.admissible == 0 {
		// One class only, or a lane the quota cannot admit anything of: the block
		// says nothing about the order of the two.
		return
	}
	o.mixed++
	o.coverages = append(o.coverages, order.coverage())

	if order.run < order.prioritized {
		// Prioritized transactions follow ordinary ones, so this block demoted some
		// of them and where its hoisted prefix ended is not observable. Anything the
		// opening run carries beyond the quota may be that remainder rather than an
		// overrun.
		o.demoted++
		return
	}
	for entity, gas := range order.runGas {
		if gas > quota {
			o.overQuota = append(o.overQuota, fmt.Sprintf(
				"block %d hoisted %d gas of %d allowed for entity %s",
				height, gas, quota, entity))
		}
	}
}

// classifiedTransaction is a transaction of a block paired with what the registry
// says about its sender.
type classifiedTransaction struct {
	class priorityClass
	gas   uint64
}

// blockOrder is what one block says about the order of prioritized
// transactions.
type blockOrder struct {
	mixed       bool              // carries transactions of both classes
	prioritized int               // prioritized transactions anywhere in the block
	admissible  int               // as many of them as the gas quota can admit
	run         int               // prioritized transactions opening the block
	runGas      map[string]uint64 // the gas of that run, per entity
}

// coverage is the share of the prioritized transactions a block could hoist that
// its opening run carries. It saturates at one: a run longer than the quota
// admits contains transactions the client demoted, which are not evidence of a
// better order than the quota allows.
func (o blockOrder) coverage() float64 {
	return min(float64(o.run)/float64(o.admissible), 1)
}

// measureOrder reads the given block. admissible replaces the plain count of
// prioritized transactions as the yardstick for the opening run: the client
// hoists them per entity only while that entity's gas quota lasts and demotes
// the rest, so a lane offering more gas than its quota necessarily leaves
// prioritized transactions among the ordinary ones. Counting those as a
// misordering would fail the check exactly when the quota does its job.
//
// It is computed by admitting the prioritized transactions in block order while
// their entity has budget left, which is an upper bound: the per-sender nonce
// contiguity the client also requires can only admit fewer.
func measureOrder(txs []classifiedTransaction, quota uint64) blockOrder {
	order := blockOrder{runGas: map[string]uint64{}}

	budget := map[string]uint64{}
	remaining := func(entity string) uint64 {
		if left, known := budget[entity]; known {
			return left
		}
		return quota
	}
	for _, tx := range txs {
		if !tx.class.Prioritized {
			continue
		}
		order.prioritized++
		if left := remaining(tx.class.Entity); tx.gas <= left {
			budget[tx.class.Entity] = left - tx.gas
			order.admissible++
		}
	}
	order.mixed = order.prioritized > 0 && order.prioritized < len(txs)

	for _, tx := range txs {
		if !tx.class.Prioritized {
			break
		}
		order.runGas[tx.class.Entity] += tx.gas
		order.run++
	}
	return order
}

// medianOf returns the median of the given values, or zero if there are none.
func medianOf(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := slices.Clone(values)
	slices.Sort(sorted)
	return sorted[len(sorted)/2]
}

// networkPrioritySource reads the blocks and the registry of a live network.
type networkPrioritySource struct {
	net      driver.Network
	client   rpc.Client
	registry *priority_registry.Registry
}

func (s *networkPrioritySource) connect() error {
	if s.client != nil {
		return nil
	}
	client, err := s.net.DialRandomRpc()
	if err != nil {
		return fmt.Errorf("failed to connect to the network: %w", err)
	}
	registry, err := priority_registry.NewRegistry(priority_registry.GetAddress(), client)
	if err != nil {
		client.Close()
		return fmt.Errorf("failed to bind to the priority registry: %w", err)
	}
	s.client, s.registry = client, registry
	return nil
}

func (s *networkPrioritySource) LatestBlock(ctx context.Context) (int, error) {
	if err := s.connect(); err != nil {
		return 0, err
	}
	height, err := s.client.BlockNumber(ctx)
	return int(height), err
}

func (s *networkPrioritySource) Block(_ context.Context, height int) ([]blockTransaction, error) {
	if err := s.connect(); err != nil {
		return nil, err
	}
	var block *struct {
		Transactions []struct {
			From common.Address `json:"from"`
			Gas  hexutil.Uint64 `json:"gas"`
			V    *hexutil.Big   `json:"v"`
			R    *hexutil.Big   `json:"r"`
		} `json:"transactions"`
	}
	if err := s.client.Call(
		&block, "eth_getBlockByNumber", hexutil.EncodeUint64(uint64(height)), true,
	); err != nil {
		return nil, err
	}
	if block == nil {
		return nil, fmt.Errorf("block %d not found", height)
	}

	txs := make([]blockTransaction, 0, len(block.Transactions))
	for _, tx := range block.Transactions {
		txs = append(txs, blockTransaction{
			From: tx.From,
			Gas:  uint64(tx.Gas),
			// The client signs its own transactions with an empty signature.
			Internal: isZero(tx.V) && isZero(tx.R),
		})
	}
	return txs, nil
}

func (s *networkPrioritySource) Prioritized(_ context.Context, sender common.Address) (priorityClass, error) {
	if err := s.connect(); err != nil {
		return priorityClass{}, err
	}
	priority, err := s.registry.SenderPriority(nil, sender)
	if err != nil {
		return priorityClass{}, err
	}
	if priority.Level == 0 {
		return priorityClass{}, nil
	}
	// The gas quota is granted per entity, so senders of the same entity - one
	// Norma application - share it.
	return priorityClass{Prioritized: true, Entity: priority.Id.String()}, nil
}

func (s *networkPrioritySource) GasQuota(_ context.Context) (uint64, error) {
	if err := s.connect(); err != nil {
		return 0, err
	}
	config, err := s.registry.GetPriorityConfig(nil)
	if err != nil {
		return 0, err
	}
	if !config.MaxGasPerEntityPerBlock.IsUint64() {
		return 0, fmt.Errorf(
			"the registry grants %v gas per entity per block, which does not fit a 64 bit value",
			config.MaxGasPerEntityPerBlock)
	}
	return config.MaxGasPerEntityPerBlock.Uint64(), nil
}

func isZero(value *hexutil.Big) bool {
	return value == nil || (*big.Int)(value).Sign() == 0
}
