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

package sonictool

import (
	"context"

	"github.com/0xsoniclabs/norma/driver/node"
	"github.com/0xsoniclabs/norma/driver/steps"
)

// ExportGenesis exports a stopped node's chain to the shared directory.
type ExportGenesis struct {
	NodeFile
}

func (*ExportGenesis) Name() string { return "exportGenesis" }

func (*ExportGenesis) Help() string {
	return "Export a stopped node's chain into a g-file in the shared directory."
}

func (s *ExportGenesis) Run(ctx context.Context, env *steps.Env) error {
	return Run(ctx, env, s.Node, func(opera *node.OperaNode, ctx context.Context) error {
		return opera.ExportGenesis(ctx, s.File)
	})
}
