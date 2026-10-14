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

	"github.com/0xsoniclabs/norma/driver/steps"
)

// AdvanceEpoch seals the current epoch through an on-chain transaction.
type AdvanceEpoch struct{}

func (*AdvanceEpoch) Name() string { return "advanceEpoch" }

func (*AdvanceEpoch) Help() string {
	return "Advance the network to the next epoch by sending transactions."
}

func (*AdvanceEpoch) AwaitsBlockProduction() bool { return true }

func (*AdvanceEpoch) Validate() error { return nil }

func (*AdvanceEpoch) UpdateModel(*steps.Model) error { return nil }

func (*AdvanceEpoch) Run(ctx context.Context, env *steps.Env) error {
	if err := steps.WaitForBlockProduction(ctx, env.Net); err != nil {
		return err
	}
	if err := env.Net.AdvanceEpoch(ctx, 1); err != nil {
		return err
	}
	return steps.WaitForBlockProduction(ctx, env.Net)
}
