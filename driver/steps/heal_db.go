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

package steps

import (
	"context"

	"github.com/0xsoniclabs/norma/driver/node"
)

// HealDb runs sonictool heal on a killed node, recovering its database.
type HealDb struct {
	Node
}

func (*HealDb) Name() string { return "healDb" }

func (*HealDb) Help() string {
	return "Run sonictool heal on a killed node to recover its database."
}

func (s *HealDb) Run(ctx context.Context, env *Env) error {
	return runSonicTool(ctx, env, s.Node, (*node.OperaNode).HealSonicd)
}
