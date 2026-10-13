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
	"log/slog"
	"time"

	"gopkg.in/yaml.v3"
)

// WaitFor pauses the scenario for a fixed duration.
type WaitFor struct {
	Duration time.Duration
}

func (*WaitFor) Name() string { return "waitFor" }

func (*WaitFor) Help() string {
	return "Pause scenario execution for a fixed duration."
}

func (s *WaitFor) DecodeValue(value *yaml.Node) error {
	if value.Kind != yaml.ScalarNode || value.Value == "" {
		return fmt.Errorf("waitFor requires a duration value (e.g., \"1s\", \"5m\")")
	}
	d, err := time.ParseDuration(value.Value)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", value.Value, err)
	}
	s.Duration = d
	return nil
}

func (s *WaitFor) Validate() error {
	if s.Duration <= 0 {
		return fmt.Errorf("waitFor requires a positive duration, got %v", s.Duration)
	}
	return nil
}

func (s *WaitFor) Run(ctx context.Context, _ *Env) error {
	slog.Info("waiting", "duration", s.Duration)
	select {
	case <-time.After(s.Duration):
		return nil
	case <-ctx.Done():
		return fmt.Errorf("context cancelled during waitFor: %w", ctx.Err())
	}
}
