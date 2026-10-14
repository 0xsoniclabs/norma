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
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/steps"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestStartNode_Validate(t *testing.T) {
	zero := 0
	tests := map[string]struct {
		step      StartNode
		wantError string
	}{
		"observer by default": {step: StartNode{Node: steps.Node{Node: "a"}}},
		"validator":           {step: StartNode{Node: steps.Node{Node: "a"}, Type: "validator"}},
		"rpc":                 {step: StartNode{Node: steps.Node{Node: "a"}, Type: "rpc"}},
		"missing name":        {step: StartNode{}, wantError: "requires a node identifier"},
		"name with spaces":    {step: StartNode{Node: steps.Node{Node: "a b"}}, wantError: "node name must match"},
		"unknown type": {
			step:      StartNode{Node: steps.Node{Node: "a"}, Type: "miner"},
			wantError: "type of node must be",
		},
		"no instances": {
			step:      StartNode{Node: steps.Node{Node: "a"}, Instances: &zero},
			wantError: "instances must be >= 1",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			err := test.step.Validate()
			if test.wantError == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, test.wantError)
			}
		})
	}
}

// Only a node expected to work leaves a network the executor should wait on.
func TestStartNode_AwaitsBlockProductionUnlessExpectedToFail(t *testing.T) {
	require.True(t, (&StartNode{}).AwaitsBlockProduction())
	require.False(t, (&StartNode{Failing: true}).AwaitsBlockProduction())
}

// startEnv returns an environment on a network with no reachable node, so
// the step's sync target defaults to block 0, and records the configs of the
// nodes the step creates.
func startEnv(
	t *testing.T, ctrl *gomock.Controller, registry steps.ValidatorRegistry,
) (*steps.Env, func() []driver.NodeConfig) {
	t.Helper()
	net := driver.NewMockNetwork(ctrl)
	net.EXPECT().DialRandomRpc().Return(nil, errors.New("no nodes")).AnyTimes()

	var mu sync.Mutex
	var configs []driver.NodeConfig
	net.EXPECT().CreateNode(gomock.Any()).DoAndReturn(func(config *driver.NodeConfig) (driver.Node, error) {
		mu.Lock()
		defer mu.Unlock()
		configs = append(configs, *config)
		node := driver.NewMockNode(ctrl)
		node.EXPECT().GetLabel().Return(config.Name).AnyTimes()
		node.EXPECT().DialRpc(gomock.Any()).Return(nil, errors.New("not ready")).AnyTimes()
		return node, nil
	}).AnyTimes()

	created := func() []driver.NodeConfig {
		mu.Lock()
		defer mu.Unlock()
		sorted := slices.Clone(configs)
		slices.SortFunc(sorted, func(a, b driver.NodeConfig) int {
			return strings.Compare(a.Name, b.Name)
		})
		return sorted
	}
	return steps.NewEnv(net, nil, registry), created
}

func TestStartNode_PassesItsParametersToEveryInstance(t *testing.T) {
	ctrl := gomock.NewController(t)
	env, created := startEnv(t, ctrl, nil)

	instances := 3
	step := &StartNode{
		Node:           steps.Node{Node: "obs"},
		DataVolume:     "vol",
		Instances:      &instances,
		ExtraArguments: "--verbosity 4",
	}
	require.NoError(t, step.Run(t.Context(), env))

	volume := "vol"
	var want []driver.NodeConfig
	for _, name := range []string{"obs-0", "obs-1", "obs-2"} {
		want = append(want, driver.NodeConfig{
			Name:           name,
			Image:          driver.DefaultClientDockerImageName,
			DataVolume:     &volume,
			ExtraArguments: "--verbosity 4",
		})
	}
	require.Equal(t, want, created())

	for _, label := range []string{"obs-0", "obs-1", "obs-2"} {
		require.Contains(t, env.Nodes, label)
		require.True(t, env.NodeHistory[label])
	}
	require.True(t, env.NodeHistory["obs"], "the base name is remembered for a rejoin")
}

// A node expected to fail may never come up, so it is neither waited for nor
// admitted to the validator set; it joins at the next seal of the scenario.
func TestStartNode_SkipsSyncAndActivation_WhenTheNodeIsExpectedToFail(t *testing.T) {
	ctrl := gomock.NewController(t)
	registry := steps.NewMockValidatorRegistry(ctrl)
	registry.EXPECT().RegisterNewValidator(gomock.Any(), uint64(0)).Return(7, nil)
	// No EnsureValidatorsActive call is expected.
	env, created := startEnv(t, ctrl, registry)

	step := &StartNode{Node: steps.Node{Node: "frail"}, Type: "validator", Failing: true}
	require.NoError(t, step.Run(t.Context(), env))

	configs := created()
	require.Len(t, configs, 1)
	require.True(t, configs[0].Failing)
	require.Equal(t, 7, *configs[0].ValidatorId)
	require.Equal(t, 7, env.ValidatorIds["frail"])
}

// Genesis validators are in the validator set from the first block on: they
// are neither registered nor activated.
func TestStartNode_ReusesGenesisValidatorIds(t *testing.T) {
	ctrl := gomock.NewController(t)
	registry := steps.NewMockValidatorRegistry(ctrl)
	// Neither RegisterNewValidator nor EnsureValidatorsActive is expected.
	env, created := startEnv(t, ctrl, registry)
	env.ValidatorIds["genesis"] = 1

	step := &StartNode{Node: steps.Node{Node: "genesis"}, Type: "validator"}
	require.NoError(t, step.Run(t.Context(), env))

	configs := created()
	require.Len(t, configs, 1)
	require.True(t, configs[0].Validator)
	require.Equal(t, 1, *configs[0].ValidatorId)
}

func TestStartNode_RegistersAndActivatesANewValidator(t *testing.T) {
	ctrl := gomock.NewController(t)
	registry := steps.NewMockValidatorRegistry(ctrl)
	stake := uint64(2_000_000)
	gomock.InOrder(
		registry.EXPECT().RegisterNewValidator(gomock.Any(), stake).Return(4, nil),
		registry.EXPECT().EnsureValidatorsActive(gomock.Any(), []int{4}).Return(nil),
	)
	env, created := startEnv(t, ctrl, registry)

	step := &StartNode{Node: steps.Node{Node: "heavy"}, Type: "validator", Stake: &stake}
	require.NoError(t, step.Run(t.Context(), env))
	require.Equal(t, 4, *created()[0].ValidatorId)
}

func TestStartNode_RejectsARejoiningValidatorWithoutItsId(t *testing.T) {
	ctrl := gomock.NewController(t)
	env, created := startEnv(t, ctrl, steps.NewMockValidatorRegistry(ctrl))
	env.NodeHistory["lost"] = true

	step := &StartNode{Node: steps.Node{Node: "lost"}, Type: "validator"}
	require.ErrorContains(t, step.Run(t.Context(), env), "no preserved validator ID")
	require.Empty(t, created(), "no node is created for it")
}

func TestStartNode_RestartsOnlyNodesNormaDrives(t *testing.T) {
	ctrl := gomock.NewController(t)
	env, _ := startEnv(t, ctrl, nil)
	env.Nodes["foreign"] = driver.NewMockNode(ctrl)

	step := &StartNode{Node: steps.Node{Node: "foreign"}}
	require.ErrorContains(t, step.Run(t.Context(), env), "is not an OperaNode")
}
