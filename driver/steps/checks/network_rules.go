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
	"fmt"

	"github.com/0xsoniclabs/norma/driver/checking"
	"github.com/0xsoniclabs/norma/genesis"
)

// NetworkRules asserts that every node converges on the expected rules.
type NetworkRules struct {
	Rules genesis.NetworkRulesPatch `yaml:"rules" help:"Expected network rules, as a NetworkRulesPatch."`
	Convergence
	Failing
}

func (*NetworkRules) Name() string { return "networkRules" }

func (*NetworkRules) Help() string {
	return "Assert that the active network rules on all nodes match the expected rules patch."
}

func (c *NetworkRules) Validate() error {
	if c.Rules == (genesis.NetworkRulesPatch{}) {
		return nil
	}
	if err := genesis.ValidateNetworkRulesPatch(c.Rules); err != nil {
		return fmt.Errorf("invalid rules: %w", err)
	}
	return nil
}

func (c *NetworkRules) Run(ctx context.Context, checks checking.Checks) error {
	config := newConfig(c.Failing)
	c.configure(config)
	if c.Rules != (genesis.NetworkRulesPatch{}) {
		config["rules"] = c.Rules
	}
	return runChecker(ctx, checks, "networkRules", config)
}
