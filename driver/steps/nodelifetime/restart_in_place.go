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

package nodelifetime

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/node"
	"github.com/0xsoniclabs/norma/driver/steps"
)

// restartable is what a restart in place needs from a node.
type restartable interface {
	driver.Node
	StartSonicd(ctx context.Context) error
	StartSonicdAsObserver(ctx context.Context) error
	WaitForSync(ctx context.Context) error
}

var _ restartable = (*node.OperaNode)(nil)

// restartInPlace restarts the client process of a node whose container is
// still alive, as is the case after killSonic followed by healDb. The data
// directory is preserved, so the node rejoins with the history it already
// had rather than syncing from genesis.
func (s *StartNode) restartInPlace(ctx context.Context, env *steps.Env, existing driver.Node) error {
	name := s.Node.Node
	opera, ok := existing.(restartable)
	if !ok {
		return fmt.Errorf("node %q is not an OperaNode", name)
	}

	// The node keeps the role it was started with; UpdateModel refuses a step
	// asking for another, and an omitted type means that role.
	validator := opera.GetValidatorId() != nil
	slog.Info("restarting node in place", "node", name, "validator", validator)

	if validator {
		if err := opera.StartSonicd(ctx); err != nil {
			return fmt.Errorf("failed to restart sonicd on %s: %w", name, err)
		}
	} else {
		if err := opera.StartSonicdAsObserver(ctx); err != nil {
			return fmt.Errorf("failed to restart sonicd on %s: %w", name, err)
		}
	}

	if err := opera.WaitForSync(ctx); err != nil {
		return fmt.Errorf("failed to sync restarted node %s: %w", name, err)
	}

	// Re-establish peer connections so other nodes discover the restarted
	// client (the peer table does not survive the restart).
	if err := env.Net.ReconnectNode(ctx, existing); err != nil {
		return fmt.Errorf("failed to reconnect node %s: %w", name, err)
	}

	// Announce availability before waiting for the node to catch up, so
	// monitoring observes the blocks it processes while doing so.
	env.Net.ResumeNode(existing)

	// A node expected to fail may never catch up, as with a fresh start.
	if !s.Failing && !existing.IsExpectedFailure() {
		if err := steps.WaitForNodeSync(ctx, env.Net, existing, env.Timeouts.NodeSync); err != nil {
			return fmt.Errorf("restarted node %s did not catch up with the network: %w", name, err)
		}
	}

	slog.Info("node restarted in place", "node", name)
	return nil
}
