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
	"errors"
	"fmt"
	"strings"

	"github.com/0xsoniclabs/norma/driver/steps"
	"github.com/0xsoniclabs/norma/driver/steps/nodelifetime"
	"github.com/0xsoniclabs/norma/genesis"
)

// Check validates semantic constraints on a scenario.
func (s *Scenario) Check() error {
	errs := []error{}

	if strings.TrimSpace(s.Name) == "" {
		errs = append(errs, fmt.Errorf("scenario name must not be empty"))
	}
	if strings.TrimSpace(s.Description) == "" {
		errs = append(errs, fmt.Errorf("scenario description must not be empty"))
	}
	// Validate initial network rules.
	if err := genesis.ValidateNetworkRulesPatch(s.InitialRules); err != nil {
		errs = append(errs, fmt.Errorf("invalid initial network rules: %w", err))
	}

	// Validate each step.
	for i, step := range s.Steps {
		if err := step.Validate(); err != nil {
			errs = append(errs, fmt.Errorf("step %d (%s): %w", i+1, step.Name(), err))
		}
	}

	return errors.Join(errs...)
}

// CheckFile parses the scenario at path and checks it: each step on its own,
// then all of them in order against a model of the network. It is what makes
// a scenario valid for norma check, build and run, and for the scenario tests.
func CheckFile(path string) (Scenario, error) {
	scenario, err := ParseFile(path)
	if err != nil {
		return Scenario{}, fmt.Errorf("failed to parse scenario file: %w", err)
	}
	if err := scenario.Check(); err != nil {
		return Scenario{}, err
	}
	if err := scenario.DryRun(); err != nil {
		return Scenario{}, fmt.Errorf("dry run failed: %w", err)
	}
	return scenario, nil
}

// DryRun applies the steps in order to a model of the network, the way a run
// does before running each step, and reports the first that could not run:
// stopping a node that is not running, healing one that was not killed,
// importing a file no step exported, and the like. Anything that depends on
// the network's behaviour, such as timing, is out of its reach.
func (s *Scenario) DryRun() error {
	if len(s.Steps) == 0 {
		return errors.New("scenario has no steps")
	}
	// The first step starts the network, so it has to start validators.
	if start, ok := s.Steps[0].(*nodelifetime.StartNode); !ok || !start.IsValidator() {
		return fmt.Errorf("step 1 (%s): the first step must start a validator", s.Steps[0].Name())
	}
	model := steps.NewModel()
	for i, step := range s.Steps {
		if err := step.UpdateModel(model); err != nil {
			return fmt.Errorf("step %d (%s): %w", i+1, strings.TrimSpace(step.Name()+" "+steps.TargetOf(step)), err)
		}
	}
	return nil
}
