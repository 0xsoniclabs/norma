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

package steps

import (
	"errors"
	"time"
)

// Timeouts are the budgets of the waits steps assert on: a wait that does not
// complete within its budget fails the step. A scenario may set them, so that
// its verdict does not hinge on budgets hidden in the code; a zero budget takes
// its default.
type Timeouts struct {
	// NodeSync bounds how long a started node may take to reach the head of
	// the network.
	NodeSync time.Duration `yaml:"NodeSync,omitempty"`
	// EpochChange bounds how long sealing an epoch, or waiting for the next
	// one, may take.
	EpochChange time.Duration `yaml:"EpochChange,omitempty"`
	// ValidatorActivation bounds how long new validators may take to join the
	// validator set once their epoch is sealed.
	ValidatorActivation time.Duration `yaml:"ValidatorActivation,omitempty"`
}

// WithDefaults returns the timeouts with every zero budget set to its default.
func (t Timeouts) WithDefaults() Timeouts {
	if t.NodeSync == 0 {
		t.NodeSync = 60 * time.Second
	}
	if t.EpochChange == 0 {
		t.EpochChange = 60 * time.Second
	}
	if t.ValidatorActivation == 0 {
		t.ValidatorActivation = 30 * time.Second
	}
	return t
}

// Validate reports negative budgets.
func (t Timeouts) Validate() error {
	if t.NodeSync < 0 || t.EpochChange < 0 || t.ValidatorActivation < 0 {
		return errors.New("timeouts must not be negative")
	}
	return nil
}
