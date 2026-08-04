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
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/checking"
	"github.com/0xsoniclabs/norma/driver/parser"
	"github.com/0xsoniclabs/norma/genesis"
	"go.uber.org/mock/gomock"
)

func TestRun_EmptyScenario(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)

	scenario := parser.Scenario{
		Name:        "Empty",
		Description: "Test scenario.",
		Steps:       []parser.Step{},
	}

	if err := run(t.Context(), net, &scenario, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRun_StartAndStopNode(t *testing.T) {
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
		registry.EXPECT().registerNewValidator(gomock.Any(), uint64(0)).Return(validatorId, nil),
		net.EXPECT().CreateNode(gomock.Any()).Return(node, nil),
		registry.EXPECT().ensureValidatorsActive(gomock.Any(), []int{validatorId}).Return(nil),
		node.EXPECT().GetValidatorId().Return(&validatorId),
		registry.EXPECT().unregisterValidator(gomock.Any(), validatorId, gomock.Any()).Return(nil),
		net.EXPECT().RemoveNode(node).Return(nil),
		node.EXPECT().Stop(gomock.Any()).Return(nil),
		node.EXPECT().Cleanup(gomock.Any()).Return(nil),
	)

	scenario := parser.Scenario{
		Name:        "Start Stop",
		Description: "Test scenario.",
		Steps: []parser.Step{
			{
				Function:   parser.FuncStartNode,
				Identifier: "validator-A",
				NodeType:   "validator",
			},
			{
				Function:          parser.FuncUndelegate,
				UndelegateTargets: []parser.UndelegateTarget{{Node: "validator-A"}},
			},
			{
				Function:   parser.FuncStopNode,
				Identifier: "validator-A",
			},
		},
	}

	if err := run(t.Context(), net, &scenario, nil, registry); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRun_StartNode_ForwardsCustomStake(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)
	registry := NewMockvalidatorRegistry(ctrl)
	node := driver.NewMockNode(ctrl)

	net.EXPECT().DialRandomRpc().
		Return(nil, fmt.Errorf("no nodes")).AnyTimes()
	node.EXPECT().GetLabel().Return("heavy").AnyTimes()
	node.EXPECT().DialRpc(gomock.Any()).
		Return(nil, fmt.Errorf("not ready")).AnyTimes()

	customStake := uint64(10_000_000)
	validatorId := 2

	gomock.InOrder(
		registry.EXPECT().
			registerNewValidator(gomock.Any(), customStake).
			Return(validatorId, nil),
		net.EXPECT().CreateNode(gomock.Any()).Return(node, nil),
		registry.EXPECT().
			ensureValidatorsActive(gomock.Any(), []int{validatorId}).Return(nil),
	)

	stake := customStake
	scenario := parser.Scenario{
		Name:             "Custom Stake",
		Description:      "Test scenario.",
		DisableEndChecks: true,
		Steps: []parser.Step{
			{
				Function:   parser.FuncStartNode,
				Identifier: "heavy",
				NodeType:   "validator",
				Stake:      &stake,
			},
		},
	}

	if err := run(
		t.Context(), net, &scenario, nil, registry,
	); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRun_UndelegateSingleInstanceWithSuffix(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)
	registry := NewMockvalidatorRegistry(ctrl)
	node := driver.NewMockNode(ctrl)

	net.EXPECT().DialRandomRpc().
		Return(nil, fmt.Errorf("no nodes")).AnyTimes()
	node.EXPECT().GetLabel().Return("heavy-0").AnyTimes()
	node.EXPECT().DialRpc(gomock.Any()).
		Return(nil, fmt.Errorf("not ready")).AnyTimes()

	validatorId := 2
	gomock.InOrder(
		registry.EXPECT().registerNewValidator(gomock.Any(), uint64(0)).
			Return(validatorId, nil),
		net.EXPECT().CreateNode(gomock.Any()).Return(node, nil),
		registry.EXPECT().
			ensureValidatorsActive(gomock.Any(), []int{validatorId}).Return(nil),
		node.EXPECT().GetValidatorId().Return(&validatorId),
		registry.EXPECT().
			unregisterValidator(gomock.Any(), validatorId, uint64(0)).Return(nil),
	)

	instances := 1
	scenario := parser.Scenario{
		Name:             "Undelegate Single Instance With Suffix",
		Description:      "Test scenario.",
		DisableEndChecks: true,
		Steps: []parser.Step{
			{
				Function:   parser.FuncStartNode,
				Identifier: "heavy",
				NodeType:   "validator",
				Instances:  &instances,
			},
			{
				Function:          parser.FuncUndelegate,
				UndelegateTargets: []parser.UndelegateTarget{{Node: "heavy"}},
			},
		},
	}

	if err := run(
		t.Context(), net, &scenario, nil, registry,
	); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRun_UndelegateMultiInstance(t *testing.T) {
	cases := map[string]struct {
		target    string
		expectErr bool
	}{
		"base name returns error": {
			target:    "validators",
			expectErr: true,
		},
		"explicit instance name succeeds": {
			target:    "validators-0",
			expectErr: false,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			net := driver.NewMockNetwork(ctrl)
			registry := NewMockvalidatorRegistry(ctrl)
			node0 := driver.NewMockNode(ctrl)
			node1 := driver.NewMockNode(ctrl)

			net.EXPECT().DialRandomRpc().Return(nil, fmt.Errorf("no nodes")).AnyTimes()
			node0.EXPECT().GetLabel().Return("validators-0").AnyTimes()
			node0.EXPECT().DialRpc(gomock.Any()).Return(nil, fmt.Errorf("not ready")).AnyTimes()
			node1.EXPECT().GetLabel().Return("validators-1").AnyTimes()
			node1.EXPECT().DialRpc(gomock.Any()).Return(nil, fmt.Errorf("not ready")).AnyTimes()

			instances := 2
			registry.EXPECT().registerNewValidator(gomock.Any(), gomock.Any()).Return(2, nil)
			registry.EXPECT().registerNewValidator(gomock.Any(), gomock.Any()).Return(3, nil)
			net.EXPECT().CreateNode(gomock.Any()).DoAndReturn(func(config *driver.NodeConfig) (driver.Node, error) {
				if config.Name == "validators-0" {
					return node0, nil
				}
				return node1, nil
			}).Times(2)
			// Both instances are admitted to the validator set by a single seal.
			registry.EXPECT().ensureValidatorsActive(gomock.Any(), []int{2, 3}).Return(nil)

			if !tc.expectErr {
				validatorId0 := 2
				node0.EXPECT().GetValidatorId().Return(&validatorId0)
				registry.EXPECT().unregisterValidator(gomock.Any(), validatorId0, uint64(0)).Return(nil)
			}

			scenario := parser.Scenario{
				Name:             "Undelegate Multi Instance",
				Description:      "Test scenario.",
				DisableEndChecks: true,
				Steps: []parser.Step{
					{
						Function:   parser.FuncStartNode,
						Identifier: "validators",
						NodeType:   "validator",
						Instances:  &instances,
					},
					{
						Function:          parser.FuncUndelegate,
						UndelegateTargets: []parser.UndelegateTarget{{Node: tc.target}},
					},
				},
			}

			err := run(t.Context(), net, &scenario, nil, registry)
			if tc.expectErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tc.expectErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestRun_StopNodeWithoutUndelegate(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)
	registry := NewMockvalidatorRegistry(ctrl)
	node := driver.NewMockNode(ctrl)

	net.EXPECT().DialRandomRpc().Return(nil, fmt.Errorf("no nodes")).AnyTimes()
	node.EXPECT().GetLabel().Return("validator-A").AnyTimes()
	node.EXPECT().DialRpc(gomock.Any()).Return(nil, fmt.Errorf("not ready")).AnyTimes()

	validatorId := 2
	gomock.InOrder(
		registry.EXPECT().registerNewValidator(gomock.Any(), gomock.Any()).Return(validatorId, nil),
		net.EXPECT().CreateNode(gomock.Any()).Return(node, nil),
		registry.EXPECT().ensureValidatorsActive(gomock.Any(), []int{validatorId}).Return(nil),
		// No unregister call expected
		net.EXPECT().RemoveNode(node).Return(nil),
		node.EXPECT().Stop(gomock.Any()).Return(nil),
		node.EXPECT().Cleanup(gomock.Any()).Return(nil),
	)

	scenario := parser.Scenario{
		Name:        "Leave",
		Description: "Test scenario.",
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

	if err := run(t.Context(), net, &scenario, nil, registry); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRun_RejoinNode(t *testing.T) {
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
		registry.EXPECT().registerNewValidator(gomock.Any(), gomock.Any()).Return(validatorId, nil),
		net.EXPECT().CreateNode(gomock.Any()).Do(func(config *driver.NodeConfig) {
			if config.ValidatorId == nil || *config.ValidatorId != validatorId {
				t.Errorf("first start: expected ValidatorId=%d, got %v", validatorId, config.ValidatorId)
			}
		}).Return(node1, nil),
		registry.EXPECT().ensureValidatorsActive(gomock.Any(), []int{validatorId}).Return(nil),
		// Stop without undelegate
		net.EXPECT().RemoveNode(node1).Return(nil),
		node1.EXPECT().Stop(gomock.Any()).Return(nil),
		node1.EXPECT().Cleanup(gomock.Any()).Return(nil),
		// Rejoin: no registration, but validator ID is preserved.
		net.EXPECT().CreateNode(gomock.Any()).Do(func(config *driver.NodeConfig) {
			if config.ValidatorId == nil || *config.ValidatorId != validatorId {
				t.Errorf("rejoin: expected ValidatorId=%d, got %v", validatorId, config.ValidatorId)
			}
		}).Return(node2, nil),
		// The preserved validator is expected to be in the set, but that is
		// checked rather than assumed: a node whose validator was undelegated
		// while it was down would otherwise rejoin as an observer.
		registry.EXPECT().ensureValidatorsActive(gomock.Any(), []int{validatorId}).Return(nil),
	)

	scenario := parser.Scenario{
		Name:        "Rejoin",
		Description: "Test scenario.",
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

	if err := run(t.Context(), net, &scenario, nil, registry); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRun_RunAndStopApp(t *testing.T) {
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

	scenario := parser.Scenario{
		Name:        "App",
		Description: "Test scenario.",
		Steps: []parser.Step{
			{
				Function:   parser.FuncRunApp,
				Identifier: "load",
				AppType:    "counter",
				Users:      new(50),
				Rate:       &parser.Rate{Constant: &rate},
			},
			{
				Function:   parser.FuncStopApp,
				Identifier: "load",
			},
		},
	}

	if err := run(t.Context(), net, &scenario, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRun_UpdateRules(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)
	net.EXPECT().DialRandomRpc().Return(nil, fmt.Errorf("no nodes")).AnyTimes()
	baseFee := genesis.BigIntValue(*big.NewInt(3000000000))

	net.EXPECT().ApplyNetworkRules(gomock.Any(), driver.NetworkRules{
		Economy: &genesis.EconomyPatch{
			MinBaseFee: &baseFee,
		},
	}).Return(nil)

	scenario := parser.Scenario{
		Name:        "Rules",
		Description: "Test scenario.",
		Steps: []parser.Step{
			{
				Function: parser.FuncUpdateRules,
				Rules: genesis.NetworkRulesPatch{
					Economy: &genesis.EconomyPatch{
						MinBaseFee: &baseFee,
					},
				},
			},
		},
	}

	if err := run(t.Context(), net, &scenario, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRun_AdvanceEpoch(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)

	net.EXPECT().AdvanceEpoch(gomock.Any(), 1).Return(nil)
	// DialRandomRpc returns error so waitForBlockProduction is skipped.
	net.EXPECT().DialRandomRpc().Return(nil, fmt.Errorf("no nodes")).AnyTimes()

	scenario := parser.Scenario{
		Name:        "Epoch",
		Description: "Test scenario.",
		Steps: []parser.Step{
			{Function: parser.FuncAdvanceEpoch},
		},
	}

	if err := run(t.Context(), net, &scenario, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRun_Check(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)
	net.EXPECT().DialRandomRpc().Return(nil, fmt.Errorf("no nodes")).AnyTimes()
	checker := checking.NewMockChecker(ctrl)

	checker.EXPECT().Check(gomock.Any()).Return(nil)

	checks := checking.Checks{"blocksRolling": checker}

	scenario := parser.Scenario{
		Name:        "Check",
		Description: "Test scenario.",
		Steps: []parser.Step{
			{
				Function: parser.FuncChecks,
				SubChecks: []parser.CheckSpec{
					{Function: parser.FuncCheckBlocksProduced},
				},
			},
		},
	}

	if err := run(t.Context(), net, &scenario, checks, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRun_ContextCancellation(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)

	ctx, cancel := context.WithCancel(t.Context())
	cancel() // Cancel immediately.

	scenario := parser.Scenario{
		Name:        "Cancelled",
		Description: "Test scenario.",
		Steps: []parser.Step{
			{Function: parser.FuncAdvanceEpoch},
		},
	}

	err := run(ctx, net, &scenario, nil, nil)
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
}

func TestRun_MultiInstanceNode(t *testing.T) {
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

	// Validators are registered sequentially, then nodes created in parallel.
	ids := make(chan int, 2)
	ids <- 2
	ids <- 3
	registry.EXPECT().registerNewValidator(gomock.Any(), uint64(0)).DoAndReturn(func(ctx context.Context, stake uint64) (int, error) {
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

	registry.EXPECT().ensureValidatorsActive(gomock.Any(), []int{2, 3}).Return(nil)

	instances := 2
	scenario := parser.Scenario{
		Name:        "Multi",
		Description: "Test scenario.",
		Steps: []parser.Step{
			{
				Function:   parser.FuncStartNode,
				Identifier: "validators",
				NodeType:   "validator",
				Instances:  &instances,
			},
		},
	}

	if err := run(t.Context(), net, &scenario, nil, registry); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExecStopNode_SingleInstanceSuffix(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)
	node := driver.NewMockNode(ctrl)

	state := &runState{
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

	state := &runState{
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

func TestExecStopNode_IgnoresNonNumericSuffix(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)
	node := driver.NewMockNode(ctrl)
	unrelated := driver.NewMockNode(ctrl)

	state := &runState{
		nodes: map[string]driver.Node{
			"validator-0":     node,
			"validator-extra": unrelated,
		},
	}

	step := &parser.Step{
		Function:   parser.FuncStopNode,
		Identifier: "validator",
	}

	net.EXPECT().RemoveNode(node).Return(nil)
	node.EXPECT().Stop(gomock.Any()).Return(nil)
	node.EXPECT().Cleanup(gomock.Any()).Return(nil)

	if err := execStopNode(t.Context(), step, net, state); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := state.nodes["validator-0"]; ok {
		t.Fatalf("expected validator-0 to be removed")
	}
	if _, ok := state.nodes["validator-extra"]; !ok {
		t.Fatalf("expected validator-extra to remain (non-numeric suffix)")
	}
}

func TestRun_RunAndCaptureEventExecution_CapturesAllSteps(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)

	scenario := parser.Scenario{
		Name:        "Capture",
		Description: "Test scenario.",
		Steps: []parser.Step{
			{Function: parser.FuncWaitFor, Duration: time.Millisecond},
			{Function: parser.FuncWaitFor, Duration: 2 * time.Millisecond},
		},
	}

	executions, err := RunAndCaptureEventExecution(
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

func TestRun_Delegate_FundsAndDelegatesForEachTarget(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)
	registry := NewMockvalidatorRegistry(ctrl)
	node := driver.NewMockNode(ctrl)

	net.EXPECT().DialRandomRpc().Return(nil, fmt.Errorf("no nodes")).AnyTimes()
	node.EXPECT().GetLabel().Return("heavy").AnyTimes()
	node.EXPECT().DialRpc(gomock.Any()).Return(nil, fmt.Errorf("not ready")).AnyTimes()

	validatorId := 2
	// startNode → registerNewValidator + CreateNode.
	registry.EXPECT().registerNewValidator(gomock.Any(), uint64(0)).Return(validatorId, nil)
	net.EXPECT().CreateNode(gomock.Any()).Return(node, nil)
	registry.EXPECT().ensureValidatorsActive(gomock.Any(), []int{validatorId}).Return(nil)
	// execDelegate calls GetValidatorId per target to resolve the node.
	node.EXPECT().GetValidatorId().Return(&validatorId).AnyTimes()

	// Expect one funding + delegate per target, in order.
	registry.EXPECT().fundDelegator(gomock.Any(), gomock.Any(), uint64(2_000_000)+delegatorGasBudget).Return(nil)
	registry.EXPECT().delegate(gomock.Any(), validatorId, uint64(2_000_000), gomock.Any()).Return(nil)

	registry.EXPECT().fundDelegator(gomock.Any(), gomock.Any(), uint64(1_000_000)+delegatorGasBudget).Return(nil)
	registry.EXPECT().delegate(gomock.Any(), validatorId, uint64(1_000_000), gomock.Any()).Return(nil)

	scenario := parser.Scenario{
		Name:             "Delegate",
		Description:      "Test scenario.",
		DisableEndChecks: true,
		Steps: []parser.Step{
			{
				Function:   parser.FuncStartNode,
				Identifier: "heavy",
				NodeType:   "validator",
			},
			{
				Function: parser.FuncDelegate,
				DelegateTargets: []parser.DelegateTarget{
					{Node: "heavy", Delegator: "alice", Stake: 2_000_000},
					{Node: "heavy", Delegator: "bob", Stake: 1_000_000},
				},
			},
		},
	}

	if err := run(
		t.Context(), net, &scenario, nil, registry,
	); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRun_UndelegateWithDelegator_UsesDelegatorKey(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)
	registry := NewMockvalidatorRegistry(ctrl)
	node := driver.NewMockNode(ctrl)

	net.EXPECT().DialRandomRpc().Return(nil, fmt.Errorf("no nodes")).AnyTimes()
	node.EXPECT().GetLabel().Return("heavy").AnyTimes()
	node.EXPECT().DialRpc(gomock.Any()).Return(nil, fmt.Errorf("not ready")).AnyTimes()

	validatorId := 2
	registry.EXPECT().registerNewValidator(gomock.Any(), uint64(0)).Return(validatorId, nil)
	net.EXPECT().CreateNode(gomock.Any()).Return(node, nil)
	registry.EXPECT().ensureValidatorsActive(gomock.Any(), []int{validatorId}).Return(nil)
	node.EXPECT().GetValidatorId().Return(&validatorId).AnyTimes()

	// First: delegate to create alice's account.
	registry.EXPECT().fundDelegator(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
	registry.EXPECT().delegate(gomock.Any(), validatorId, uint64(1_000_000), gomock.Any()).Return(nil)
	// Then: undelegate as alice → undelegateAs is called with her key.
	registry.EXPECT().undelegateAs(gomock.Any(), validatorId, uint64(400_000), gomock.Any()).Return(nil)
	// The regular self-undelegate path must NOT be called.

	partial := uint64(400_000)
	scenario := parser.Scenario{
		Name:             "Undelegate as delegator",
		Description:      "Test scenario.",
		DisableEndChecks: true,
		Steps: []parser.Step{
			{
				Function:   parser.FuncStartNode,
				Identifier: "heavy",
				NodeType:   "validator",
			},
			{
				Function: parser.FuncDelegate,
				DelegateTargets: []parser.DelegateTarget{
					{Node: "heavy", Delegator: "alice", Stake: 1_000_000},
				},
			},
			{
				Function: parser.FuncUndelegate,
				UndelegateTargets: []parser.UndelegateTarget{
					{Node: "heavy", Delegator: "alice", Stake: &partial},
				},
			},
		},
	}

	if err := run(
		t.Context(), net, &scenario, nil, registry,
	); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRun_UndelegateWithUnknownDelegator_ReturnsError(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)
	registry := NewMockvalidatorRegistry(ctrl)
	node := driver.NewMockNode(ctrl)

	net.EXPECT().DialRandomRpc().Return(nil, fmt.Errorf("no nodes")).AnyTimes()
	node.EXPECT().GetLabel().Return("heavy").AnyTimes()
	node.EXPECT().DialRpc(gomock.Any()).Return(nil, fmt.Errorf("not ready")).AnyTimes()

	validatorId := 2
	registry.EXPECT().registerNewValidator(gomock.Any(), uint64(0)).Return(validatorId, nil)
	net.EXPECT().CreateNode(gomock.Any()).Return(node, nil)
	registry.EXPECT().ensureValidatorsActive(gomock.Any(), []int{validatorId}).Return(nil)
	node.EXPECT().GetValidatorId().Return(&validatorId).AnyTimes()
	// No undelegate call should occur.

	scenario := parser.Scenario{
		Name:             "Unknown delegator",
		Description:      "Test scenario.",
		DisableEndChecks: true,
		Steps: []parser.Step{
			{
				Function:   parser.FuncStartNode,
				Identifier: "heavy",
				NodeType:   "validator",
			},
			{
				Function: parser.FuncUndelegate,
				UndelegateTargets: []parser.UndelegateTarget{
					{Node: "heavy", Delegator: "ghost"},
				},
			},
		},
	}

	err := run(t.Context(), net, &scenario, nil, registry)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "never used") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestRun_VerifyStakes_PassesWhenAllStakesMatch(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)
	registry := NewMockvalidatorRegistry(ctrl)
	node := driver.NewMockNode(ctrl)

	net.EXPECT().DialRandomRpc().Return(nil, fmt.Errorf("no nodes")).AnyTimes()
	node.EXPECT().GetLabel().Return("heavy").AnyTimes()
	node.EXPECT().DialRpc(gomock.Any()).Return(nil, fmt.Errorf("not ready")).AnyTimes()

	validatorId := 2
	registry.EXPECT().registerNewValidator(gomock.Any(), uint64(0)).Return(validatorId, nil)
	net.EXPECT().CreateNode(gomock.Any()).Return(node, nil)
	registry.EXPECT().ensureValidatorsActive(gomock.Any(), []int{validatorId}).Return(nil)
	node.EXPECT().GetValidatorId().Return(&validatorId).AnyTimes()

	registry.EXPECT().fundDelegator(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
	registry.EXPECT().delegate(gomock.Any(), validatorId, uint64(500_000), gomock.Any()).Return(nil)

	// verifyStakes should query alice's stake and see the expected value.
	registry.EXPECT().
		getDelegatorStake(gomock.Any(), validatorId).
		Return(uint64(500_000), nil)

	scenario := parser.Scenario{
		Name:             "Verify stakes",
		Description:      "Test scenario.",
		DisableEndChecks: true,
		Steps: []parser.Step{
			{
				Function:   parser.FuncStartNode,
				Identifier: "heavy",
				NodeType:   "validator",
			},
			{
				Function: parser.FuncDelegate,
				DelegateTargets: []parser.DelegateTarget{
					{Node: "heavy", Delegator: "alice", Stake: 500_000},
				},
			},
			{Function: parser.FuncVerifyStakes},
		},
	}

	if err := run(
		t.Context(), net, &scenario, nil, registry,
	); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRun_VerifyStakes_FailsOnMismatch(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)
	registry := NewMockvalidatorRegistry(ctrl)
	node := driver.NewMockNode(ctrl)

	net.EXPECT().DialRandomRpc().Return(nil, fmt.Errorf("no nodes")).AnyTimes()
	node.EXPECT().GetLabel().Return("heavy").AnyTimes()
	node.EXPECT().DialRpc(gomock.Any()).Return(nil, fmt.Errorf("not ready")).AnyTimes()

	validatorId := 2
	registry.EXPECT().registerNewValidator(gomock.Any(), uint64(0)).Return(validatorId, nil)
	net.EXPECT().CreateNode(gomock.Any()).Return(node, nil)
	registry.EXPECT().ensureValidatorsActive(gomock.Any(), []int{validatorId}).Return(nil)
	node.EXPECT().GetValidatorId().Return(&validatorId).AnyTimes()

	registry.EXPECT().fundDelegator(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
	registry.EXPECT().delegate(gomock.Any(), validatorId, uint64(500_000), gomock.Any()).Return(nil)

	// Chain reports a different value than expected.
	registry.EXPECT().
		getDelegatorStake(gomock.Any(), validatorId).
		Return(uint64(499_000), nil)

	scenario := parser.Scenario{
		Name:             "Verify stakes mismatch",
		Description:      "Test scenario.",
		DisableEndChecks: true,
		Steps: []parser.Step{
			{
				Function:   parser.FuncStartNode,
				Identifier: "heavy",
				NodeType:   "validator",
			},
			{
				Function: parser.FuncDelegate,
				DelegateTargets: []parser.DelegateTarget{
					{Node: "heavy", Delegator: "alice", Stake: 500_000},
				},
			},
			{Function: parser.FuncVerifyStakes},
		},
	}

	err := run(t.Context(), net, &scenario, nil, registry)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "mismatch") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestGetOrCreateDelegator_IsDeterministicPerName(t *testing.T) {
	state := &runState{
		delegators: map[string]*delegatorAccount{},
	}
	a1, err := getOrCreateDelegator(state, "alice")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	a2, err := getOrCreateDelegator(state, "alice")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a1 != a2 {
		t.Fatalf("expected same account instance on repeated lookup, got different pointers")
	}

	bob, err := getOrCreateDelegator(state, "bob")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a1.address == bob.address {
		t.Fatalf("expected distinct addresses for distinct names, got same %s", a1.address.Hex())
	}
}

func TestRun_VerifyStakes_ChecksFullyUndelegatedPairIsZero(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)
	registry := NewMockvalidatorRegistry(ctrl)
	node := driver.NewMockNode(ctrl)

	net.EXPECT().DialRandomRpc().Return(nil, fmt.Errorf("no nodes")).AnyTimes()
	node.EXPECT().GetLabel().Return("heavy").AnyTimes()
	node.EXPECT().DialRpc(gomock.Any()).Return(nil, fmt.Errorf("not ready")).AnyTimes()

	validatorId := 2
	registry.EXPECT().registerNewValidator(gomock.Any(), uint64(0)).Return(validatorId, nil)
	net.EXPECT().CreateNode(gomock.Any()).Return(node, nil)
	registry.EXPECT().ensureValidatorsActive(gomock.Any(), []int{validatorId}).Return(nil)
	node.EXPECT().GetValidatorId().Return(&validatorId).AnyTimes()

	registry.EXPECT().fundDelegator(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
	registry.EXPECT().delegate(gomock.Any(), validatorId, uint64(500_000), gomock.Any()).Return(nil)
	registry.EXPECT().undelegateAs(gomock.Any(), validatorId, uint64(0), gomock.Any()).Return(nil)
	// The fully undelegated pair must still be verified against the chain.
	registry.EXPECT().getDelegatorStake(gomock.Any(), validatorId).Return(uint64(0), nil)

	scenario := parser.Scenario{
		Name:             "Verify zero stake",
		Description:      "Test scenario.",
		DisableEndChecks: true,
		Steps: []parser.Step{
			{
				Function:   parser.FuncStartNode,
				Identifier: "heavy",
				NodeType:   "validator",
			},
			{
				Function: parser.FuncDelegate,
				DelegateTargets: []parser.DelegateTarget{
					{Node: "heavy", Delegator: "alice", Stake: 500_000},
				},
			},
			{
				Function: parser.FuncUndelegate,
				UndelegateTargets: []parser.UndelegateTarget{
					{Node: "heavy", Delegator: "alice"},
				},
			},
			{Function: parser.FuncVerifyStakes},
		},
	}

	if err := run(
		t.Context(), net, &scenario, nil, registry,
	); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
