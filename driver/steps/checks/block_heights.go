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
)

// BlockHeights asserts that all nodes converge on the same block height.
type BlockHeights struct {
	Tolerance *int `yaml:"tolerance" help:"Allowed deviation (int, in blocks) between the heights of the nodes."`
	Convergence
	Failing
}

func (*BlockHeights) Name() string { return "blockHeights" }

func (*BlockHeights) Help() string {
	return "Assert that all nodes are within tolerance of the same block height, " +
		"waiting up to duration for them to agree."
}

// Validate rejects only a negative tolerance: demanding exact agreement is
// legitimate.
func (c *BlockHeights) Validate() error {
	if c.Tolerance != nil && *c.Tolerance < 0 {
		return fmt.Errorf("tolerance must be non-negative, got %d", *c.Tolerance)
	}
	return nil
}

func (c *BlockHeights) Run(ctx context.Context, checks checking.Checks) error {
	config := newConfig(c.Failing)
	c.configure(config)
	if c.Tolerance != nil {
		config["slack"] = *c.Tolerance
	}
	return runChecker(ctx, checks, "blockHeight", config)
}
