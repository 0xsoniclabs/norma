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

// Package sonictool holds the steps driving sonictool against a node's
// database: healing it, and moving its chain or event DAG through the
// network's shared directory.
package sonictool

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/0xsoniclabs/norma/driver/node"
	"github.com/0xsoniclabs/norma/driver/steps"
)

// SharedFile is the parameter naming a file in the network's shared
// directory.
type SharedFile struct {
	File string `yaml:"file" help:"Required. Plain name of a file in the network's shared directory."`
}

func (f SharedFile) Validate() error {
	if f.File == "" {
		return errors.New("requires a 'file' parameter")
	}
	if !node.FilePattern.MatchString(f.File) {
		return fmt.Errorf("file name must match %v, got %v", node.FilePattern, f.File)
	}
	return nil
}

// NodeFile is the base of steps moving a file between a stopped node and the
// shared directory.
type NodeFile struct {
	steps.Node
	SharedFile
}

func (f NodeFile) Validate() error {
	return errors.Join(f.Node.Validate(), f.SharedFile.Validate())
}

// sonicToolTimeout caps a single sonictool invocation. Each walks the whole
// node state, so the bound is generous for slow CI hosts.
const sonicToolTimeout = 3 * time.Minute

// Run runs tool against the node n names, bounded by
// sonicToolTimeout.
func Run(
	ctx context.Context, env *steps.Env, n steps.Node,
	tool func(*node.OperaNode, context.Context) error,
) error {
	opera, err := n.Opera(env)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, sonicToolTimeout)
	defer cancel()
	return tool(opera, ctx)
}
