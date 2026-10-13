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
	"errors"
	"fmt"
	"time"

	"github.com/0xsoniclabs/norma/driver/node"
	"gopkg.in/yaml.v3"
)

// Node is the base of steps acting on the node named by their value, as in
// `- stopNode: val-1`.
type Node struct {
	Node string
}

func (n *Node) DecodeValue(value *yaml.Node) error {
	return decodeName(value, &n.Node)
}

func (n Node) Target() string {
	return n.Node
}

func (n Node) Validate() error {
	if n.Node == "" {
		return errors.New("requires a node identifier")
	}
	return matchName("node", n.Node)
}

// Opera resolves the node to the tracked OperaNode norma drives through its
// container.
func (n Node) Opera(env *Env) (*node.OperaNode, error) {
	tracked, ok := env.Nodes[n.Node]
	if !ok {
		return nil, fmt.Errorf("node %q not found in active nodes", n.Node)
	}
	opera, ok := tracked.(*node.OperaNode)
	if !ok {
		return nil, fmt.Errorf("node %q is not an OperaNode", n.Node)
	}
	return opera, nil
}

// App is the base of steps acting on the application named by their value,
// as in `- stopApp: load`.
type App struct {
	App string
}

func (a *App) DecodeValue(value *yaml.Node) error {
	return decodeName(value, &a.App)
}

func (a App) Target() string {
	return a.App
}

func (a App) Validate() error {
	if a.App == "" {
		return errors.New("requires an app identifier")
	}
	return matchName("app", a.App)
}

// Failing is the parameter marking a step or check as expected to fail.
type Failing struct {
	Failing bool `yaml:"failing" help:"When true, failure is expected; a passing result is treated as an error."`
}

// Duration is a non-negative time.Duration written as a string such as "30s".
type Duration time.Duration

func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err != nil {
		return err
	}
	parsed, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", s, err)
	}
	if parsed < 0 {
		return fmt.Errorf("duration must be non-negative, got %s", parsed)
	}
	*d = Duration(parsed)
	return nil
}

func decodeName(value *yaml.Node, name *string) error {
	if value.Kind != yaml.ScalarNode {
		return errors.New("value must be a name")
	}
	if !IsNull(value) {
		*name = value.Value
	}
	return nil
}

func matchName(kind, name string) error {
	if !node.NamePattern.MatchString(name) {
		return fmt.Errorf("%s name must match %v, got %v", kind, node.NamePattern, name)
	}
	return nil
}
