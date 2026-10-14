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

	"github.com/0xsoniclabs/norma/driver/steps"
	"github.com/0xsoniclabs/norma/driver/steps/sonictool"

	"github.com/0xsoniclabs/norma/driver/node"
)

// CheckDb verifies a stopped node's state database.
type CheckDb struct {
	steps.Node
	Mode string `yaml:"mode" help:"Which state database to check: \"live\" (the default) or \"archive\"."`
}

func (*CheckDb) Name() string { return "checkDb" }

func (*CheckDb) Help() string {
	return "Verify a stopped node's state database with sonictool check."
}

func (s *CheckDb) Validate() error {
	var errs []error
	errs = append(errs, s.Node.Validate())
	switch s.Mode {
	case "", node.DbCheckModeLive, node.DbCheckModeArchive:
	default:
		errs = append(errs, fmt.Errorf(
			"checkDb mode must be %q or %q, got %v",
			node.DbCheckModeLive, node.DbCheckModeArchive, s.Mode))
	}
	return errors.Join(errs...)
}

func (s *CheckDb) UpdateModel(m *steps.Model) error {
	return m.Apply(s.Node.Node, node.Maintenance, "check the database of")
}

func (s *CheckDb) Run(ctx context.Context, env *steps.Env) error {
	return sonictool.Run(ctx, env, s.Node, func(opera *node.OperaNode, ctx context.Context) error {
		return opera.CheckDatabase(ctx, s.Mode)
	})
}
