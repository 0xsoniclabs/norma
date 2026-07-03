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

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/monitoring"
	"github.com/0xsoniclabs/norma/driver/rpc"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

// defaultThrottleCeiling is the default maximum ratio (in percent)
// between the least-emitting and most-emitting non-dominant validators.
const defaultThrottleCeiling = 50

func init() {
	RegisterNetworkCheck("eventThrottled",
		func(net driver.Network, _ *monitoring.Monitor) Checker {
			return &eventThrottledChecker{
				net:     net,
				ceiling: defaultThrottleCeiling,
			}
		})
}

// eventThrottledChecker verifies that the event throttler is effective
// by comparing event emission among non-dominant validators. When some
// validators have the throttler enabled and others do not, the throttled
// ones should emit significantly fewer events.
//
// The check passes when the minimum-emitting non-dominant validator
// produces at most ceiling% of the events that the maximum-emitting
// non-dominant validator produces.
type eventThrottledChecker struct {
	net     driver.Network
	ceiling int // max percentage (0–100)
}

func (c *eventThrottledChecker) Configure(config CheckerConfig) Checker {
	ceiling := defaultThrottleCeiling
	if v, ok := config["ceiling"]; ok {
		if i, ok := v.(int); ok {
			ceiling = i
		}
	}
	return &eventThrottledChecker{net: c.net, ceiling: ceiling}
}

func (c *eventThrottledChecker) Check(ctx context.Context) error {
	ceiling := c.ceiling

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

	return verifyThrottling(counts, labels, ceiling)
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

// verifyThrottling checks that the least-emitting non-dominant
// validator produced at most ceiling% of the most-emitting
// non-dominant validator's events.
func verifyThrottling(counts map[uint64]int, labels map[int]string, ceiling int) error {
	// Identify the dominant validator (most events). On ties the
	// choice is arbitrary, but the check remains valid because
	// it only compares non-dominant validators against each other.
	maxCount := 0
	var dominantCreator uint64
	for creator, count := range counts {
		if count > maxCount {
			maxCount = count
			dominantCreator = creator
		}
	}

	// Among non-dominant validators, find min and max emitters.
	minNonDom := math.MaxInt
	maxNonDom := 0
	var minCreator, maxCreator uint64
	nonDomCount := 0
	for creator, count := range counts {
		if creator == dominantCreator {
			continue
		}
		nonDomCount++
		if count < minNonDom {
			minNonDom = count
			minCreator = creator
		}
		if count > maxNonDom {
			maxNonDom = count
			maxCreator = creator
		}
	}

	if nonDomCount < 2 {
		return fmt.Errorf(
			"need at least 2 non-dominant validators, got %d",
			nonDomCount,
		)
	}
	if maxNonDom == 0 {
		return fmt.Errorf(
			"max non-dominant validator emitted 0 events",
		)
	}

	ratio := float64(minNonDom) / float64(maxNonDom) * 100
	slog.Info("event throttle comparison",
		"throttled_validator", minCreator,
		"throttled_node", labels[int(minCreator)],
		"throttled_events", minNonDom,
		"unthrottled_validator", maxCreator,
		"unthrottled_node", labels[int(maxCreator)],
		"unthrottled_events", maxNonDom,
		"ratio_percent", fmt.Sprintf("%.1f%%", ratio),
		"ceiling_percent", ceiling,
	)

	if ratio > float64(ceiling) {
		return fmt.Errorf(
			"throttled validator %d (%s) emitted %d events "+
				"(%.1f%% of unthrottled validator %d (%s) "+
				"with %d events); expected at most %d%%",
			minCreator, labels[int(minCreator)], minNonDom,
			ratio,
			maxCreator, labels[int(maxCreator)], maxNonDom,
			ceiling,
		)
	}

	return nil
}
