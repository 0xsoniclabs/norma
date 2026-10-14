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
	"fmt"
	"time"

	"github.com/0xsoniclabs/norma/driver/node"
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

// UpdateModel requires a client started to stop by itself; waiting for any
// other would only time out.
func (s *WaitForSonicExit) UpdateModel(m *steps.Model) error {
	n, err := m.Node(s.Node.Node)
	if err != nil {
		return err
	}
	// The node accepts a killed client only for a self-exit its watcher saw
	// first; in a scenario, a killed client was killed, and must be healed.
	if n.Client == node.NodeStateKilled {
		return fmt.Errorf("cannot wait for the exit of node %q while its client is killed", s.Node.Node)
	}
	if !n.ExitsWhenSynced {
		return fmt.Errorf("node %q was not started with --exitwhensynced, so its client will not stop by itself",
			s.Node.Node)
	}
	return m.Apply(s.Node.Node, node.AwaitClientExit, "wait for the exit of")
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
