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
	"sync"

	mon "github.com/0xsoniclabs/norma/driver/monitoring"
)

// All transaction metrics of one monitor read the same tracker: following a
// transaction is done once, and each metric is one view on the result.
var (
	trackersMu sync.Mutex
	trackers   = map[*mon.Monitor]*trackerInstance{}
)

// trackerInstance is a tracker together with the observers feeding it.
type trackerInstance struct {
	tracker *Tracker
	stop    context.CancelFunc
	stopped bool
}

// tracker returns the tracker of the given monitor, attaching it to the network
// on first use.
func tracker(monitor *mon.Monitor) *Tracker {
	trackersMu.Lock()
	defer trackersMu.Unlock()

	if instance, found := trackers[monitor]; found {
		return instance.tracker
	}

	tracker := NewTracker()
	ctx, cancel := context.WithCancel(context.Background())
	instance := &trackerInstance{tracker: tracker, stop: cancel}
	trackers[monitor] = instance

	network := monitor.Network()
	clients := networkClients{network: network}

	// Submissions are reported by the network, so they need no polling.
	network.RegisterTransactionObserver(tracker)

	// Inclusions follow the blocks Norma already observes through the node logs.
	inclusions := newInclusionObserver(tracker, clients)
	monitor.NodeLogProvider().RegisterLogListener(inclusions)
	go inclusions.run(ctx)

	go newEmissionObserver(tracker, clients).run(ctx)

	return tracker
}

// stopTracker stops observing transactions for the given monitor. The data
// collected so far remains available, so it can still be exported. Stopping an
// already stopped tracker does nothing, which lets every metric sharing it ask
// for the stop when it is shut down.
func stopTracker(monitor *mon.Monitor) {
	trackersMu.Lock()
	defer trackersMu.Unlock()

	instance, found := trackers[monitor]
	if !found || instance.stopped {
		return
	}
	instance.stopped = true
	instance.stop()
	instance.tracker.LogSummary()
}
