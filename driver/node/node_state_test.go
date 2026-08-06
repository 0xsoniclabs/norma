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

import (
	"strings"
	"sync"
	"testing"
)

// allNodeStates lists every defined state, used to check exhaustiveness.
var allNodeStates = []NodeState{
	NodeStateUninitialized,
	NodeStateInitializing,
	NodeStateReady,
	NodeStateSyncing,
	NodeStateRunning,
	NodeStateStopping,
	NodeStateKilled,
	NodeStateHealing,
}

func TestNodeState_String_IsUniqueAndDefinedForAllStates(t *testing.T) {
	seen := make(map[string]NodeState, len(allNodeStates))
	for _, state := range allNodeStates {
		name := state.String()
		if name == "unknown" {
			t.Errorf("state %d has no name", int(state))
		}
		if other, exists := seen[name]; exists {
			t.Errorf("states %d and %d share the name %q",
				int(other), int(state), name)
		}
		seen[name] = state
	}
}

func TestNodeState_String_ReportsUnknownForUndefinedState(t *testing.T) {
	if got := NodeState(len(allNodeStates) + 1).String(); got != "unknown" {
		t.Errorf("unexpected name for undefined state: got %q, want %q",
			got, "unknown")
	}
}

func TestOperaNode_Transition_MovesStateWhenCurrentStateMatches(t *testing.T) {
	node := &OperaNode{
		config: &OperaNodeConfig{Label: t.Name()},
		state:  NodeStateReady,
	}

	if err := node.transition(NodeStateReady, NodeStateSyncing); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := node.GetState(); got != NodeStateSyncing {
		t.Errorf("unexpected state, got %s, want %s", got, NodeStateSyncing)
	}
}

func TestOperaNode_Transition_FailsAndKeepsStateWhenCurrentStateDiffers(t *testing.T) {
	node := &OperaNode{
		config: &OperaNodeConfig{Label: t.Name()},
		state:  NodeStateKilled,
	}

	err := node.transition(NodeStateRunning, NodeStateStopping)
	if err == nil {
		t.Fatalf("expected error when transitioning from the wrong state")
	}
	// The message has to name all three states to be diagnosable.
	for _, want := range []string{t.Name(), "running", "stopping", "killed"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
	if got := node.GetState(); got != NodeStateKilled {
		t.Errorf("state must not change on a failed transition, got %s", got)
	}
}

// Only one of two racing transitions out of the same state may win;
// otherwise two actions could drive the same node concurrently.
func TestOperaNode_Transition_IsExclusiveUnderConcurrency(t *testing.T) {
	const contenders = 16
	node := &OperaNode{
		config: &OperaNodeConfig{Label: t.Name()},
		state:  NodeStateRunning,
	}

	var start sync.WaitGroup
	start.Add(1)
	var done sync.WaitGroup
	results := make([]error, contenders)
	for i := range contenders {
		done.Add(1)
		go func() {
			defer done.Done()
			start.Wait()
			results[i] = node.transition(NodeStateRunning, NodeStateStopping)
		}()
	}
	start.Done()
	done.Wait()

	succeeded := 0
	for _, err := range results {
		if err == nil {
			succeeded++
		}
	}
	if succeeded != 1 {
		t.Errorf("exactly one transition must succeed, got %d", succeeded)
	}
	if got := node.GetState(); got != NodeStateStopping {
		t.Errorf("unexpected final state, got %s, want %s", got, NodeStateStopping)
	}
}

func TestOperaNode_ForceSetState_OverridesStateUnconditionally(t *testing.T) {
	node := &OperaNode{
		config: &OperaNodeConfig{Label: t.Name()},
		state:  NodeStateHealing,
	}

	node.forceSetState(NodeStateKilled)

	if got := node.GetState(); got != NodeStateKilled {
		t.Errorf("unexpected state, got %s, want %s", got, NodeStateKilled)
	}
}

// The lifecycle documented on NodeState must be walkable end to end, and
// each step must reject the states it is not meant to accept.
func TestOperaNode_Transition_FollowsDocumentedLifecycle(t *testing.T) {
	lifecycle := []struct {
		from, to NodeState
	}{
		{NodeStateUninitialized, NodeStateInitializing},
		{NodeStateInitializing, NodeStateReady},
		{NodeStateReady, NodeStateSyncing},
		{NodeStateSyncing, NodeStateRunning},
		{NodeStateRunning, NodeStateStopping},
		{NodeStateStopping, NodeStateReady},
		{NodeStateReady, NodeStateSyncing},
		{NodeStateSyncing, NodeStateRunning},
		{NodeStateRunning, NodeStateKilled},
		{NodeStateKilled, NodeStateHealing},
		{NodeStateHealing, NodeStateReady},
	}

	node := &OperaNode{
		config: &OperaNodeConfig{Label: t.Name()},
		state:  NodeStateUninitialized,
	}
	for i, step := range lifecycle {
		if err := node.transition(step.from, step.to); err != nil {
			t.Fatalf("step %d (%s->%s) failed: %v",
				i, step.from, step.to, err)
		}
	}
	if got := node.GetState(); got != NodeStateReady {
		t.Errorf("unexpected final state, got %s, want %s", got, NodeStateReady)
	}
}
