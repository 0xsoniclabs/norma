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

package node

// NodeState represents the lifecycle state of an OperaNode.
//
// Every action claims a transitional state before doing any work, so that
// two concurrent actions cannot interleave, and restores the previous
// state if it fails. Transitions are enforced by OperaNode.transition:
//
//	Uninitialized --Initialize-------> Initializing --> Ready
//	Ready         --StartSonicd------> Syncing
//	Syncing       --WaitForSync------> Running
//	Running       --StopSonicd-------> Stopping     --> Ready
//	Running       --ForceStopSonicd--> Killed
//	Stopping      --ForceStopSonicd--> Killed
//	Killed        --HealSonicd-------> Healing      --> Ready
//
// Failure returns the node to the state the action started from, except
// for ForceStopSonicd: once a kill has been attempted the database must be
// assumed dirty, so the node stays in Killed and heal is the only way out.
//
// The state records what norma is allowed to do next; it is not by itself
// evidence about the client process. Three mechanisms keep the two from
// drifting apart:
//
//   - Syncing→Running is only taken once the client has answered over RPC,
//     so Running is never asserted without proof.
//   - A client that exits without being asked to moves the node to Killed
//     from a background watcher, so a crash is a transition rather than
//     something a later action stumbles over.
//   - Before starting a client and before healing, the container is scanned
//     for live client processes, so those two operations do not rely on the
//     recorded state being accurate.
type NodeState int

const (
	// NodeStateUninitialized is the initial state: container exists but
	// nothing has been written to the data directory yet.
	NodeStateUninitialized NodeState = iota
	// NodeStateInitializing is the transitional state entered while the
	// data directory is being populated.
	NodeStateInitializing
	// NodeStateReady means the data directory is initialized and sonicd
	// can be started.
	NodeStateReady
	// NodeStateSyncing means sonicd is running but has not yet reached
	// the network head.
	NodeStateSyncing
	// NodeStateRunning means sonicd is running and synced.
	NodeStateRunning
	// NodeStateStopping is the transitional state entered while sonicd
	// is being asked to shut down gracefully. A node stays here if the
	// shutdown does not complete, because the client may still hold the
	// data directory; ForceStopSonicd escalates out of it.
	NodeStateStopping
	// NodeStateKilled means sonicd stopped without shutting down cleanly,
	// either because it was killed or because it exited on its own, so the
	// on-disk database must be assumed dirty. See
	// OperaNode.ClientExitWasUnexpected to tell the two apart.
	NodeStateKilled
	// NodeStateHealing is the transitional state entered while
	// sonictool heal is running against a killed node.
	NodeStateHealing
)

// String returns a human-readable name for the state, used in error
// messages produced by transition.
func (s NodeState) String() string {
	switch s {
	case NodeStateUninitialized:
		return "uninitialized"
	case NodeStateInitializing:
		return "initializing"
	case NodeStateReady:
		return "ready"
	case NodeStateSyncing:
		return "syncing"
	case NodeStateRunning:
		return "running"
	case NodeStateStopping:
		return "stopping"
	case NodeStateKilled:
		return "killed"
	case NodeStateHealing:
		return "healing"
	default:
		return "unknown"
	}
}
