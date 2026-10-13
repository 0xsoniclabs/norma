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

	"github.com/0xsoniclabs/norma/driver/steps"

	"github.com/0xsoniclabs/norma/driver"
	"golang.org/x/sync/errgroup"
)

// StopNode stops a node, or every numbered instance started under its name,
// and removes its container.
type StopNode struct {
	steps.Node
}

func (*StopNode) Name() string { return "stopNode" }

func (*StopNode) Help() string {
	return "Stop a running network node by name."
}

func (s *StopNode) Run(ctx context.Context, env *steps.Env) error {
	base := s.Node.Node

	type stopTarget struct {
		name string
		node driver.Node
	}
	targets := make([]stopTarget, 0)
	for name, node := range env.Nodes {
		if name == base || steps.IsInstanceLabel(name, base) {
			targets = append(targets, stopTarget{name: name, node: node})
		}
	}

	if len(targets) == 0 {
		return fmt.Errorf("node %q not found in active nodes", base)
	}

	g, gctx := errgroup.WithContext(ctx)
	for _, target := range targets {
		g.Go(func() error {
			if err := env.Net.RemoveNode(target.node); err != nil {
				return fmt.Errorf("failed to remove node %s: %w", target.name, err)
			}
			if err := target.node.Stop(gctx); err != nil {
				return fmt.Errorf("failed to stop node %s: %w", target.name, err)
			}
			if err := target.node.Cleanup(gctx); err != nil {
				return fmt.Errorf("failed to cleanup node %s: %w", target.name, err)
			}
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return err
	}

	for _, target := range targets {
		delete(env.Nodes, target.name)
	}

	return nil
}
