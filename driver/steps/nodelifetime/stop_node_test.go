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

package nodelifetime

import (
	"testing"

	"github.com/0xsoniclabs/norma/driver/steps"

	"github.com/0xsoniclabs/norma/driver"
	"go.uber.org/mock/gomock"
)

func TestStopNode_SingleInstanceSuffix(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)
	node := driver.NewMockNode(ctrl)

	env := steps.NewEnv(net, nil, nil)
	env.Nodes["validator-A-0"] = node

	net.EXPECT().RemoveNode(node).Return(nil)
	node.EXPECT().Stop(gomock.Any()).Return(nil)
	node.EXPECT().Cleanup(gomock.Any()).Return(nil)

	step := &StopNode{steps.Node{Node: "validator-A"}}
	if err := step.Run(t.Context(), env); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := env.Nodes["validator-A-0"]; ok {
		t.Fatalf("expected node validator-A-0 to be removed from state")
	}
}

func TestStopNode_MultipleInstances(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)
	node0 := driver.NewMockNode(ctrl)
	node1 := driver.NewMockNode(ctrl)
	other := driver.NewMockNode(ctrl)

	env := steps.NewEnv(net, nil, nil)
	env.Nodes["validators-0"] = node0
	env.Nodes["validators-1"] = node1
	env.Nodes["other"] = other

	net.EXPECT().RemoveNode(node0).Return(nil)
	node0.EXPECT().Stop(gomock.Any()).Return(nil)
	node0.EXPECT().Cleanup(gomock.Any()).Return(nil)

	net.EXPECT().RemoveNode(node1).Return(nil)
	node1.EXPECT().Stop(gomock.Any()).Return(nil)
	node1.EXPECT().Cleanup(gomock.Any()).Return(nil)

	step := &StopNode{steps.Node{Node: "validators"}}
	if err := step.Run(t.Context(), env); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := env.Nodes["validators-0"]; ok {
		t.Fatalf("expected node validators-0 to be removed from state")
	}
	if _, ok := env.Nodes["validators-1"]; ok {
		t.Fatalf("expected node validators-1 to be removed from state")
	}
	if _, ok := env.Nodes["other"]; !ok {
		t.Fatalf("expected unrelated node to remain in state")
	}
}

func TestStopNode_IgnoresNonNumericSuffix(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)
	node := driver.NewMockNode(ctrl)
	unrelated := driver.NewMockNode(ctrl)

	env := steps.NewEnv(net, nil, nil)
	env.Nodes["validator-0"] = node
	env.Nodes["validator-extra"] = unrelated

	net.EXPECT().RemoveNode(node).Return(nil)
	node.EXPECT().Stop(gomock.Any()).Return(nil)
	node.EXPECT().Cleanup(gomock.Any()).Return(nil)

	step := &StopNode{steps.Node{Node: "validator"}}
	if err := step.Run(t.Context(), env); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := env.Nodes["validator-0"]; ok {
		t.Fatalf("expected validator-0 to be removed")
	}
	if _, ok := env.Nodes["validator-extra"]; !ok {
		t.Fatalf("expected validator-extra to remain (non-numeric suffix)")
	}
}
