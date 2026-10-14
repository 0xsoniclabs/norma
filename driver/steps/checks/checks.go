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

package checks

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/0xsoniclabs/norma/driver/checking"
	"github.com/0xsoniclabs/norma/driver/steps"
	"gopkg.in/yaml.v3"
)

// Check is an assertion listed in a checks step.
type Check interface {
	steps.Function
	Run(ctx context.Context, checks checking.Checks) error
}

// Kinds lists every check a checks step accepts.
var Kinds = []func() Check{
	func() Check { return &BlockGasRate{} },
	func() Check { return &BlockHashes{} },
	func() Check { return &BlockHeights{} },
	func() Check { return &BlocksHalted{} },
	func() Check { return &BlocksProduced{} },
	func() Check { return &EventThrottled{} },
	func() Check { return &NetworkRules{} },
	func() Check { return &ValidatorsActive{} },
}

// Checks runs its checks in order, failing on the first that fails.
type Checks struct {
	Items []Check
}

func (*Checks) Name() string { return "checks" }

func (*Checks) Help() string {
	return "Run one or more checks (see 'Available checks' below)."
}

func (s *Checks) DecodeValue(value *yaml.Node) error {
	if value.Kind != yaml.SequenceNode {
		if steps.IsNull(value) {
			return nil
		}
		return fmt.Errorf("checks requires a list of check functions, got %q", value.Value)
	}
	for _, item := range value.Content {
		check, err := steps.Decode(item, Kinds, "check")
		if err != nil {
			return fmt.Errorf("invalid checks list: %w", err)
		}
		s.Items = append(s.Items, check)
	}
	return nil
}

func (s *Checks) Validate() error {
	if len(s.Items) == 0 {
		return errors.New("checks step requires at least one sub-check")
	}
	var errs []error
	for i, check := range s.Items {
		if err := check.Validate(); err != nil {
			errs = append(errs, fmt.Errorf("sub-check %d (%s): %w", i+1, check.Name(), err))
		}
	}
	return errors.Join(errs...)
}

func (*Checks) UpdateModel(*steps.Model) error { return nil }

func (s *Checks) Run(ctx context.Context, env *steps.Env) error {
	for i, check := range s.Items {
		if err := check.Run(ctx, env.Checks); err != nil {
			return fmt.Errorf("check %d (%s): %w", i+1, check.Name(), err)
		}
	}
	return nil
}

// Window is the base of checks observing the network forward in time, whose
// window is given either as a duration or as a number of monitoring samples.
type Window struct {
	// Tolerance is the window in monitoring samples, taken one per second.
	Tolerance *int `yaml:"tolerance" help:"Observation window in monitoring samples; duration overrides it."`
	// Duration is how long to actively observe the network; only data
	// collected while waiting is judged.
	Duration *steps.Duration `yaml:"duration" help:"How long to observe the network, e.g. \"30s\"; at least 2s."`
}

// Validate rejects windows too short to hold two samples, here rather than
// minutes into the run.
func (w Window) Validate() error {
	if w.Duration != nil {
		if d := time.Duration(*w.Duration); d < checking.MinObservationDuration {
			return fmt.Errorf("duration must be at least %s, got %s", checking.MinObservationDuration, d)
		}
		return nil
	}
	// A duration, when given, overrides the tolerance.
	if w.Tolerance != nil && *w.Tolerance < checking.MinObservationSamples {
		return fmt.Errorf("tolerance is an observation window in monitoring samples "+
			"and must be at least %d, got %d", checking.MinObservationSamples, *w.Tolerance)
	}
	return nil
}

func (w Window) configure(config checking.CheckerConfig) {
	if w.Tolerance != nil {
		config["tolerance"] = *w.Tolerance
	}
	setDuration(config, w.Duration)
}

// Convergence is the base of checks retrying until the nodes agree. Unlike an
// observation window, a short budget, or none at all, is a legitimate choice.
type Convergence struct {
	Duration *steps.Duration `yaml:"duration" help:"How long the nodes get to converge, e.g. \"30s\"; 0 tries once."`
}

func (c Convergence) configure(config checking.CheckerConfig) {
	setDuration(config, c.Duration)
}

// Failing is the parameter marking a check as expected to fail.
type Failing struct {
	Failing bool `yaml:"failing" help:"When true, the check must fail; a pass is treated as an error."`
}

// newConfig returns the checker configuration every check starts from.
func newConfig(failing Failing) checking.CheckerConfig {
	config := checking.CheckerConfig{}
	if failing.Failing {
		config["failing"] = true
	}
	return config
}

func setDuration(config checking.CheckerConfig, duration *steps.Duration) {
	if duration != nil {
		config["duration"] = int64(*duration)
	}
}

// runChecker runs the checker registered under name, configured when config
// is not empty.
func runChecker(
	ctx context.Context, checks checking.Checks, name string, config checking.CheckerConfig,
) error {
	if checks == nil {
		slog.Warn("checks skipped (no checker configured)", "check", name)
		return nil
	}

	checker := checks.GetCheckerByName(name)
	if checker == nil {
		return fmt.Errorf("checker %q not found", name)
	}
	if len(config) > 0 {
		checker = checking.NewFailingChecker(checker).Configure(config)
	}
	return checker.Check(ctx)
}
