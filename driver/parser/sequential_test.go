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
	"strings"
	"testing"
	"time"

	"github.com/0xsoniclabs/norma/genesis"
	"github.com/stretchr/testify/require"
)

func TestAllStepFunctionsAreDocumented(t *testing.T) {
	for _, fn := range allStepFunctions {
		desc, ok := stepFunctionDescriptions[fn]
		require.Truef(t, ok, "step function %q has no entry in stepFunctionDescriptions", fn)
		require.NotEmptyf(t, strings.TrimSpace(desc), "step function %q has empty description", fn)

		_, ok = allowedParams[fn]
		require.Truef(t, ok, "step function %q has no entry in allowedParams", fn)
	}
}

func TestAllParamKeysAreDocumented(t *testing.T) {
	seen := map[string]bool{}
	for _, params := range allowedParams {
		for _, p := range params {
			seen[p] = true
		}
	}
	for p := range seen {
		desc, ok := paramDescriptions[p]
		require.Truef(t, ok, "parameter %q has no entry in paramDescriptions", p)
		require.NotEmptyf(t, strings.TrimSpace(desc), "parameter %q has empty description", p)
	}
}

func TestParseSequential_MinimalScenario(t *testing.T) {
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
	scenario, err := ParseSequentialBytes([]byte(input))
	require.NoError(t, err)
	require.Equal(t, "Minimal Test", scenario.Name)
	require.Len(t, scenario.Steps, 8)

	// Verify step 1: startNode
	step := scenario.Steps[0]
	require.Equal(t, FuncStartNode, step.Function)
	require.Equal(t, "validator-A", step.Identifier)
	require.Equal(t, "validator", step.NodeType)

	// Verify step 2: runApp
	step = scenario.Steps[1]
	require.Equal(t, FuncRunApp, step.Function)
	require.Equal(t, "load", step.Identifier)
	require.Equal(t, "counter", step.AppType)
	require.NotNil(t, step.Users)
	require.Equal(t, 10, *step.Users)
	require.NotNil(t, step.Rate)
	require.NotNil(t, step.Rate.Constant)
	require.EqualValues(t, 5, *step.Rate.Constant)

	// Verify step 3: advanceEpoch
	step = scenario.Steps[2]
	require.Equal(t, FuncAdvanceEpoch, step.Function)

	// Verify step 4: stopApp
	step = scenario.Steps[3]
	require.Equal(t, FuncStopApp, step.Function)
	require.Equal(t, "load", step.Identifier)

	// Verify step 5: stopNode
	step = scenario.Steps[4]
	require.Equal(t, FuncStopNode, step.Function)
	require.Equal(t, "validator-A", step.Identifier)
}

func TestParseSequential_InitialRules(t *testing.T) {
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
	scenario, err := ParseSequentialBytes([]byte(input))
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

func TestParseSequential_UpdateRules(t *testing.T) {
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
	scenario, err := ParseSequentialBytes([]byte(input))
	require.NoError(t, err)
	require.Len(t, scenario.Steps, 5)

	step := scenario.Steps[1]
	require.Equal(t, FuncUpdateRules, step.Function)
	require.NotNil(t, step.Rules.Economy)
	require.NotNil(t, step.Rules.Economy.MinBaseFee)
	if got := big.Int(*step.Rules.Economy.MinBaseFee); got.Cmp(big.NewInt(3000000000)) != 0 {
		require.Failf(t, "unexpected min base fee", "expected Economy.MinBaseFee=3000000000, got %s", got.String())
	}
	require.NotNil(t, step.Rules.Blocks)
	require.NotNil(t, step.Rules.Blocks.MaxBlockGas)
	require.EqualValues(t, 100000, *step.Rules.Blocks.MaxBlockGas)
}

func TestParseSequential_StartNodeWithOptions(t *testing.T) {
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
	scenario, err := ParseSequentialBytes([]byte(input))
	require.NoError(t, err)

	step := scenario.Steps[0]
	require.Equal(t, "my-node", step.Identifier)
	require.Equal(t, "validator", step.NodeType)
	require.Equal(t, "sonic:v2.0.2", step.ImageName)
	require.NotNil(t, step.Instances)
	require.Equal(t, 3, *step.Instances)
	require.Equal(t, "vol-A", step.DataVolume)
	require.True(t, step.Failing)
}

func TestParseSequential_Undelegate(t *testing.T) {
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
	scenario, err := ParseSequentialBytes([]byte(input))
	require.NoError(t, err)

	step := scenario.Steps[0]
	require.Equal(t, FuncStartNode, step.Function)
	require.Equal(t, "validator-A", step.Identifier)

	step = scenario.Steps[1]
	require.Equal(t, FuncUndelegate, step.Function)
	require.Len(t, step.UndelegateTargets, 1)
	require.Equal(t, "validator-A", step.UndelegateTargets[0].Node)
	require.Nil(t, step.UndelegateTargets[0].Stake)

	step = scenario.Steps[2]
	require.Equal(t, FuncUndelegate, step.Function)
	require.Len(t, step.UndelegateTargets, 1)
	require.Equal(t, "validator-A", step.UndelegateTargets[0].Node)
	require.NotNil(t, step.UndelegateTargets[0].Stake)
	require.EqualValues(t, 2_000_000, *step.UndelegateTargets[0].Stake)
}

func TestParseSequential_Checks(t *testing.T) {
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
	scenario, err := ParseSequentialBytes([]byte(input))
	require.NoError(t, err)
	require.Len(t, scenario.Steps, 4)

	step := scenario.Steps[0]
	require.Equal(t, FuncChecks, step.Function)
	require.Len(t, step.SubChecks, 4)

	// blocksHalted with failing
	check := step.SubChecks[0]
	require.Equal(t, FuncCheckBlocksHalted, check.Function)
	require.True(t, check.Failing)

	// blockHeights with tolerance
	check = step.SubChecks[1]
	require.Equal(t, FuncCheckBlockHeights, check.Function)
	require.NotNil(t, check.Tolerance)
	require.Equal(t, 5, *check.Tolerance)

	// blockGasRate with ceiling + failing
	check = step.SubChecks[2]
	require.Equal(t, FuncCheckBlockGasRate, check.Function)
	require.NotNil(t, check.Ceiling)
	require.EqualValues(t, 16500000, *check.Ceiling)
	require.True(t, check.Failing)

	// blockHashes
	check = step.SubChecks[3]
	require.Equal(t, FuncCheckBlockHashes, check.Function)
}

func TestParseSequential_CheckParams_NestedForm(t *testing.T) {
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
	scenario, err := ParseSequentialBytes([]byte(input))
	require.NoError(t, err)
	step := scenario.Steps[0]
	require.Equal(t, FuncChecks, step.Function)
	require.Len(t, step.SubChecks, 2)

	et := step.SubChecks[0]
	require.Equal(t, FuncCheckEventThrottled, et.Function)
	require.True(t, et.Failing)
	require.Equal(
		t, []string{"validator-dominant", "validator-B"},
		et.ThrottledNodes,
	)

	gr := step.SubChecks[1]
	require.Equal(t, FuncCheckBlockGasRate, gr.Function)
	require.NotNil(t, gr.Ceiling)
	require.EqualValues(t, 1000, *gr.Ceiling)
	require.True(t, gr.Failing)
}

func TestParseSequential_CheckParams_RejectsUnknown(t *testing.T) {
	input := `
Name: Unknown Param Test
Scenario:
  - checks:
      - eventThrottled:
          bogus: 42
`
	_, err := ParseSequentialBytes([]byte(input))
	require.Error(t, err)
	require.Contains(t, err.Error(), `parameter "bogus" is not valid`)
}

func TestParseSequential_CheckParams_RejectsWrongParamForFunction(t *testing.T) {
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
			_, err := ParseSequentialBytes([]byte(input))
			require.Error(t, err)
			require.Contains(t, err.Error(), "is not valid for check")
		})
	}
}

func TestAllCheckFunctionsAreDocumented(t *testing.T) {
	for _, fn := range allCheckFunctions {
		desc, ok := checkFunctionDescriptions[fn]
		require.Truef(t, ok, "check function %q has no entry in checkFunctionDescriptions", fn)
		require.NotEmptyf(t, strings.TrimSpace(desc), "check function %q has empty description", fn)

		_, ok = checkFunctionParams[fn]
		require.Truef(t, ok, "check function %q has no entry in checkFunctionParams", fn)
	}
}

func TestAllCheckParamKeysAreDocumented(t *testing.T) {
	seen := map[string]bool{}
	for _, params := range checkFunctionParams {
		for _, p := range params {
			seen[p] = true
		}
	}
	for p := range seen {
		desc, ok := checkParamDescriptions[p]
		require.Truef(t, ok, "check parameter %q has no entry in checkParamDescriptions", p)
		require.NotEmptyf(t, strings.TrimSpace(desc), "check parameter %q has empty description", p)
	}
}

func TestParseSequential_ThrottledNodes_FlatForm(t *testing.T) {
	input := `
Name: Flat Throttled Nodes Test
Scenario:
  - checks:
      - eventThrottled:
        failing: true
        throttledNodes:
          - validator-A
`
	scenario, err := ParseSequentialBytes([]byte(input))
	require.NoError(t, err)
	step := scenario.Steps[0]
	require.Equal(t, FuncChecks, step.Function)
	require.Len(t, step.SubChecks, 1)
	et := step.SubChecks[0]
	require.Equal(t, FuncCheckEventThrottled, et.Function)
	require.True(t, et.Failing)
	require.Equal(t, []string{"validator-A"}, et.ThrottledNodes)
}

func TestParseSequential_SlopeRate(t *testing.T) {
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
	scenario, err := ParseSequentialBytes([]byte(input))
	require.NoError(t, err)

	step := scenario.Steps[0]
	require.NotNil(t, step.Rate)
	require.NotNil(t, step.Rate.Slope)
	require.EqualValues(t, 20, step.Rate.Slope.Start)
	require.EqualValues(t, 5, step.Rate.Slope.Increment)
}

func TestParseSequential_DefaultsApplied(t *testing.T) {
	input := `
Name: Defaults Test
Scenario:
  - advanceEpoch
`
	scenario, err := ParseSequentialBytes([]byte(input))
	require.NoError(t, err)
	require.NotNil(t, scenario.InitialRules.Epochs)
	require.NotNil(t, scenario.InitialRules.Epochs.MaxEpochDuration)
	require.Equal(
		t,
		int64(15*time.Second),
		int64(*scenario.InitialRules.Epochs.MaxEpochDuration),
	)

	require.Len(t, scenario.Steps, 4)
	require.Equal(t, FuncAdvanceEpoch, scenario.Steps[1].Function)
	require.Equal(t, FuncAdvanceEpoch, scenario.Steps[2].Function)
	require.Equal(t, FuncChecks, scenario.Steps[3].Function)
}

func TestParseSequential_DisableEndChecks(t *testing.T) {
	input := `
Name: Disable End Checks Test
DisableEndChecks: true
Scenario:
  - advanceEpoch
`
	scenario, err := ParseSequentialBytes([]byte(input))
	require.NoError(t, err)
	require.Len(t, scenario.Steps, 1)
}

func TestParseSequential_UnknownFunction(t *testing.T) {
	input := `
Name: Bad Function
Scenario:
  - Do something weird: foo
`
	_, err := ParseSequentialBytes([]byte(input))
	require.Error(t, err)
}

func TestSequentialCheck_EmptyName(t *testing.T) {
	scenario := SequentialScenario{
		Name:        "",
		Description: "A test scenario.",
		Steps:       []Step{{Function: FuncAdvanceEpoch}},
	}
	err := scenario.Check()
	require.Error(t, err)
}

func TestSequentialCheck_EmptyDescription(t *testing.T) {
	scenario := SequentialScenario{
		Name:        "Test",
		Description: "",
		Steps:       []Step{{Function: FuncAdvanceEpoch}},
	}
	err := scenario.Check()
	require.Error(t, err)
}

func TestSequentialCheck_InvalidNodeName(t *testing.T) {
	scenario := SequentialScenario{
		Name:        "Test",
		Description: "A test scenario.",
		Steps: []Step{{
			Function:   FuncStartNode,
			Identifier: "invalid name with spaces",
			NodeType:   "validator",
		}},
	}
	err := scenario.Check()
	require.Error(t, err)
}

func TestSequentialCheck_RunAppMissingRate(t *testing.T) {
	scenario := SequentialScenario{
		Name:        "Test",
		Description: "A test scenario.",
		Steps: []Step{{
			Function:   FuncRunApp,
			Identifier: "load",
			AppType:    "counter",
		}},
	}
	err := scenario.Check()
	require.Error(t, err)
}

func TestSequentialCheck_UpdateRulesEmpty(t *testing.T) {
	scenario := SequentialScenario{
		Name:        "Test",
		Description: "A test scenario.",
		Steps: []Step{{
			Function: FuncUpdateRules,
			Rules:    genesis.NetworkRulesPatch{},
		}},
	}
	err := scenario.Check()
	require.Error(t, err)
}

func TestParseSequential_FullBlackoutScenario(t *testing.T) {
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
	scenario, err := ParseSequentialBytes([]byte(input))
	require.NoError(t, err)
	require.Equal(t, "Single Proposer Blackout", scenario.Name)
	require.Len(t, scenario.Steps, 18)

	// Verify the rejoin pattern: validator-before-1 is started, stopped, then started again
	starts := 0
	for _, step := range scenario.Steps {
		if step.Function == FuncStartNode && step.Identifier == "validator-before-1" {
			starts++
		}
	}
	require.Equal(t, 2, starts)
}

func TestParseSequentialFile_NonExistentFile(t *testing.T) {
	_, err := ParseSequentialFile("/non/existent/path.yml")
	require.Error(t, err)
}

func TestParseSequentialFile_InvalidContent(t *testing.T) {
	// Write invalid YAML to a temp file.
	path := t.TempDir() + "/bad.yml"
	require.NoError(t, os.WriteFile(path, []byte(":::not valid yaml"), 0644))
	_, err := ParseSequentialFile(path)
	require.Error(t, err)
}

func TestParseSequential_InvalidParamForFunction(t *testing.T) {
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
			_, err := ParseSequentialBytes([]byte(tt.input))
			require.Error(t, err)
			require.Contains(t, err.Error(), "is not valid for")
		})
	}
}

func TestParseSequential_NonScalarStringParam(t *testing.T) {
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
			_, err := ParseSequentialBytes([]byte(tt.input))
			require.Error(t, err)
		})
	}
}

func TestParseSequential_TypeOrderIndependent(t *testing.T) {
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
	scenario, err := ParseSequentialBytes([]byte(input))
	require.NoError(t, err)

	step := scenario.Steps[0]
	require.Equal(t, FuncRunApp, step.Function)
	require.Equal(t, "counter", step.AppType)
	require.Empty(t, step.NodeType)
}

func TestParseSequential_RulesPatchAllCategories(t *testing.T) {
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
	scenario, err := ParseSequentialBytes([]byte(input))
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
	step := scenario.Steps[1]
	require.Equal(t, FuncUpdateRules, step.Function)
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

func TestSequentialCheck_EmptySubChecks(t *testing.T) {
	scenario := SequentialScenario{
		Name:        "Test",
		Description: "A test scenario.",
		Steps: []Step{{
			Function:  FuncChecks,
			SubChecks: nil,
		}},
	}
	err := scenario.Check()
	require.Error(t, err)
	require.Contains(t, err.Error(), "at least one sub-check")
}

func TestParseSequential_UnknownCheckFunction(t *testing.T) {
	input := `
Name: Bad Check
Scenario:
  - checks:
      - unknownCheck
`
	_, err := ParseSequentialBytes([]byte(input))
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown check function")
}
