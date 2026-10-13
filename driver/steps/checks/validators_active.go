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

// ValidatorsActive asserts that every running validator node takes part in
// consensus.
type ValidatorsActive struct {
	Failing
}

func (*ValidatorsActive) Name() string { return "validatorsActive" }

func (*ValidatorsActive) Help() string {
	return "Assert that every running validator node is in the current epoch's validator set."
}

func (*ValidatorsActive) Validate() error { return nil }

func (c *ValidatorsActive) Run(ctx context.Context, checks checking.Checks) error {
	return runChecker(ctx, checks, "validatorsActive", newConfig(c.Failing))
}
