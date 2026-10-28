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
	"errors"
	"strings"
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

	net.EXPECT().RemoveNode(gomock.Any(), node).Return(nil)
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

	net.EXPECT().RemoveNode(gomock.Any(), node0).Return(nil)
	node0.EXPECT().Stop(gomock.Any()).Return(nil)
	node0.EXPECT().Cleanup(gomock.Any()).Return(nil)

	net.EXPECT().RemoveNode(gomock.Any(), node1).Return(nil)
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

// Only labels startNode gives its instances are stopped; other nodes sharing
// the prefix are unrelated, even when their suffix parses as a number.
func TestStopNode_IgnoresNodesThatAreNotItsInstances(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)
	node := driver.NewMockNode(ctrl)

	env := steps.NewEnv(net, nil, nil)
	env.Nodes["validator-0"] = node
	unrelated := []string{"validator-extra", "validator--1", "validator-01"}
	for _, label := range unrelated {
		env.Nodes[label] = driver.NewMockNode(ctrl)
	}

	net.EXPECT().RemoveNode(gomock.Any(), node).Return(nil)
	node.EXPECT().Stop(gomock.Any()).Return(nil)
	node.EXPECT().Cleanup(gomock.Any()).Return(nil)

	step := &StopNode{steps.Node{Node: "validator"}}
	if err := step.Run(t.Context(), env); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := env.Nodes["validator-0"]; ok {
		t.Fatalf("expected validator-0 to be removed")
	}
	for _, label := range unrelated {
		if _, ok := env.Nodes[label]; !ok {
			t.Errorf("expected %s to remain", label)
		}
	}
}

// A node the network failed to unpeer is out of it all the same; left running,
// nothing would ever stop it, as the network no longer lists it.
func TestStopNode_StopsTheNode_WhenRemovingItFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)
	node := driver.NewMockNode(ctrl)

	env := steps.NewEnv(net, nil, nil)
	env.Nodes["observer"] = node

	net.EXPECT().RemoveNode(gomock.Any(), node).Return(errors.New("peer unreachable"))
	node.EXPECT().Stop(gomock.Any()).Return(nil)
	node.EXPECT().Cleanup(gomock.Any()).Return(nil)

	err := (&StopNode{steps.Node{Node: "observer"}}).Run(t.Context(), env)
	if err == nil || !strings.Contains(err.Error(), "peer unreachable") {
		t.Fatalf("expected the removal error, got %v", err)
	}
}
