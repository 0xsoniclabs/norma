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
	"fmt"
	"log/slog"
	"math"
	"math/big"
	"sort"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/monitoring"
	"github.com/0xsoniclabs/norma/driver/rpc"
	"github.com/0xsoniclabs/sonic/gossip/contract/sfc100"
	"github.com/0xsoniclabs/sonic/opera/contracts/sfc"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

// defaultThrottleCeiling is the default maximum ratio (in percent) between
// the least-emitting non-dominant validator and the max-emitting dominant
// validator. Values above the ceiling indicate the throttler is not
// effective enough.
const defaultThrottleCeiling = 50

// defaultDominantStakeThreshold matches sonic's emitter throttler default
// (see emitter/config.DefaultThrottlerConfig). The dominant set is the
// smallest set of highest-staked validators whose cumulative stake meets
// or exceeds threshold * total stake.
const defaultDominantStakeThreshold = 0.75

func init() {
	RegisterNetworkCheck("eventThrottled",
		func(net driver.Network, _ *monitoring.Monitor) Checker {
			return newEventThrottledChecker(net)
		})
}

func newEventThrottledChecker(net driver.Network) *eventThrottledChecker {
	return &eventThrottledChecker{
		net:            net,
		ceiling:        defaultThrottleCeiling,
		stakeThreshold: defaultDominantStakeThreshold,
		fetchStakes:    fetchValidatorStakes,
	}
}

// eventThrottledChecker verifies that the event throttler is effective by
// comparing event emission of throttled validators against the
// unthrottled reference set. The throttled set is either specified
// explicitly via the `throttledNodes` config (a list of node labels) or,
// when unspecified, derived from stake: the dominant set (as computed by
// sonic's emitter throttler) forms the unthrottled reference, and all
// non-dominant validators are expected to be throttled.
type eventThrottledChecker struct {
	net            driver.Network
	ceiling        int      // max percentage (0–100)
	stakeThreshold float64  // cumulative stake fraction that defines dominance
	throttledNodes []string // explicit node labels expected to be throttled
	// fetchStakes returns the current-epoch received stake per validator
	// ID. Overridable for tests.
	fetchStakes func(client rpc.Client) (map[uint64]*big.Int, error)
}

func (c *eventThrottledChecker) Configure(config CheckerConfig) Checker {
	ceiling := defaultThrottleCeiling
	if v, ok := config["ceiling"]; ok {
		if i, ok := v.(int); ok {
			ceiling = i
		}
	}
	threshold := defaultDominantStakeThreshold
	if v, ok := config["stakeThreshold"]; ok {
		if f, ok := v.(float64); ok {
			threshold = f
		}
	}
	var throttled []string
	if v, ok := config["throttledNodes"]; ok {
		switch list := v.(type) {
		case []string:
			throttled = list
		case []any:
			for _, item := range list {
				if s, ok := item.(string); ok {
					throttled = append(throttled, s)
				}
			}
		}
	}
	return &eventThrottledChecker{
		net:            c.net,
		ceiling:        ceiling,
		stakeThreshold: threshold,
		throttledNodes: throttled,
		fetchStakes:    c.fetchStakes,
	}
}

func (c *eventThrottledChecker) Check(ctx context.Context) error {
	nodes := c.net.GetActiveNodes()
	if len(nodes) == 0 {
		return fmt.Errorf("no active nodes")
	}

	client, err := dialFirstReachable(ctx, nodes)
	if err != nil {
		return err
	}
	defer client.Close()

	counts, err := collectEpochEventCounts(ctx, client)
	if err != nil {
		return err
	}

	labels := nodeLabels(nodes)
	logEmissionStats(counts, labels)

	throttledSet, err := c.resolveThrottledSet(client, nodes, labels)
	if err != nil {
		return err
	}

	return verifyThrottling(counts, throttledSet, labels, c.ceiling)
}

// resolveThrottledSet returns the set of validator IDs that are expected
// to be throttled. When explicit node labels are configured, they take
// precedence over stake-based inference.
func (c *eventThrottledChecker) resolveThrottledSet(
	client rpc.Client,
	nodes []driver.Node,
	labels map[int]string,
) (map[uint64]struct{}, error) {
	if len(c.throttledNodes) > 0 {
		set, err := throttledSetFromLabels(c.throttledNodes, nodes)
		if err != nil {
			return nil, err
		}
		logThrottledSet(set, labels, "explicit")
		return set, nil
	}

	stakes, err := c.fetchStakes(client)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to fetch validator stakes: %w", err,
		)
	}
	dominantSet := computeDominantSet(stakes, c.stakeThreshold)
	logDominantSet(dominantSet, stakes, labels)

	// Non-dominant validators (by stake) are the implicit throttled set.
	set := make(map[uint64]struct{})
	for id := range stakes {
		if _, isDominant := dominantSet[id]; !isDominant {
			set[id] = struct{}{}
		}
	}
	return set, nil
}

// throttledSetFromLabels resolves node labels to validator IDs, returning
// an error when any configured label does not match an active validator.
func throttledSetFromLabels(
	labelsWanted []string, nodes []driver.Node,
) (map[uint64]struct{}, error) {
	byLabel := make(map[string]uint64, len(nodes))
	for _, n := range nodes {
		id := n.GetValidatorId()
		if id == nil {
			continue
		}
		byLabel[n.GetLabel()] = uint64(*id)
	}
	set := make(map[uint64]struct{}, len(labelsWanted))
	for _, label := range labelsWanted {
		id, ok := byLabel[label]
		if !ok {
			return nil, fmt.Errorf(
				"throttledNodes: no active validator with label %q",
				label,
			)
		}
		set[id] = struct{}{}
	}
	return set, nil
}

// dialFirstReachable returns an RPC client for the first non-failing
// node that accepts a connection.
func dialFirstReachable(ctx context.Context, nodes []driver.Node) (rpc.Client, error) {
	for _, n := range nodes {
		if n.IsExpectedFailure() {
			continue
		}
		client, err := n.DialRpc(ctx)
		if err == nil {
			return client, nil
		}
	}
	return nil, fmt.Errorf("no reachable node for DAG query")
}

// dagEvent represents a single DAG event with its creator and parents.
type dagEvent struct {
	creator uint64
	parents []common.Hash
}

// collectEpochEventCounts fetches all events in the current epoch
// via the DAG API and returns per-creator event counts.
func collectEpochEventCounts(ctx context.Context, client rpc.Client) (map[uint64]int, error) {
	var epoch hexutil.Uint64
	if err := client.Call(
		&epoch, "eth_currentEpoch",
	); err != nil {
		return nil, fmt.Errorf("failed to get current epoch: %w", err)
	}

	var headHexes []string
	if err := client.Call(
		&headHexes, "dag_getHeads", epoch.String(),
	); err != nil {
		return nil, fmt.Errorf("failed to get DAG heads: %w", err)
	}
	if len(headHexes) == 0 {
		return nil, fmt.Errorf("no events in current epoch")
	}

	visited, err := walkDAG(ctx, client, headHexes)
	if err != nil {
		return nil, err
	}
	if len(visited) == 0 {
		return nil, fmt.Errorf("collected no events from DAG")
	}

	counts := make(map[uint64]int, len(visited))
	for _, ev := range visited {
		counts[ev.creator]++
	}
	return counts, nil
}

// walkDAG performs a DFS from the given head hashes, fetching each
// event and its parents until no more unvisited ancestors remain.
func walkDAG(ctx context.Context, client rpc.Client, headHexes []string,
) (map[common.Hash]dagEvent, error) {
	visited := make(map[common.Hash]dagEvent, len(headHexes))
	queue := make([]common.Hash, 0, len(headHexes))
	for _, h := range headHexes {
		queue = append(queue, common.HexToHash(h))
	}

	for len(queue) > 0 {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		id := queue[len(queue)-1]
		queue = queue[:len(queue)-1]

		if _, seen := visited[id]; seen {
			continue
		}

		ev, err := fetchEvent(client, id)
		if err != nil {
			return nil, err
		}
		if ev == nil {
			continue
		}

		visited[id] = *ev
		for _, p := range ev.parents {
			if _, seen := visited[p]; !seen {
				queue = append(queue, p)
			}
		}
	}
	return visited, nil
}

// fetchEvent retrieves a single DAG event by its hash.
// Returns nil without error when the event does not exist.
func fetchEvent(client rpc.Client, id common.Hash) (*dagEvent, error) {
	var result map[string]any
	if err := client.Call(
		&result, "dag_getEvent", id.Hex(),
	); err != nil {
		return nil, fmt.Errorf(
			"failed to get event %s: %w", id.Hex(), err,
		)
	}
	if result == nil {
		return nil, nil
	}

	creatorStr, ok := result["creator"].(string)
	if !ok {
		return nil, fmt.Errorf(
			"event %s: missing or invalid creator field", id.Hex(),
		)
	}
	var creator hexutil.Uint64
	if err := creator.UnmarshalText(
		[]byte(creatorStr),
	); err != nil {
		return nil, fmt.Errorf("failed to parse creator: %w", err)
	}

	parents := make([]common.Hash, 0)
	if ps, ok := result["parents"].([]any); ok {
		for _, p := range ps {
			s, ok := p.(string)
			if !ok {
				continue
			}
			parents = append(parents, common.HexToHash(s))
		}
	}

	return &dagEvent{
		creator: uint64(creator),
		parents: parents,
	}, nil
}

// nodeLabels builds a map from validator ID to the node label.
func nodeLabels(nodes []driver.Node) map[int]string {
	labels := make(map[int]string, len(nodes))
	for _, n := range nodes {
		if id := n.GetValidatorId(); id != nil {
			labels[*id] = n.GetLabel()
		}
	}
	return labels
}

// logEmissionStats logs per-validator event counts.
func logEmissionStats(counts map[uint64]int, labels map[int]string) {
	total := 0
	for _, c := range counts {
		total += c
	}
	for creator, count := range counts {
		pct := float64(count) / float64(total) * 100
		slog.Info("event emission stats",
			"validator", creator,
			"node", labels[int(creator)],
			"events", count,
			"total", total,
			"percent", fmt.Sprintf("%.1f%%", pct),
		)
	}
}

// verifyThrottling checks that the least-emitting throttled validator
// produced at most ceiling% of the events of the max-emitting
// unthrottled validator. The `throttledSet` names the validators
// expected to be throttled; all other observed validators serve as the
// unthrottled reference.
func verifyThrottling(
	counts map[uint64]int,
	throttledSet map[uint64]struct{},
	labels map[int]string,
	ceiling int,
) error {
	// Find the max-emitting unthrottled validator as the reference.
	// Only validators actually observed in the DAG are considered.
	maxUnthrottled := 0
	var unthrottledCreator uint64
	for creator, count := range counts {
		if _, isThrottled := throttledSet[creator]; isThrottled {
			continue
		}
		if count > maxUnthrottled {
			maxUnthrottled = count
			unthrottledCreator = creator
		}
	}

	// Find the least-emitting throttled validator among those observed.
	minThrottled := math.MaxInt
	var minCreator uint64
	observedThrottled := 0
	for creator, count := range counts {
		if _, isThrottled := throttledSet[creator]; !isThrottled {
			continue
		}
		observedThrottled++
		if count < minThrottled {
			minThrottled = count
			minCreator = creator
		}
	}

	if observedThrottled < 1 {
		return fmt.Errorf("need at least 1 throttled validator, got %d",
			observedThrottled,
		)
	}
	if maxUnthrottled == 0 {
		return fmt.Errorf("no unthrottled validator emitted events")
	}

	ratio := float64(minThrottled) / float64(maxUnthrottled) * 100
	slog.Info("event throttle comparison",
		"throttled_validator", minCreator,
		"throttled_node", labels[int(minCreator)],
		"throttled_events", minThrottled,
		"unthrottled_validator", unthrottledCreator,
		"unthrottled_node", labels[int(unthrottledCreator)],
		"unthrottled_events", maxUnthrottled,
		"ratio_percent", fmt.Sprintf("%.1f%%", ratio),
		"ceiling_percent", ceiling,
	)

	if ratio > float64(ceiling) {
		return fmt.Errorf(
			"throttled validator %d (%s) emitted %d events "+
				"(%.1f%% of unthrottled validator %d (%s) "+
				"with %d events); expected at most %d%%",
			minCreator, labels[int(minCreator)], minThrottled,
			ratio,
			unthrottledCreator, labels[int(unthrottledCreator)],
			maxUnthrottled,
			ceiling,
		)
	}

	return nil
}

// fetchValidatorStakes queries the SFC contract for the current-epoch
// received stake of each registered validator.
func fetchValidatorStakes(client rpc.Client) (map[uint64]*big.Int, error) {
	sfcContract, err := sfc100.NewContract(sfc.ContractAddress, client)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to create SFC contract binding: %w", err,
		)
	}
	lastValidatorID, err := sfcContract.LastValidatorID(nil)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to query last validator id: %w", err,
		)
	}
	epoch, err := sfcContract.CurrentEpoch(nil)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to query current epoch: %w", err,
		)
	}

	stakes := make(map[uint64]*big.Int)
	last := lastValidatorID.Uint64()
	for id := uint64(1); id <= last; id++ {
		stake, err := sfcContract.GetEpochReceivedStake(
			nil, epoch, new(big.Int).SetUint64(id),
		)
		if err != nil {
			return nil, fmt.Errorf(
				"failed to query stake for validator %d: %w", id, err,
			)
		}
		stakes[id] = stake
	}
	return stakes, nil
}

// computeDominantSet returns the smallest set of highest-staked
// validators whose cumulative stake meets or exceeds threshold * total
// stake. Validators with zero stake are excluded from the ordering.
// Ties are broken by ascending validator ID for determinism. This
// mirrors sonic's emitter/throttler.computeDominantSet semantics.
func computeDominantSet(
	stakes map[uint64]*big.Int, threshold float64,
) map[uint64]struct{} {
	res := make(map[uint64]struct{})
	type pair struct {
		id    uint64
		stake *big.Int
	}
	pairs := make([]pair, 0, len(stakes))
	total := new(big.Int)
	for id, s := range stakes {
		if s == nil || s.Sign() <= 0 {
			continue
		}
		pairs = append(pairs, pair{id, s})
		total = new(big.Int).Add(total, s)
	}
	if len(pairs) == 0 {
		return res
	}
	sort.Slice(pairs, func(i, j int) bool {
		if cmp := pairs[i].stake.Cmp(pairs[j].stake); cmp != 0 {
			return cmp > 0
		}
		return pairs[i].id < pairs[j].id
	})

	// needed = ceil(total * threshold), computed via big.Float.
	needed := new(big.Float).Mul(
		new(big.Float).SetInt(total),
		big.NewFloat(threshold),
	)

	accumulated := new(big.Int)
	for _, p := range pairs {
		if new(big.Float).SetInt(accumulated).Cmp(needed) >= 0 {
			return res
		}
		accumulated = new(big.Int).Add(accumulated, p.stake)
		res[p.id] = struct{}{}
	}
	return res
}

// logDominantSet logs which validators form the dominant set.
func logDominantSet(
	dominantSet map[uint64]struct{},
	stakes map[uint64]*big.Int,
	labels map[int]string,
) {
	for id := range dominantSet {
		slog.Info("dominant validator (by stake)",
			"validator", id,
			"node", labels[int(id)],
			"stake", stakes[id],
		)
	}
}

// logThrottledSet logs which validators are expected to be throttled.
func logThrottledSet(
	throttledSet map[uint64]struct{},
	labels map[int]string,
	source string,
) {
	for id := range throttledSet {
		slog.Info("throttled validator",
			"validator", id,
			"node", labels[int(id)],
			"source", source,
		)
	}
}
