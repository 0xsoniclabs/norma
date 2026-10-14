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
	"github.com/0xsoniclabs/norma/driver/steps/nodelifetime"
	"github.com/stretchr/testify/require"
)

// TestCheckScenarios iterates through all scenarios in this directory, its
// sub-directories and the experiments directory, and checks whether the
// contained YAML files define valid scenarios.
func TestCheckScenarios(t *testing.T) {
	files, err := listAll(".")
	require.NoError(t, err, "failed to get list of all scenario files")
	require.NotEmpty(t, files, "failed to locate any scenario files")
	experiments, err := listAll(experimentsDir)
	require.NoError(t, err, "failed to get list of all experiment files")
	require.NotEmpty(t, experiments, "failed to locate any experiment files")
	for _, file := range append(files, experiments...) {
		t.Run(file, func(t *testing.T) {
			scenario, err := parser.ParseFile(file)
			require.NoError(t, err, "failed to parse file", file)
			require.NoError(t, scenario.Check(), "scenario check failed for file", file)
			require.NoError(t, scenario.DryRun(), "dry run failed for file", file)
		})
	}
}

// TestReleaseTestingScenarios_UseLocalClient checks that every release testing
// scenario starts at least one node running the local sonic sources. A scenario
// that only uses released images would not test the code under release.
func TestReleaseTestingScenarios_UseLocalClient(t *testing.T) {
	files, err := listAll(".")
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
				start, ok := step.(*nodelifetime.StartNode)
				if ok && driver.ResolveClientImageName(start.ImageName) == driver.DefaultClientDockerImageName {
					return
				}
			}
			t.Errorf("scenario starts no node using %s", driver.DefaultClientDockerImageName)
		})
	}
	require.True(t, found, "failed to locate any scenario in %s", releaseTestingDir)
}

const releaseTestingDir = "release_testing"

// experimentsDir holds scenarios for one-off experiments, kept valid too.
const experimentsDir = "../experiments"

func listAll(root string) ([]string, error) {
	files := []string{}
	err := filepath.Walk(root,
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
