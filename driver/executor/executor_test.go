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
	"fmt"
	"reflect"
	"syscall"
	"testing"

	"github.com/0xsoniclabs/norma/driver/checking"
	"github.com/0xsoniclabs/norma/driver/monitoring"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/parser"
	"go.uber.org/mock/gomock"
)

func TestExecutor_RunEmptyScenario(t *testing.T) {
	ctrl := gomock.NewController(t)
	clock := NewSimClock()
	net := driver.NewMockNetwork(ctrl)
	scenario := parser.Scenario{
		Name:       "Test",
		Duration:   10,
		Validators: []parser.Validator{{Name: "validator"}},
	}

	if err := Run(clock, net, &scenario, nil); err != nil {
		t.Errorf("failed to run empty scenario: %v", err)
	}
	want := Seconds(10)
	if got := clock.Now(); got < want {
		t.Errorf("scenario execution did not complete all steps, expected end time %v, got %v", want, got)
	}
}

func TestExecutor_RunSingleNodeScenario(t *testing.T) {

	clock := NewSimClock()
	scenario := parser.Scenario{
		Name:       "Test",
		Duration:   10,
		Validators: []parser.Validator{{Name: "validator"}},
		Nodes: []parser.Node{{
			Name:  "A",
			Start: New[float32](3),
			End:   New[float32](7),
		}},
	}

	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)
	node := driver.NewMockNode(ctrl)

	// In this scenario, a node is expected to be created and shut down.
	gomock.InOrder(
		net.EXPECT().CreateNode(gomock.Any()).Return(node, nil),
		net.EXPECT().RemoveNode(node),
		node.EXPECT().Stop(),
		node.EXPECT().Cleanup(),
	)

	if err := Run(clock, net, &scenario, nil); err != nil {
		t.Errorf("failed to run scenario: %v", err)
	}
	want := Seconds(10)
	if got := clock.Now(); got < want {
		t.Errorf("scenario execution did not complete all steps, expected end time %v, got %v", want, got)
	}
}

func TestExecutor_RunMultipleNodeScenario(t *testing.T) {

	clock := NewSimClock()
	scenario := parser.Scenario{
		Name:       "Test",
		Duration:   10,
		Validators: []parser.Validator{{Name: "validator"}},
		Nodes: []parser.Node{{
			Name:      "A",
			Instances: New(2),
			Start:     New[float32](3),
			End:       New[float32](7),
		}},
	}

	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)
	node1 := driver.NewMockNode(ctrl)
	node2 := driver.NewMockNode(ctrl)

	// In this scenario, two nodes are created and stopped.
	gomock.InOrder(
		net.EXPECT().CreateNode(gomock.Any()).Return(node1, nil),
		net.EXPECT().RemoveNode(newIs(node1)),
		node1.EXPECT().Stop(),
		node1.EXPECT().Cleanup(),
	)
	gomock.InOrder(
		net.EXPECT().CreateNode(gomock.Any()).Return(node2, nil),
		net.EXPECT().RemoveNode(newIs(node2)),
		node2.EXPECT().Stop(),
		node2.EXPECT().Cleanup(),
	)

	if err := Run(clock, net, &scenario, nil); err != nil {
		t.Errorf("failed to run scenario: %v", err)
	}
	want := Seconds(10)
	if got := clock.Now(); got < want {
		t.Errorf("scenario execution did not complete all steps, expected end time %v, got %v", want, got)
	}
}

func TestExecutor_Validator_StartEndRejoinKill(t *testing.T) {
	var two, three int = 2, 3
	clock := NewSimClock()
	scenario := parser.Scenario{
		Name:       "Test",
		Duration:   10,
		Validators: []parser.Validator{{Name: "validator"}}, //id=1
		Nodes: []parser.Node{
			{
				Name:  "start-kill",
				Start: New[float32](1),
				Kill:  New[float32](2),
				Client: parser.ClientType{
					Type:        "validator",
					ValidatorId: &two, // ensure that this is 2
				},
			},
			{
				Name:  "start-end",
				Start: New[float32](3),
				End:   New[float32](4),
				Client: parser.ClientType{
					Type: "validator", // auto-assigned 3
				},
			},
			{
				Name:   "rejoin-kill",
				Rejoin: New[float32](5),
				Kill:   New[float32](6),
				Client: parser.ClientType{
					Type:        "validator",
					ValidatorId: &two, // start as 2
				},
			},
			{
				Name:   "rejoin-end",
				Rejoin: New[float32](7),
				End:    New[float32](8),
				Client: parser.ClientType{
					Type:        "validator",
					ValidatorId: &two, // start as 2
				},
			},
		},
	}

	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)
	registry := NewMockvalidatorRegistry(ctrl)

	// node1 starts with assertion that id=2
	node1 := driver.NewMockNode(ctrl)
	gomock.InOrder(
		// start = expect register
		registry.EXPECT().registerNewValidator().Return(two, nil),
		net.EXPECT().CreateNode(gomock.Any()).Return(node1, nil),
		// kill = no unregister
		net.EXPECT().RemoveNode(node1),
		node1.EXPECT().Stop(),
		node1.EXPECT().Cleanup(),
	)

	// node2 starts with no id, get assigned 3
	node2 := driver.NewMockNode(ctrl)
	gomock.InOrder(
		// start = expect register
		registry.EXPECT().registerNewValidator().Return(three, nil),
		net.EXPECT().CreateNode(gomock.Any()).Return(node2, nil),
		// end = expect unregister
		node2.EXPECT().GetValidatorId().Return(&three),
		registry.EXPECT().unregisterValidator(three).Return(nil),
		net.EXPECT().RemoveNode(node2),
		node2.EXPECT().Stop(),
		node2.EXPECT().Cleanup(),
	)

	// node3 rejoins with id=2
	node3 := driver.NewMockNode(ctrl)
	gomock.InOrder(
		// rejoin = no register
		net.EXPECT().CreateNode(gomock.Any()).Return(node3, nil),
		// kill = no unregister
		net.EXPECT().RemoveNode(node3),
		node3.EXPECT().Stop(),
		node3.EXPECT().Cleanup(),
	)

	// node4 rejoins with id=2
	node4 := driver.NewMockNode(ctrl)
	gomock.InOrder(
		// rejoin = no register
		net.EXPECT().CreateNode(gomock.Any()).Return(node4, nil),
		// end = expect unregister
		node4.EXPECT().GetValidatorId().Return(&two),
		registry.EXPECT().unregisterValidator(two).Return(nil),
		net.EXPECT().RemoveNode(node4),
		node4.EXPECT().Stop(),
		node4.EXPECT().Cleanup(),
	)

	if err := run(clock, net, &scenario, nil, registry); err != nil {
		t.Errorf("failed to run scenario: %v", err)
	}
}

func TestExecutor_RunSingleApplicationScenario(t *testing.T) {

	clock := NewSimClock()
	scenario := parser.Scenario{
		Name:       "Test",
		Duration:   10,
		Validators: []parser.Validator{{Name: "validator"}},
		Applications: []parser.Application{{
			Name:  "A",
			Type:  "counter",
			Start: New[float32](3),
			End:   New[float32](7),
			Rate:  parser.Rate{Constant: New[float32](10)},
		}},
	}

	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)
	app := driver.NewMockApplication(ctrl)

	// In this scenario, an application is expected to be created and shut down.
	net.EXPECT().CreateApplication(gomock.Any()).Return(app, nil)
	app.EXPECT().Start()
	app.EXPECT().Stop()

	if err := Run(clock, net, &scenario, nil); err != nil {
		t.Errorf("failed to run scenario: %v", err)
	}
	want := Seconds(10)
	if got := clock.Now(); got < want {
		t.Errorf("scenario execution did not complete all steps, expected end time %v, got %v", want, got)
	}
}

func TestExecutor_RunMultipleApplicationScenario(t *testing.T) {

	clock := NewSimClock()
	scenario := parser.Scenario{
		Name:       "Test",
		Duration:   10,
		Validators: []parser.Validator{{Name: "validator"}},
		Applications: []parser.Application{{
			Name:      "A",
			Type:      "counter",
			Instances: New(2),
			Start:     New[float32](3),
			End:       New[float32](7),
			Rate:      parser.Rate{Constant: New[float32](10)},
		}},
	}

	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)
	app1 := driver.NewMockApplication(ctrl)
	app2 := driver.NewMockApplication(ctrl)

	// In this scenario, an application is expected to be created and shut down.
	net.EXPECT().CreateApplication(gomock.Any()).Return(app1, nil)
	net.EXPECT().CreateApplication(gomock.Any()).Return(app2, nil)
	app1.EXPECT().Start()
	app1.EXPECT().Stop()
	app2.EXPECT().Start()
	app2.EXPECT().Stop()

	if err := Run(clock, net, &scenario, nil); err != nil {
		t.Errorf("failed to run scenario: %v", err)
	}
	want := Seconds(10)
	if got := clock.Now(); got < want {
		t.Errorf("scenario execution did not complete all steps, expected end time %v, got %v", want, got)
	}
}

func TestExecutor_TestUserAbort(t *testing.T) {

	clock := NewWallTimeClock()
	scenario := parser.Scenario{
		Name:       "Test",
		Duration:   5,
		Validators: []parser.Validator{{Name: "validator"}},
		Nodes: []parser.Node{{
			Name:  "A",
			Start: New[float32](1),
			End:   New[float32](3),
		}},
	}

	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)
	node := driver.NewMockNode(ctrl)

	// In this scenario, a node is created, after which a user interrupt is send.
	net.EXPECT().CreateNode(gomock.Any()).Do(func(_ any) {
		fmt.Printf("Sending interrupt signal to local process ..\n")
		syscall.Kill(syscall.Getpid(), syscall.SIGINT)
	}).Return(node, nil)

	if err := Run(clock, net, &scenario, nil); err == nil {
		t.Errorf("a user interrupt error should be reported")
	}
	want := Seconds(1)
	if got := clock.Now(); got < want || got > want+Seconds(1) {
		t.Errorf("scenario execution did not complete on user interrupt, expected end time %v, got %v", want, got)
	}
}

func TestExecutor_RunScenarioWithDefaultChecks(t *testing.T) {
	ctrl := gomock.NewController(t)
	clock := NewSimClock()
	net := driver.NewMockNetwork(ctrl)
	scenario := parser.Scenario{
		Name:       "Test",
		Duration:   10,
		Validators: []parser.Validator{{Name: "validator"}},
	}

	// Mock Default Checks
	checkBlockHeight := checking.NewMockChecker(ctrl)
	checking.RegisterNetworkCheck("block_height", func(driver.Network, *monitoring.Monitor) checking.Checker {
		return checkBlockHeight
	})
	checkBlocksHashes := checking.NewMockChecker(ctrl)
	checking.RegisterNetworkCheck("blocks_hashes", func(driver.Network, *monitoring.Monitor) checking.Checker {
		return checkBlocksHashes
	})
	checkBlocksRolling := checking.NewMockChecker(ctrl)
	checking.RegisterNetworkCheck("blocks_rolling", func(driver.Network, *monitoring.Monitor) checking.Checker {
		return checkBlocksRolling
	})
	checkBlockGasRate := checking.NewMockChecker(ctrl)
	checking.RegisterNetworkCheck("block_gas_rate", func(driver.Network, *monitoring.Monitor) checking.Checker {
		return checkBlockGasRate
	})

	checkBlockHeight.EXPECT().Check().Return(nil)
	checkBlocksHashes.EXPECT().Check().Return(nil)
	checkBlocksRolling.EXPECT().Check().Return(nil)
	checkBlockGasRate.EXPECT().Check().Return(nil)

	checks := checking.InitNetworkChecks(net, nil)
	if err := Run(clock, net, &scenario, checks); err != nil {
		t.Errorf("failed to run scenario with default checks: %v", err)
	}
	want := Seconds(10)
	if got := clock.Now(); got < want {
		t.Errorf("scenario execution did not complete all steps, expected end time %v, got %v", want, got)
	}
}

func TestExecutor_scheduleNetworkRulesEvents(t *testing.T) {
	clock := NewSimClock()
	scenario := parser.Scenario{
		Name:     "Test",
		Duration: 10,
		NetworkRules: parser.NetworkRules{
			Updates: []parser.NetworkRulesUpdate{
				{Time: 2, Rules: map[string]string{"MAX_BLOCK_GAS": "20500000000"}},
				{Time: 6, Rules: map[string]string{"MAX_EPOCH_GAS": "1500000000000"}},
			},
		},
	}

	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)
	gomock.InOrder(
		net.EXPECT().ApplyNetworkRules(map[string]string{"MAX_BLOCK_GAS": "20500000000"}),
		net.EXPECT().ApplyNetworkRules(map[string]string{"MAX_EPOCH_GAS": "1500000000000"}),
	)

	if err := Run(clock, net, &scenario, nil); err != nil {
		t.Errorf("failed to run scenario: %v", err)
	}
}

func New[T any](value T) *T {
	res := new(T)
	*res = value
	return res
}

type is[T any] struct {
	x T
}

func (e *is[T]) Matches(a any) bool {
	x, ok := a.(T)
	return ok && reflect.ValueOf(e.x) == reflect.ValueOf(x)
}

func (e *is[T]) String() string {
	return fmt.Sprintf("is %v", e.x)
}

func newIs[T any](node T) *is[T] {
	return &is[T]{node}
}

func TestExecutor_scheduleAdvanceEpochEvents(t *testing.T) {
	one, three, five, seven := 1, 3, 5, 7

	clock := NewSimClock()
	scenario := parser.Scenario{
		Name:     "Test",
		Duration: 10,
		AdvanceEpoch: []parser.AdvanceEpoch{
			parser.AdvanceEpoch{Time: 1, Epochs: &one},
			parser.AdvanceEpoch{Time: 3, Epochs: &three},
			parser.AdvanceEpoch{Time: 7, Epochs: &seven},
			parser.AdvanceEpoch{Time: 5, Epochs: &five},
		},
	}

	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)
	gomock.InOrder(
		net.EXPECT().AdvanceEpoch(1),
		net.EXPECT().AdvanceEpoch(3),
		net.EXPECT().AdvanceEpoch(5),
		net.EXPECT().AdvanceEpoch(7),
	)

	if err := Run(clock, net, &scenario, nil); err != nil {
		t.Errorf("failed to run scenario: %v", err)
	}
}
