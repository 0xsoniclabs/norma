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
	"strings"
	"testing"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/steps"
	"go.uber.org/mock/gomock"
)

// checkDb drives sonictool through the node's container, so naming a node
// norma did not create must be reported rather than reaching a nil one.
func TestCheckDb_RejectsNodesItCannotDrive(t *testing.T) {
	tests := map[string]struct {
		nodes     map[string]driver.Node
		wantError string
	}{
		"unknown node": {
			wantError: "not found in active nodes",
		},
		"node norma does not own": {
			nodes:     map[string]driver.Node{"ghost": nil},
			wantError: "is not an OperaNode",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			env := steps.NewEnv(driver.NewMockNetwork(gomock.NewController(t)), nil, nil)
			for label, node := range test.nodes {
				env.Nodes[label] = node
			}
			err := (&CheckDb{Node: steps.Node{Node: "ghost"}}).Run(t.Context(), env)
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}
