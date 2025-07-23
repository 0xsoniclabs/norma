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

package checking

import (
	"errors"
	"fmt"
	"strings"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/monitoring"
)

// Factory is a function that creates a Checker.
type Factory func(driver.Network, *monitoring.Monitor) Checker

// registry is a mapping of Checker registrations.
type registry map[string]Factory

var registrations = make(registry)

//go:generate mockgen -source checker.go -destination checker_mock.go -package checking

// Checker does the consistency check at the end of the scenario.
type Checker interface {
	Check() error
	Configure(CheckerConfig) (Checker, error)
}

// CheckerConfig is used to configure Checker
type CheckerConfig map[string]any

// copyExceptError makes a copy of the CheckerConfig except without the key "error"
func (cfg *CheckerConfig) copyExceptError() CheckerConfig {
	newConfig := make(CheckerConfig)
	for k, v := range *cfg {
		if k != "error" {
			newConfig[k] = v
		}
	}
	return newConfig
}

// Checks is a slice of Checker.
type Checks map[string]Checker

// RegisterNetworkCheck registers a new Checker via its factory.
func RegisterNetworkCheck(name string, factory Factory) {
	registrations[name] = factory
}

// InitNetworkChecks initializes the Checks with the given network.
func InitNetworkChecks(network driver.Network, monitor *monitoring.Monitor) Checks {
	checkers := make(map[string]Checker, len(registrations))
	for name, factory := range registrations {
		checker := factory(network, monitor)
		checkers[name] = checker
	}

	return checkers
}

// Check executes all checkers and returns an error if any of them find an issue.
func (c Checks) Check() error {
	errs := make([]error, len(c))
	for _, checker := range c {
		errs = append(errs, checker.Check())
	}

	return errors.Join(errs...)
}

// GetCheckerByName retrieves a Checker by its name.
// It returns nil if the Checker is not found.
func (c Checks) GetCheckerByName(name string) Checker {
	return c[name]
}

// errorChecker is used to create a checker that expects an error
type errorChecker struct {
	checker Checker
	err     string
}

func NewErrorChecker(checker Checker, config CheckerConfig) (Checker, error) {
	if val, exist := config["error"]; exist {
		emsg, ok := val.(string)
		if !ok {
			return nil, fmt.Errorf("failed to convert error; %v", val)
		}

		ec := &errorChecker{checker, emsg}
		configuredErrorChecker, err := ec.Configure(config)
		if err != nil {
			return nil, err
		}
		return configuredErrorChecker, nil
	}
	return nil, fmt.Errorf("no configured error")
}

func (c *errorChecker) Check() error {
	if c.checker == nil {
		return fmt.Errorf("checker is nil")
	}

	if err := c.checker.Check(); err == nil || !strings.Contains(err.Error(), c.err) {
		return fmt.Errorf("expected error %s", c.err)
	}
	return nil
}

func (c *errorChecker) Configure(config CheckerConfig) (Checker, error) {
	checker, err := c.checker.Configure(config.copyExceptError())
	if err != nil {
		return nil, err
	}

	return &errorChecker{
		checker: checker,
		err:     c.err,
	}, nil
}
