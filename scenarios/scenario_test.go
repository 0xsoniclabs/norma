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

	"github.com/0xsoniclabs/norma/driver/parser"
)

// TestCheckScenarios iterates through all scenarios in this directory
// and its sub-directories and checks whether the contained YAML files
// define valid scenarios.
func TestCheckScenarios(t *testing.T) {
	files, err := listAll()
	if err != nil {
		t.Fatalf("failed to get list of all scenario files: %v", err)
	}
	if len(files) == 0 {
		t.Fatalf("failed to locate any scenario files!")
	}
	for _, file := range files {
		t.Run(file, func(t *testing.T) {
			scenario, err := parser.ParseFile(file)
			if err != nil {
				t.Fatalf("failed to parse file: %v", err)
			}
			if err = scenario.Check(); err != nil {
				t.Fatalf("scenario check failed for: %s: %v", file, err)
			}
		})
	}
}

func listAll() ([]string, error) {
	files := []string{}
	err := filepath.Walk(".",
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if strings.HasSuffix(path, ".yml") {
				files = append(files, path)
			}
			return nil
		})
	if err != nil {
		return nil, err
	}
	return files, nil
}
