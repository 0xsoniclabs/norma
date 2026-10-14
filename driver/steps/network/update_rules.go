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

package network

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/0xsoniclabs/norma/driver/steps"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/genesis"
	"gopkg.in/yaml.v3"
)

// UpdateRules applies a network rules patch at runtime.
type UpdateRules struct {
	Rules genesis.NetworkRulesPatch
}

func (*UpdateRules) Name() string { return "updateRules" }

func (*UpdateRules) Help() string {
	return "Update one or more network rules (key/value pairs)."
}

func (*UpdateRules) AwaitsBlockProduction() bool { return true }

func (s *UpdateRules) DecodeValue(value *yaml.Node) error {
	if value.Kind == yaml.MappingNode {
		if err := steps.DecodeKnown(value, &s.Rules); err != nil {
			return fmt.Errorf("invalid updateRules value: %w", err)
		}
		return nil
	}
	if !steps.IsNull(value) {
		return fmt.Errorf("updateRules value must be a mapping, got %q", value.Value)
	}
	return nil
}

func (s *UpdateRules) Validate() error {
	if b, err := json.Marshal(s.Rules); err != nil || string(b) == "{}" {
		return fmt.Errorf("update rules requires at least one rule")
	}
	return nil
}

func (s *UpdateRules) Run(ctx context.Context, env *steps.Env) error {
	if err := env.Net.ApplyNetworkRules(ctx, driver.NetworkRules(s.Rules)); err != nil {
		return fmt.Errorf("failed to apply network rules: %w", err)
	}
	return nil
}
