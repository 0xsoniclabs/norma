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
	"testing"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/steps"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// The step hands the g-file and image to the network and asks for nothing
// else: no validator registration, no sync wait.
func TestPrepareNode_BootstrapsFromTheGFileWithoutStarting(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)
	prepared := driver.NewMockNode(ctrl)
	net.EXPECT().CreateNode(gomock.Any(), &driver.NodeConfig{
		Name:        "fresh",
		Image:       "sonic:v2.2.0",
		GenesisFile: "from-new.g",
	}).Return(prepared, nil)

	env := steps.NewEnv(net, nil, nil)
	step := &PrepareNode{
		Node:      steps.Node{Node: "fresh"},
		File:      "from-new.g",
		ImageName: "sonic:v2.2.0",
	}
	require.NoError(t, step.Run(t.Context(), env))
	require.Equal(t, prepared, env.Nodes["fresh"])
	require.True(t, env.NodeHistory["fresh"])
}
