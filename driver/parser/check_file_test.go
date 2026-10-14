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

package parser

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCheckFile_ReturnsTheScenario_WhenItIsValid(t *testing.T) {
	path := writeScenario(t, "Name: Valid\nDescription: d\nScenario:\n  - startNode: val\n    type: validator\n")
	scenario, err := CheckFile(path)
	require.NoError(t, err)
	require.Equal(t, "Valid", scenario.Name)
}

// Each kind of problem is reported, as norma check, build and run all rely on
// this one function.
func TestCheckFile_ReportsTheProblem_WhenTheScenarioIsNotValid(t *testing.T) {
	tests := map[string]struct {
		content   string
		wantError string
	}{
		"not a scenario": {
			content:   ":::not yaml",
			wantError: "failed to parse scenario file",
		},
		"a step failing its own check": {
			content:   "Name: T\nDescription: d\nScenario:\n  - startNode: val\n    type: miner\n",
			wantError: "type of node must be",
		},
		"a step that cannot run after the ones before it": {
			content:   "Name: T\nDescription: d\nScenario:\n  - startNode: val\n    type: validator\n  - healDb: val\n",
			wantError: `dry run failed: step 2 (healDb val): cannot heal node "val" while its client is running`,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := CheckFile(writeScenario(t, test.content))
			require.ErrorContains(t, err, test.wantError)
		})
	}

	_, err := CheckFile(filepath.Join(t.TempDir(), "missing.yml"))
	require.ErrorContains(t, err, "failed to parse scenario file")
}

func writeScenario(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "scenario.yml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}
