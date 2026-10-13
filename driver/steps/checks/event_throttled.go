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

package checks

import (
	"context"

	"github.com/0xsoniclabs/norma/driver/checking"
	"github.com/0xsoniclabs/norma/driver/steps"
)

// EventThrottled asserts that the listed validators emit events markedly
// slower than the rest.
type EventThrottled struct {
	ThrottledNodes []string `yaml:"throttledNodes" help:"List of node labels expected to be throttled."`
	steps.Failing
}

func (*EventThrottled) Name() string { return "eventThrottled" }

func (*EventThrottled) Help() string {
	return "Assert that validators listed in throttledNodes emit events at a significantly lower rate than the rest."
}

func (*EventThrottled) Validate() error { return nil }

func (c *EventThrottled) Run(ctx context.Context, checks checking.Checks) error {
	config := newConfig(c.Failing)
	if len(c.ThrottledNodes) > 0 {
		config["throttledNodes"] = c.ThrottledNodes
	}
	return runChecker(ctx, checks, "eventThrottled", config)
}
