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
)

func TestAllStepFunctionsAreDocumented(t *testing.T) {
	for _, fn := range allStepFunctions {
		if desc, ok := stepFunctionDescriptions[fn]; !ok || strings.TrimSpace(desc) == "" {
			t.Errorf("step function %q has no entry in stepFunctionDescriptions", fn)
		}
		if _, ok := allowedParams[fn]; !ok {
			t.Errorf("step function %q has no entry in allowedParams", fn)
		}
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
		if desc, ok := paramDescriptions[p]; !ok || strings.TrimSpace(desc) == "" {
			t.Errorf("parameter %q has no entry in paramDescriptions", p)
		}
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
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	if scenario.Name != "Minimal Test" {
		t.Errorf("expected name 'Minimal Test', got %q", scenario.Name)
	}
	if len(scenario.Steps) != 9 {
		t.Fatalf("expected 9 steps, got %d", len(scenario.Steps))
	}

	// Verify step 1: startNode
	step := scenario.Steps[0]
	if step.Function != FuncStartNode {
		t.Errorf("step 0: expected FuncStartNode, got %q", step.Function)
	}
	if step.Identifier != "validator-A" {
		t.Errorf("step 0: expected identifier 'validator-A', got %q", step.Identifier)
	}
	if step.NodeType != "validator" {
		t.Errorf("step 0: expected node type 'validator', got %q", step.NodeType)
	}

	// Verify step 2: runApp
	step = scenario.Steps[1]
	if step.Function != FuncRunApp {
		t.Errorf("step 1: expected FuncRunApp, got %q", step.Function)
	}
	if step.Identifier != "load" {
		t.Errorf("step 1: expected identifier 'load', got %q", step.Identifier)
	}
	if step.AppType != "counter" {
		t.Errorf("step 1: expected app type 'counter', got %q", step.AppType)
	}
	if step.Users == nil || *step.Users != 10 {
		t.Errorf("step 1: expected users=10")
	}
	if step.Rate == nil || step.Rate.Constant == nil || *step.Rate.Constant != 5 {
		t.Errorf("step 1: expected constant rate=5")
	}

	// Verify step 3: advanceEpoch
	step = scenario.Steps[2]
	if step.Function != FuncAdvanceEpoch {
		t.Errorf("step 2: expected FuncAdvanceEpoch, got %q", step.Function)
	}

	// Verify step 4: stopApp
	step = scenario.Steps[3]
	if step.Function != FuncStopApp {
		t.Errorf("step 3: expected FuncStopApp, got %q", step.Function)
	}
	if step.Identifier != "load" {
		t.Errorf("step 3: expected identifier 'load', got %q", step.Identifier)
	}

	// Verify step 5: stopNode
	step = scenario.Steps[4]
	if step.Function != FuncStopNode {
		t.Errorf("step 4: expected FuncStopNode, got %q", step.Function)
	}
	if step.Identifier != "validator-A" {
		t.Errorf("step 4: expected identifier 'validator-A', got %q", step.Identifier)
	}
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
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	if scenario.InitialRules.Upgrades == nil || scenario.InitialRules.Upgrades.Sonic == nil || !*scenario.InitialRules.Upgrades.Sonic {
		t.Errorf("expected Upgrades.Sonic=true")
	}
	if scenario.InitialRules.Upgrades == nil || scenario.InitialRules.Upgrades.Allegro == nil || !*scenario.InitialRules.Upgrades.Allegro {
		t.Errorf("expected Upgrades.Allegro=true")
	}
	if scenario.InitialRules.Epochs == nil || scenario.InitialRules.Epochs.MaxEpochDuration == nil ||
		int64(*scenario.InitialRules.Epochs.MaxEpochDuration) != int64(10*time.Second) {
		t.Errorf("expected Epochs.MaxEpochDuration=10s")
	}
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
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	if len(scenario.Steps) != 6 {
		t.Fatalf("expected 6 steps, got %d", len(scenario.Steps))
	}

	step := scenario.Steps[1]
	if step.Function != FuncUpdateRules {
		t.Errorf("expected FuncUpdateRules, got %q", step.Function)
	}
	if step.Rules.Economy == nil || step.Rules.Economy.MinBaseFee == nil {
		t.Fatal("expected Economy.MinBaseFee to be set")
	}
	if got := big.Int(*step.Rules.Economy.MinBaseFee); got.Cmp(big.NewInt(3000000000)) != 0 {
		t.Errorf("expected Economy.MinBaseFee=3000000000, got %s", got.String())
	}
	if step.Rules.Blocks == nil || step.Rules.Blocks.MaxBlockGas == nil || *step.Rules.Blocks.MaxBlockGas != 100000 {
		t.Errorf("expected Blocks.MaxBlockGas=100000")
	}
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
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	step := scenario.Steps[0]
	if step.Identifier != "my-node" {
		t.Errorf("expected identifier 'my-node', got %q", step.Identifier)
	}
	if step.NodeType != "validator" {
		t.Errorf("expected type 'validator', got %q", step.NodeType)
	}
	if step.ImageName != "sonic:v2.0.2" {
		t.Errorf("expected imagename 'sonic:v2.0.2', got %q", step.ImageName)
	}
	if step.Instances == nil || *step.Instances != 3 {
		t.Errorf("expected instances=3")
	}
	if step.DataVolume != "vol-A" {
		t.Errorf("expected datavolume 'vol-A', got %q", step.DataVolume)
	}
	if !step.Failing {
		t.Errorf("expected failing=true")
	}
}

func TestParseSequential_Undelegate(t *testing.T) {
	input := `
Name: Stop Node Test
Scenario:
  - undelegate: validator-A
`
	scenario, err := ParseSequentialBytes([]byte(input))
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	step := scenario.Steps[0]
	if step.Function != FuncUndelegate {
		t.Errorf("expected FuncUndelegate, got %q", step.Function)
	}
	if step.Identifier != "validator-A" {
		t.Errorf("expected identifier 'validator-A', got %q", step.Identifier)
	}
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
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	if len(scenario.Steps) != 5 {
		t.Fatalf("expected 5 steps, got %d", len(scenario.Steps))
	}

	step := scenario.Steps[0]
	if step.Function != FuncChecks {
		t.Errorf("expected FuncChecks, got %q", step.Function)
	}
	if len(step.SubChecks) != 4 {
		t.Fatalf("expected 4 sub-checks, got %d", len(step.SubChecks))
	}

	// blocksHalted with failing
	check := step.SubChecks[0]
	if check.Function != FuncCheckBlocksHalted {
		t.Errorf("check 0: expected FuncCheckBlocksHalted, got %q", check.Function)
	}
	if !check.Failing {
		t.Errorf("check 0: expected failing=true")
	}

	// blockHeights with tolerance
	check = step.SubChecks[1]
	if check.Function != FuncCheckBlockHeights {
		t.Errorf("check 1: expected FuncCheckBlockHeights, got %q", check.Function)
	}
	if check.Tolerance == nil || *check.Tolerance != 5 {
		t.Errorf("check 1: expected tolerance=5")
	}

	// blockGasRate with ceiling + failing
	check = step.SubChecks[2]
	if check.Function != FuncCheckBlockGasRate {
		t.Errorf("check 2: expected FuncCheckBlockGasRate, got %q", check.Function)
	}
	if check.Ceiling == nil || *check.Ceiling != 16500000 {
		t.Errorf("check 2: expected ceiling=16500000")
	}
	if !check.Failing {
		t.Errorf("check 2: expected failing=true")
	}

	// blockHashes
	check = step.SubChecks[3]
	if check.Function != FuncCheckBlockHashes {
		t.Errorf("check 3: expected FuncCheckBlockHashes, got %q", check.Function)
	}
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
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	step := scenario.Steps[0]
	if step.Rate == nil {
		t.Fatal("expected rate to be set")
	}
	if step.Rate.Slope == nil {
		t.Fatal("expected slope rate")
	}
	if step.Rate.Slope.Start != 20 {
		t.Errorf("expected slope start=20, got %f", step.Rate.Slope.Start)
	}
	if step.Rate.Slope.Increment != 5 {
		t.Errorf("expected slope increment=5, got %f", step.Rate.Slope.Increment)
	}
}

func TestParseSequential_DefaultsApplied(t *testing.T) {
	input := `
Name: Defaults Test
Scenario:
  - advanceEpoch
`
	scenario, err := ParseSequentialBytes([]byte(input))
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	if scenario.InitialRules.Epochs == nil || scenario.InitialRules.Epochs.MaxEpochDuration == nil ||
		int64(*scenario.InitialRules.Epochs.MaxEpochDuration) != int64(15*time.Second) {
		t.Errorf("expected default Epochs.MaxEpochDuration=15s")
	}

	if len(scenario.Steps) != 5 {
		t.Fatalf("expected 5 steps with default implicit end-checks, got %d", len(scenario.Steps))
	}

	if scenario.Steps[1].Function != FuncAdvanceEpoch {
		t.Errorf("expected implicit step 2 to be advanceEpoch, got %q", scenario.Steps[1].Function)
	}
	if scenario.Steps[2].Function != FuncAdvanceEpoch {
		t.Errorf("expected implicit step 3 to be advanceEpoch, got %q", scenario.Steps[2].Function)
	}
	if scenario.Steps[3].Function != FuncCheckBlockHashes {
		t.Errorf("expected implicit step 4 to be checkBlockHashes, got %q", scenario.Steps[3].Function)
	}
	if scenario.Steps[4].Function != FuncCheckBlockHeights {
		t.Errorf("expected implicit step 5 to be checkBlockHeights, got %q", scenario.Steps[4].Function)
	}
}

func TestParseSequential_DisableEndChecks(t *testing.T) {
	input := `
Name: Disable End Checks Test
DisableEndChecks: true
Scenario:
  - advanceEpoch
`
	scenario, err := ParseSequentialBytes([]byte(input))
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	if len(scenario.Steps) != 1 {
		t.Fatalf("expected 1 step when DisableEndChecks=true, got %d", len(scenario.Steps))
	}
}

func TestParseSequential_UnknownFunction(t *testing.T) {
	input := `
Name: Bad Function
Scenario:
  - Do something weird: foo
`
	_, err := ParseSequentialBytes([]byte(input))
	if err == nil {
		t.Fatal("expected parse error for unknown function")
	}
}

func TestSequentialCheck_EmptyName(t *testing.T) {
	scenario := SequentialScenario{
		Name:  "",
		Steps: []Step{{Function: FuncAdvanceEpoch}},
	}
	err := scenario.Check()
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestSequentialCheck_InvalidNodeName(t *testing.T) {
	scenario := SequentialScenario{
		Name: "Test",
		Steps: []Step{{
			Function:   FuncStartNode,
			Identifier: "invalid name with spaces",
			NodeType:   "validator",
		}},
	}
	err := scenario.Check()
	if err == nil {
		t.Fatal("expected error for invalid node name")
	}
}

func TestSequentialCheck_RunAppMissingRate(t *testing.T) {
	scenario := SequentialScenario{
		Name: "Test",
		Steps: []Step{{
			Function:   FuncRunApp,
			Identifier: "load",
			AppType:    "counter",
		}},
	}
	err := scenario.Check()
	if err == nil {
		t.Fatal("expected error for missing rate")
	}
}

func TestSequentialCheck_UpdateRulesEmpty(t *testing.T) {
	scenario := SequentialScenario{
		Name: "Test",
		Steps: []Step{{
			Function: FuncUpdateRules,
			Rules:    genesis.NetworkRulesPatch{},
		}},
	}
	err := scenario.Check()
	if err == nil {
		t.Fatal("expected error for empty rules")
	}
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
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	if scenario.Name != "Single Proposer Blackout" {
		t.Errorf("wrong name: %q", scenario.Name)
	}
	if len(scenario.Steps) != 19 {
		t.Errorf("expected 19 steps, got %d", len(scenario.Steps))
	}

	// Verify the rejoin pattern: validator-before-1 is started, stopped, then started again
	starts := 0
	for _, step := range scenario.Steps {
		if step.Function == FuncStartNode && step.Identifier == "validator-before-1" {
			starts++
		}
	}
	if starts != 2 {
		t.Errorf("expected validator-before-1 started 2 times (start + rejoin), got %d", starts)
	}
}

func TestParseSequentialFile_NonExistentFile(t *testing.T) {
	_, err := ParseSequentialFile("/non/existent/path.yml")
	if err == nil {
		t.Fatal("expected error for non-existent file")
	}
}

func TestParseSequentialFile_InvalidContent(t *testing.T) {
	// Write invalid YAML to a temp file.
	path := t.TempDir() + "/bad.yml"
	if err := os.WriteFile(path, []byte(":::not valid yaml"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := ParseSequentialFile(path)
	if err == nil {
		t.Fatal("expected error for invalid YAML file")
	}
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
			if err == nil {
				t.Fatalf("expected error for %s, but parsing succeeded", tt.name)
			}
			if !strings.Contains(err.Error(), "is not valid for") {
				t.Fatalf("expected 'is not valid for' error, got: %v", err)
			}
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
			if err == nil {
				t.Fatalf("expected error for non-scalar %s, but parsing succeeded", tt.name)
			}
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
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	step := scenario.Steps[0]
	if step.Function != FuncRunApp {
		t.Errorf("expected FuncRunApp, got %q", step.Function)
	}
	if step.AppType != "counter" {
		t.Errorf("expected app type 'counter', got %q", step.AppType)
	}
	if step.NodeType != "" {
		t.Errorf("expected empty NodeType, got %q", step.NodeType)
	}
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
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	r := scenario.InitialRules

	// Epochs
	if r.Epochs == nil {
		t.Fatal("expected Epochs to be set")
	}
	if got, want := *r.Epochs.MaxEpochGas, uint64(5000000000); got != want {
		t.Errorf("MaxEpochGas: got %d, want %d", got, want)
	}
	if got, want := int64(*r.Epochs.MaxEpochDuration), int64(30*time.Second); got != want {
		t.Errorf("MaxEpochDuration: got %d, want %d", got, want)
	}

	// Blocks
	if r.Blocks == nil {
		t.Fatal("expected Blocks to be set")
	}
	if got, want := *r.Blocks.MaxBlockGas, uint64(20500000000); got != want {
		t.Errorf("MaxBlockGas: got %d, want %d", got, want)
	}
	if got, want := int64(*r.Blocks.MaxEmptyBlockSkipPeriod), int64(3*time.Second); got != want {
		t.Errorf("MaxEmptyBlockSkipPeriod: got %d, want %d", got, want)
	}

	// Economy (BigIntValue fields)
	if r.Economy == nil || r.Economy.MinBaseFee == nil {
		t.Fatal("expected Economy.MinBaseFee to be set")
	}
	if r.Economy.MinGasPrice == nil {
		t.Fatal("expected Economy.MinGasPrice to be set")
	}

	// Upgrades
	if r.Upgrades == nil {
		t.Fatal("expected Upgrades to be set")
	}
	if r.Upgrades.Sonic == nil || !*r.Upgrades.Sonic {
		t.Error("expected Upgrades.Sonic=true")
	}
	if r.Upgrades.Allegro == nil || !*r.Upgrades.Allegro {
		t.Error("expected Upgrades.Allegro=true")
	}
	if r.Upgrades.Brio == nil || *r.Upgrades.Brio {
		t.Error("expected Upgrades.Brio=false")
	}

	// Unset categories should be nil
	if r.Dag != nil {
		t.Error("expected Dag to be nil")
	}
	if r.Emitter != nil {
		t.Error("expected Emitter to be nil")
	}

	// Verify updateRules step
	step := scenario.Steps[1]
	if step.Function != FuncUpdateRules {
		t.Fatalf("expected FuncUpdateRules, got %q", step.Function)
	}
	if step.Rules.Blocks == nil || *step.Rules.Blocks.MaxBlockGas != 30000000000 {
		t.Error("expected updateRules Blocks.MaxBlockGas=30000000000")
	}
	if step.Rules.Economy == nil || step.Rules.Economy.MinBaseFee == nil {
		t.Error("expected updateRules Economy.MinBaseFee to be set")
	}
	if step.Rules.Upgrades == nil || step.Rules.Upgrades.Brio == nil || !*step.Rules.Upgrades.Brio {
		t.Error("expected updateRules Upgrades.Brio=true")
	}
	// Fields not in the update should remain nil
	if step.Rules.Epochs != nil {
		t.Error("expected updateRules Epochs to be nil")
	}
}

func TestSequentialCheck_EmptySubChecks(t *testing.T) {
	scenario := SequentialScenario{
		Name: "Test",
		Steps: []Step{{
			Function:  FuncChecks,
			SubChecks: nil,
		}},
	}
	err := scenario.Check()
	if err == nil {
		t.Fatal("expected error for empty sub-checks")
	}
	if !strings.Contains(err.Error(), "at least one sub-check") {
		t.Fatalf("expected 'at least one sub-check' error, got: %v", err)
	}
}

func TestParseSequential_UnknownCheckFunction(t *testing.T) {
	input := `
Name: Bad Check
Scenario:
  - checks:
      - unknownCheck
`
	_, err := ParseSequentialBytes([]byte(input))
	if err == nil {
		t.Fatal("expected error for unknown check function")
	}
	if !strings.Contains(err.Error(), "unknown check function") {
		t.Fatalf("expected 'unknown check function' error, got: %v", err)
	}
}
