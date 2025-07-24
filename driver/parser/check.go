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
	"regexp"
	"strings"

	"github.com/0xsoniclabs/norma/genesistools/genesis"

	"github.com/0xsoniclabs/norma/load/app"
)

const namePatternStr = `^[A-Za-z0-9-.]+$`

var namePattern = regexp.MustCompile(namePatternStr)

// Check tests semantic constraints on the configuration of a scenario.
func (s *Scenario) Check() error {
	errs := []error{}
	if strings.TrimSpace(s.Name) == "" {
		errs = append(errs, fmt.Errorf("scenario name must not be empty"))
	}
	if s.Duration <= 0 {
		errs = append(errs, fmt.Errorf("scenario duration must be > 0"))
	}
	if s.RoundTripTime != nil && *s.RoundTripTime < 0 {
		errs = append(errs, fmt.Errorf("round trip time must be >= 0, is %v", *s.RoundTripTime))
	}

	names := map[string]bool{}
	for _, validator := range s.Validators {
		if err := validator.Check(s); err != nil {
			errs = append(errs, err)
		}
		if _, exists := names[validator.Name]; exists {
			errs = append(errs, fmt.Errorf("validator names must be unique, %s encountered multiple times", validator.Name))
		} else {
			names[validator.Name] = true
		}
	}
	for _, node := range s.Nodes {
		if err := node.Check(s); err != nil {
			errs = append(errs, err)
		}
		if _, exists := names[node.Name]; exists {
			errs = append(errs, fmt.Errorf("node names must be unique, %s encountered multiple times", node.Name))
		} else {
			names[node.Name] = true
		}
	}
	names = map[string]bool{}
	for _, application := range s.Applications {
		if err := application.Check(s); err != nil {
			errs = append(errs, err)
		}
		if _, exists := names[application.Name]; exists {
			errs = append(errs, fmt.Errorf("application names must be unique, %s encountered multiple times", application.Name))
		} else {
			names[application.Name] = true
		}
	}
	names = map[string]bool{}
	for _, cheat := range s.Cheats {
		if err := cheat.Check(s); err != nil {
			errs = append(errs, err)
		}
		if _, exists := names[cheat.Name]; exists {
			errs = append(errs, fmt.Errorf("cheat names must be unique, %s encountered multiple times", cheat.Name))
		} else {
			names[cheat.Name] = true
		}
	}

	for key := range s.NetworkRules.Genesis {
		if !genesis.IsSupportedNetworkRule(key) {
			errs = append(errs, fmt.Errorf("unknown network rule: %v", key))
		}
	}

	for _, rule := range s.NetworkRules.Updates {
		if rule.Time < 0 {
			errs = append(errs, fmt.Errorf("network rule update time must be >= 0, is %f", rule.Time))
		}
		for key := range rule.Rules {
			if !genesis.IsSupportedNetworkRule(key) {
				errs = append(errs, fmt.Errorf("unknown network rule: %v", key))
			}
		}
	}

	for _, adv := range s.AdvanceEpoch {
		if adv.Time < 0 || adv.Time > s.Duration {
			errs = append(errs, fmt.Errorf("invalid timing for advance epoch: %f", adv.Time))
		}

		if adv.Epochs != nil && *adv.Epochs < 1 {
			errs = append(errs, fmt.Errorf("minimum epoch to advance must be 1, got: %d", *adv.Epochs))
		}
	}

	for _, chk := range s.Checks {
		if chk.Time < 0 || chk.Time > s.Duration {
			errs = append(errs, fmt.Errorf("invalid timing for checks: %f", chk.Time))
		}
		err := chk.Config.Check()
		if err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

// Check tests semantic constraints on the validator configuration of a scenario.
func (v *Validator) Check(scenario *Scenario) error {
	errs := []error{}

	if len(v.Name) != 0 && !namePattern.Match([]byte(v.Name)) {
		errs = append(errs, fmt.Errorf("validator name must match %v, got %v", namePatternStr, v.Name))
	}

	if v.Instances != nil && *v.Instances < 0 {
		errs = append(errs, fmt.Errorf("number of instances must be >= 0, is %d", *v.Instances))
	}

	if err := checkTimeInterval(nil, v.End, scenario.Duration); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// Check tests semantic constraints on the node configuration of a scenario.
func (n *Node) Check(scenario *Scenario) error {
	errs := []error{}
	if !namePattern.Match([]byte(n.Name)) {
		errs = append(errs, fmt.Errorf("node name must match %v, got %v", namePatternStr, n.Name))
	}
	if n.Instances != nil && *n.Instances < 0 {
		errs = append(errs, fmt.Errorf("number of instances must be >= 0, is %d", *n.Instances))
	}
	if n.Client.Type == "" {
		n.Client.Type = "observer"
	}

	if n.Start != nil && n.Rejoin != nil {
		errs = append(errs, fmt.Errorf("node cannot have both start and rejoin; start=%f, rejoin=%f", *n.Start, *n.Rejoin))
	}

	if n.End != nil && n.Leave != nil {
		errs = append(errs, fmt.Errorf("node cannot have both end and leave; end=%f, leave=%f", *n.End, *n.Leave))
	}

	if err := checkTimeInterval(n.Start, n.End, scenario.Duration); err != nil {
		errs = append(errs, err)
	}
	if err := checkTimeInterval(n.Start, n.Leave, scenario.Duration); err != nil {
		errs = append(errs, err)
	}
	if err := checkTimeInterval(n.Rejoin, n.End, scenario.Duration); err != nil {
		errs = append(errs, err)
	}
	if err := checkTimeInterval(n.Rejoin, n.Leave, scenario.Duration); err != nil {
		errs = append(errs, err)
	}

	if err := n.isTypeValid(); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// isTypeValid returns true if the node has valid type, false otherwise
func (n *Node) isTypeValid() error {
	return isTypeValid(n.Client.Type)
}

func isTypeValid(t string) error {
	switch t {
	case
		"validator",
		"rpc",
		"observer":
		return nil
	}
	return fmt.Errorf("type of node must be observer, rpc or validator, was set to %s", t)
}

// Check tests semantic constraints on the application configuration of a scenario.
func (a *Application) Check(scenario *Scenario) error {
	errs := []error{}

	if !namePattern.Match([]byte(a.Name)) {
		errs = append(errs, fmt.Errorf("application name must match %v, got %v", namePatternStr, a.Name))
	}

	if a.Type == "" {
		errs = append(errs, fmt.Errorf("application type must be specified"))
	} else if !app.IsSupportedApplicationType(a.Type) {
		errs = append(errs, fmt.Errorf("unknown application type: %v", a.Type))
	}

	if a.Instances != nil && *a.Instances < 0 {
		errs = append(errs, fmt.Errorf("number of instances must be >= 0, is %d", *a.Instances))
	}

	if a.Users != nil && *a.Users < 1 {
		errs = append(errs, fmt.Errorf("number of users must be >= 1, is %d", *a.Users))
	}

	if err := checkTimeInterval(a.Start, a.End, scenario.Duration); err != nil {
		errs = append(errs, err)
	}

	if err := a.Rate.Check(scenario); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// Check tests semantic constraints on the cheat configuration of a scenario.
func (c *Cheat) Check(scenario *Scenario) error {
	errs := []error{}

	if !namePattern.Match([]byte(c.Name)) {
		errs = append(errs, fmt.Errorf("cheat name must match %v, got %v", namePatternStr, c.Name))
	}

	if err := checkTimeInterval(c.Start, nil, scenario.Duration); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// Check tests semantic constraints on the traffic shape configuration of a source.
func (r *Rate) Check(scenario *Scenario) error {
	count := 0
	if r.Constant != nil {
		count++
	}
	if r.Slope != nil {
		count++
	}
	if r.Wave != nil {
		count++
	}
	if r.Auto != nil {
		count++
	}
	if count != 1 {
		return fmt.Errorf("application must specify exactly one load shape, got %d", count)
	}

	if r.Constant != nil && *r.Constant < 0 {
		return fmt.Errorf("constant transaction rate must be >= 0, got %f", *r.Constant)
	}
	if r.Slope != nil {
		return r.Slope.Check()
	}
	if r.Wave != nil {
		return r.Wave.Check()
	}
	if r.Auto != nil {
		return r.Auto.Check()
	}
	return nil
}

// Check tests semantic constraints on the configuration of a slope traffic pattern.
func (s *Slope) Check() error {
	errs := []error{}

	if s.Start < 0 {
		errs = append(errs, fmt.Errorf("initial transaction rate must be >= 0, got %f", s.Start))
	}

	return errors.Join(errs...)
}

// Check tests semantic constraints on the configuration of a wave-shaped traffic pattern.
func (w *Wave) Check() error {
	errs := []error{}

	min := float32(0.0)
	if w.Min != nil {
		min = *w.Min
	}
	max := w.Max

	if min < 0 {
		errs = append(errs, fmt.Errorf("minimum transaction rate must be >= 0, got %f", min))
	}
	if max < 0 {
		errs = append(errs, fmt.Errorf("maximum transaction rate must be >= 0, got %f", max))
	}
	if min > max {
		errs = append(errs, fmt.Errorf("minimum transaction rate must be <= maximum rate, got %f > %f", min, max))
	}

	if w.Period <= 0 {
		errs = append(errs, fmt.Errorf("wave priode must be > 0, got %f", w.Period))
	}

	return errors.Join(errs...)
}

// Check tests semantic constraints on the configuration of a auto-shaped traffic pattern.
func (a *Auto) Check() error {
	errs := []error{}

	if a.Increase != nil {
		if *a.Increase <= 0 {
			errs = append(errs, fmt.Errorf("traffic rate increase per second must be positive, got %f", *a.Increase))
		}
	}
	if a.Decrease != nil {
		if *a.Decrease < 0 || *a.Decrease > 1 {
			errs = append(errs, fmt.Errorf("traffic decrease rate must be between 0 and 1, got %f", *a.Decrease))
		}
	}

	return errors.Join(errs...)
}

// Check tests parsing of each configuration
func (config *CheckerConfig) Check() error {
	errs := []error{}

	isBool := func(v any) bool { _, ok := v.(bool); return ok }
	isPositiveInt := func(v any) bool { i, ok := v.(int); return ok && i >= 0 }
	isUint8 := func(v any) bool { i, ok := v.(int); return ok && i >= 0 && i < 256 }
	isPositiveFloat64 := func(v any) bool { f, ok := v.(float64); return isPositiveInt(v) || (ok && f >= 0) }

	checks := map[string]func(any) bool{
		"failing":   isBool,
		"tolerance": isPositiveInt,
		"start":     isPositiveInt,
		"ceiling":   isPositiveFloat64,
		"slack":     isUint8,
	}

	for key, check := range checks {
		if val, exist := (*config)[key]; exist && !check(val) {
			errs = append(errs, fmt.Errorf("error parsing %s", key))
		}
	}

	return errors.Join(errs...)
}

// checkTimeInterval is a utility function checking the validity of a start/end time pair.
func checkTimeInterval(start, end *float32, duration float32) error {
	realStart := float32(0.0)
	if start != nil {
		realStart = *start
	}
	realEnd := duration
	if end != nil {
		realEnd = *end
	}
	errs := []error{}
	if realStart < 0 {
		errs = append(errs, fmt.Errorf("start time must be >= 0, is %f", realStart))
	}
	if realStart > duration {
		errs = append(errs, fmt.Errorf("start time must be <= scenario duration (=%fs), is %f", duration, realStart))
	}
	if realEnd < realStart {
		errs = append(errs, fmt.Errorf("end time must be >= start time,  end=%fs, start=%fs", realEnd, realStart))
	} else {
		if realEnd > duration {
			errs = append(errs, fmt.Errorf("end time must be <= scenario duration, end=%fs, duration=%fs", realEnd, duration))
		}
	}
	return errors.Join(errs...)
}
