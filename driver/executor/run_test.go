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
	"maps"
	"math/big"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/checking"
	"github.com/0xsoniclabs/norma/driver/parser"
	"github.com/0xsoniclabs/norma/driver/steps"
	"github.com/0xsoniclabs/norma/driver/steps/apps"
	"github.com/0xsoniclabs/norma/driver/steps/checks"
	"github.com/0xsoniclabs/norma/driver/steps/network"
	"github.com/0xsoniclabs/norma/driver/steps/nodelifetime"
	"github.com/0xsoniclabs/norma/driver/steps/wait"
	"github.com/0xsoniclabs/norma/genesis"
	"github.com/0xsoniclabs/norma/load/shaper"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestRun_EmptyScenario(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)

	scenario := parser.Scenario{
		Name:        "Empty",
		Description: "Test scenario.",
		Steps:       parser.Steps{},
	}

	if err := runChecked(t, t.Context(), net, &scenario, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRun_StartAndStopNode(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)
	registry := steps.NewMockValidatorRegistry(ctrl)
	node := driver.NewMockNode(ctrl)

	// An empty network has nothing to sync with or wait for.
	net.EXPECT().DialRandomRpc().Return(nil, driver.ErrEmptyNetwork).AnyTimes()
	net.EXPECT().GetActiveNodes().Return(nil).AnyTimes()
	node.EXPECT().GetLabel().Return("validator-A").AnyTimes()
	node.EXPECT().DialRpc(gomock.Any()).Return(nil, fmt.Errorf("not ready")).AnyTimes()

	validatorId := 2
	node.EXPECT().GetValidatorId().Return(&validatorId).AnyTimes()
	gomock.InOrder(
		registry.EXPECT().RegisterNewValidator(gomock.Any(), uint64(0)).Return(validatorId, nil),
		net.EXPECT().CreateNode(gomock.Any()).Return(node, nil),
		registry.EXPECT().EnsureValidatorsActive(gomock.Any(), []int{validatorId}).Return(nil),
		registry.EXPECT().UnregisterValidator(gomock.Any(), validatorId, gomock.Any()).Return(nil),
		net.EXPECT().RemoveNode(node).Return(nil),
		node.EXPECT().Stop(gomock.Any()).Return(nil),
		node.EXPECT().Cleanup(gomock.Any()).Return(nil),
	)

	scenario := parser.Scenario{
		Name:        "Start Stop",
		Description: "Test scenario.",
		Steps: parser.Steps{
			startValidator("validator-A"),
			&network.Undelegate{
				Targets: []network.UndelegateTarget{{Node: "validator-A"}},
			},
			&nodelifetime.StopNode{Node: steps.Node{Node: "validator-A"}},
		},
	}

	if err := runChecked(t, t.Context(), net, &scenario, nil, registry); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRun_StartNode_ForwardsCustomStake(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)
	registry := steps.NewMockValidatorRegistry(ctrl)
	node := driver.NewMockNode(ctrl)

	net.EXPECT().DialRandomRpc().
		Return(nil, driver.ErrEmptyNetwork).AnyTimes()
	net.EXPECT().GetActiveNodes().Return(nil).AnyTimes()
	node.EXPECT().GetLabel().Return("heavy").AnyTimes()
	node.EXPECT().DialRpc(gomock.Any()).
		Return(nil, fmt.Errorf("not ready")).AnyTimes()

	customStake := uint64(10_000_000)
	validatorId := 2
	node.EXPECT().GetValidatorId().Return(&validatorId).AnyTimes()

	gomock.InOrder(
		registry.EXPECT().
			RegisterNewValidator(gomock.Any(), customStake).
			Return(validatorId, nil),
		net.EXPECT().CreateNode(gomock.Any()).Return(node, nil),
		registry.EXPECT().
			EnsureValidatorsActive(gomock.Any(), []int{validatorId}).Return(nil),
	)

	stake := customStake
	scenario := parser.Scenario{
		Name:             "Custom Stake",
		Description:      "Test scenario.",
		DisableEndChecks: true,
		Steps: parser.Steps{
			&nodelifetime.StartNode{Node: steps.Node{Node: "heavy"}, Type: "validator", Stake: &stake},
		},
	}

	if err := runChecked(
		t, t.Context(), net, &scenario, nil, registry,
	); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRun_UndelegateSingleInstanceWithSuffix(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)
	registry := steps.NewMockValidatorRegistry(ctrl)
	node := driver.NewMockNode(ctrl)

	net.EXPECT().DialRandomRpc().
		Return(nil, driver.ErrEmptyNetwork).AnyTimes()
	net.EXPECT().GetActiveNodes().Return(nil).AnyTimes()
	node.EXPECT().GetLabel().Return("heavy-0").AnyTimes()
	node.EXPECT().DialRpc(gomock.Any()).
		Return(nil, fmt.Errorf("not ready")).AnyTimes()

	validatorId := 2
	node.EXPECT().GetValidatorId().Return(&validatorId).AnyTimes()
	gomock.InOrder(
		registry.EXPECT().RegisterNewValidator(gomock.Any(), uint64(0)).
			Return(validatorId, nil),
		net.EXPECT().CreateNode(gomock.Any()).Return(node, nil),
		registry.EXPECT().
			EnsureValidatorsActive(gomock.Any(), []int{validatorId}).Return(nil),
		registry.EXPECT().
			UnregisterValidator(gomock.Any(), validatorId, uint64(0)).Return(nil),
	)

	instances := 1
	scenario := parser.Scenario{
		Name:             "Undelegate Single Instance With Suffix",
		Description:      "Test scenario.",
		DisableEndChecks: true,
		Steps: parser.Steps{
			&nodelifetime.StartNode{Node: steps.Node{Node: "heavy"}, Type: "validator", Instances: &instances},
			&network.Undelegate{
				Targets: []network.UndelegateTarget{{Node: "heavy"}},
			},
		},
	}

	if err := runChecked(
		t, t.Context(), net, &scenario, nil, registry,
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
			registry := steps.NewMockValidatorRegistry(ctrl)
			node0 := driver.NewMockNode(ctrl)
			node1 := driver.NewMockNode(ctrl)

			net.EXPECT().DialRandomRpc().Return(nil, driver.ErrEmptyNetwork).AnyTimes()
			net.EXPECT().GetActiveNodes().Return(nil).AnyTimes()
			node0.EXPECT().GetLabel().Return("validators-0").AnyTimes()
			node0.EXPECT().DialRpc(gomock.Any()).Return(nil, fmt.Errorf("not ready")).AnyTimes()
			node1.EXPECT().GetLabel().Return("validators-1").AnyTimes()
			node1.EXPECT().DialRpc(gomock.Any()).Return(nil, fmt.Errorf("not ready")).AnyTimes()
			validatorId0, validatorId1 := 2, 3
			node0.EXPECT().GetValidatorId().Return(&validatorId0).AnyTimes()
			node1.EXPECT().GetValidatorId().Return(&validatorId1).AnyTimes()

			instances := 2
			registry.EXPECT().RegisterNewValidator(gomock.Any(), gomock.Any()).Return(2, nil)
			registry.EXPECT().RegisterNewValidator(gomock.Any(), gomock.Any()).Return(3, nil)
			net.EXPECT().CreateNode(gomock.Any()).DoAndReturn(func(config *driver.NodeConfig) (driver.Node, error) {
				if config.Name == "validators-0" {
					return node0, nil
				}
				return node1, nil
			}).Times(2)
			// Both instances are admitted to the validator set by a single seal.
			registry.EXPECT().EnsureValidatorsActive(gomock.Any(), []int{2, 3}).Return(nil)

			if !tc.expectErr {
				registry.EXPECT().UnregisterValidator(gomock.Any(), validatorId0, uint64(0)).Return(nil)
			}

			scenario := parser.Scenario{
				Name:             "Undelegate Multi Instance",
				Description:      "Test scenario.",
				DisableEndChecks: true,
				Steps: parser.Steps{
					&nodelifetime.StartNode{
						Node:      steps.Node{Node: "validators"},
						Type:      "validator",
						Instances: &instances,
					},
					&network.Undelegate{
						Targets: []network.UndelegateTarget{{Node: tc.target}},
					},
				},
			}

			err := runChecked(t, t.Context(), net, &scenario, nil, registry)
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
	registry := steps.NewMockValidatorRegistry(ctrl)
	node := driver.NewMockNode(ctrl)

	net.EXPECT().DialRandomRpc().Return(nil, driver.ErrEmptyNetwork).AnyTimes()
	net.EXPECT().GetActiveNodes().Return(nil).AnyTimes()
	node.EXPECT().GetLabel().Return("validator-A").AnyTimes()
	node.EXPECT().DialRpc(gomock.Any()).Return(nil, fmt.Errorf("not ready")).AnyTimes()

	validatorId := 2
	node.EXPECT().GetValidatorId().Return(&validatorId).AnyTimes()
	gomock.InOrder(
		registry.EXPECT().RegisterNewValidator(gomock.Any(), gomock.Any()).Return(validatorId, nil),
		net.EXPECT().CreateNode(gomock.Any()).Return(node, nil),
		registry.EXPECT().EnsureValidatorsActive(gomock.Any(), []int{validatorId}).Return(nil),
		// No unregister call expected
		net.EXPECT().RemoveNode(node).Return(nil),
		node.EXPECT().Stop(gomock.Any()).Return(nil),
		node.EXPECT().Cleanup(gomock.Any()).Return(nil),
	)

	scenario := parser.Scenario{
		Name:        "Leave",
		Description: "Test scenario.",
		Steps: parser.Steps{
			startValidator("validator-A"),
			// Stop without undelegate
			&nodelifetime.StopNode{Node: steps.Node{Node: "validator-A"}},
		},
	}

	if err := runChecked(t, t.Context(), net, &scenario, nil, registry); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRun_RejoinNode(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)
	registry := steps.NewMockValidatorRegistry(ctrl)
	node1 := driver.NewMockNode(ctrl)
	node2 := driver.NewMockNode(ctrl)

	net.EXPECT().DialRandomRpc().Return(nil, driver.ErrEmptyNetwork).AnyTimes()
	net.EXPECT().GetActiveNodes().Return(nil).AnyTimes()
	node1.EXPECT().GetLabel().Return("validator-A").AnyTimes()
	node1.EXPECT().DialRpc(gomock.Any()).Return(nil, fmt.Errorf("not ready")).AnyTimes()
	node2.EXPECT().GetLabel().Return("validator-A").AnyTimes()
	node2.EXPECT().DialRpc(gomock.Any()).Return(nil, fmt.Errorf("not ready")).AnyTimes()

	validatorId := 2
	node1.EXPECT().GetValidatorId().Return(&validatorId).AnyTimes()
	node2.EXPECT().GetValidatorId().Return(&validatorId).AnyTimes()
	gomock.InOrder(
		// First start: registers as new validator
		registry.EXPECT().RegisterNewValidator(gomock.Any(), gomock.Any()).Return(validatorId, nil),
		net.EXPECT().CreateNode(gomock.Any()).Do(func(config *driver.NodeConfig) {
			if config.ValidatorId == nil || *config.ValidatorId != validatorId {
				t.Errorf("first start: expected ValidatorId=%d, got %v", validatorId, config.ValidatorId)
			}
		}).Return(node1, nil),
		registry.EXPECT().EnsureValidatorsActive(gomock.Any(), []int{validatorId}).Return(nil),
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
		registry.EXPECT().EnsureValidatorsActive(gomock.Any(), []int{validatorId}).Return(nil),
	)

	scenario := parser.Scenario{
		Name:        "Rejoin",
		Description: "Test scenario.",
		Steps: parser.Steps{
			startValidator("validator-A"),
			&nodelifetime.StopNode{Node: steps.Node{Node: "validator-A"}},
			startValidator("validator-A"),
		},
	}

	if err := runChecked(t, t.Context(), net, &scenario, nil, registry); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRun_RunAndStopApp(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)
	net.EXPECT().DialRandomRpc().Return(nil, driver.ErrEmptyNetwork).AnyTimes()
	net.EXPECT().GetActiveNodes().Return(nil).AnyTimes()
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
		Steps: parser.Steps{
			&apps.RunApp{
				App:   steps.App{App: "load"},
				Type:  "counter",
				Users: new(50),
				Rate:  &shaper.Rate{Constant: &rate},
			},
			&apps.StopApp{App: steps.App{App: "load"}},
		},
	}

	if err := runChecked(t, t.Context(), net, &scenario, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRun_UpdateRules(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)
	net.EXPECT().DialRandomRpc().Return(nil, driver.ErrEmptyNetwork).AnyTimes()
	net.EXPECT().GetActiveNodes().Return(nil).AnyTimes()
	baseFee := genesis.BigIntValue(*big.NewInt(3000000000))

	net.EXPECT().ApplyNetworkRules(gomock.Any(), driver.NetworkRules{
		Economy: &genesis.EconomyPatch{
			MinBaseFee: &baseFee,
		},
	}).Return(nil)

	scenario := parser.Scenario{
		Name:        "Rules",
		Description: "Test scenario.",
		Steps: parser.Steps{
			&network.UpdateRules{
				Rules: genesis.NetworkRulesPatch{
					Economy: &genesis.EconomyPatch{
						MinBaseFee: &baseFee,
					},
				},
			},
		},
	}

	if err := runChecked(t, t.Context(), net, &scenario, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRun_AdvanceEpoch(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)

	net.EXPECT().AdvanceEpoch(gomock.Any(), 1).Return(nil)
	// An empty network has no block production to wait for.
	net.EXPECT().DialRandomRpc().Return(nil, driver.ErrEmptyNetwork).AnyTimes()
	net.EXPECT().GetActiveNodes().Return(nil).AnyTimes()

	scenario := parser.Scenario{
		Name:        "Epoch",
		Description: "Test scenario.",
		Steps: parser.Steps{
			&network.AdvanceEpoch{},
		},
	}

	if err := runChecked(t, t.Context(), net, &scenario, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRun_Check(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)
	net.EXPECT().DialRandomRpc().Return(nil, driver.ErrEmptyNetwork).AnyTimes()
	net.EXPECT().GetActiveNodes().Return(nil).AnyTimes()
	checker := checking.NewMockChecker(ctrl)

	checker.EXPECT().Check(gomock.Any()).Return(nil)

	checkers := checking.Checks{"blocksRolling": checker}

	scenario := parser.Scenario{
		Name:        "Check",
		Description: "Test scenario.",
		Steps: parser.Steps{
			&checks.Checks{Items: []checks.Check{&checks.BlocksProduced{}}},
		},
	}

	if err := runChecked(t, t.Context(), net, &scenario, checkers, nil); err != nil {
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
		Steps: parser.Steps{
			&network.AdvanceEpoch{},
		},
	}

	err := runChecked(t, ctx, net, &scenario, nil, nil)
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
}

func TestRun_MultiInstanceNode(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)
	registry := steps.NewMockValidatorRegistry(ctrl)
	node1 := driver.NewMockNode(ctrl)
	node2 := driver.NewMockNode(ctrl)

	net.EXPECT().DialRandomRpc().Return(nil, driver.ErrEmptyNetwork).AnyTimes()
	net.EXPECT().GetActiveNodes().Return(nil).AnyTimes()
	node1.EXPECT().GetLabel().Return("validators-0").AnyTimes()
	node1.EXPECT().DialRpc(gomock.Any()).Return(nil, fmt.Errorf("not ready")).AnyTimes()
	node2.EXPECT().GetLabel().Return("validators-1").AnyTimes()
	node2.EXPECT().DialRpc(gomock.Any()).Return(nil, fmt.Errorf("not ready")).AnyTimes()
	firstId, secondId := 2, 3
	node1.EXPECT().GetValidatorId().Return(&firstId).AnyTimes()
	node2.EXPECT().GetValidatorId().Return(&secondId).AnyTimes()

	// Validators are registered sequentially, then nodes created in parallel.
	ids := make(chan int, 2)
	ids <- 2
	ids <- 3
	registry.EXPECT().RegisterNewValidator(gomock.Any(), uint64(0)).DoAndReturn(func(
		ctx context.Context, stake uint64,
	) (int, error) {
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

	registry.EXPECT().EnsureValidatorsActive(gomock.Any(), []int{2, 3}).Return(nil)

	instances := 2
	scenario := parser.Scenario{
		Name:        "Multi",
		Description: "Test scenario.",
		Steps: parser.Steps{
			&nodelifetime.StartNode{Node: steps.Node{Node: "validators"}, Type: "validator", Instances: &instances},
		},
	}

	if err := runChecked(t, t.Context(), net, &scenario, nil, registry); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRun_RunAndCaptureEventExecution_CapturesAllSteps(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)

	scenario := parser.Scenario{
		Name:        "Capture",
		Description: "Test scenario.",
		Steps: parser.Steps{
			&wait.WaitFor{Duration: time.Millisecond},
			&wait.WaitFor{Duration: 2 * time.Millisecond},
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
		if !strings.Contains(execution.Name, "waitFor") {
			t.Fatalf("step %d captured unexpected name: %q", i+1, execution.Name)
		}
	}
}

func TestRun_UndelegateWithDelegator_UsesDelegatorKey(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)
	registry := steps.NewMockValidatorRegistry(ctrl)
	node := driver.NewMockNode(ctrl)

	net.EXPECT().DialRandomRpc().Return(nil, driver.ErrEmptyNetwork).AnyTimes()
	net.EXPECT().GetActiveNodes().Return(nil).AnyTimes()
	node.EXPECT().GetLabel().Return("heavy").AnyTimes()
	node.EXPECT().DialRpc(gomock.Any()).Return(nil, fmt.Errorf("not ready")).AnyTimes()

	validatorId := 2
	registry.EXPECT().RegisterNewValidator(gomock.Any(), uint64(0)).Return(validatorId, nil)
	net.EXPECT().CreateNode(gomock.Any()).Return(node, nil)
	registry.EXPECT().EnsureValidatorsActive(gomock.Any(), []int{validatorId}).Return(nil)
	node.EXPECT().GetValidatorId().Return(&validatorId).AnyTimes()

	// First: delegate to create alice's account.
	registry.EXPECT().FundDelegator(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
	registry.EXPECT().Delegate(gomock.Any(), validatorId, uint64(1_000_000), gomock.Any()).Return(nil)
	// Then: undelegate as alice → undelegateAs is called with her key.
	registry.EXPECT().UndelegateAs(gomock.Any(), validatorId, uint64(400_000), gomock.Any()).Return(nil)
	// The regular self-undelegate path must NOT be called.

	partial := uint64(400_000)
	scenario := parser.Scenario{
		Name:             "Undelegate as delegator",
		Description:      "Test scenario.",
		DisableEndChecks: true,
		Steps: parser.Steps{
			startValidator("heavy"),
			&network.Delegate{
				Targets: []network.DelegateTarget{
					{Node: "heavy", Delegator: "alice", Stake: 1_000_000},
				},
			},
			&network.Undelegate{
				Targets: []network.UndelegateTarget{
					{Node: "heavy", Delegator: "alice", Stake: &partial},
				},
			},
		},
	}

	if err := runChecked(
		t, t.Context(), net, &scenario, nil, registry,
	); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRun_UndelegateWithUnknownDelegator_ReturnsError(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)
	registry := steps.NewMockValidatorRegistry(ctrl)
	node := driver.NewMockNode(ctrl)

	net.EXPECT().DialRandomRpc().Return(nil, driver.ErrEmptyNetwork).AnyTimes()
	net.EXPECT().GetActiveNodes().Return(nil).AnyTimes()
	node.EXPECT().GetLabel().Return("heavy").AnyTimes()
	node.EXPECT().DialRpc(gomock.Any()).Return(nil, fmt.Errorf("not ready")).AnyTimes()

	validatorId := 2
	registry.EXPECT().RegisterNewValidator(gomock.Any(), uint64(0)).Return(validatorId, nil)
	net.EXPECT().CreateNode(gomock.Any()).Return(node, nil)
	registry.EXPECT().EnsureValidatorsActive(gomock.Any(), []int{validatorId}).Return(nil)
	node.EXPECT().GetValidatorId().Return(&validatorId).AnyTimes()
	// No undelegate call should occur.

	scenario := parser.Scenario{
		Name:             "Unknown delegator",
		Description:      "Test scenario.",
		DisableEndChecks: true,
		Steps: parser.Steps{
			startValidator("heavy"),
			&network.Undelegate{
				Targets: []network.UndelegateTarget{
					{Node: "heavy", Delegator: "ghost"},
				},
			},
		},
	}

	err := runChecked(t, t.Context(), net, &scenario, nil, registry)
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
	registry := steps.NewMockValidatorRegistry(ctrl)
	node := driver.NewMockNode(ctrl)

	net.EXPECT().DialRandomRpc().Return(nil, driver.ErrEmptyNetwork).AnyTimes()
	net.EXPECT().GetActiveNodes().Return(nil).AnyTimes()
	node.EXPECT().GetLabel().Return("heavy").AnyTimes()
	node.EXPECT().DialRpc(gomock.Any()).Return(nil, fmt.Errorf("not ready")).AnyTimes()

	validatorId := 2
	registry.EXPECT().RegisterNewValidator(gomock.Any(), uint64(0)).Return(validatorId, nil)
	net.EXPECT().CreateNode(gomock.Any()).Return(node, nil)
	registry.EXPECT().EnsureValidatorsActive(gomock.Any(), []int{validatorId}).Return(nil)
	node.EXPECT().GetValidatorId().Return(&validatorId).AnyTimes()

	registry.EXPECT().FundDelegator(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
	registry.EXPECT().Delegate(gomock.Any(), validatorId, uint64(500_000), gomock.Any()).Return(nil)

	// verifyStakes should query alice's stake and see the expected value.
	registry.EXPECT().
		GetDelegatorStake(gomock.Any(), validatorId).
		Return(uint64(500_000), nil)

	scenario := parser.Scenario{
		Name:             "Verify stakes",
		Description:      "Test scenario.",
		DisableEndChecks: true,
		Steps: parser.Steps{
			startValidator("heavy"),
			&network.Delegate{
				Targets: []network.DelegateTarget{
					{Node: "heavy", Delegator: "alice", Stake: 500_000},
				},
			},
			&network.VerifyStakes{},
		},
	}

	if err := runChecked(
		t, t.Context(), net, &scenario, nil, registry,
	); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRun_VerifyStakes_FailsOnMismatch(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)
	registry := steps.NewMockValidatorRegistry(ctrl)
	node := driver.NewMockNode(ctrl)

	net.EXPECT().DialRandomRpc().Return(nil, driver.ErrEmptyNetwork).AnyTimes()
	net.EXPECT().GetActiveNodes().Return(nil).AnyTimes()
	node.EXPECT().GetLabel().Return("heavy").AnyTimes()
	node.EXPECT().DialRpc(gomock.Any()).Return(nil, fmt.Errorf("not ready")).AnyTimes()

	validatorId := 2
	registry.EXPECT().RegisterNewValidator(gomock.Any(), uint64(0)).Return(validatorId, nil)
	net.EXPECT().CreateNode(gomock.Any()).Return(node, nil)
	registry.EXPECT().EnsureValidatorsActive(gomock.Any(), []int{validatorId}).Return(nil)
	node.EXPECT().GetValidatorId().Return(&validatorId).AnyTimes()

	registry.EXPECT().FundDelegator(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
	registry.EXPECT().Delegate(gomock.Any(), validatorId, uint64(500_000), gomock.Any()).Return(nil)

	// Chain reports a different value than expected.
	registry.EXPECT().
		GetDelegatorStake(gomock.Any(), validatorId).
		Return(uint64(499_000), nil)

	scenario := parser.Scenario{
		Name:             "Verify stakes mismatch",
		Description:      "Test scenario.",
		DisableEndChecks: true,
		Steps: parser.Steps{
			startValidator("heavy"),
			&network.Delegate{
				Targets: []network.DelegateTarget{
					{Node: "heavy", Delegator: "alice", Stake: 500_000},
				},
			},
			&network.VerifyStakes{},
		},
	}

	err := runChecked(t, t.Context(), net, &scenario, nil, registry)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "mismatch") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestRun_VerifyStakes_ChecksFullyUndelegatedPairIsZero(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)
	registry := steps.NewMockValidatorRegistry(ctrl)
	node := driver.NewMockNode(ctrl)

	net.EXPECT().DialRandomRpc().Return(nil, driver.ErrEmptyNetwork).AnyTimes()
	net.EXPECT().GetActiveNodes().Return(nil).AnyTimes()
	node.EXPECT().GetLabel().Return("heavy").AnyTimes()
	node.EXPECT().DialRpc(gomock.Any()).Return(nil, fmt.Errorf("not ready")).AnyTimes()

	validatorId := 2
	registry.EXPECT().RegisterNewValidator(gomock.Any(), uint64(0)).Return(validatorId, nil)
	net.EXPECT().CreateNode(gomock.Any()).Return(node, nil)
	registry.EXPECT().EnsureValidatorsActive(gomock.Any(), []int{validatorId}).Return(nil)
	node.EXPECT().GetValidatorId().Return(&validatorId).AnyTimes()

	registry.EXPECT().FundDelegator(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
	registry.EXPECT().Delegate(gomock.Any(), validatorId, uint64(500_000), gomock.Any()).Return(nil)
	registry.EXPECT().UndelegateAs(gomock.Any(), validatorId, uint64(0), gomock.Any()).Return(nil)
	// The fully undelegated pair must still be verified against the chain.
	registry.EXPECT().GetDelegatorStake(gomock.Any(), validatorId).Return(uint64(0), nil)

	scenario := parser.Scenario{
		Name:             "Verify zero stake",
		Description:      "Test scenario.",
		DisableEndChecks: true,
		Steps: parser.Steps{
			startValidator("heavy"),
			&network.Delegate{
				Targets: []network.DelegateTarget{
					{Node: "heavy", Delegator: "alice", Stake: 500_000},
				},
			},
			&network.Undelegate{
				Targets: []network.UndelegateTarget{
					{Node: "heavy", Delegator: "alice"},
				},
			},
			&network.VerifyStakes{},
		},
	}

	if err := runChecked(
		t, t.Context(), net, &scenario, nil, registry,
	); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRun_KeepsCallerDeadline_WhenLongerThanDefault(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)

	want := time.Now().Add(2 * DefaultScenarioTimeout)
	ctx, cancel := context.WithDeadline(t.Context(), want)
	defer cancel()

	net.EXPECT().WaitForEpochChange(gomock.Any()).DoAndReturn(func(ctx context.Context) error {
		got, _ := ctx.Deadline()
		require.Equal(t, want, got)
		return nil
	})

	scenario := parser.Scenario{
		Name:        "Deadline",
		Description: "Test scenario.",
		Steps:       parser.Steps{&wait.WaitForEpoch{}},
	}

	require.NoError(t, runChecked(t, ctx, net, &scenario, nil, nil))
}

func TestRun_AbortsAtCallerDeadline_WhenStepBlocks(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer cancel()

	scenario := parser.Scenario{
		Name:        "Deadline",
		Description: "Test scenario.",
		Steps:       parser.Steps{&wait.WaitFor{Duration: time.Minute}},
	}

	start := time.Now()
	err := runChecked(t, ctx, net, &scenario, nil, nil)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Less(t, time.Since(start), time.Second)
}

// A step the model rejects fails before it acts: undelegating more than was
// delegated must not reach the chain.
func TestRun_DoesNotActOnTheNetwork_WhenTheModelRejectsAStep(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)
	registry := steps.NewMockValidatorRegistry(ctrl)
	node := driver.NewMockNode(ctrl)

	net.EXPECT().DialRandomRpc().Return(nil, driver.ErrEmptyNetwork).AnyTimes()
	net.EXPECT().GetActiveNodes().Return(nil).AnyTimes()
	node.EXPECT().GetLabel().Return("heavy").AnyTimes()
	node.EXPECT().DialRpc(gomock.Any()).Return(nil, fmt.Errorf("not ready")).AnyTimes()
	node.EXPECT().GetValidatorId().Return(new(2)).AnyTimes()
	registry.EXPECT().RegisterNewValidator(gomock.Any(), gomock.Any()).Return(2, nil)
	net.EXPECT().CreateNode(gomock.Any()).Return(node, nil)
	registry.EXPECT().EnsureValidatorsActive(gomock.Any(), []int{2}).Return(nil)
	registry.EXPECT().FundDelegator(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
	registry.EXPECT().Delegate(gomock.Any(), 2, uint64(100), gomock.Any()).Return(nil)
	// No UndelegateAs: the mock fails the test if the undelegation is sent.

	tooMuch := uint64(500)
	scenario := parser.Scenario{
		Name:             "Rejected",
		Description:      "Test scenario.",
		DisableEndChecks: true,
		Steps: parser.Steps{
			startValidator("heavy"),
			&network.Delegate{Targets: []network.DelegateTarget{{Node: "heavy", Delegator: "alice", Stake: 100}}},
			&network.Undelegate{Targets: []network.UndelegateTarget{
				{Node: "heavy", Delegator: "alice", Stake: &tooMuch},
			}},
		},
	}
	err := runChecked(t, t.Context(), net, &scenario, nil, registry)
	require.ErrorContains(t, err, "cannot undelegate 500 S")
}

// A validator's label may rejoin as an observer. The run keeps the stopped
// validator's id for a later rejoin, so the role is that of the node running.
func TestRun_TracksTheObserverRole_WhenAValidatorLabelRejoinsAsAnObserver(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)
	registry := steps.NewMockValidatorRegistry(ctrl)
	validator := driver.NewMockNode(ctrl)
	observer := driver.NewMockNode(ctrl)

	net.EXPECT().DialRandomRpc().Return(nil, driver.ErrEmptyNetwork).AnyTimes()
	net.EXPECT().GetActiveNodes().Return(nil).AnyTimes()
	for _, node := range []*driver.MockNode{validator, observer} {
		node.EXPECT().GetLabel().Return("flex").AnyTimes()
		node.EXPECT().DialRpc(gomock.Any()).Return(nil, fmt.Errorf("not ready")).AnyTimes()
	}
	validatorId := 2
	validator.EXPECT().GetValidatorId().Return(&validatorId).AnyTimes()
	observer.EXPECT().GetValidatorId().Return(nil).AnyTimes()
	gomock.InOrder(
		registry.EXPECT().RegisterNewValidator(gomock.Any(), uint64(0)).Return(validatorId, nil),
		net.EXPECT().CreateNode(gomock.Any()).Return(validator, nil),
		registry.EXPECT().EnsureValidatorsActive(gomock.Any(), []int{validatorId}).Return(nil),
		net.EXPECT().RemoveNode(validator).Return(nil),
		validator.EXPECT().Stop(gomock.Any()).Return(nil),
		validator.EXPECT().Cleanup(gomock.Any()).Return(nil),
		net.EXPECT().CreateNode(gomock.Any()).Return(observer, nil),
	)

	scenario := parser.Scenario{
		Name:             "Rejoin as observer",
		Description:      "Test scenario.",
		DisableEndChecks: true,
		Steps: parser.Steps{
			startValidator("flex"),
			&nodelifetime.StopNode{Node: steps.Node{Node: "flex"}},
			&nodelifetime.StartNode{Node: steps.Node{Node: "flex"}},
		},
	}
	require.NoError(t, runChecked(t, t.Context(), net, &scenario, nil, registry))
}

// runChecked runs the scenario like Run, and after every step asserts that the
// environment the steps built agrees with the model the executor applied
// before them, so the model cannot drift from what the steps do.
func runChecked(
	t *testing.T,
	ctx context.Context,
	network driver.Network,
	scenario *parser.Scenario,
	checks checking.Checks,
	registry steps.ValidatorRegistry,
) error {
	t.Helper()
	return runWithObserver(ctx, network, scenario, checks, registry, nil, nil, func(env *steps.Env) {
		requireModelMatches(t, env)
	})
}

func requireModelMatches(t *testing.T, env *steps.Env) {
	t.Helper()
	require.ElementsMatch(t, slices.Collect(maps.Keys(env.Nodes)), slices.Collect(maps.Keys(env.Model.Nodes)),
		"the model tracks other nodes than the run")
	// The role of the node running now: ValidatorIds also keeps the ids of
	// stopped validators, whose label may rejoin as an observer.
	for label, node := range env.Model.Nodes {
		isValidator := env.Nodes[label].GetValidatorId() != nil
		require.Equalf(t, isValidator, node.Validator, "the model has another role for %s than the run", label)
	}
	require.ElementsMatch(t, slices.Collect(maps.Keys(env.Apps)), slices.Collect(maps.Keys(env.Model.Apps)),
		"the model runs other apps than the run")
	stakes := map[steps.StakeKey]uint64{}
	for delegation, stake := range env.Model.Delegations {
		key := steps.StakeKey{Delegator: delegation.Delegator, ValidatorId: env.ValidatorIds[delegation.Node]}
		stakes[key] = stake
	}
	require.Equal(t, env.Stakes.Expected, stakes, "the model tracks other stakes than the run")
}

func startValidator(name string) *nodelifetime.StartNode {
	return &nodelifetime.StartNode{Node: steps.Node{Node: name}, Type: "validator"}
}
