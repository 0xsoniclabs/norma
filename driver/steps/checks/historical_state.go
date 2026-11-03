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

// HistoricalState asserts that all nodes serve the same logs and past account
// state.
type HistoricalState struct {
	Failing
}

func (*HistoricalState) Name() string { return "historicalState" }

func (*HistoricalState) Help() string {
	return "Assert that all nodes serve the same logs, from receipts and log index alike, " +
		"and the same account state at sampled past blocks."
}

func (*HistoricalState) Validate() error { return nil }

func (c *HistoricalState) Run(ctx context.Context, checks checking.Checks) error {
	return runChecker(ctx, checks, "historicalState", newConfig(c.Failing))
}
