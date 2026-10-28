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

package wait

import (
	"context"

	"github.com/0xsoniclabs/norma/driver/steps"
)

// WaitForEpoch waits until the network reaches the next epoch boundary.
type WaitForEpoch struct{}

func (*WaitForEpoch) Name() string { return "waitForEpoch" }

func (*WaitForEpoch) Help() string {
	return "Wait until the network reaches the next epoch boundary."
}

func (*WaitForEpoch) Validate() error { return nil }

func (*WaitForEpoch) UpdateModel(*steps.Model) error { return nil }

func (*WaitForEpoch) Run(ctx context.Context, env *steps.Env) error {
	return steps.WaitForEpochChange(ctx, env.Net, env.Timeouts.EpochChange)
}
