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
	"math"

	"github.com/0xsoniclabs/norma/driver/checking"
)

// BlockGasRate asserts that no block in the observation window exceeds a gas
// rate ceiling.
type BlockGasRate struct {
	Ceiling *float64 `yaml:"ceiling" help:"Maximum allowed gas rate of a block, a non-negative whole number."`
	Window
	Failing
}

func (*BlockGasRate) Name() string { return "blockGasRate" }

func (*BlockGasRate) Help() string {
	return "Assert that every block produced during the observation window has a gas rate at or below a ceiling."
}

func (c *BlockGasRate) Validate() error {
	// The checker compares against a whole number, so a fraction would be
	// truncated.
	if c.Ceiling != nil && (*c.Ceiling < 0 || *c.Ceiling != math.Trunc(*c.Ceiling)) {
		return fmt.Errorf("ceiling must be a non-negative whole number, got %v", *c.Ceiling)
	}
	return c.Window.Validate()
}

func (c *BlockGasRate) Run(ctx context.Context, checks checking.Checks) error {
	config := newConfig(c.Failing)
	c.configure(config)
	if c.Ceiling != nil {
		config["ceiling"] = int(*c.Ceiling)
	}
	return runChecker(ctx, checks, "blockGasRate", config)
}
