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

	"github.com/0xsoniclabs/norma/driver/checking"
)

// BlocksHalted asserts that block production has stopped.
type BlocksHalted struct {
	Window
	Failing
}

func (*BlocksHalted) Name() string { return "blocksHalted" }

func (*BlocksHalted) Help() string {
	return "Assert that block production has halted, by observing the network for the given duration."
}

func (c *BlocksHalted) Run(ctx context.Context, checks checking.Checks) error {
	config := newConfig(c.Failing)
	c.configure(config)
	return runChecker(ctx, checks, "blocksHalted", config)
}
