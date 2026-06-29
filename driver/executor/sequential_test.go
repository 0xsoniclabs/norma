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

package executor

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/checking"
	"github.com/0xsoniclabs/norma/driver/parser"
	"go.uber.org/mock/gomock"
)

func TestSequential_EmptyScenario(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)

	scenario := parser.SequentialScenario{
		Name:  "Empty",
		Steps: []parser.Step{},
	}

	if err := runSequential(t.Context(), net, &scenario, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSequential_StartAndStopNode(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)
	registry := NewMockvalidatorRegistry(ctrl)
	node := driver.NewMockNode(ctrl)

	// DialRandomRpc returns error so sync wait is skipped.
	net.EXPECT().DialRandomRpc().Return(nil, fmt.Errorf("no nodes")).AnyTimes()
	node.EXPECT().GetLabel().Return("validator-A").AnyTimes()
	node.EXPECT().DialRpc(gomock.Any()).Return(nil, fmt.Errorf("not ready")).AnyTimes()

	validatorId := 2
	gomock.InOrder(
		registry.EXPECT().registerNewValidator(gomock.Any()).Return(validatorId, nil),
		net.EXPECT().CreateNode(gomock.Any()).Return(node, nil),
		node.EXPECT().GetValidatorId().Return(&validatorId),
		registry.EXPECT().unregisterValidator(validatorId).Return(nil),
		net.EXPECT().RemoveNode(node).Return(nil),
		node.EXPECT().Stop(gomock.Any()).Return(nil),
		node.EXPECT().Cleanup(gomock.Any()).Return(nil),
	)

	scenario := parser.SequentialScenario{
		Name: "Start Stop",
		Steps: []parser.Step{
			{
				Function:   parser.FuncStartNode,
				Identifier: "validator-A",
				NodeType:   "validator",
			},
			{
				Function:   parser.FuncUndelegate,
				Identifier: "validator-A",
			},
			{
				Function:   parser.FuncStopNode,
				Identifier: "validator-A",
			},
		},
	}

	if err := runSequential(t.Context(), net, &scenario, nil, registry); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSequential_StopNodeWithoutUndelegate(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)
	registry := NewMockvalidatorRegistry(ctrl)
	node := driver.NewMockNode(ctrl)

	net.EXPECT().DialRandomRpc().Return(nil, fmt.Errorf("no nodes")).AnyTimes()
	node.EXPECT().GetLabel().Return("validator-A").AnyTimes()
	node.EXPECT().DialRpc(gomock.Any()).Return(nil, fmt.Errorf("not ready")).AnyTimes()

	validatorId := 2
	gomock.InOrder(
		registry.EXPECT().registerNewValidator(gomock.Any()).Return(validatorId, nil),
		net.EXPECT().CreateNode(gomock.Any()).Return(node, nil),
		// No unregister call expected
		net.EXPECT().RemoveNode(node).Return(nil),
		node.EXPECT().Stop(gomock.Any()).Return(nil),
		node.EXPECT().Cleanup(gomock.Any()).Return(nil),
	)

	scenario := parser.SequentialScenario{
		Name: "Leave",
		Steps: []parser.Step{
			{
				Function:   parser.FuncStartNode,
				Identifier: "validator-A",
				NodeType:   "validator",
			},
			{
				// Stop without undelegate
				Function:   parser.FuncStopNode,
				Identifier: "validator-A",
			},
		},
	}

	if err := runSequential(t.Context(), net, &scenario, nil, registry); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSequential_RejoinNode(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)
	registry := NewMockvalidatorRegistry(ctrl)
	node1 := driver.NewMockNode(ctrl)
	node2 := driver.NewMockNode(ctrl)

	net.EXPECT().DialRandomRpc().Return(nil, fmt.Errorf("no nodes")).AnyTimes()
	node1.EXPECT().GetLabel().Return("validator-A").AnyTimes()
	node1.EXPECT().DialRpc(gomock.Any()).Return(nil, fmt.Errorf("not ready")).AnyTimes()
	node2.EXPECT().GetLabel().Return("validator-A").AnyTimes()
	node2.EXPECT().DialRpc(gomock.Any()).Return(nil, fmt.Errorf("not ready")).AnyTimes()

	validatorId := 2
	gomock.InOrder(
		// First start: registers as new validator
		registry.EXPECT().registerNewValidator(gomock.Any()).Return(validatorId, nil),
		net.EXPECT().CreateNode(gomock.Any()).Do(func(config *driver.NodeConfig) {
			if config.ValidatorId == nil || *config.ValidatorId != validatorId {
				t.Errorf("first start: expected ValidatorId=%d, got %v", validatorId, config.ValidatorId)
			}
		}).Return(node1, nil),
		// Stop without undelegate
		net.EXPECT().RemoveNode(node1).Return(nil),
		node1.EXPECT().Stop(gomock.Any()).Return(nil),
		node1.EXPECT().Cleanup(gomock.Any()).Return(nil),
		// Rejoin: no registration, but validator ID is preserved
		net.EXPECT().CreateNode(gomock.Any()).Do(func(config *driver.NodeConfig) {
			if config.ValidatorId == nil || *config.ValidatorId != validatorId {
				t.Errorf("rejoin: expected ValidatorId=%d, got %v", validatorId, config.ValidatorId)
			}
		}).Return(node2, nil),
	)

	scenario := parser.SequentialScenario{
		Name: "Rejoin",
		Steps: []parser.Step{
			{
				Function:   parser.FuncStartNode,
				Identifier: "validator-A",
				NodeType:   "validator",
			},
			{
				Function:   parser.FuncStopNode,
				Identifier: "validator-A",
			},
			{
				Function:   parser.FuncStartNode,
				Identifier: "validator-A",
				NodeType:   "validator",
			},
		},
	}

	if err := runSequential(t.Context(), net, &scenario, nil, registry); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSequential_RunAndStopApp(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)
	net.EXPECT().DialRandomRpc().Return(nil, fmt.Errorf("no nodes")).AnyTimes()
	app := driver.NewMockApplication(ctrl)

	rate := float32(10)

	gomock.InOrder(
		net.EXPECT().CreateApplication(gomock.Any(), gomock.Any()).Return(app, nil),
		app.EXPECT().Start(gomock.Any()).Return(nil),
		app.EXPECT().Stop().Return(nil),
	)

	scenario := parser.SequentialScenario{
		Name: "App",
		Steps: []parser.Step{
			{
				Function:   parser.FuncRunApp,
				Identifier: "load",
				AppType:    "counter",
				Users:      New(50),
				Rate:       &parser.Rate{Constant: &rate},
			},
			{
				Function:   parser.FuncStopApp,
				Identifier: "load",
			},
		},
	}

	if err := runSequential(t.Context(), net, &scenario, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSequential_UpdateRules(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)
	net.EXPECT().DialRandomRpc().Return(nil, fmt.Errorf("no nodes")).AnyTimes()

	net.EXPECT().ApplyNetworkRules(driver.NetworkRules(map[string]string{
		"MIN_BASE_FEE": "3000000000",
	})).Return(nil)

	scenario := parser.SequentialScenario{
		Name: "Rules",
		Steps: []parser.Step{
			{
				Function: parser.FuncUpdateRules,
				Rules:    map[string]string{"MIN_BASE_FEE": "3000000000"},
			},
		},
	}

	if err := runSequential(t.Context(), net, &scenario, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSequential_AdvanceEpoch(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)

	net.EXPECT().AdvanceEpoch(1).Return(nil)
	// DialRandomRpc returns error so waitForBlockProduction is skipped.
	net.EXPECT().DialRandomRpc().Return(nil, fmt.Errorf("no nodes")).AnyTimes()

	scenario := parser.SequentialScenario{
		Name: "Epoch",
		Steps: []parser.Step{
			{Function: parser.FuncAdvanceEpoch},
		},
	}

	if err := runSequential(t.Context(), net, &scenario, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSequential_Check(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)
	net.EXPECT().DialRandomRpc().Return(nil, fmt.Errorf("no nodes")).AnyTimes()
	checker := checking.NewMockChecker(ctrl)

	checker.EXPECT().Check(gomock.Any()).Return(nil)

	checks := checking.Checks{"blocks_rolling": checker}

	scenario := parser.SequentialScenario{
		Name: "Check",
		Steps: []parser.Step{
			{Function: parser.FuncCheckBlocksProduced},
		},
	}

	if err := runSequential(t.Context(), net, &scenario, checks, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSequential_ContextCancellation(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)

	ctx, cancel := context.WithCancel(t.Context())
	cancel() // Cancel immediately.

	scenario := parser.SequentialScenario{
		Name: "Cancelled",
		Steps: []parser.Step{
			{Function: parser.FuncAdvanceEpoch},
		},
	}

	err := runSequential(ctx, net, &scenario, nil, nil)
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
}

func TestSequential_MultiInstanceNode(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)
	registry := NewMockvalidatorRegistry(ctrl)
	node1 := driver.NewMockNode(ctrl)
	node2 := driver.NewMockNode(ctrl)

	net.EXPECT().DialRandomRpc().Return(nil, fmt.Errorf("no nodes")).AnyTimes()
	node1.EXPECT().GetLabel().Return("validators-0").AnyTimes()
	node1.EXPECT().DialRpc(gomock.Any()).Return(nil, fmt.Errorf("not ready")).AnyTimes()
	node2.EXPECT().GetLabel().Return("validators-1").AnyTimes()
	node2.EXPECT().DialRpc(gomock.Any()).Return(nil, fmt.Errorf("not ready")).AnyTimes()

	ids := make(chan int, 2)
	ids <- 2
	ids <- 3
	registry.EXPECT().registerNewValidator(gomock.Any()).DoAndReturn(func(stake uint64) (int, error) {
		return <-ids, nil
	}).Times(2)

	net.EXPECT().CreateNode(gomock.Any()).DoAndReturn(func(config *driver.NodeConfig) (driver.Node, error) {
		switch config.Name {
		case "validators-0":
			return node1, nil
		case "validators-1":
			return node2, nil
		default:
			return nil, fmt.Errorf("unexpected node name %q", config.Name)
		}
	}).Times(2)

	instances := 2
	scenario := parser.SequentialScenario{
		Name: "Multi",
		Steps: []parser.Step{
			{
				Function:   parser.FuncStartNode,
				Identifier: "validators",
				NodeType:   "validator",
				Instances:  &instances,
			},
		},
	}

	if err := runSequential(t.Context(), net, &scenario, nil, registry); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExecStopNode_SingleInstanceSuffix(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)
	node := driver.NewMockNode(ctrl)

	state := &sequentialState{
		nodes: map[string]driver.Node{
			"validator-A-0": node,
		},
	}

	step := &parser.Step{
		Function:   parser.FuncStopNode,
		Identifier: "validator-A",
	}

	net.EXPECT().RemoveNode(node).Return(nil)
	node.EXPECT().Stop(gomock.Any()).Return(nil)
	node.EXPECT().Cleanup(gomock.Any()).Return(nil)

	if err := execStopNode(t.Context(), step, net, state); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := state.nodes["validator-A-0"]; ok {
		t.Fatalf("expected node validator-A-0 to be removed from state")
	}
}

func TestExecStopNode_MultipleInstances(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)
	node0 := driver.NewMockNode(ctrl)
	node1 := driver.NewMockNode(ctrl)
	other := driver.NewMockNode(ctrl)

	state := &sequentialState{
		nodes: map[string]driver.Node{
			"validators-0": node0,
			"validators-1": node1,
			"other":        other,
		},
	}

	step := &parser.Step{
		Function:   parser.FuncStopNode,
		Identifier: "validators",
	}

	net.EXPECT().RemoveNode(node0).Return(nil)
	node0.EXPECT().Stop(gomock.Any()).Return(nil)
	node0.EXPECT().Cleanup(gomock.Any()).Return(nil)

	net.EXPECT().RemoveNode(node1).Return(nil)
	node1.EXPECT().Stop(gomock.Any()).Return(nil)
	node1.EXPECT().Cleanup(gomock.Any()).Return(nil)

	if err := execStopNode(t.Context(), step, net, state); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := state.nodes["validators-0"]; ok {
		t.Fatalf("expected node validators-0 to be removed from state")
	}
	if _, ok := state.nodes["validators-1"]; ok {
		t.Fatalf("expected node validators-1 to be removed from state")
	}
	if _, ok := state.nodes["other"]; !ok {
		t.Fatalf("expected unrelated node to remain in state")
	}
}

func TestSequential_RunAndCaptureEventExecution_CapturesAllSteps(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)

	scenario := parser.SequentialScenario{
		Name: "Capture",
		Steps: []parser.Step{
			{Function: parser.FuncWaitFor, Duration: time.Millisecond},
			{Function: parser.FuncWaitFor, Duration: 2 * time.Millisecond},
		},
	}

	executions, err := RunSequentialAndCaptureEventExecution(
		t.Context(),
		net,
		&scenario,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got, want := len(executions), len(scenario.Steps); got != want {
		t.Fatalf("unexpected number of captured steps: got %d, want %d", got, want)
	}

	for i, execution := range executions {
		if !execution.Start.Before(execution.End) && !execution.Start.Equal(execution.End) {
			t.Fatalf("step %d has invalid timestamps: start=%v end=%v", i+1, execution.Start, execution.End)
		}
		if !strings.Contains(execution.Name, string(parser.FuncWaitFor)) {
			t.Fatalf("step %d captured unexpected name: %q", i+1, execution.Name)
		}
	}
}
