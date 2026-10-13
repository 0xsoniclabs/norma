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

package sonictool

import (
	"strings"
	"testing"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/steps"
	"go.uber.org/mock/gomock"
)

// sonictool runs inside a node's container, so naming a node norma did not
// create must be reported rather than reaching a nil one.
func TestSonicToolSteps_RejectNodesTheyCannotDrive(t *testing.T) {
	target := steps.Node{Node: "ghost"}
	file := NodeFile{Node: target, SharedFile: SharedFile{File: "exported.g"}}
	all := map[string]steps.Step{
		"healDb":        &HealDb{target},
		"exportGenesis": &ExportGenesis{file},
		"importGenesis": &ImportGenesis{file},
		"exportEvents":  &ExportEvents{file},
		"importEvents":  &ImportEvents{file},
	}

	targets := map[string]struct {
		nodes     map[string]driver.Node
		wantError string
	}{
		"unknown node": {
			nodes:     nil,
			wantError: "not found in active nodes",
		},
		"node norma does not own": {
			nodes:     map[string]driver.Node{"ghost": nil},
			wantError: "is not an OperaNode",
		},
	}

	for stepName, step := range all {
		for targetName, target := range targets {
			t.Run(stepName+"/"+targetName, func(t *testing.T) {
				// No call is expected on the network: the step must fail
				// before it announces anything.
				env := steps.NewEnv(driver.NewMockNetwork(gomock.NewController(t)), nil, nil)
				for name, n := range target.nodes {
					env.Nodes[name] = n
				}

				err := step.Run(t.Context(), env)
				if err == nil {
					t.Fatalf("expected %s to fail", stepName)
				}
				if !strings.Contains(err.Error(), target.wantError) {
					t.Errorf("unexpected error: %v", err)
				}
			})
		}
	}
}
