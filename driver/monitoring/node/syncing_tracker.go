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

package nodemon

import (
	"sync"

	"github.com/0xsoniclabs/norma/driver/monitoring"
)

// syncingTracker tracks which nodes are currently catching up with the network
// so that out-of-order block errors emitted during re-sync can be suppressed.
type syncingTracker struct {
	syncingNodes map[monitoring.Node]bool
	syncingMutex sync.Mutex
}

func newSyncingTracker() syncingTracker {
	return syncingTracker{
		syncingNodes: make(map[monitoring.Node]bool, 50),
	}
}

func (s *syncingTracker) markNodeAsSyncing(node monitoring.Node) {
	s.syncingMutex.Lock()
	defer s.syncingMutex.Unlock()
	if _, exists := s.syncingNodes[node]; !exists {
		s.syncingNodes[node] = true
	}
}

func (s *syncingTracker) markNodeAsSynced(node monitoring.Node) {
	s.syncingMutex.Lock()
	defer s.syncingMutex.Unlock()
	s.syncingNodes[node] = false
}

func (s *syncingTracker) isNodeSyncing(node monitoring.Node) bool {
	s.syncingMutex.Lock()
	defer s.syncingMutex.Unlock()
	syncing, exists := s.syncingNodes[node]
	if !exists {
		return true
	}
	return syncing
}

func (s *syncingTracker) shouldSuppressAppendConflict(node monitoring.Node, err error) bool {
	if !monitoring.IsOutOfOrderAppendError(err) {
		return false
	}
	return s.isNodeSyncing(node)
}
