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

// Package steps holds the operations a scenario is made of. Each step is a
// type owning its YAML shape, its validation, its help text and its execution;
// families of steps share behaviour by embedding the bases in base.go.
package steps

import (
	"context"

	"gopkg.in/yaml.v3"
)

// Function is anything written in a scenario as a function key: a step, or a
// check inside a checks step. Its parameters are its fields tagged `yaml`,
// documented by a `help` tag.
type Function interface {
	// Name is the function key, e.g. "startNode".
	Name() string
	// Help describes the function for `norma scenario-help`.
	Help() string
	// Validate reports semantic errors before the scenario runs.
	Validate() error
}

// Step is one blocking operation of a scenario.
type Step interface {
	Function
	// UpdateModel applies the step to a model of the network: it reports
	// what would make the step fail, and records the state the step leaves.
	// The executor applies it before Run, so a dry run and a real run reject
	// the same scenarios.
	UpdateModel(m *Model) error
	Run(ctx context.Context, env *Env) error
}

// ValueDecoder is implemented by functions that take a value under their key,
// such as the node name in `- stopNode: val-1`. A function without it accepts
// only a null value or a mapping of its parameters.
type ValueDecoder interface {
	DecodeValue(value *yaml.Node) error
}

// Targeted is implemented by steps acting on a named node or app, the name
// being reported in logs and step execution records.
type Targeted interface {
	Target() string
}

// Stabilizing is implemented by steps after which the network is expected to
// keep producing blocks, which the executor confirms before the next step.
type Stabilizing interface {
	AwaitsBlockProduction() bool
}

// TargetOf returns the target of step, or "" when it has none.
func TargetOf(step Step) string {
	if t, ok := step.(Targeted); ok {
		return t.Target()
	}
	return ""
}
