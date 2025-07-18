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
