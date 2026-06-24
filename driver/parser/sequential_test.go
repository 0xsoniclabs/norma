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
	"os"
	"testing"
)

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
	if len(scenario.Steps) != 5 {
		t.Fatalf("expected 5 steps, got %d", len(scenario.Steps))
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
  UPGRADES_SONIC: "true"
  UPGRADES_ALLEGRO: "true"
  MAX_EPOCH_DURATION: 10s
Scenario:
  - startNode: validator
    type: validator
`
	scenario, err := ParseSequentialBytes([]byte(input))
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	if scenario.InitialRules["UPGRADES_SONIC"] != "true" {
		t.Errorf("expected UPGRADES_SONIC=true, got %q", scenario.InitialRules["UPGRADES_SONIC"])
	}
	if scenario.InitialRules["UPGRADES_ALLEGRO"] != "true" {
		t.Errorf("expected UPGRADES_ALLEGRO=true, got %q", scenario.InitialRules["UPGRADES_ALLEGRO"])
	}
	if scenario.InitialRules["MAX_EPOCH_DURATION"] != "10s" {
		t.Errorf("expected MAX_EPOCH_DURATION=10s, got %q", scenario.InitialRules["MAX_EPOCH_DURATION"])
	}
}

func TestParseSequential_UpdateRules(t *testing.T) {
	input := `
Name: Update Rules Test
Scenario:
  - startNode: validator
    type: validator
  - updateRules:
      MIN_BASE_FEE: "3000000000"
      MAX_BLOCK_GAS: "100000"
`
	scenario, err := ParseSequentialBytes([]byte(input))
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	if len(scenario.Steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(scenario.Steps))
	}

	step := scenario.Steps[1]
	if step.Function != FuncUpdateRules {
		t.Errorf("expected FuncUpdateRules, got %q", step.Function)
	}
	if step.Rules["MIN_BASE_FEE"] != "3000000000" {
		t.Errorf("expected MIN_BASE_FEE=3000000000, got %q", step.Rules["MIN_BASE_FEE"])
	}
	if step.Rules["MAX_BLOCK_GAS"] != "100000" {
		t.Errorf("expected MAX_BLOCK_GAS=100000, got %q", step.Rules["MAX_BLOCK_GAS"])
	}
}

func TestParseSequential_StartNodeWithOptions(t *testing.T) {
	input := `
Name: Node Options Test
Scenario:
  - startNode: my-node
    type: validator
    imagename: "sonic:v2.0.2"
    instances: 3
    datavolume: "vol-A"
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
  - checkBlocksProduced: my-node
  - checkBlocksHalted:
    failing: true
  - checkBlockHeights:
    tolerance: 5
  - checkBlockGasRate:
    ceiling: 16500000
    failing: true
  - checkBlockHashes:
`
	scenario, err := ParseSequentialBytes([]byte(input))
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	if len(scenario.Steps) != 5 {
		t.Fatalf("expected 5 steps, got %d", len(scenario.Steps))
	}

	// checkBlocksProduced with identifier
	step := scenario.Steps[0]
	if step.Function != FuncCheckBlocksProduced {
		t.Errorf("step 0: expected FuncCheckBlocksProduced, got %q", step.Function)
	}
	if step.Identifier != "my-node" {
		t.Errorf("step 0: expected identifier 'my-node', got %q", step.Identifier)
	}

	// checkBlocksHalted with failing
	step = scenario.Steps[1]
	if step.Function != FuncCheckBlocksHalted {
		t.Errorf("step 1: expected FuncCheckBlocksHalted, got %q", step.Function)
	}
	if !step.Failing {
		t.Errorf("step 1: expected failing=true")
	}

	// checkBlockHeights with tolerance
	step = scenario.Steps[2]
	if step.Function != FuncCheckBlockHeights {
		t.Errorf("step 2: expected FuncCheckBlockHeights, got %q", step.Function)
	}
	if step.Tolerance == nil || *step.Tolerance != 5 {
		t.Errorf("step 2: expected tolerance=5")
	}

	// checkBlockGasRate with ceiling + failing
	step = scenario.Steps[3]
	if step.Function != FuncCheckBlockGasRate {
		t.Errorf("step 3: expected FuncCheckBlockGasRate, got %q", step.Function)
	}
	if step.Ceiling == nil || *step.Ceiling != 16500000 {
		t.Errorf("step 3: expected ceiling=16500000")
	}
	if !step.Failing {
		t.Errorf("step 3: expected failing=true")
	}

	// checkBlockHashes
	step = scenario.Steps[4]
	if step.Function != FuncCheckBlockHashes {
		t.Errorf("step 4: expected FuncCheckBlockHashes, got %q", step.Function)
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

	if scenario.InitialRules["MAX_EPOCH_DURATION"] != "15s" {
		t.Errorf("expected default MAX_EPOCH_DURATION=15s, got %q", scenario.InitialRules["MAX_EPOCH_DURATION"])
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
		Name:    "",
		Steps:   []Step{{Function: FuncAdvanceEpoch}},
	}
	err := scenario.Check()
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestSequentialCheck_InvalidNodeName(t *testing.T) {
	scenario := SequentialScenario{
		Name:    "Test",
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
		Name:    "Test",
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
		Name:    "Test",
		Steps: []Step{{
			Function: FuncUpdateRules,
			Rules:    map[string]string{},
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
  UPGRADES_SONIC: "true"
  UPGRADES_ALLEGRO: "true"
  UPGRADES_SINGLE_PROPOSER: "true"
  MAX_EPOCH_DURATION: 1000s
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
  - checkBlocksProduced
  - stopNode: validator-before-1
  - stopNode: validator-before-2
  - checkBlocksHalted
  - startNode: validator-before-1
    type: validator
  - startNode: validator-before-2
    type: validator
  - advanceEpoch
  - checkBlocksProduced
  - checkBlockHeights
  - checkBlockHashes
  - stopApp: load
`
	scenario, err := ParseSequentialBytes([]byte(input))
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	if scenario.Name != "Single Proposer Blackout" {
		t.Errorf("wrong name: %q", scenario.Name)
	}
	if len(scenario.Steps) != 17 {
		t.Errorf("expected 17 steps, got %d", len(scenario.Steps))
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
