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

// BlockHashes asserts that all nodes agree on the block hashes.
type BlockHashes struct {
	Failing
}

func (*BlockHashes) Name() string { return "blockHashes" }

func (*BlockHashes) Help() string {
	return "Assert that all nodes agree on the same block hashes."
}

func (*BlockHashes) Validate() error { return nil }

func (c *BlockHashes) Run(ctx context.Context, checks checking.Checks) error {
	return runChecker(ctx, checks, "blocksHashes", newConfig(c.Failing))
}
