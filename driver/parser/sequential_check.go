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
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/0xsoniclabs/norma/genesis"
	"github.com/0xsoniclabs/norma/load/app"
)

// Check validates semantic constraints on a sequential scenario.
func (s *SequentialScenario) Check() error {
	errs := []error{}

	if strings.TrimSpace(s.Name) == "" {
		errs = append(errs, fmt.Errorf("scenario name must not be empty"))
	}
	// Validate initial network rules.
	if err := genesis.ValidateNetworkRulesPatch(s.InitialRules); err != nil {
		errs = append(errs, fmt.Errorf("invalid initial network rules: %w", err))
	}

	// Validate each step.
	for i, step := range s.Steps {
		if err := step.Check(); err != nil {
			errs = append(errs, fmt.Errorf("step %d (%s): %w", i+1, step.Function, err))
		}
	}

	return errors.Join(errs...)
}

// Check validates semantic constraints on a single step.
func (s *Step) Check() error {
	switch s.Function {
	case FuncStartNode:
		return s.checkStartNode()
	case FuncStopNode:
		return s.checkStopNode()
	case FuncRunApp:
		return s.checkRunApp()
	case FuncStopApp:
		return s.checkStopApp()
	case FuncUpdateRules:
		return s.checkUpdateRules()
	case FuncUndelegate:
		if s.Identifier == "" {
			return fmt.Errorf("undelegate requires a node name")
		}
		if !NamePattern.Match([]byte(s.Identifier)) {
			return fmt.Errorf("node name must match %v, got %v", namePatternStr, s.Identifier)
		}
		return nil
	case FuncWaitFor:
		if s.Duration <= 0 {
			return fmt.Errorf("waitFor requires a positive duration, got %v", s.Duration)
		}
		return nil
	case FuncAdvanceEpoch, FuncWaitForEpoch:
		return nil
	case FuncChecks:
		return s.checkSubChecks()
	default:
		return fmt.Errorf("unknown function: %q", s.Function)
	}
}

func (s *Step) checkStartNode() error {
	errs := []error{}

	if s.Identifier == "" {
		errs = append(errs, fmt.Errorf("start node requires an identifier (name)"))
	} else if !NamePattern.Match([]byte(s.Identifier)) {
		errs = append(errs, fmt.Errorf("node name must match %v, got %v", namePatternStr, s.Identifier))
	}

	// Validate node type if specified.
	nodeType := s.NodeType
	if nodeType == "" {
		nodeType = "observer"
	}
	if err := isTypeValid(nodeType); err != nil {
		errs = append(errs, err)
	}

	if s.Instances != nil && *s.Instances < 1 {
		errs = append(errs, fmt.Errorf("number of instances must be >= 1, got %d", *s.Instances))
	}

	return errors.Join(errs...)
}

func (s *Step) checkStopNode() error {
	errs := []error{}

	if s.Identifier == "" {
		errs = append(errs, fmt.Errorf("stop node requires an identifier (name)"))
	} else if !NamePattern.Match([]byte(s.Identifier)) {
		errs = append(errs, fmt.Errorf("node name must match %v, got %v", namePatternStr, s.Identifier))
	}

	return errors.Join(errs...)
}

func (s *Step) checkRunApp() error {
	errs := []error{}

	if s.Identifier == "" {
		errs = append(errs, fmt.Errorf("run app requires an identifier (name)"))
	} else if !NamePattern.Match([]byte(s.Identifier)) {
		errs = append(errs, fmt.Errorf("app name must match %v, got %v", namePatternStr, s.Identifier))
	}

	appType := s.AppType
	if appType == "" {
		errs = append(errs, fmt.Errorf("run app requires a type"))
	} else if !app.IsSupportedApplicationType(appType) {
		errs = append(errs, fmt.Errorf("unknown application type: %v", appType))
	}

	if s.Rate == nil {
		errs = append(errs, fmt.Errorf("run app requires a rate"))
	} else if err := s.Rate.Check(nil); err != nil {
		errs = append(errs, err)
	}

	if s.Users != nil && *s.Users < 1 {
		errs = append(errs, fmt.Errorf("number of users must be >= 1, got %d", *s.Users))
	}

	return errors.Join(errs...)
}

func (s *Step) checkStopApp() error {
	errs := []error{}

	if s.Identifier == "" {
		errs = append(errs, fmt.Errorf("stop app requires an identifier (name)"))
	} else if !NamePattern.Match([]byte(s.Identifier)) {
		errs = append(errs, fmt.Errorf("app name must match %v, got %v", namePatternStr, s.Identifier))
	}

	return errors.Join(errs...)
}

func (s *Step) checkUpdateRules() error {
	errs := []error{}

	b, err := json.Marshal(s.Rules)
	if err != nil || string(b) == "{}" {
		errs = append(errs, fmt.Errorf("update rules requires at least one rule"))
	}

	return errors.Join(errs...)
}

func (s *Step) checkSubChecks() error {
	if len(s.SubChecks) == 0 {
		return fmt.Errorf("checks step requires at least one sub-check")
	}

	errs := []error{}
	for i, check := range s.SubChecks {
		if check.Function == "" {
			errs = append(errs, fmt.Errorf("sub-check %d: missing check function", i+1))
			continue
		}

		// Validate that rules patches are valid when provided.
		if check.Function == FuncCheckNetworkRules && check.Rules != (genesis.NetworkRulesPatch{}) {
			if err := genesis.ValidateNetworkRulesPatch(check.Rules); err != nil {
				errs = append(errs, fmt.Errorf("sub-check %d (%s): invalid rules: %w", i+1, check.Function, err))
			}
		}
	}

	return errors.Join(errs...)
}
