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
	"fmt"
)

// StopApp stops a running application.
type StopApp struct {
	App
}

func (*StopApp) Name() string { return "stopApp" }

func (*StopApp) Help() string {
	return "Stop a running load-generating application by name."
}

func (s *StopApp) Run(_ context.Context, env *Env) error {
	name := s.App.App
	application, ok := env.Apps[name]
	if !ok {
		return fmt.Errorf("application %q not found in active apps", name)
	}

	if err := application.Stop(); err != nil {
		return fmt.Errorf("failed to stop application %s: %w", name, err)
	}

	delete(env.Apps, name)
	return nil
}
