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

import "slices"

// Transition is what an action on a node requires and achieves: the states it
// may start from, and the state it leaves the node in when it succeeds.
type Transition struct {
	From []NodeState
	To   NodeState
}

// Accepts reports whether the action may start from state.
func (t Transition) Accepts(state NodeState) bool {
	return slices.Contains(t.From, state)
}

// The transitions of the actions a scenario drives a node's client through.
// The node's methods refuse to start outside From; a scenario is checked
// against both From and To before it runs.
var (
	// StartClient is StartSonicd or StartSonicdAsObserver, then WaitForSync.
	StartClient = Transition{From: []NodeState{NodeStateReady}, To: NodeStateRunning}
	// StopClient is StopSonicd.
	StopClient = Transition{From: []NodeState{NodeStateRunning}, To: NodeStateReady}
	// KillClient is ForceStopSonicd, which may also escalate a graceful stop
	// that did not complete.
	KillClient = Transition{
		From: []NodeState{NodeStateRunning, NodeStateStopping},
		To:   NodeStateKilled,
	}
	// AwaitClientExit is AwaitSonicdExit. Killed is accepted because the exit
	// watcher may record the self-exit before the wait starts.
	AwaitClientExit = Transition{
		From: []NodeState{NodeStateRunning, NodeStateKilled},
		To:   NodeStateReady,
	}
	// HealDatabase is HealSonicd.
	HealDatabase = Transition{From: []NodeState{NodeStateKilled}, To: NodeStateReady}
	// Maintenance is ExportGenesis, ImportGenesis, ExportEvents, ImportEvents
	// and CheckDatabase, each of which runs sonictool on a stopped client.
	Maintenance = Transition{From: []NodeState{NodeStateReady}, To: NodeStateReady}
)
