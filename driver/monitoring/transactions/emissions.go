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
	"time"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/rpc"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

// dagPollInterval is how often the DAG is asked for new events. Validators emit
// an event every few hundred milliseconds, and an event reaches a block about a
// second later, so polling faster buys nothing while polling much slower would
// let transactions be included before their emission was seen.
const dagPollInterval = 500 * time.Millisecond

// emissionObserver marks transactions as emitted by walking the event DAG.
//
// Every poll starts at the heads of the current and the previous epoch and
// follows parents until it reaches events that were already visited, so each
// event is read exactly once over a run. The moment of emission is the creation
// time the emitting validator recorded in the event itself, which is more
// precise than when this poll happened to find it.
type emissionObserver struct {
	tracker  *Tracker
	clients  clientSource
	visited  map[common.Hash]bool
	events   int
	failures int
}

func newEmissionObserver(tracker *Tracker, clients clientSource) *emissionObserver {
	return &emissionObserver{
		tracker: tracker,
		clients: clients,
		visited: map[common.Hash]bool{},
	}
}

// run polls the DAG until the context is cancelled.
func (o *emissionObserver) run(ctx context.Context) {
	var client rpc.Client
	defer func() {
		if client != nil {
			client.Close()
		}
	}()

	ticker := time.NewTicker(dagPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			slog.Info("stopped following the event DAG",
				"events", o.events, "failedReads", o.failures)
			return
		case <-ticker.C:
			if client == nil {
				var err error
				if client, err = o.clients.dial(ctx); err != nil {
					continue
				}
			}
			if err := o.poll(ctx, client); err != nil {
				o.failures++
				if o.failures == 1 {
					// A monitor that quietly observes nothing is worse than one
					// that says why, so the first failure is reported loudly.
					slog.Warn("failed to read the event DAG, transaction emissions will be missing",
						"error", err)
				} else {
					slog.Debug("failed to read the event DAG", "error", err)
				}
				client.Close()
				client = nil
			}
		}
	}
}

// poll reads every event that appeared since the previous poll.
//
// Only the current epoch can be walked: a client keeps the heads of that one
// only. The events created in the last poll interval before an epoch is sealed
// are therefore unreachable, and the transactions they carried - if they were
// not carried by a later event too - end up without a measured emission.
func (o *emissionObserver) poll(ctx context.Context, client rpc.Client) error {
	var epoch hexutil.Uint64
	if err := client.Call(&epoch, "eth_currentEpoch"); err != nil {
		return fmt.Errorf("failed to get current epoch: %w", err)
	}

	var heads []common.Hash
	if err := client.Call(
		&heads, "dag_getHeads", epoch.String(),
	); err != nil {
		return fmt.Errorf("failed to get heads of epoch %d: %w", uint64(epoch), err)
	}
	return o.walk(ctx, client, heads)
}

// walk visits the given events and their unvisited ancestors.
func (o *emissionObserver) walk(
	ctx context.Context, client rpc.Client, heads []common.Hash,
) error {
	pending := make([]common.Hash, 0, len(heads))
	pending = append(pending, heads...)

	for len(pending) > 0 {
		if ctx.Err() != nil {
			return nil
		}

		id := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if o.visited[id] {
			continue
		}
		o.visited[id] = true

		event, err := eventPayload(client, id)
		if err != nil {
			// The event stays marked as visited: a payload that cannot be read
			// now is unlikely to be readable later, and retrying it forever
			// would stall the walk.
			return err
		}
		if event == nil {
			continue
		}
		o.events++

		emittedAt := time.Unix(0, int64(event.CreationTime))
		for _, hash := range event.Transactions {
			o.tracker.MarkEmitted(hash, emittedAt)
		}
		pending = append(pending, event.Parents...)
	}
	return nil
}

// event is the part of a DAG event's payload this package reads.
type event struct {
	// CreationTime is when the emitting validator created the event, in
	// nanoseconds since the Unix epoch.
	CreationTime hexutil.Uint64 `json:"creationTime"`
	Parents      []common.Hash  `json:"parents"`
	Transactions []common.Hash  `json:"transactions"`
}

// eventPayload retrieves one event including the hashes of its transactions.
// Returns nil without an error if the event does not exist.
func eventPayload(client rpc.Client, id common.Hash) (*event, error) {
	var payload *event
	if err := client.Call(
		&payload, "dag_getEventPayload", id.Hex(), true,
	); err != nil {
		return nil, fmt.Errorf("failed to get event %s: %w", id.Hex(), err)
	}
	return payload, nil
}

// clientSource provides connections to the network under observation.
type clientSource interface {
	dial(ctx context.Context) (rpc.Client, error)
}

// networkClients dials a random node of a network, retrying a few times before
// giving up so that a node restart does not end the observation.
type networkClients struct {
	network driver.Network
}

func (c networkClients) dial(context.Context) (rpc.Client, error) {
	client, err := c.network.DialRandomRpc()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to the network: %w", err)
	}
	return client, nil
}
