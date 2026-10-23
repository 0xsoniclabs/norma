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

package parser

import (
	"math/big"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/0xsoniclabs/norma/driver/checking"
	"github.com/0xsoniclabs/norma/driver/node"
	"github.com/0xsoniclabs/norma/driver/steps"
	"github.com/0xsoniclabs/norma/driver/steps/apps"
	"github.com/0xsoniclabs/norma/driver/steps/checks"
	"github.com/0xsoniclabs/norma/driver/steps/network"
	"github.com/0xsoniclabs/norma/driver/steps/nodelifetime"
	"github.com/0xsoniclabs/norma/driver/steps/sonictool"
	"github.com/0xsoniclabs/norma/driver/steps/wait"
	"github.com/0xsoniclabs/norma/genesis"
	"github.com/stretchr/testify/require"
)

// stepAs returns step as a T, failing the test when it is of another kind.
func stepAs[T any](t *testing.T, step any) T {
	t.Helper()
	typed, ok := step.(T)
	require.Truef(t, ok, "unexpected kind %T", step)
	return typed
}

func durationOf(d time.Duration) *steps.Duration {
	v := steps.Duration(d)
	return &v
}

func TestAllFunctionsAndParamsAreDocumented(t *testing.T) {
	var fns []steps.Function
	for _, kind := range stepKinds {
		fns = append(fns, kind())
	}
	for _, kind := range checks.Kinds {
		fns = append(fns, kind())
	}
	names := map[string]bool{}
	for _, fn := range fns {
		require.Falsef(t, names[fn.Name()], "function %q is listed twice", fn.Name())
		names[fn.Name()] = true
		require.NotEmptyf(t, strings.TrimSpace(fn.Help()), "function %q has empty description", fn.Name())
		for _, p := range steps.Params(fn) {
			require.NotEmptyf(t, strings.TrimSpace(p.Help),
				"parameter %q of %q has empty description", p.Key, fn.Name())
		}
	}
}

// The executor waits for a new block after these steps only; the others may
// legitimately leave the network stalled, or do not change it.
func TestStepKinds_AwaitBlockProductionAfterStepsThatChangeTheNetwork(t *testing.T) {
	want := []string{"advanceEpoch", "runApp", "startNode", "updateRules"}
	var got []string
	for _, kind := range stepKinds {
		step := kind()
		if s, ok := step.(steps.Stabilizing); ok && s.AwaitsBlockProduction() {
			got = append(got, step.Name())
		}
	}
	slices.Sort(got)
	require.Equal(t, want, got)
}

func TestParseBytes_MinimalScenario(t *testing.T) {
	input := `
Name: Minimal Test
Scenario:
  - startNode: validator-A
    type: validator
  - runApp: load
    type: counter
    users: 10
    rate:
      constant: 5
  - advanceEpoch
  - stopApp: load
  - stopNode: validator-A
`
	scenario, err := ParseBytes([]byte(input))
	require.NoError(t, err)
	require.Equal(t, "Minimal Test", scenario.Name)
	require.Len(t, scenario.Steps, 8)

	start := stepAs[*nodelifetime.StartNode](t, scenario.Steps[0])
	require.Equal(t, "validator-A", start.Target())
	require.Equal(t, "validator", start.Type)

	runApp := stepAs[*apps.RunApp](t, scenario.Steps[1])
	require.Equal(t, "load", runApp.Target())
	require.Equal(t, "counter", runApp.Type)
	require.NotNil(t, runApp.Users)
	require.Equal(t, 10, *runApp.Users)
	require.NotNil(t, runApp.Rate)
	require.NotNil(t, runApp.Rate.Constant)
	require.EqualValues(t, 5, *runApp.Rate.Constant)

	stepAs[*network.AdvanceEpoch](t, scenario.Steps[2])

	stopApp := stepAs[*apps.StopApp](t, scenario.Steps[3])
	require.Equal(t, "load", stopApp.Target())

	stopNode := stepAs[*nodelifetime.StopNode](t, scenario.Steps[4])
	require.Equal(t, "validator-A", stopNode.Target())
}

func TestParseBytes_InitialRules(t *testing.T) {
	input := `
Name: Rules Test
InitialNetworkRules:
  Upgrades:
    Sonic: true
    Allegro: true
  Epochs:
    MaxEpochDuration: 10s
Scenario:
  - startNode: validator
    type: validator
`
	scenario, err := ParseBytes([]byte(input))
	require.NoError(t, err)
	require.NotNil(t, scenario.InitialRules.Upgrades)
	require.NotNil(t, scenario.InitialRules.Upgrades.Sonic)
	require.True(t, *scenario.InitialRules.Upgrades.Sonic)
	require.NotNil(t, scenario.InitialRules.Upgrades.Allegro)
	require.True(t, *scenario.InitialRules.Upgrades.Allegro)
	require.NotNil(t, scenario.InitialRules.Epochs)
	require.NotNil(t, scenario.InitialRules.Epochs.MaxEpochDuration)
	require.Equal(
		t,
		int64(10*time.Second),
		int64(*scenario.InitialRules.Epochs.MaxEpochDuration),
	)
}

func TestParseBytes_UpdateRules(t *testing.T) {
	input := `
Name: Update Rules Test
Scenario:
  - startNode: validator
    type: validator
  - updateRules:
      Economy:
        MinBaseFee: "3000000000"
      Blocks:
        MaxBlockGas: 100000
`
	scenario, err := ParseBytes([]byte(input))
	require.NoError(t, err)
	require.Len(t, scenario.Steps, 5)

	step := stepAs[*network.UpdateRules](t, scenario.Steps[1])
	require.NotNil(t, step.Rules.Economy)
	require.NotNil(t, step.Rules.Economy.MinBaseFee)
	if got := big.Int(*step.Rules.Economy.MinBaseFee); got.Cmp(big.NewInt(3000000000)) != 0 {
		require.Failf(t, "unexpected min base fee", "expected Economy.MinBaseFee=3000000000, got %s", got.String())
	}
	require.NotNil(t, step.Rules.Blocks)
	require.NotNil(t, step.Rules.Blocks.MaxBlockGas)
	require.EqualValues(t, 100000, *step.Rules.Blocks.MaxBlockGas)
}

func TestParseBytes_StartNodeWithOptions(t *testing.T) {
	input := `
Name: Node Options Test
Scenario:
  - startNode: my-node
    type: validator
    imageName: "sonic:v2.0.2"
    instances: 3
    dataVolume: "vol-A"
    failing: true
`
	scenario, err := ParseBytes([]byte(input))
	require.NoError(t, err)

	step := stepAs[*nodelifetime.StartNode](t, scenario.Steps[0])
	require.Equal(t, "my-node", step.Target())
	require.Equal(t, "validator", step.Type)
	require.Equal(t, "sonic:v2.0.2", step.ImageName)
	require.NotNil(t, step.Instances)
	require.Equal(t, 3, *step.Instances)
	require.Equal(t, "vol-A", step.DataVolume)
	require.True(t, step.Failing)
}

func TestParseBytes_Undelegate(t *testing.T) {
	input := `
Name: Stop Node Test
Scenario:
  - startNode: validator-A
    type: validator

  - undelegate:
    - node: validator-A

  - undelegate:
    - node: validator-A
      stake: 2_000_000
`
	scenario, err := ParseBytes([]byte(input))
	require.NoError(t, err)

	start := stepAs[*nodelifetime.StartNode](t, scenario.Steps[0])
	require.Equal(t, "validator-A", start.Target())

	step := stepAs[*network.Undelegate](t, scenario.Steps[1])
	require.Len(t, step.Targets, 1)
	require.Equal(t, "validator-A", step.Targets[0].Node)
	require.Nil(t, step.Targets[0].Stake)

	step = stepAs[*network.Undelegate](t, scenario.Steps[2])
	require.Len(t, step.Targets, 1)
	require.Equal(t, "validator-A", step.Targets[0].Node)
	require.NotNil(t, step.Targets[0].Stake)
	require.EqualValues(t, 2_000_000, *step.Targets[0].Stake)
}

func TestParseBytes_Delegate_ParsesAllTargetFields(t *testing.T) {
	input := `
Name: Delegate Test
Description: t
Scenario:
  - startNode: heavy
    type: validator

  - delegate:
    - node: heavy
      delegator: alice
      stake: 2_000_000
    - node: heavy
      delegator: bob
      stake: 1_000_000
`
	scenario, err := ParseBytes([]byte(input))
	require.NoError(t, err)

	step := stepAs[*network.Delegate](t, scenario.Steps[1])
	require.Len(t, step.Targets, 2)

	require.Equal(t, "heavy", step.Targets[0].Node)
	require.Equal(t, "alice", step.Targets[0].Delegator)
	require.EqualValues(t, 2_000_000, step.Targets[0].Stake)

	require.Equal(t, "heavy", step.Targets[1].Node)
	require.Equal(t, "bob", step.Targets[1].Delegator)
	require.EqualValues(t, 1_000_000, step.Targets[1].Stake)
}

func TestParseBytes_Delegate_RejectsScalarShorthand(t *testing.T) {
	input := `
Name: Delegate Test
Description: t
Scenario:
  - delegate: heavy
`
	_, err := ParseBytes([]byte(input))
	require.Error(t, err)
	require.Contains(t, err.Error(), "list of targets")
}

func TestParseBytes_UndelegateWithDelegator_ParsesDelegatorField(t *testing.T) {
	input := `
Name: Undelegate With Delegator
Description: t
Scenario:
  - startNode: heavy
    type: validator

  - undelegate:
    - node: heavy
      delegator: alice
    - node: heavy
      delegator: bob
      stake: 500_000
`
	scenario, err := ParseBytes([]byte(input))
	require.NoError(t, err)

	step := stepAs[*network.Undelegate](t, scenario.Steps[1])
	require.Len(t, step.Targets, 2)

	require.Equal(t, "heavy", step.Targets[0].Node)
	require.Equal(t, "alice", step.Targets[0].Delegator)
	require.Nil(t, step.Targets[0].Stake)

	require.Equal(t, "heavy", step.Targets[1].Node)
	require.Equal(t, "bob", step.Targets[1].Delegator)
	require.NotNil(t, step.Targets[1].Stake)
	require.EqualValues(t, 500_000, *step.Targets[1].Stake)
}

func TestParseBytes_VerifyStakes_ParsesEmptyStep(t *testing.T) {
	input := `
Name: Verify Stakes Test
Description: t
Scenario:
  - verifyStakes
`
	scenario, err := ParseBytes([]byte(input))
	require.NoError(t, err)

	stepAs[*network.VerifyStakes](t, scenario.Steps[0])
}

func TestStepCheck_Delegate_ReportsSemanticErrors(t *testing.T) {
	cases := map[string]struct {
		targets   []network.DelegateTarget
		errSubstr string
	}{
		"no targets": {
			errSubstr: "at least one target",
		},
		"missing node": {
			targets: []network.DelegateTarget{
				{Delegator: "alice", Stake: 100},
			},
			errSubstr: "'node' field",
		},
		"missing delegator": {
			targets: []network.DelegateTarget{
				{Node: "val", Stake: 100},
			},
			errSubstr: "'delegator' field",
		},
		"zero stake": {
			targets: []network.DelegateTarget{
				{Node: "val", Delegator: "alice", Stake: 0},
			},
			errSubstr: "stake must be greater than zero",
		},
		"invalid delegator name": {
			targets: []network.DelegateTarget{
				{Node: "val", Delegator: "not a name", Stake: 100},
			},
			errSubstr: "delegator name must match",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			step := &network.Delegate{Targets: tc.targets}
			err := step.Validate()
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.errSubstr)
		})
	}
}

func TestStepCheck_UndelegateWithDelegator_RejectsInvalidDelegatorName(t *testing.T) {
	step := &network.Undelegate{
		Targets: []network.UndelegateTarget{
			{Node: "val", Delegator: "not a name"},
		},
	}
	err := step.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "delegator name must match")
}

func TestParseBytes_Checks(t *testing.T) {
	input := `
Name: Checks Test
Scenario:
  - checks:
      - blocksHalted:
        failing: true
      - blockHeights:
        tolerance: 5
      - blockGasRate:
        ceiling: 16500000
        failing: true
      - blockHashes
`
	scenario, err := ParseBytes([]byte(input))
	require.NoError(t, err)
	require.Len(t, scenario.Steps, 4)

	step := stepAs[*checks.Checks](t, scenario.Steps[0])
	require.Len(t, step.Items, 4)

	halted := stepAs[*checks.BlocksHalted](t, step.Items[0])
	require.True(t, halted.Failing.Failing)

	heights := stepAs[*checks.BlockHeights](t, step.Items[1])
	require.NotNil(t, heights.Tolerance)
	require.Equal(t, 5, *heights.Tolerance)

	gasRate := stepAs[*checks.BlockGasRate](t, step.Items[2])
	require.NotNil(t, gasRate.Ceiling)
	require.EqualValues(t, 16500000, *gasRate.Ceiling)
	require.True(t, gasRate.Failing.Failing)

	stepAs[*checks.BlockHashes](t, step.Items[3])
}

func TestParseBytes_CheckParams_NestedForm(t *testing.T) {
	input := `
Name: Nested Check Params Test
Scenario:
  - checks:
      - eventThrottled:
          failing: true
          throttledNodes:
            - validator-dominant
            - validator-B
      - blockGasRate:
          ceiling: 1000
          failing: true
`
	scenario, err := ParseBytes([]byte(input))
	require.NoError(t, err)
	step := stepAs[*checks.Checks](t, scenario.Steps[0])
	require.Len(t, step.Items, 2)

	et := stepAs[*checks.EventThrottled](t, step.Items[0])
	require.True(t, et.Failing.Failing)
	require.Equal(
		t, []string{"validator-dominant", "validator-B"},
		et.ThrottledNodes,
	)

	gr := stepAs[*checks.BlockGasRate](t, step.Items[1])
	require.NotNil(t, gr.Ceiling)
	require.EqualValues(t, 1000, *gr.Ceiling)
	require.True(t, gr.Failing.Failing)
}

func TestParseBytes_CheckParams_RejectsUnknown(t *testing.T) {
	input := `
Name: Unknown Param Test
Scenario:
  - checks:
      - eventThrottled:
          bogus: 42
`
	_, err := ParseBytes([]byte(input))
	require.Error(t, err)
	require.Contains(t, err.Error(), `parameter "bogus" is not valid`)
}

func TestParseBytes_CheckParams_RejectsWrongParamForFunction(t *testing.T) {
	cases := map[string]string{
		"throttledNodes on blocksProduced": `
Name: Wrong Param Test
Scenario:
  - checks:
      - blocksProduced:
          throttledNodes:
            - validator-A
`,
		"ceiling on blocksHalted": `
Name: Wrong Param Test
Scenario:
  - checks:
      - blocksHalted:
          ceiling: 5
`,
		"tolerance on eventThrottled": `
Name: Wrong Param Test
Scenario:
  - checks:
      - eventThrottled:
          tolerance: 5
`,
		"rules on blockGasRate": `
Name: Wrong Param Test
Scenario:
  - checks:
      - blockGasRate:
          rules:
            Epochs:
              MaxEpochDuration: 5s
`,
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := ParseBytes([]byte(input))
			require.Error(t, err)
			require.Contains(t, err.Error(), "is not valid for check")
		})
	}
}

func TestParseBytes_ThrottledNodes_FlatForm(t *testing.T) {
	input := `
Name: Flat Throttled Nodes Test
Scenario:
  - checks:
      - eventThrottled:
        failing: true
        throttledNodes:
          - validator-A
`
	scenario, err := ParseBytes([]byte(input))
	require.NoError(t, err)
	step := stepAs[*checks.Checks](t, scenario.Steps[0])
	require.Len(t, step.Items, 1)
	et := stepAs[*checks.EventThrottled](t, step.Items[0])
	require.True(t, et.Failing.Failing)
	require.Equal(t, []string{"validator-A"}, et.ThrottledNodes)
}

func TestParseBytes_BlocksProduced_ParsesDuration(t *testing.T) {
	input := `
Name: Duration Test
Scenario:
  - checks:
      - blocksProduced:
        tolerance: 10
        duration: 30s
`
	scenario, err := ParseBytes([]byte(input))
	require.NoError(t, err)
	step := stepAs[*checks.Checks](t, scenario.Steps[0])
	require.Len(t, step.Items, 1)

	check := stepAs[*checks.BlocksProduced](t, step.Items[0])
	require.NotNil(t, check.Tolerance)
	require.Equal(t, 10, *check.Tolerance)
	require.NotNil(t, check.Duration)
	require.Equal(t, 30*time.Second, time.Duration(*check.Duration))
}

func TestParseBytes_BlocksProduced_RejectsInvalidDuration(t *testing.T) {
	cases := map[string]string{
		"non-duration string": `
Name: Bad
Scenario:
  - checks:
      - blocksProduced:
        duration: not-a-duration
`,
		"negative duration": `
Name: Bad
Scenario:
  - checks:
      - blocksProduced:
        duration: -5s
`,
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := ParseBytes([]byte(input))
			require.Error(t, err)
		})
	}
}

func TestParseBytes_SlopeRate(t *testing.T) {
	input := `
Name: Slope Rate Test
Scenario:
  - runApp: subsidies
    type: counter
    users: 10
    rate:
      slope:
        start: 20
        increment: 5
`
	scenario, err := ParseBytes([]byte(input))
	require.NoError(t, err)

	step := stepAs[*apps.RunApp](t, scenario.Steps[0])
	require.NotNil(t, step.Rate)
	require.NotNil(t, step.Rate.Slope)
	require.EqualValues(t, 20, step.Rate.Slope.Start)
	require.EqualValues(t, 5, step.Rate.Slope.Increment)
}

func TestParseBytes_DefaultsApplied(t *testing.T) {
	input := `
Name: Defaults Test
Scenario:
  - advanceEpoch
`
	scenario, err := ParseBytes([]byte(input))
	require.NoError(t, err)
	require.NotNil(t, scenario.InitialRules.Epochs)
	require.NotNil(t, scenario.InitialRules.Epochs.MaxEpochDuration)
	require.Equal(
		t,
		int64(15*time.Second),
		int64(*scenario.InitialRules.Epochs.MaxEpochDuration),
	)

	require.Len(t, scenario.Steps, 4)
	stepAs[*network.AdvanceEpoch](t, scenario.Steps[1])
	stepAs[*network.AdvanceEpoch](t, scenario.Steps[2])
	endChecks := stepAs[*checks.Checks](t, scenario.Steps[3])
	require.Len(t, endChecks.Items, 2)
	stepAs[*checks.BlockHashes](t, endChecks.Items[0])
	stepAs[*checks.BlockHeights](t, endChecks.Items[1])
}

func TestParseBytes_DisableEndChecks(t *testing.T) {
	input := `
Name: Disable End Checks Test
DisableEndChecks: true
Scenario:
  - advanceEpoch
`
	scenario, err := ParseBytes([]byte(input))
	require.NoError(t, err)
	require.Len(t, scenario.Steps, 1)
}

func TestParseBytes_UnknownFunction(t *testing.T) {
	input := `
Name: Bad Function
Scenario:
  - Do something weird: foo
`
	_, err := ParseBytes([]byte(input))
	require.Error(t, err)
}

func TestCheck_EmptyName(t *testing.T) {
	scenario := Scenario{
		Name:        "",
		Description: "A test scenario.",
		Steps:       Steps{&network.AdvanceEpoch{}},
	}
	err := scenario.Check()
	require.Error(t, err)
}

func TestCheck_EmptyDescription(t *testing.T) {
	scenario := Scenario{
		Name:        "Test",
		Description: "",
		Steps:       Steps{&network.AdvanceEpoch{}},
	}
	err := scenario.Check()
	require.Error(t, err)
}

func TestCheck_InvalidNodeName(t *testing.T) {
	scenario := Scenario{
		Name:        "Test",
		Description: "A test scenario.",
		Steps: Steps{&nodelifetime.StartNode{
			Node: steps.Node{Node: "invalid name with spaces"},
			Type: "validator",
		}},
	}
	err := scenario.Check()
	require.Error(t, err)
}

func TestCheck_RunAppMissingRate(t *testing.T) {
	scenario := Scenario{
		Name:        "Test",
		Description: "A test scenario.",
		Steps: Steps{&apps.RunApp{
			App:  steps.App{App: "load"},
			Type: "counter",
		}},
	}
	err := scenario.Check()
	require.Error(t, err)
}

func TestCheck_UpdateRulesEmpty(t *testing.T) {
	scenario := Scenario{
		Name:        "Test",
		Description: "A test scenario.",
		Steps: Steps{&network.UpdateRules{
			Rules: genesis.NetworkRulesPatch{},
		}},
	}
	err := scenario.Check()
	require.Error(t, err)
}

func TestParseBytes_FullBlackoutScenario(t *testing.T) {
	input := `
Name: Single Proposer Blackout
InitialNetworkRules:
  Upgrades:
    Sonic: true
    Allegro: true
    SingleProposerBlockFormation: true
  Epochs:
    MaxEpochDuration: 1000s
Scenario:
  - startNode: validator
    type: validator
    instances: 2
  - runApp: load
    type: counter
    users: 50
    rate:
      constant: 50
  - startNode: validator-before-1
    type: validator
  - advanceEpoch
  - startNode: validator-before-2
    type: validator
  - advanceEpoch
  - checks:
      - blocksProduced
  - stopNode: validator-before-1
  - stopNode: validator-before-2
  - checks:
      - blocksHalted
  - startNode: validator-before-1
    type: validator
  - startNode: validator-before-2
    type: validator
  - advanceEpoch
  - checks:
      - blocksProduced
      - blockHeights
      - blockHashes
  - stopApp: load
`
	scenario, err := ParseBytes([]byte(input))
	require.NoError(t, err)
	require.Equal(t, "Single Proposer Blackout", scenario.Name)
	require.Len(t, scenario.Steps, 18)

	// Verify the rejoin pattern: validator-before-1 is started, stopped, then started again
	starts := 0
	for _, step := range scenario.Steps {
		if start, ok := step.(*nodelifetime.StartNode); ok && start.Target() == "validator-before-1" {
			starts++
		}
	}
	require.Equal(t, 2, starts)
}

func TestParseFile_NonExistentFile(t *testing.T) {
	_, err := ParseFile("/non/existent/path.yml")
	require.Error(t, err)
}

func TestParseFile_InvalidContent(t *testing.T) {
	// Write invalid YAML to a temp file.
	path := t.TempDir() + "/bad.yml"
	require.NoError(t, os.WriteFile(path, []byte(":::not valid yaml"), 0644))
	_, err := ParseFile(path)
	require.Error(t, err)
}

func TestParseBytes_InvalidParamForFunction(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name: "users on startNode",
			input: `
Name: Test
Scenario:
  - startNode: node-A
    type: validator
    users: 10
`,
		},
		{
			name: "rate on startNode",
			input: `
Name: Test
Scenario:
  - startNode: node-A
    type: validator
    rate:
      constant: 5
`,
		},
		{
			name: "instances on runApp",
			input: `
Name: Test
Scenario:
  - runApp: load
    type: counter
    instances: 3
    rate:
      constant: 5
`,
		},
		{
			name: "tolerance on stopNode",
			input: `
Name: Test
Scenario:
  - stopNode: node-A
    tolerance: 5
`,
		},
		{
			name: "ceiling on checks blocks produced",
			input: `
Name: Test
Scenario:
  - checks:
    blocksProduced:
    ceiling: 100
`,
		},
		{
			name: "type on advanceEpoch",
			input: `
Name: Test
Scenario:
  - advanceEpoch:
    type: validator
`,
		},
		{
			name: "imageName on runApp",
			input: `
Name: Test
Scenario:
  - runApp: load
    type: counter
    imageName: "sonic:v2.0.0"
    rate:
      constant: 5
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseBytes([]byte(tt.input))
			require.Error(t, err)
			require.Contains(t, err.Error(), "is not valid for")
		})
	}
}

func TestParseBytes_RejectsAParameterGivenTwice(t *testing.T) {
	input := `
Name: Test
Scenario:
  - startNode: node-A
    type: validator
    type: observer
`
	_, err := ParseBytes([]byte(input))
	require.ErrorContains(t, err, `line 6: parameter "type" is given twice`)
}

func TestParseBytes_RejectsUnknownKeysInsideParameterValues(t *testing.T) {
	input := `
Name: Test
Scenario:
  - runApp: load
    type: counter
    rate:
      constant: 5
      constnat: 50
`
	_, err := ParseBytes([]byte(input))
	require.ErrorContains(t, err, "line 6: invalid rate value: field constnat not found")
}

// The checker compares against a whole number; a fraction would be truncated.
func TestParseBytes_RejectsAFractionalGasRateCeiling(t *testing.T) {
	input := `
Name: Test
Scenario:
  - checks:
      - blockGasRate:
          ceiling: 0.9
`
	scenario, err := ParseBytes([]byte(input))
	require.NoError(t, err)
	err = scenario.Check()
	require.ErrorContains(t, err, "ceiling must be a non-negative whole number")
	require.ErrorContains(t, err, "got 0.9")
}

// startNode's failing marks a diverging node; only for checks does it turn a
// pass into a failure.
func TestParams_DescribeFailingForWhatItMeansToEachFunction(t *testing.T) {
	helpOf := func(fn steps.Function) string {
		for _, p := range steps.Params(fn) {
			if p.Key == "failing" {
				return p.Help
			}
		}
		t.Fatalf("%s has no failing parameter", fn.Name())
		return ""
	}
	require.Contains(t, helpOf(&nodelifetime.StartNode{}), "diverge")
	require.NotContains(t, helpOf(&nodelifetime.StartNode{}), "treated as an error")
	require.Contains(t, helpOf(&checks.BlockHashes{}), "treated as an error")
}

func TestParseBytes_RejectsUnknownInitialNetworkRules(t *testing.T) {
	input := `
Name: Test
InitialNetworkRules:
  Blocks:
    MaxBlokGas: 1
Scenario:
  - advanceEpoch
`
	_, err := ParseBytes([]byte(input))
	require.ErrorContains(t, err, "field MaxBlokGas not found")
}

// Parameter values are decoded strictly apart from the document; an alias to
// an anchor in an earlier step still resolves.
func TestParseBytes_ResolvesAliasesAcrossSteps(t *testing.T) {
	input := `
Name: Test
Scenario:
  - startNode: val
    type: validator
    imageName: &image "sonic:v2.1.6"
  - startNode: obs
    imageName: *image
`
	scenario, err := ParseBytes([]byte(input))
	require.NoError(t, err)
	require.Equal(t, "sonic:v2.1.6", stepAs[*nodelifetime.StartNode](t, scenario.Steps[1]).ImageName)
}

func TestParseBytes_NonScalarStringParam(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name: "type as list",
			input: `
Name: Test
Scenario:
  - startNode: node-A
    type: [validator, observer]
`,
		},
		{
			name: "imageName as mapping",
			input: `
Name: Test
Scenario:
  - startNode: node-A
    type: validator
    imageName:
      name: sonic
      version: v2.0.0
`,
		},
		{
			name: "dataVolume as list",
			input: `
Name: Test
Scenario:
  - startNode: node-A
    type: validator
    dataVolume: [vol-1, vol-2]
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseBytes([]byte(tt.input))
			require.Error(t, err)
		})
	}
}

func TestParseBytes_TypeOrderIndependent(t *testing.T) {
	// type listed before the function key — should still be parsed correctly.
	input := `
Name: Order Test
Scenario:
  - type: counter
    runApp: my-app
    users: 5
    rate:
      constant: 10
`
	scenario, err := ParseBytes([]byte(input))
	require.NoError(t, err)

	step := stepAs[*apps.RunApp](t, scenario.Steps[0])
	require.Equal(t, "counter", step.Type)
	require.Equal(t, "my-app", step.Target())
}

func TestParseBytes_RulesPatchAllCategories(t *testing.T) {
	input := `
Name: Full Patch Test
InitialNetworkRules:
  Epochs:
    MaxEpochGas: 5000000000
    MaxEpochDuration: 30s
  Blocks:
    MaxBlockGas: 20500000000
    MaxEmptyBlockSkipPeriod: 3s
  Economy:
    MinBaseFee: "1000000000"
    MinGasPrice: "500000000"
  Upgrades:
    Sonic: true
    Allegro: true
    Brio: false
Scenario:
  - startNode: validator
    type: validator
  - updateRules:
      Blocks:
        MaxBlockGas: 30000000000
      Economy:
        MinBaseFee: "2000000000"
      Upgrades:
        Brio: true
`
	scenario, err := ParseBytes([]byte(input))
	require.NoError(t, err)

	r := scenario.InitialRules

	// Epochs
	require.NotNil(t, r.Epochs)
	if got, want := *r.Epochs.MaxEpochGas, uint64(5000000000); got != want {
		require.Failf(t, "unexpected MaxEpochGas", "MaxEpochGas: got %d, want %d", got, want)
	}
	if got, want := int64(*r.Epochs.MaxEpochDuration), int64(30*time.Second); got != want {
		require.Failf(t, "unexpected MaxEpochDuration", "MaxEpochDuration: got %d, want %d", got, want)
	}

	// Blocks
	require.NotNil(t, r.Blocks)
	if got, want := *r.Blocks.MaxBlockGas, uint64(20500000000); got != want {
		require.Failf(t, "unexpected MaxBlockGas", "MaxBlockGas: got %d, want %d", got, want)
	}
	if got, want := int64(*r.Blocks.MaxEmptyBlockSkipPeriod), int64(3*time.Second); got != want {
		require.Failf(t, "unexpected MaxEmptyBlockSkipPeriod", "MaxEmptyBlockSkipPeriod: got %d, want %d", got, want)
	}

	// Economy (BigIntValue fields)
	require.NotNil(t, r.Economy)
	require.NotNil(t, r.Economy.MinBaseFee)
	require.NotNil(t, r.Economy.MinGasPrice)

	// Upgrades
	require.NotNil(t, r.Upgrades)
	require.NotNil(t, r.Upgrades.Sonic)
	require.True(t, *r.Upgrades.Sonic)
	require.NotNil(t, r.Upgrades.Allegro)
	require.True(t, *r.Upgrades.Allegro)
	require.NotNil(t, r.Upgrades.Brio)
	require.False(t, *r.Upgrades.Brio)

	// Unset categories should be nil
	require.Nil(t, r.Dag)
	require.Nil(t, r.Emitter)

	// Verify updateRules step
	step := stepAs[*network.UpdateRules](t, scenario.Steps[1])
	require.NotNil(t, step.Rules.Blocks)
	require.NotNil(t, step.Rules.Blocks.MaxBlockGas)
	require.EqualValues(t, 30000000000, *step.Rules.Blocks.MaxBlockGas)
	require.NotNil(t, step.Rules.Economy)
	require.NotNil(t, step.Rules.Economy.MinBaseFee)
	require.NotNil(t, step.Rules.Upgrades)
	require.NotNil(t, step.Rules.Upgrades.Brio)
	require.True(t, *step.Rules.Upgrades.Brio)
	// Fields not in the update should remain nil
	require.Nil(t, step.Rules.Epochs)
}

func TestCheck_EmptySubChecks(t *testing.T) {
	scenario := Scenario{
		Name:        "Test",
		Description: "A test scenario.",
		Steps:       Steps{&checks.Checks{}},
	}
	err := scenario.Check()
	require.Error(t, err)
	require.Contains(t, err.Error(), "at least one sub-check")
}

func TestCheck_ObservationWindowTooShort(t *testing.T) {
	tooShort := checking.MinObservationDuration - time.Millisecond
	err := checkScenario(&checks.BlocksProduced{
		Duration: durationOf(tooShort),
	}).Check()
	require.Error(t, err)
	require.Contains(t, err.Error(), "duration must be at least")
}

func TestCheck_ObservationWindowAtTheMinimumIsAccepted(t *testing.T) {
	require.NoError(t, checkScenario(&checks.BlocksProduced{
		Duration: durationOf(checking.MinObservationDuration),
	}).Check())
}

// checkScenario wraps a single sub-check into an otherwise valid scenario.
func checkScenario(check checks.Check) *Scenario {
	return &Scenario{
		Name:        "Test",
		Description: "A test scenario.",
		Steps:       Steps{&checks.Checks{Items: []checks.Check{check}}},
	}
}

// windowChecks builds each check that observes the network over a window.
var windowChecks = map[string]func(checks.Window) checks.Check{
	"blocksProduced": func(w checks.Window) checks.Check { return &checks.BlocksProduced{Window: w} },
	"blocksHalted":   func(w checks.Window) checks.Check { return &checks.BlocksHalted{Window: w} },
	"blockGasRate": func(w checks.Window) checks.Check {
		ceiling := 1e6
		return &checks.BlockGasRate{Ceiling: &ceiling, Window: w}
	},
}

func TestCheck_ToleranceTooShortForAnObservationWindow(t *testing.T) {
	// tolerance is the window most scenarios actually use, so it must be
	// rejected here rather than minutes into the run, just like duration.
	for name, newCheck := range windowChecks {
		t.Run(name, func(t *testing.T) {
			for _, tolerance := range []int{0, 1} {
				err := checkScenario(newCheck(checks.Window{Tolerance: &tolerance})).Check()
				require.Error(t, err, "tolerance %d", tolerance)
				require.Contains(t, err.Error(), "tolerance is an observation window")
			}
		})
	}
}

func TestCheck_ToleranceAtTheMinimumIsAccepted(t *testing.T) {
	minimum := checking.MinObservationSamples
	require.NoError(t, checkScenario(&checks.BlocksProduced{
		Tolerance: &minimum,
	}).Check())
}

func TestCheck_ShortToleranceIsAcceptedWhenDurationOverridesIt(t *testing.T) {
	// duration wins over tolerance at run time, so a small tolerance next to a
	// usable duration never reaches the checker.
	tolerance := 1
	require.NoError(t, checkScenario(&checks.BlocksProduced{
		Tolerance: &tolerance,
		Duration:  durationOf(checking.MinObservationDuration),
	}).Check())
}

func TestCheck_ZeroToleranceIsAcceptedForAHeightCheck(t *testing.T) {
	// For blockHeights, tolerance is a slack in blocks; demanding exact
	// agreement is legitimate.
	zero := 0
	require.NoError(t, checkScenario(&checks.BlockHeights{Tolerance: &zero}).Check())
}

func TestCheck_ShortDurationIsAcceptedAsAConvergenceBudget(t *testing.T) {
	// For blockHeights and networkRules, duration bounds how long the nodes are
	// given to agree, not how long the network is observed. Nothing has to
	// collect two monitoring samples, so a short budget - or none at all,
	// meaning a single attempt - is a legitimate choice and must not be
	// rejected by the observation-window floor.
	convergenceChecks := map[string]func(checks.Convergence) checks.Check{
		"blockHeights": func(c checks.Convergence) checks.Check { return &checks.BlockHeights{Convergence: c} },
		"networkRules": func(c checks.Convergence) checks.Check {
			maxBlockGas := uint64(1)
			return &checks.NetworkRules{
				Rules:       genesis.NetworkRulesPatch{Blocks: &genesis.BlocksPatch{MaxBlockGas: &maxBlockGas}},
				Convergence: c,
			}
		},
	}
	for name, newCheck := range convergenceChecks {
		t.Run(name, func(t *testing.T) {
			for _, duration := range []time.Duration{
				0, time.Millisecond, checking.MinObservationDuration - time.Millisecond,
			} {
				require.NoError(t, checkScenario(newCheck(checks.Convergence{
					Duration: durationOf(duration),
				})).Check(), "duration %s", duration)
			}
		})
	}
}

func TestCheck_ShortDurationIsRejectedForAnObservationWindow(t *testing.T) {
	// The counterpart to the above: where duration really is the window, it
	// still has to be long enough to hold two samples.
	tooShort := checking.MinObservationDuration - time.Millisecond
	for name, newCheck := range windowChecks {
		t.Run(name, func(t *testing.T) {
			err := checkScenario(newCheck(checks.Window{Duration: durationOf(tooShort)})).Check()
			require.Error(t, err)
			require.Contains(t, err.Error(), "duration must be at least")
		})
	}
}

func TestParseBytes_UnknownCheckFunction(t *testing.T) {
	input := `
Name: Bad Check
Scenario:
  - checks:
      - unknownCheck
`
	_, err := ParseBytes([]byte(input))
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown check function")
}

func TestParseBytes_ReadsTheBlockCount_WhenWaitingForBlocks(t *testing.T) {
	input := `
Name: Wait For Blocks Test
Description: t
Scenario:
  - waitForBlocks: 1_100
`
	scenario, err := ParseBytes([]byte(input))
	require.NoError(t, err)
	step := stepAs[*wait.WaitForBlocks](t, scenario.Steps[0])
	require.Equal(t, uint64(1100), step.Blocks)
}

func TestParseBytes_Fails_WhenTheBlockCountIsInvalid(t *testing.T) {
	cases := map[string]string{
		"missing":  "",
		"word":     "ten",
		"negative": "-5",
		"float":    "1.5",
	}
	for name, value := range cases {
		t.Run(name, func(t *testing.T) {
			input := "Name: T\nDescription: t\nScenario:\n" +
				"  - waitForBlocks: " + value + "\n"
			_, err := ParseBytes([]byte(input))
			require.Error(t, err)
		})
	}
}

func TestCheck_Fails_WhenTheBlockCountIsZero(t *testing.T) {
	scenario := Scenario{
		Name:        "Test",
		Description: "A test scenario.",
		Steps:       Steps{&wait.WaitForBlocks{}},
	}
	require.ErrorContains(t, scenario.Check(), "positive block count")
}

func TestParseBytes_StopSonic(t *testing.T) {
	scenario, err := ParseBytes([]byte(`
Name: Test
Description: Stops a client gracefully.
Scenario:
  - stopSonic: node-A
`))
	require.NoError(t, err)
	require.NoError(t, scenario.Check())
	require.Equal(t, "node-A", stepAs[*nodelifetime.StopSonic](t, scenario.Steps[0]).Target())

	scenario, err = ParseBytes([]byte(`
Name: Test
Scenario:
  - stopSonic
`))
	require.NoError(t, err)
	require.ErrorContains(t, scenario.Check(), "requires a node identifier")
}

func TestParseBytes_WaitForSonicExit(t *testing.T) {
	scenario, err := ParseBytes([]byte(`
Name: Test
Description: A client that stops itself.
Scenario:
  - startNode: exporter
    type: observer
    extraArguments: "--exitwhensynced.epoch 3"
  - waitForSonicExit: exporter
`))
	require.NoError(t, err)
	require.NoError(t, scenario.Check())
	require.Equal(t, "exporter", stepAs[*wait.WaitForSonicExit](t, scenario.Steps[1]).Target())

	scenario, err = ParseBytes([]byte(`
Name: Test
Scenario:
  - waitForSonicExit
`))
	require.NoError(t, err)
	require.ErrorContains(t, scenario.Check(), "requires a node identifier")
}

func TestParseBytes_DecodesEveryParameter_OfAPrepareNodeStep(t *testing.T) {
	input := `
Name: Bootstrap Test
Description: Bootstraps a fresh node from a g-file.
Scenario:
  - prepareNode: fresh-old
    type: observer
    imageName: "sonic:v2.2.0"
    file: from-new.g

  - checkDb: fresh-old

  - startNode: fresh-old
    type: observer
`
	scenario, err := ParseBytes([]byte(input))
	require.NoError(t, err)
	require.NoError(t, scenario.Check())

	step := stepAs[*nodelifetime.PrepareNode](t, scenario.Steps[0])
	require.Equal(t, "fresh-old", step.Target())
	require.Equal(t, "observer", step.Type)
	require.Equal(t, "sonic:v2.2.0", step.ImageName)
	require.Equal(t, "from-new.g", step.File)
}

func TestParseBytes_EventsSteps(t *testing.T) {
	input := `
Name: Events Test
Description: Moves an event DAG between two nodes.
Scenario:
  - exportEvents: exporter
    file: from-old.events

  - importEvents: fresh
    file: from-old.events
`
	scenario, err := ParseBytes([]byte(input))
	require.NoError(t, err)
	require.NoError(t, scenario.Check())

	exported := stepAs[*sonictool.ExportEvents](t, scenario.Steps[0])
	require.Equal(t, "exporter", exported.Target())
	require.Equal(t, "from-old.events", exported.File)

	imported := stepAs[*sonictool.ImportEvents](t, scenario.Steps[1])
	require.Equal(t, "fresh", imported.Target())
	require.Equal(t, "from-old.events", imported.File)
}

func TestParseBytes_FileManagementSteps(t *testing.T) {
	input := `
Name: Genesis Exchange Test
Description: Swaps g-files between two nodes.
Scenario:
  - startNode: node-A
    type: observer

  - stopSonic: node-A

  - exportGenesis: node-A
    file: from-a.g

  - importGenesis: node-A
    file: from-b.g

  - checkDb: node-A
    mode: archive

  - checkDb: node-A
`
	scenario, err := ParseBytes([]byte(input))
	require.NoError(t, err)
	require.NoError(t, scenario.Check())

	all := scenario.Steps
	require.Equal(t, "node-A", stepAs[*nodelifetime.StopSonic](t, all[1]).Target())

	exported := stepAs[*sonictool.ExportGenesis](t, all[2])
	require.Equal(t, "node-A", exported.Target())
	require.Equal(t, "from-a.g", exported.File)

	require.Equal(t, "from-b.g", stepAs[*sonictool.ImportGenesis](t, all[3]).File)

	require.Equal(t, node.DbCheckModeArchive, stepAs[*checks.CheckDb](t, all[4]).Mode)

	// An omitted mode is left to the node to default.
	require.Empty(t, stepAs[*checks.CheckDb](t, all[5]).Mode)
}

func TestParseBytes_FileManagementSteps_RejectInvalidInput(t *testing.T) {
	tests := map[string]string{
		"file on a step that takes none": `
Name: Test
Scenario:
  - stopSonic: node-A
    file: from-a.g
`,
		"mode on exportGenesis": `
Name: Test
Scenario:
  - exportGenesis: node-A
    mode: live
`,
		"file as a list": `
Name: Test
Scenario:
  - exportGenesis: node-A
    file: [a.g, b.g]
`,
	}

	for name, input := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := ParseBytes([]byte(input))
			require.Error(t, err)
		})
	}
}

// A bad file name is refused when the scenario loads, not minutes into a
// run.
func TestScenario_Check_FileManagementSteps(t *testing.T) {
	tests := map[string]struct {
		input     string
		wantError string
	}{
		"export events without a file": {
			input: `
Name: Test
Description: Test
Scenario:
  - exportEvents: node-A
`,
			wantError: "requires a 'file' parameter",
		},
		"import events with a path in the file name": {
			input: `
Name: Test
Description: Test
Scenario:
  - importEvents: node-A
    file: /etc/passwd
`,
			wantError: "file name must match",
		},
		"prepare without a file": {
			input: `
Name: Test
Description: Test
Scenario:
  - prepareNode: fresh-old
    type: observer
`,
			wantError: "requires a 'file' parameter",
		},
		"prepare a validator": {
			input: `
Name: Test
Description: Test
Scenario:
  - prepareNode: fresh-val
    type: validator
    file: from-new.g
`,
			wantError: "cannot prepare a validator",
		},
		"prepare with a path in the file name": {
			input: `
Name: Test
Description: Test
Scenario:
  - prepareNode: fresh-old
    type: observer
    file: ../escape.g
`,
			wantError: "file name must match",
		},
		"export without a file": {
			input: `
Name: Test
Description: Test
Scenario:
  - exportGenesis: node-A
`,
			wantError: "requires a 'file' parameter",
		},
		"import without a file": {
			input: `
Name: Test
Description: Test
Scenario:
  - importGenesis: node-A
    file: ""
`,
			wantError: "requires a 'file' parameter",
		},
		"file in a subdirectory": {
			input: `
Name: Test
Description: Test
Scenario:
  - exportGenesis: node-A
    file: sub/from-a.g
`,
			wantError: "file name must match",
		},
		"file escaping the shared directory": {
			input: `
Name: Test
Description: Test
Scenario:
  - importGenesis: node-A
    file: ../from-a.g
`,
			wantError: "file name must match",
		},
		"file that looks like a flag": {
			input: `
Name: Test
Description: Test
Scenario:
  - importGenesis: node-A
    file: --datadir
`,
			wantError: "file name must match",
		},
		"unknown check mode": {
			input: `
Name: Test
Description: Test
Scenario:
  - checkDb: node-A
    mode: both
`,
			wantError: "checkDb mode must be",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			scenario, err := ParseBytes([]byte(tt.input))
			require.NoError(t, err)
			err = scenario.Check()
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.wantError)
		})
	}
}

// A step the help output does not describe cannot be discovered by scenario
// authors.
func TestPrintHelp_CoversTheFileManagementSteps(t *testing.T) {
	var out strings.Builder
	require.NoError(t, PrintHelp(&out))

	for _, name := range []string{
		"stopSonic", "exportGenesis", "importGenesis", "checkDb",
	} {
		require.Contains(t, out.String(), name)
	}
	require.Contains(t, out.String(), "shared directory")
}
