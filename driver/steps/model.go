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

package steps

import (
	"fmt"

	"github.com/0xsoniclabs/norma/driver/node"
)

// Model is what a scenario has done to the network so far, as far as it can
// be known without running it. A dry run applies every step to a model; a real
// run does too, before running the step, so both reject the same scenarios.
type Model struct {
	// Nodes holds the nodes started and not stopped, by label.
	Nodes map[string]*ModelNode
	// Started holds every label and base name started so far, to recognise a
	// rejoin.
	Started map[string]bool
	// Validators holds every label that has been given a validator id,
	// including stopped ones, which can rejoin under it.
	Validators map[string]bool
	// Apps holds the running applications.
	Apps map[string]bool
	// Delegators holds every delegator account used so far.
	Delegators map[string]bool
	// Delegations is the stake (in S) delegated per delegator and node label.
	// Fully undelegated pairs stay at 0, as in a real run's ledger.
	Delegations map[Delegation]uint64
	// SharedFiles holds the files exported to the shared directory.
	SharedFiles map[string]bool
}

// ModelNode is a node of the model.
type ModelNode struct {
	Validator bool
	// Client is the state of the node's client: Running, Ready when it was
	// stopped, or Killed.
	Client node.NodeState
	// ExitsWhenSynced records that the client was told to stop by itself.
	ExitsWhenSynced bool
}

// Delegation identifies the stake a delegator holds on a node.
type Delegation struct {
	Delegator string
	Node      string
}

// NewModel returns the model of a network nothing has happened to yet.
func NewModel() *Model {
	return &Model{
		Nodes:       map[string]*ModelNode{},
		Started:     map[string]bool{},
		Validators:  map[string]bool{},
		Apps:        map[string]bool{},
		Delegators:  map[string]bool{},
		Delegations: map[Delegation]uint64{},
		SharedFiles: map[string]bool{},
	}
}

// Node returns the running or stopped node with the given label.
func (m *Model) Node(label string) (*ModelNode, error) {
	n, ok := m.Nodes[label]
	if !ok {
		return nil, fmt.Errorf("node %q not found in active nodes", label)
	}
	return n, nil
}

// Apply moves the client of the node with the given label through t, failing
// as the node itself would when its client is not in a state t starts from.
// action describes t in the error, e.g. "heal".
func (m *Model) Apply(label string, t node.Transition, action string) error {
	n, err := m.Node(label)
	if err != nil {
		return err
	}
	if !t.Accepts(n.Client) {
		return fmt.Errorf("cannot %s node %q while its client is %s", action, label, clientState(n.Client))
	}
	n.Client = t.To
	return nil
}

// Validator resolves a node name to the label of a validator, as the staking
// steps do.
func (m *Model) Validator(name string) (string, error) {
	label, err := ResolveLabel(m.Nodes, name)
	if err != nil {
		return "", err
	}
	if !m.Nodes[label].Validator {
		return "", fmt.Errorf("node %q is not a validator", name)
	}
	return label, nil
}

func clientState(state node.NodeState) string {
	switch state {
	case node.NodeStateRunning:
		return "running"
	case node.NodeStateReady:
		return "stopped"
	case node.NodeStateKilled:
		return "killed"
	}
	return state.String()
}

// ResolveLabel maps a node name used by a step to the label of the node it
// means: the name itself, or the only instance started under it.
func ResolveLabel[V any](nodes map[string]V, name string) (string, error) {
	if _, ok := nodes[name]; ok {
		return name, nil
	}
	first := InstanceLabel(name, 0)
	if _, ok := nodes[first]; !ok {
		return "", fmt.Errorf("node %q not found in active nodes", name)
	}
	instances := 0
	for label := range nodes {
		if IsInstanceLabel(label, name) {
			instances++
		}
	}
	if instances > 1 {
		return "", fmt.Errorf(
			"node %q has multiple instances; use an explicit instance name (e.g. %q)", name, first)
	}
	return first, nil
}
