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

package shaper

import (
	"errors"
	"fmt"
)

// Rate defines the shape of traffic to be generated. There are three types
// currently supported:
//   - constant ... traffic is created at a constant rate
//   - slope    ... traffic rate starts at a given rate and is linearly increased
//   - saturate ... traffic is submitted as fast as the network accepts it
//
// Only one of those options can be set for a single source.
type Rate struct {
	// Only one of the next fields may be set.
	Constant *float32 `yaml:",omitempty"`
	Slope    *Slope   `yaml:",omitempty"`
	Saturate bool     `yaml:",omitempty"`
}

// Slope defines the parameters of a linearly increasing traffic pattern.
// The pattern is defined by a starting Tx/s rate and an increment per second.
type Slope struct {
	Start     float32 // starting Tx/s
	Increment float32 // increment by given Tx/s per second
}

// Check tests semantic constraints on the traffic shape configuration of a source.
func (r *Rate) Check() error {
	count := 0
	if r.Constant != nil {
		count++
	}
	if r.Slope != nil {
		count++
	}
	if r.Saturate {
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
