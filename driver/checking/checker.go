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

// registrations are mandatory default checks that will be carry out
// if --skip-check is not enabled.
var registrations = make(registry)

// supportedCustomChecks contains all supported checking types that
// could be configured into scenario yml config through "checks"
var supportedCustomChecks = make(registry)

// Checker does the consistency check at the end of the scenario.
type Checker interface {
	Check() error
}

// Checks is a slice of Checker.
type Checks []Checker

// RegisterNetworkCheck registers a new Checker via its factory.
func RegisterNetworkCheck(name string, factory Factory) {
	registrations[name] = factory
}

// RegisterSupportedCheck registered a support checker type.
func RegisterSupportedCheck(name string, factory Factory) {
	supportedCustomChecks[name] = factory
}

// IsSupportedCheck returns true iff the check of provided name is supported
func IsSupportedCheck(name string) bool {
	_, ok := supportedCustomChecks[name]
	return ok
}

// InitNetworkChecks initializes the Checks with the given network.
func InitNetworkChecks(network driver.Network, monitor *monitoring.Monitor) Checks {
	var checkers []Checker
	for _, factory := range registrations {
		checker := factory(network, monitor)
		checkers = append(checkers, checker)
	}

	return checkers
}

// Check executes all checkers and returns an error if any of them find an issue.
func (c Checks) Check() error {
	errs := make([]error, len(c))
	for i, checker := range c {
		errs[i] = checker.Check()
	}
	return errors.Join(errs...)
}
