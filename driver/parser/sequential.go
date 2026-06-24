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
	"time"

	"gopkg.in/yaml.v3"
)

// StepFunction identifies the type of operation a step performs.
type StepFunction string

const (
	FuncStartNode           StepFunction = "startNode"
	FuncStopNode            StepFunction = "stopNode"
	FuncUndelegate          StepFunction = "undelegate"
	FuncUpdateRules         StepFunction = "updateRules"
	FuncAdvanceEpoch        StepFunction = "advanceEpoch"
	FuncWaitForEpoch        StepFunction = "waitForEpoch"
	FuncRunApp              StepFunction = "runApp"
	FuncStopApp             StepFunction = "stopApp"
	FuncCheckBlocksProduced StepFunction = "checkBlocksProduced"
	FuncCheckBlocksHalted   StepFunction = "checkBlocksHalted"
	FuncCheckBlockHashes    StepFunction = "checkBlockHashes"
	FuncCheckBlockHeights   StepFunction = "checkBlockHeights"
	FuncCheckBlockGasRate   StepFunction = "checkBlockGasRate"
	FuncWaitFor             StepFunction = "waitFor"
)

// allStepFunctions lists every known step function constant.
var allStepFunctions = [...]StepFunction{
	FuncStartNode,
	FuncStopNode,
	FuncUndelegate,
	FuncUpdateRules,
	FuncAdvanceEpoch,
	FuncWaitForEpoch,
	FuncRunApp,
	FuncStopApp,
	FuncCheckBlocksProduced,
	FuncCheckBlocksHalted,
	FuncCheckBlockHashes,
	FuncCheckBlockHeights,
	FuncCheckBlockGasRate,
	FuncWaitFor,
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

// SequentialScenario is the root element of a sequential scenario description.
// Unlike the time-based Scenario, it defines an ordered list of blocking steps.
type SequentialScenario struct {
	Name         string            `yaml:"Name"`
	InitialRules map[string]string `yaml:"InitialNetworkRules"`
	Steps        []Step            `yaml:"Scenario"`
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
	Rules              map[string]string
	WaitForEpochChange bool

	// Check parameters
	Ceiling   *float64
	Tolerance *int

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
	// Iterate through key-value pairs in the mapping.
	for i := 0; i < len(node.Content); i += 2 {
		keyNode := node.Content[i]
		valNode := node.Content[i+1]
		key := keyNode.Value

		// Check if this key is a function name.
		if fn, err := toStepFunction(key); err == nil {
			if s.Function != "" {
				return fmt.Errorf("line %d: step contains multiple function keys: %q and %q", keyNode.Line, s.Function, fn)
			}
			s.Function = fn
			if err := s.parseFunctionValue(fn, valNode); err != nil {
				return fmt.Errorf("line %d: %w", valNode.Line, err)
			}
			continue
		}

		// Otherwise, parse as a parameter.
		if err := s.parseParam(key, valNode); err != nil {
			return fmt.Errorf("line %d: %w", keyNode.Line, err)
		}
	}

	if s.Function == "" {
		return fmt.Errorf("line %d: no known function found in step mapping", node.Line)
	}
	return nil
}

// parseFunctionValue parses the value associated with the function key.
func (s *Step) parseFunctionValue(fn StepFunction, val *yaml.Node) error {
	switch fn {
	case FuncUpdateRules:
		// Value is a map of rules.
		if val.Kind == yaml.MappingNode {
			s.Rules = make(map[string]string)
			for i := 0; i < len(val.Content); i += 2 {
				k := val.Content[i].Value
				v := val.Content[i+1].Value
				s.Rules[k] = v
			}
		} else if val.Tag != "!!null" && val.Value != "" {
			return fmt.Errorf("Update rules value must be a mapping, got %q", val.Value)
		}
	case FuncAdvanceEpoch, FuncWaitForEpoch:
		// These take no value (or null).
		if val.Tag != "!!null" && val.Value != "" && val.Value != "null" {
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
	default:
		// Value is a string identifier (or null/empty for optional).
		if val.Kind == yaml.ScalarNode && val.Tag != "!!null" && val.Value != "" {
			s.Identifier = val.Value
		}
	}
	return nil
}

// parseParam parses a single parameter key-value pair.
func (s *Step) parseParam(key string, val *yaml.Node) error {
	switch key {
	case "type":
		switch s.Function {
		case FuncRunApp:
			s.AppType = val.Value
		default:
			s.NodeType = val.Value
		}
	case "imagename":
		s.ImageName = val.Value
	case "datavolume":
		s.DataVolume = val.Value
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
	case "ceiling":
		var v float64
		if err := val.Decode(&v); err != nil {
			return fmt.Errorf("invalid ceiling value: %w", err)
		}
		s.Ceiling = &v
	case "tolerance":
		var v int
		if err := val.Decode(&v); err != nil {
			return fmt.Errorf("invalid tolerance value: %w", err)
		}
		s.Tolerance = &v
	case "wait for epoch change":
		var v bool
		if err := val.Decode(&v); err != nil {
			return fmt.Errorf("invalid 'wait for epoch change' value: %w", err)
		}
		s.WaitForEpochChange = v
	default:
		return fmt.Errorf("unknown parameter: %q", key)
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

// DefaultMaxEpochDuration is applied to sequential scenarios that do not
// explicitly set MAX_EPOCH_DURATION in their InitialNetworkRules.
const DefaultMaxEpochDuration = "15s"

// setDefaults sets default values on the sequential scenario.
func (s *SequentialScenario) setDefaults() {
	if s.InitialRules == nil {
		s.InitialRules = make(map[string]string)
	}
	if _, explicit := s.InitialRules["MAX_EPOCH_DURATION"]; !explicit {
		s.InitialRules["MAX_EPOCH_DURATION"] = DefaultMaxEpochDuration
	}
}
