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

package scenarios

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/parser"
	"github.com/stretchr/testify/require"
)

// TestCheckScenarios iterates through all scenarios in this directory
// and its sub-directories and checks whether the contained YAML files
// define valid scenarios.
func TestCheckScenarios(t *testing.T) {
	files, err := listAll()
	require.NoError(t, err, "failed to get list of all scenario files")
	require.NotEmpty(t, files, "failed to locate any scenario files")
	for _, file := range files {
		t.Run(file, func(t *testing.T) {
			scenario, err := parser.ParseFile(file)
			require.NoError(t, err, "failed to parse file", file)
			require.NoError(t, scenario.Check(), "scenario check failed for file", file)
		})
	}
}

// TestReleaseTestingScenarios_UseLocalClient checks that every release testing
// scenario starts at least one node running the local sonic sources. A scenario
// that only uses released images would not test the code under release.
func TestReleaseTestingScenarios_UseLocalClient(t *testing.T) {
	files, err := listAll()
	require.NoError(t, err, "failed to get list of all scenario files")

	prefix := releaseTestingDir + string(filepath.Separator)
	found := false
	for _, file := range files {
		if !strings.HasPrefix(file, prefix) {
			continue
		}
		found = true
		t.Run(file, func(t *testing.T) {
			scenario, err := parser.ParseFile(file)
			require.NoError(t, err, "failed to parse file", file)

			for _, step := range scenario.Steps {
				if step.Function != parser.FuncStartNode {
					continue
				}
				if driver.ResolveClientImageName(step.ImageName) == driver.DefaultClientDockerImageName {
					return
				}
			}
			t.Errorf("scenario starts no node using %s", driver.DefaultClientDockerImageName)
		})
	}
	require.True(t, found, "failed to locate any scenario in %s", releaseTestingDir)
}

const releaseTestingDir = "release_testing"

func listAll() ([]string, error) {
	files := []string{}
	err := filepath.Walk(".",
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			if strings.HasSuffix(path, ".yml") || strings.HasSuffix(path, ".yaml") {
				files = append(files, path)
			}
			return nil
		})
	if err != nil {
		return nil, err
	}
	return files, nil
}
