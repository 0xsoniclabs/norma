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
	"time"

	"github.com/0xsoniclabs/norma/driver/steps"
)

// clientExitTimeout caps the wait for a client that stops itself, which
// happens no sooner than the epoch boundary it was told to stop at.
const clientExitTimeout = 5 * time.Minute

// WaitForSonicExit waits for a client that stops itself and leaves the node
// as stopSonic would. Suspended first, so nothing dials a vanishing RPC.
type WaitForSonicExit struct {
	steps.Node
}

func (*WaitForSonicExit) Name() string { return "waitForSonicExit" }

func (*WaitForSonicExit) Help() string {
	return "Wait for a sonicd process that stops itself, such as one " +
		"started with --exitwhensynced.epoch, then treat the node as stopped by stopSonic."
}

func (s *WaitForSonicExit) Run(ctx context.Context, env *steps.Env) error {
	opera, err := s.Opera(env)
	if err != nil {
		return err
	}
	env.Net.SuspendNode(opera)

	ctx, cancel := context.WithTimeout(ctx, clientExitTimeout)
	defer cancel()
	return opera.AwaitSonicdExit(ctx)
}
