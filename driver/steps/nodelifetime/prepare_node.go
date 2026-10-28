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
	"errors"
	"fmt"
	"log/slog"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/node"
	"github.com/0xsoniclabs/norma/driver/steps"
	"github.com/0xsoniclabs/norma/driver/steps/sonictool"
)

// PrepareNode creates a node bootstrapped from a g-file in the shared
// directory, leaving its client stopped so checkDb can verify what the file
// produced before the node joins. A later startNode with the same identifier
// restarts it in place. Validators are refused: their stake has to be
// registered before they join, which only startNode does.
type PrepareNode struct {
	steps.Node
	sonictool.SharedFile
	Type           string `yaml:"type" help:"Node type: \"observer\" (the default) or \"rpc\"."`
	ImageName      string `yaml:"imageName" help:"Docker image name to use for the node."`
	Failing        bool   `yaml:"failing" help:"When true, the node is expected to diverge from the network."`
	ExtraArguments string `yaml:"extraArguments" help:"Extra command line arguments for sonicd."`
}

func (*PrepareNode) Name() string { return "prepareNode" }

func (*PrepareNode) Help() string {
	return "Create a node bootstrapped from a g-file in the shared directory, leaving its client stopped; " +
		"a later startNode with the same identifier starts it."
}

func (s *PrepareNode) Validate() error {
	errs := []error{s.Node.Validate(), s.SharedFile.Validate()}
	switch s.Type {
	case "", "observer", "rpc":
	case "validator":
		errs = append(errs, errors.New("prepareNode cannot prepare a validator, only an observer or rpc node"))
	default:
		errs = append(errs, fmt.Errorf("type of node must be observer or rpc, was set to %s", s.Type))
	}
	return errors.Join(errs...)
}

// UpdateModel adds the node with its client stopped. The g-file has to have
// been exported by an earlier step: the shared directory starts empty.
func (s *PrepareNode) UpdateModel(m *steps.Model) error {
	name := s.Node.Node
	if _, exists := m.Nodes[name]; exists {
		return fmt.Errorf("node %q already exists", name)
	}
	for label := range m.Nodes {
		if steps.IsInstanceLabel(label, name) {
			return fmt.Errorf("node %q still has active instances, such as %q; stop it with stopNode first",
				name, label)
		}
	}
	if !m.SharedFiles[s.File] {
		return fmt.Errorf("file %q is not in the shared directory; export it in an earlier step", s.File)
	}
	m.Nodes[name] = &steps.ModelNode{
		Client:          node.NodeStateReady,
		ExitsWhenSynced: exitsWhenSynced(s.ExtraArguments),
	}
	m.Started[name] = true
	return nil
}

func (s *PrepareNode) Run(ctx context.Context, env *steps.Env) error {
	name := s.Node.Node
	image := driver.DefaultClientDockerImageName
	if s.ImageName != "" {
		image = s.ImageName
	}

	prepared, err := env.Net.CreateNode(ctx, &driver.NodeConfig{
		Name:           name,
		Failing:        s.Failing,
		Image:          image,
		ExtraArguments: s.ExtraArguments,
		GenesisFile:    s.File,
	})
	if err != nil {
		return fmt.Errorf("failed to prepare node %s: %w", name, err)
	}

	env.Nodes[name] = prepared
	env.NodeHistory[name] = true
	slog.Info("node prepared from g-file", "node", name, "file", s.File, "image", image)
	return nil
}
