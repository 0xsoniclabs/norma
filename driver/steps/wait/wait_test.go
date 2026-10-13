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

package wait

import (
	"context"
	"testing"
	"time"

	"github.com/0xsoniclabs/norma/driver/steps"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func decode(t *testing.T, input string) (steps.Step, error) {
	t.Helper()
	var node yaml.Node
	require.NoError(t, yaml.Unmarshal([]byte(input), &node))
	kinds := []func() steps.Step{
		func() steps.Step { return &WaitFor{} },
		func() steps.Step { return &WaitForBlocks{} },
	}
	return steps.Decode(node.Content[0], kinds, "step")
}

func TestWaitFor_ReadsItsDuration(t *testing.T) {
	step, err := decode(t, "waitFor: 1m30s\n")
	require.NoError(t, err)
	require.Equal(t, &WaitFor{Duration: 90 * time.Second}, step)
	require.NoError(t, step.Validate())
}

func TestWaitFor_RejectsAnInvalidDuration(t *testing.T) {
	tests := map[string]struct {
		input     string
		wantError string
	}{
		"missing":      {"waitFor:\n", "requires a duration value"},
		"without unit": {"waitFor: 5\n", `invalid duration "5"`},
		"word":         {"waitFor: soon\n", `invalid duration "soon"`},
		"list":         {"waitFor: [1s]\n", "requires a duration value"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := decode(t, test.input)
			require.ErrorContains(t, err, test.wantError)
		})
	}
}

func TestWaitFor_RejectsANonPositiveDuration(t *testing.T) {
	for _, input := range []string{"waitFor: 0s\n", "waitFor: -1s\n"} {
		step, err := decode(t, input)
		require.NoError(t, err)
		require.ErrorContains(t, step.Validate(), "positive duration", input)
	}
}

func TestWaitFor_StopsWaiting_WhenTheScenarioIsCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	err := (&WaitFor{Duration: time.Hour}).Run(ctx, nil)
	require.ErrorIs(t, err, context.Canceled)
}

func TestWaitForBlocks_RejectsANonCount(t *testing.T) {
	for _, input := range []string{"waitForBlocks:\n", "waitForBlocks: 1.5\n", "waitForBlocks: -1\n", "waitForBlocks: [3]\n"} {
		_, err := decode(t, input)
		require.Error(t, err, input)
	}
	step, err := decode(t, "waitForBlocks: 0\n")
	require.NoError(t, err)
	require.ErrorContains(t, step.Validate(), "positive block count")
}
