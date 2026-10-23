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

package nodelifetime

import (
	"maps"
	"strings"
	"testing"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/steps"
	"go.uber.org/mock/gomock"
)

// The client steps act through a node's container, so naming a node norma did
// not create must be reported rather than reaching a nil one.
func TestClientSteps_RejectNodesTheyCannotDrive(t *testing.T) {
	target := steps.Node{Node: "ghost"}
	all := map[string]steps.Step{
		"stopSonic": &StopSonic{target},
		"killSonic": &KillSonic{target},
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
				maps.Copy(env.Nodes, target.nodes)

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
