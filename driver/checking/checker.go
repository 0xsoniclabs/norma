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

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/monitoring"
	"github.com/0xsoniclabs/norma/driver/parser"
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
	Configure(parser.CheckerConfig) (Checker, error)
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

// failingChecker is used to create a checker that expects an error
type failingChecker struct {
	checker Checker
}

// NewFailingChecker returns checks if an input Checker returns an error
func NewFailingChecker(checker Checker) Checker {
	return &failingChecker{checker}
}

func (c *failingChecker) Check() error {
	if err := c.checker.Check(); err == nil {
		return fmt.Errorf("failing expected")
	}
	return nil
}

func (c *failingChecker) Configure(config parser.CheckerConfig) (Checker, error) {
	configured, err := c.checker.Configure(config)
	if err != nil {
		return nil, fmt.Errorf("failed to configure checker; %v", err)
	}

	val, exist := config["failing"]
	if !exist {
		return configured, nil
	}

	failing, ok := val.(bool)
	if !ok {
		return nil, fmt.Errorf("failed to convert failing; %v", val)
	}
	if !failing {
		return configured, nil
	}

	return &failingChecker{configured}, nil
}
