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
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/0xsoniclabs/norma/genesis"
	"gopkg.in/yaml.v3"
)

// StepFunction identifies the type of operation a step performs.
type StepFunction string

const (
	FuncStartNode    StepFunction = "startNode"
	FuncStopNode     StepFunction = "stopNode"
	FuncUndelegate   StepFunction = "undelegate"
	FuncUpdateRules  StepFunction = "updateRules"
	FuncAdvanceEpoch StepFunction = "advanceEpoch"
	FuncWaitForEpoch StepFunction = "waitForEpoch"
	FuncRunApp       StepFunction = "runApp"
	FuncStopApp      StepFunction = "stopApp"
	FuncChecks       StepFunction = "checks"
	FuncWaitFor      StepFunction = "waitFor"

	// Check functions used as items inside a checks: step.
	FuncCheckBlockGasRate   StepFunction = "blockGasRate"
	FuncCheckBlockHashes    StepFunction = "blockHashes"
	FuncCheckBlockHeights   StepFunction = "blockHeights"
	FuncCheckBlocksHalted   StepFunction = "blocksHalted"
	FuncCheckBlocksProduced StepFunction = "blocksProduced"
	FuncCheckNetworkRules   StepFunction = "networkRules"
)

// allStepFunctions lists every known top-level step function constant.
var allStepFunctions = [...]StepFunction{
	FuncStartNode,
	FuncStopNode,
	FuncUndelegate,
	FuncUpdateRules,
	FuncAdvanceEpoch,
	FuncWaitForEpoch,
	FuncRunApp,
	FuncStopApp,
	FuncChecks,
	FuncWaitFor,
}

// allCheckFunctions lists every check function valid as a sub-item of a checks: step.
var allCheckFunctions = [...]StepFunction{
	FuncCheckBlockGasRate,
	FuncCheckBlockHashes,
	FuncCheckBlockHeights,
	FuncCheckBlocksHalted,
	FuncCheckBlocksProduced,
	FuncCheckNetworkRules,
}

// toStepFunction returns the StepFunction for a given string, or an error if not recognized.
func toStepFunction(s string) (StepFunction, error) {
	for _, fn := range allStepFunctions {
		if string(fn) == s {
			return fn, nil
		}
	}
	return "", fmt.Errorf("unknown function: %q", s)
}

// toCheckFunction returns the StepFunction for a given check function string, or an error.
func toCheckFunction(s string) (StepFunction, error) {
	for _, fn := range allCheckFunctions {
		if string(fn) == s {
			return fn, nil
		}
	}
	return "", fmt.Errorf("unknown check function: %q", s)
}

// CheckSpec represents a single check inside a checks: step.
type CheckSpec struct {
	Function  StepFunction
	Ceiling   *float64
	Tolerance *int
	Failing   bool
	Rules     genesis.NetworkRulesPatch
}

// UnmarshalYAML implements custom YAML unmarshalling for CheckSpec.
// A check can be either a plain function name string or a mapping with
// the function name as key and optional parameters as siblings.
func (c *CheckSpec) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		fn, err := toCheckFunction(value.Value)
		if err != nil {
			return fmt.Errorf("line %d: %w", value.Line, err)
		}
		c.Function = fn
		return nil
	case yaml.MappingNode:
		return c.unmarshalCheckMapping(value)
	default:
		return fmt.Errorf("line %d: check must be a string or mapping", value.Line)
	}
}

func (c *CheckSpec) unmarshalCheckMapping(node *yaml.Node) error {
	for i := 0; i < len(node.Content); i += 2 {
		keyNode := node.Content[i]
		valNode := node.Content[i+1]
		key := keyNode.Value

		if fn, err := toCheckFunction(key); err == nil {
			c.Function = fn
			continue
		}

		switch key {
		case "tolerance":
			var v int
			if err := valNode.Decode(&v); err != nil {
				return fmt.Errorf("line %d: invalid tolerance: %w", keyNode.Line, err)
			}
			c.Tolerance = &v
		case "ceiling":
			var v float64
			if err := valNode.Decode(&v); err != nil {
				return fmt.Errorf("line %d: invalid ceiling: %w", keyNode.Line, err)
			}
			c.Ceiling = &v
		case "failing":
			var v bool
			if err := valNode.Decode(&v); err != nil {
				return fmt.Errorf("line %d: invalid failing: %w", keyNode.Line, err)
			}
			c.Failing = v
		case "rules":
			var patch genesis.NetworkRulesPatch
			if err := valNode.Decode(&patch); err != nil {
				return fmt.Errorf("line %d: invalid rules: %w", keyNode.Line, err)
			}
			c.Rules = patch
		default:
			return fmt.Errorf("line %d: unknown check parameter %q", keyNode.Line, key)
		}
	}
	if c.Function == "" {
		return fmt.Errorf("line %d: no known check function found in mapping", node.Line)
	}
	return nil
}

// SequentialScenario is the root element of a sequential scenario description.
// Unlike the time-based Scenario, it defines an ordered list of blocking steps.
type SequentialScenario struct {
	Name             string                    `yaml:"Name"`
	Description      string                    `yaml:"Description"`
	InitialRules     genesis.NetworkRulesPatch `yaml:"InitialNetworkRules"`
	DisableEndChecks bool                      `yaml:"DisableEndChecks,omitempty"`
	Steps            []Step                    `yaml:"Scenario"`
}

// Step is a single blocking operation in a sequential scenario.
type Step struct {
	// Function identifies which operation this step performs.
	Function StepFunction

	// Identifier is the main argument (node name, app name, check target).
	Identifier string

	// Node parameters
	NodeType   string // "validator", "observer", "rpc"
	ImageName  string
	DataVolume string
	Stake      *uint64
	Instances  *int
	Failing    bool

	// App parameters
	AppType string
	Users   *int
	Rate    *Rate

	// Update rules parameters
	Rules genesis.NetworkRulesPatch

	// Checks step parameters
	SubChecks []CheckSpec

	// WaitFor parameters
	Duration time.Duration
}

// UnmarshalYAML implements custom YAML unmarshalling for Step.
// A step can be either:
//   - A scalar string (e.g., "advanceEpoch")
//   - A mapping where one key is the function name (e.g., "startNode: validator-A")
func (s *Step) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		fn, err := toStepFunction(value.Value)
		if err != nil {
			return fmt.Errorf("line %d: %w", value.Line, err)
		}
		s.Function = fn
		return nil
	case yaml.MappingNode:
		return s.unmarshalMapping(value)
	default:
		return fmt.Errorf("line %d: step must be a string or mapping, got kind %d", value.Line, value.Kind)
	}
}

// unmarshalMapping parses a mapping node into a Step.
// It identifies the function key and parses the remaining keys as parameters.
func (s *Step) unmarshalMapping(node *yaml.Node) error {
	// First pass: find the function key so that parameter parsing (e.g. "type")
	// can depend on which function this step represents.
	for i := 0; i < len(node.Content); i += 2 {
		keyNode := node.Content[i]
		valNode := node.Content[i+1]
		key := keyNode.Value

		if fn, err := toStepFunction(key); err == nil {
			if s.Function != "" {
				return fmt.Errorf("line %d: step contains multiple function keys: %q and %q", keyNode.Line, s.Function, fn)
			}
			s.Function = fn
			if err := s.parseFunctionValue(fn, valNode); err != nil {
				return fmt.Errorf("line %d: %w", valNode.Line, err)
			}
		}
	}

	if s.Function == "" {
		return fmt.Errorf("line %d: no known function found in step mapping", node.Line)
	}

	// Second pass: parse parameters now that s.Function is known.
	for i := 0; i < len(node.Content); i += 2 {
		keyNode := node.Content[i]
		valNode := node.Content[i+1]
		key := keyNode.Value

		// Skip function keys (already handled above).
		if _, err := toStepFunction(key); err == nil {
			continue
		}

		if err := s.parseParam(key, valNode); err != nil {
			return fmt.Errorf("line %d: %w", keyNode.Line, err)
		}
	}

	return nil
}

// parseFunctionValue parses the value associated with the function key.
func (s *Step) parseFunctionValue(fn StepFunction, val *yaml.Node) error {
	switch fn {
	case FuncUpdateRules:
		// Value is a NetworkRulesPatch mapping.
		if val.Kind == yaml.MappingNode {
			var patch genesis.NetworkRulesPatch
			if err := val.Decode(&patch); err != nil {
				return fmt.Errorf("invalid updateRules value: %w", err)
			}
			s.Rules = patch
		} else if val.Tag != "!!null" && val.Value != "" {
			return fmt.Errorf("updateRules value must be a mapping, got %q", val.Value)
		}
	case FuncAdvanceEpoch, FuncWaitForEpoch:
		// These take no value (or null).
		if val.Kind != yaml.ScalarNode || (val.Tag != "!!null" && val.Value != "" && val.Value != "null") {
			return fmt.Errorf("%s does not take a value, got %q", fn, val.Value)
		}
	case FuncWaitFor:
		// Value is a duration string (e.g., "1s", "5m", "1h").
		if val.Kind != yaml.ScalarNode || val.Value == "" {
			return fmt.Errorf("waitFor requires a duration value (e.g., \"1s\", \"5m\")")
		}
		d, err := time.ParseDuration(val.Value)
		if err != nil {
			return fmt.Errorf("invalid duration %q: %w", val.Value, err)
		}
		if d <= 0 {
			return fmt.Errorf("waitFor duration must be positive, got %s", d)
		}
		s.Duration = d
	case FuncChecks:
		// Value is a sequence of check specifications.
		if val.Kind == yaml.SequenceNode {
			var specs []CheckSpec
			if err := val.Decode(&specs); err != nil {
				return fmt.Errorf("invalid checks list: %w", err)
			}
			s.SubChecks = specs
		} else if val.Tag != "!!null" && val.Value != "" {
			return fmt.Errorf("checks requires a list of check functions, got %q", val.Value)
		}
	default:
		// Value is a string identifier (or null/empty for optional).
		if val.Kind == yaml.ScalarNode && val.Tag != "!!null" && val.Value != "" {
			s.Identifier = val.Value
		}
	}
	return nil
}

// stepFunctionDescriptions provides a human-readable description for each step function.
var stepFunctionDescriptions = map[StepFunction]string{
	FuncStartNode:    "Start a new network node (validator, observer, or rpc).",
	FuncStopNode:     "Stop a running network node by name.",
	FuncUndelegate:   "Undelegate stake from a validator node.",
	FuncUpdateRules:  "Update one or more network rules (key/value pairs).",
	FuncAdvanceEpoch: "Advance the network to the next epoch by sending transactions.",
	FuncWaitForEpoch: "Wait until the network reaches the next epoch boundary.",
	FuncRunApp:       "Start a load-generating application.",
	FuncStopApp:      "Stop a running load-generating application by name.",
	FuncChecks:       "Run one or more checks (see 'Available checks' below).",
	FuncWaitFor:      "Pause scenario execution for a fixed duration.",
}

// paramDescriptions provides a human-readable description for each parameter key.
var paramDescriptions = map[string]string{
	"type":       "Node type (\"validator\", \"observer\", \"rpc\") for startNode; application type for runApp.",
	"imageName":  "Docker image name to use for the node.",
	"dataVolume": "Docker volume name to mount as the node data directory.",
	"stake":      "Initial stake amount (uint64) for a validator node.",
	"instances":  "Number of node instances to start.",
	"failing":    "When true, the step is expected to fail; a passing result is treated as an error.",
	"users":      "Number of concurrent user accounts the application should simulate.",
	"rate":       "Transaction rate configuration for the application.",
}

// allowedParams defines which parameter keys are valid for each step function.
var allowedParams = map[StepFunction][]string{
	FuncStartNode:    {"type", "imageName", "dataVolume", "stake", "instances", "failing"},
	FuncStopNode:     {},
	FuncRunApp:       {"type", "users", "rate"},
	FuncStopApp:      {},
	FuncUpdateRules:  {},
	FuncUndelegate:   {},
	FuncAdvanceEpoch: {},
	FuncWaitForEpoch: {},
	FuncWaitFor:      {},
	FuncChecks:       {},
}

// parseParam parses a single parameter key-value pair.
func (s *Step) parseParam(key string, val *yaml.Node) error {
	// Validate that this parameter is allowed for the current step function.
	allowed, known := allowedParams[s.Function]
	if !known || !slices.Contains(allowed, key) {
		return fmt.Errorf("parameter %q is not valid for %s", key, s.Function)
	}

	switch key {
	case "type":
		var v string
		if err := val.Decode(&v); err != nil {
			return fmt.Errorf("invalid type value: %w", err)
		}
		switch s.Function {
		case FuncRunApp:
			s.AppType = v
		default:
			s.NodeType = v
		}
	case "imageName":
		var v string
		if err := val.Decode(&v); err != nil {
			return fmt.Errorf("invalid imageName value: %w", err)
		}
		s.ImageName = v
	case "dataVolume":
		var v string
		if err := val.Decode(&v); err != nil {
			return fmt.Errorf("invalid dataVolume value: %w", err)
		}
		s.DataVolume = v
	case "stake":
		var v uint64
		if err := val.Decode(&v); err != nil {
			return fmt.Errorf("invalid stake value: %w", err)
		}
		s.Stake = &v
	case "instances":
		var v int
		if err := val.Decode(&v); err != nil {
			return fmt.Errorf("invalid instances value: %w", err)
		}
		s.Instances = &v
	case "failing":
		var v bool
		if err := val.Decode(&v); err != nil {
			return fmt.Errorf("invalid failing value: %w", err)
		}
		s.Failing = v
	case "users":
		var v int
		if err := val.Decode(&v); err != nil {
			return fmt.Errorf("invalid users value: %w", err)
		}
		s.Users = &v
	case "rate":
		var r Rate
		if err := val.Decode(&r); err != nil {
			return fmt.Errorf("invalid rate value: %w", err)
		}
		s.Rate = &r
	}
	return nil
}

// ParseSequential parses a sequential scenario from the given reader.
func ParseSequential(reader io.Reader) (SequentialScenario, error) {
	var res SequentialScenario
	decoder := yaml.NewDecoder(reader)
	decoder.KnownFields(true)
	err := decoder.Decode(&res)
	if err != nil {
		return SequentialScenario{}, err
	}
	res.setDefaults()
	return res, nil
}

// ParseSequentialBytes parses a sequential scenario from a byte slice.
func ParseSequentialBytes(data []byte) (SequentialScenario, error) {
	return ParseSequential(bytes.NewReader(data))
}

// ParseSequentialFile parses a sequential scenario from a file.
func ParseSequentialFile(path string) (scenario SequentialScenario, err error) {
	reader, err := os.Open(path)
	if err != nil {
		return SequentialScenario{}, err
	}
	defer func() { err = errors.Join(err, reader.Close()) }()
	return ParseSequential(reader)
}

// setDefaults sets default values on the sequential scenario.
func (s *SequentialScenario) setDefaults() {
	ensureDefaultEpochDuration(&s.InitialRules)

	// if the scenario does not disable end checks, append the default end
	// checks to the steps list
	if !s.DisableEndChecks {
		s.Steps = append(s.Steps,
			Step{Function: FuncAdvanceEpoch},
			Step{Function: FuncAdvanceEpoch},
			Step{
				Function: FuncChecks,
				SubChecks: []CheckSpec{
					{Function: FuncCheckBlockHashes},
					{Function: FuncCheckBlockHeights},
				},
			},
		)
	}
}

// checkFunctionDescriptions provides a human-readable description for each sub-check function.
var checkFunctionDescriptions = map[StepFunction]string{
	FuncCheckBlockGasRate:   "Assert that the block gas rate is at or below a ceiling.",
	FuncCheckBlockHashes:    "Assert that all nodes agree on the same block hashes.",
	FuncCheckBlockHeights:   "Assert that all nodes are within tolerance of the same block height.",
	FuncCheckBlocksHalted:   "Assert that block production has halted.",
	FuncCheckBlocksProduced: "Assert that all nodes have produced blocks within tolerance.",
	FuncCheckNetworkRules:   "Assert that the active network rules on all nodes match the expected rules patch.",
}

// checkFunctionParams lists the optional parameters accepted by each sub-check function.
var checkFunctionParams = map[StepFunction][]string{
	FuncCheckBlockGasRate:   {"ceiling", "failing"},
	FuncCheckBlockHashes:    {"failing"},
	FuncCheckBlockHeights:   {"tolerance", "failing"},
	FuncCheckBlocksHalted:   {"failing"},
	FuncCheckBlocksProduced: {"tolerance", "failing"},
	FuncCheckNetworkRules:   {"rules", "failing"},
}

// checkParamDescriptions provides a human-readable description for each sub-check parameter.
var checkParamDescriptions = map[string]string{
	"ceiling":   "Maximum allowed value (float64) for a gas rate check.",
	"failing":   "When true, the check is expected to fail; a passing result is treated as an error.",
	"rules":     "Expected network rules patch (NetworkRulesPatch field structure).",
	"tolerance": "Allowed deviation (int, in blocks) between nodes for a height/production check.",
}

// PrintSequentialHelp writes a formatted summary of all available sequential
// scenario step functions, their descriptions, and accepted parameters to w.
// It returns the first write error encountered, if any.
func PrintSequentialHelp(w io.Writer) error {
	stepFns := slices.SortedFunc(slices.Values(allStepFunctions[:]), func(a, b StepFunction) int {
		return strings.Compare(string(a), string(b))
	})
	checkFns := slices.SortedFunc(slices.Values(allCheckFunctions[:]), func(a, b StepFunction) int {
		return strings.Compare(string(a), string(b))
	})

	ew := &errWriter{w: w}
	ew.printf("Sequential scenario step functions:\n\n")
	for _, fn := range stepFns {
		desc := stepFunctionDescriptions[fn]
		params := allowedParams[fn]
		ew.printf("  %-26s %s\n", fn, desc)
		for _, p := range params {
			ew.printf("      %-22s %s\n", p+":", paramDescriptions[p])
		}
		if fn == FuncChecks {
			ew.printf("\n    Available checks:\n")
			for _, cfn := range checkFns {
				cdesc := checkFunctionDescriptions[cfn]
				cparams := checkFunctionParams[cfn]
				ew.printf("      %-22s %s\n", cfn, cdesc)
				for _, p := range cparams {
					ew.printf("          %-18s %s\n", p+":", checkParamDescriptions[p])
				}
			}
		}
		if len(params) > 0 || fn == FuncChecks {
			ew.printf("\n")
		}
	}
	return ew.err
}

// errWriter wraps an io.Writer and records the first write error so that
// callers can chain multiple writes without checking each one individually.
type errWriter struct {
	w   io.Writer
	err error
}

func (ew *errWriter) printf(format string, args ...any) {
	if ew.err != nil {
		return
	}
	_, ew.err = fmt.Fprintf(ew.w, format, args...)
}
