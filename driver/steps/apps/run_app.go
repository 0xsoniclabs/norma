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

	"github.com/0xsoniclabs/norma/driver"
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

	return errors.Join(errs...)
}

func (s *RunApp) Run(ctx context.Context, env *steps.Env) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	users := 1
	if s.Users != nil {
		users = *s.Users
	}

	application, err := env.Net.CreateApplication(ctx, &driver.ApplicationConfig{
		Name:  s.App.App,
		Type:  s.Type,
		Rate:  s.Rate,
		Users: users,
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
