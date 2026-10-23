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
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/monitoring"
	"github.com/0xsoniclabs/norma/genesis"
)

// Factory is a function that creates a Checker.
type Factory func(driver.Network, *monitoring.Monitor) Checker

// registry is a mapping of Checker registrations.
type registry map[string]Factory

var registrations = make(registry)

//go:generate mockgen -source checker.go -destination checker_mock.go -package checking

// Checker does the consistency check at the end of the scenario.
type Checker interface {
	Check(ctx context.Context) error
	Configure(CheckerConfig) Checker
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
func (c Checks) Check(ctx context.Context) error {
	errs := make([]error, 0, len(c))
	for _, checker := range c {
		errs = append(errs, checker.Check(ctx))
	}

	return errors.Join(errs...)
}

// GetCheckerByName retrieves a Checker by its name.
// It returns nil if the Checker is not found.
func (c Checks) GetCheckerByName(name string) Checker {
	return c[name]
}

// ErrAssertion marks an error as the network failing what a checker asserts,
// as opposed to the checker being unable to decide, for instance because a
// node could not be reached.
var ErrAssertion = errors.New("assertion failed")

type assertionError struct{ error }

func (assertionError) Is(target error) bool { return target == ErrAssertion }

func (e assertionError) Unwrap() error { return e.error }

// assertionf formats an error that matches ErrAssertion.
func assertionf(format string, args ...any) error {
	return assertionError{fmt.Errorf(format, args...)}
}

// failingChecker is used to create a checker that expects an error
type failingChecker struct {
	checker Checker
}

// NewFailingChecker returns checks if an input Checker returns an error
func NewFailingChecker(checker Checker) Checker {
	return &failingChecker{checker}
}

// Check passes only when the checker's assertion fails. Any other error means
// the expected failure was not observed, so it fails the check.
func (c *failingChecker) Check(ctx context.Context) error {
	err := c.checker.Check(ctx)
	if err == nil {
		return fmt.Errorf("failure expected")
	}
	if !errors.Is(err, ErrAssertion) {
		return fmt.Errorf("failure expected, but the check could not decide: %w", err)
	}
	slog.Info("check failed as expected", "reason", err)
	return nil
}

func (c *failingChecker) Configure(config CheckerConfig) Checker {
	configured := c.checker.Configure(config)
	if failing, exist := config["failing"]; !exist || !failing.(bool) {
		return configured
	}
	return &failingChecker{configured}
}

type CheckerConfig map[string]any

func (config *CheckerConfig) Check() error {
	errs := []error{}

	isBool := func(v any) bool { _, ok := v.(bool); return ok }
	isPositiveInt := func(v any) bool { i, ok := v.(int); return ok && i >= 0 }
	isNonNegativeInt64 := func(v any) bool { i, ok := v.(int64); return ok && i >= 0 }
	isRulesPatch := func(v any) bool {
		if _, ok := v.(map[string]any); ok {
			return true
		}
		_, ok := v.(genesis.NetworkRulesPatch)
		return ok
	}

	checks := map[string]func(any) bool{
		"failing":   isBool,
		"tolerance": isPositiveInt,
		"duration":  isNonNegativeInt64,
		"ceiling":   isPositiveInt,
		"slack":     isPositiveInt,
		"rules":     isRulesPatch,
	}

	for key, check := range checks {
		if val, exist := (*config)[key]; exist && !check(val) {
			errs = append(errs, fmt.Errorf("error parsing %s", key))
		}
	}

	return errors.Join(errs...)
}
