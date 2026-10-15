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

package apps

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/node"
	"github.com/0xsoniclabs/norma/driver/steps"
	"github.com/0xsoniclabs/norma/load/app"
	"github.com/0xsoniclabs/norma/load/shaper"
)

// RunApp creates and starts a load-generating application.
type RunApp struct {
	steps.App
	Type  string       `yaml:"type" help:"Application type."`
	Users *int         `yaml:"users" help:"Number of concurrent user accounts the application should simulate."`
	Rate  *shaper.Rate `yaml:"rate" help:"Transaction rate configuration for the application."`
	Nodes []string     `yaml:"nodes" help:"Nodes the application submits its transactions through, each selecting the node with that name and every instance started under it. All nodes when omitted."`
}

func (*RunApp) Name() string { return "runApp" }

func (*RunApp) Help() string {
	return "Start a load-generating application."
}

func (*RunApp) AwaitsBlockProduction() bool { return true }

func (s *RunApp) Validate() error {
	errs := []error{s.App.Validate()}

	if s.Type == "" {
		errs = append(errs, fmt.Errorf("run app requires a type"))
	} else if !app.IsSupportedApplicationType(s.Type) {
		errs = append(errs, fmt.Errorf("unknown application type: %v", s.Type))
	}

	if s.Rate == nil {
		errs = append(errs, fmt.Errorf("run app requires a rate"))
	} else if err := s.Rate.Check(); err != nil {
		errs = append(errs, err)
	}

	if s.Users != nil && *s.Users < 1 {
		errs = append(errs, fmt.Errorf("number of users must be >= 1, got %d", *s.Users))
	}

	for _, name := range s.Nodes {
		errs = append(errs, steps.Node{Node: name}.Validate())
	}

	return errors.Join(errs...)
}

func (s *RunApp) UpdateModel(m *steps.Model) error {
	if m.Apps[s.App.App] {
		return fmt.Errorf("application %q is already running", s.App.App)
	}
	for _, name := range s.Nodes {
		labels := steps.SelectLabels(m.Nodes, name)
		if len(labels) == 0 {
			return fmt.Errorf("node %q not found in active nodes", name)
		}
		if !slices.ContainsFunc(labels, func(label string) bool {
			return m.Nodes[label].Client == node.NodeStateRunning
		}) {
			return fmt.Errorf("node %q has no running client to submit through", name)
		}
	}
	m.Apps[s.App.App] = true
	return nil
}

func (s *RunApp) Run(ctx context.Context, env *steps.Env) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	users := 1
	if s.Users != nil {
		users = *s.Users
	}

	var nodes []string
	for _, name := range s.Nodes {
		nodes = append(nodes, steps.SelectLabels(env.Nodes, name)...)
	}

	application, err := env.Net.CreateApplication(ctx, &driver.ApplicationConfig{
		Name:  s.App.App,
		Type:  s.Type,
		Rate:  s.Rate,
		Users: users,
		Nodes: nodes,
	})
	if err != nil {
		return fmt.Errorf("failed to create application %s: %w", s.App.App, err)
	}

	if err := application.Start(ctx); err != nil {
		return fmt.Errorf("failed to start application %s: %w", s.App.App, err)
	}

	env.Apps[s.App.App] = application
	return nil
}
