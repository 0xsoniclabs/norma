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
	"time"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/monitoring"
	"github.com/0xsoniclabs/norma/driver/rpc"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

// minGapRatio is the minimum ratio required between the slowest
// unthrottled validator and the fastest expected-throttled validator.
// Below this ratio the split is considered too narrow to conclude that
// throttling is occurring.
const minGapRatio = 2.0

// defaultSampleWindow is the interval between the two DAG snapshots used
// to compute per-validator emission rates. Longer windows produce more
// stable rate estimates but are more likely to straddle an epoch
// boundary and force a retry.
const defaultSampleWindow = 5 * time.Second

// maxSampleAttempts caps the number of sampling attempts before giving
// up. An attempt is discarded whenever the epoch rolls over between the
// two snapshots that make up the window.
const maxSampleAttempts = 5

func init() {
	RegisterNetworkCheck("eventThrottled",
		func(net driver.Network, _ *monitoring.Monitor) Checker {
			return newEventThrottledChecker(net)
		})
}

func newEventThrottledChecker(net driver.Network) *eventThrottledChecker {
	return &eventThrottledChecker{
		net:          net,
		sampleWindow: defaultSampleWindow,
		collectRates: collectEmissionRates,
	}
}

// eventThrottledChecker verifies the event throttler by measuring
// per-validator event emission rates over a fixed sampling window and
// checking that every validator listed in `throttledNodes` emits at a
// rate at most 1/`minGapRatio` of every other observed validator's rate.
type eventThrottledChecker struct {
	net            driver.Network
	throttledNodes []string // node labels expected to be throttled
	sampleWindow   time.Duration
	// collectRates measures per-validator emission rates over the given
	// window. Overridable for tests.
	collectRates func(
		ctx context.Context, client rpc.Client, window time.Duration,
	) (map[uint64]float64, error)
}

func (c *eventThrottledChecker) Configure(config CheckerConfig) Checker {
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
		throttledNodes: throttled,
		sampleWindow:   c.sampleWindow,
		collectRates:   c.collectRates,
	}
}

func (c *eventThrottledChecker) Check(ctx context.Context) error {
	if len(c.throttledNodes) == 0 {
		return fmt.Errorf("throttledNodes must not be empty")
	}

	nodes := c.net.GetActiveNodes()
	if len(nodes) == 0 {
		return fmt.Errorf("no active nodes")
	}

	// Resolve labels first so a misspelled throttledNodes entry is
	// reported immediately, before the multi-second DAG sampling.
	labels, expected, err := resolveLabels(nodes, c.throttledNodes)
	if err != nil {
		return err
	}

	client, err := dialFirstReachable(ctx, nodes)
	if err != nil {
		return err
	}
	defer client.Close()

	rates, err := c.collectRates(ctx, client, c.sampleWindow)
	if err != nil {
		return err
	}

	logEmissionRates(rates, labels)
	logThrottledSet(expected, labels)

	return verifyThrottled(expected, rates)
}

// resolveLabels walks the node list once, returning both a
// validator-id -> label map (for logging) and the set of validator ids
// that match the configured throttledNodes labels. Every label listed
// in throttledNodes must resolve to an active validator.
func resolveLabels(
	nodes []driver.Node, throttledNodes []string,
) (map[uint64]string, map[uint64]struct{}, error) {
	labels := make(map[uint64]string, len(nodes))
	byLabel := make(map[string]uint64, len(nodes))
	for _, n := range nodes {
		id := n.GetValidatorId()
		if id == nil {
			continue
		}
		vid := uint64(*id)
		label := n.GetLabel()
		labels[vid] = label
		byLabel[label] = vid
	}
	throttled := make(map[uint64]struct{}, len(throttledNodes))
	for _, label := range throttledNodes {
		id, ok := byLabel[label]
		if !ok {
			return nil, nil, fmt.Errorf(
				"throttledNodes: no active validator with label %q",
				label,
			)
		}
		throttled[id] = struct{}{}
	}
	return labels, throttled, nil
}

// dialFirstReachable returns an RPC client for the first non-failing
// node that accepts a connection.
func dialFirstReachable(
	ctx context.Context, nodes []driver.Node,
) (rpc.Client, error) {
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

// rawEvent is the wire representation of a DAG event as returned by
// dag_getEvent. Struct tags let the JSON-RPC client unmarshal directly
// into it, avoiding a manual walk over map[string]any.
type rawEvent struct {
	Creator hexutil.Uint64 `json:"creator"`
	Parents []common.Hash  `json:"parents"`
}

// collectEmissionRates repeatedly calls sampleEmissionRates until one
// attempt produces a window that does not straddle an epoch boundary,
// giving up after maxSampleAttempts. This is necessary because Sonic
// epochs can be shorter than a useful sampling window.
func collectEmissionRates(
	ctx context.Context, client rpc.Client, window time.Duration,
) (map[uint64]float64, error) {
	var lastErr error
	for attempt := range maxSampleAttempts {
		rates, err := sampleEmissionRates(ctx, client, window)
		if err == nil {
			return rates, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		lastErr = err
		slog.Info(
			"emission-rate sample discarded, retrying",
			"attempt", attempt+1,
			"max_attempts", maxSampleAttempts,
			"error", err,
		)
	}
	return nil, fmt.Errorf(
		"could not obtain a stable emission-rate sample after %d "+
			"attempts: %w",
		maxSampleAttempts, lastErr,
	)
}

// sampleEmissionRates takes two DAG snapshots separated by `window` and
// returns each validator's emission rate in events per second. The
// current epoch must not roll over during the window; on rollover an
// error is returned so the caller can retry.
func sampleEmissionRates(
	ctx context.Context, client rpc.Client, window time.Duration,
) (map[uint64]float64, error) {
	epoch1, counts1, err := snapshotEpochCounts(ctx, client)
	if err != nil {
		return nil, fmt.Errorf("first snapshot failed: %w", err)
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(window):
	}

	epoch2, counts2, err := snapshotEpochCounts(ctx, client)
	if err != nil {
		return nil, fmt.Errorf("second snapshot failed: %w", err)
	}

	if epoch1 != epoch2 {
		return nil, fmt.Errorf(
			"epoch changed during sampling window (%d -> %d)",
			epoch1, epoch2,
		)
	}

	seconds := window.Seconds()
	rates := make(map[uint64]float64, len(counts2))
	// Both snapshots walk the same epoch's DAG, so counts2[id] is always
	// >= counts1[id] and every id in counts1 is also in counts2.
	for id, c2 := range counts2 {
		rates[id] = float64(c2-counts1[id]) / seconds
	}
	return rates, nil
}

// snapshotEpochCounts queries the current epoch and counts events per
// validator by walking the DAG of that epoch.
func snapshotEpochCounts(
	ctx context.Context, client rpc.Client,
) (uint64, map[uint64]int, error) {
	var epoch hexutil.Uint64
	if err := client.Call(
		&epoch, "eth_currentEpoch",
	); err != nil {
		return 0, nil, fmt.Errorf("failed to get current epoch: %w", err)
	}
	var headHexes []string
	if err := client.Call(
		&headHexes, "dag_getHeads", epoch.String(),
	); err != nil {
		return 0, nil, fmt.Errorf("failed to get DAG heads: %w", err)
	}
	if len(headHexes) == 0 {
		// Transient: a freshly rolled-over epoch may have no heads yet.
		// Returning an error triggers a retry in collectEmissionRates.
		return 0, nil, fmt.Errorf(
			"epoch %s has no DAG heads yet", epoch.String(),
		)
	}
	counts, err := countEventsFromHeads(ctx, client, headHexes)
	if err != nil {
		return 0, nil, err
	}
	if len(counts) == 0 {
		return 0, nil, fmt.Errorf("collected no events from DAG")
	}
	return uint64(epoch), counts, nil
}

// countEventsFromHeads walks the DAG from the given head hashes via DFS
// and returns per-creator event counts. Each event is fetched at most
// once. Missing events (nil RPC result) are skipped.
func countEventsFromHeads(
	ctx context.Context, client rpc.Client, headHexes []string,
) (map[uint64]int, error) {
	counts := make(map[uint64]int)
	visited := make(map[common.Hash]struct{}, len(headHexes))
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
		visited[id] = struct{}{}

		ev, err := fetchEvent(client, id)
		if err != nil {
			return nil, err
		}
		if ev == nil {
			continue
		}

		counts[uint64(ev.Creator)]++
		for _, p := range ev.Parents {
			if _, seen := visited[p]; !seen {
				queue = append(queue, p)
			}
		}
	}
	return counts, nil
}

// fetchEvent retrieves a single DAG event by its hash.
// Returns nil without error when the event does not exist.
func fetchEvent(client rpc.Client, id common.Hash) (*rawEvent, error) {
	var ev *rawEvent
	if err := client.Call(
		&ev, "dag_getEvent", id.Hex(),
	); err != nil {
		return nil, fmt.Errorf(
			"failed to get event %s: %w", id.Hex(), err,
		)
	}
	return ev, nil
}

// verifyThrottled fails when the slowest validator not in the expected
// set does not emit at least `minGapRatio` times as many events per
// second as the fastest validator in the expected set. This one
// invariant catches all misclassification cases: rates that are uniform
// (no throttling active), a listed validator emitting at full speed, or
// an unlisted validator emitting suspiciously slowly.
func verifyThrottled(
	expected map[uint64]struct{},
	rates map[uint64]float64,
) error {
	var (
		maxListed    float64
		minUnlisted  float64
		haveListed   bool
		haveUnlisted bool
	)
	for id, r := range rates {
		if _, ok := expected[id]; ok {
			if !haveListed || r > maxListed {
				maxListed = r
			}
			haveListed = true
		} else {
			if !haveUnlisted || r < minUnlisted {
				minUnlisted = r
			}
			haveUnlisted = true
		}
	}
	if !haveListed {
		return fmt.Errorf("no expected-throttled validator observed")
	}
	if !haveUnlisted {
		return fmt.Errorf(
			"no unthrottled validator observed for comparison",
		)
	}

	// If listed validators emit nothing, any positive unlisted rate is
	// clear evidence of throttling. Both zero means nothing is
	// happening at all, which is not the same as throttling.
	if maxListed == 0 {
		if minUnlisted == 0 {
			return fmt.Errorf(
				"no throttling detected: no validator emitted " +
					"events over the sampling window",
			)
		}
		return nil
	}

	ratio := minUnlisted / maxListed
	if ratio >= minGapRatio {
		return nil
	}
	return fmt.Errorf(
		"no throttling detected: slowest unthrottled validator emits "+
			"at %.2f/s but fastest expected-throttled validator emits "+
			"at %.2f/s (ratio %.2f < required %.2f)",
		minUnlisted, maxListed, ratio, minGapRatio,
	)
}

// logEmissionRates logs per-validator emission rates.
func logEmissionRates(
	rates map[uint64]float64, labels map[uint64]string,
) {
	total := 0.0
	for _, r := range rates {
		total += r
	}
	for id, r := range rates {
		pct := 0.0
		if total > 0 {
			pct = r / total * 100
		}
		slog.Info("event emission rate",
			"validator", id,
			"node", labels[id],
			"rate_per_sec", fmt.Sprintf("%.2f", r),
			"total_per_sec", fmt.Sprintf("%.2f", total),
			"percent", fmt.Sprintf("%.1f%%", pct),
		)
	}
}

// logThrottledSet logs which validators are expected to be throttled.
func logThrottledSet(
	throttledSet map[uint64]struct{},
	labels map[uint64]string,
) {
	for id := range throttledSet {
		slog.Info("expected throttled validator",
			"validator", id,
			"node", labels[id],
		)
	}
}
