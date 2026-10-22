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
	"context"
	"testing"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/rpc"
	"github.com/0xsoniclabs/norma/driver/steps"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// A restart in place keeps the role the node was started with, also when the
// step omits the type: a validator restarts with its validator flags, an
// observer without.
func TestStartNode_RestartsWithTheNodesRole_WhenTheTypeIsOmitted(t *testing.T) {
	validatorId := 3
	tests := map[string]struct {
		validatorId *int
		wantStart   string
	}{
		"validator": {validatorId: &validatorId, wantStart: "validator"},
		"observer":  {validatorId: nil, wantStart: "observer"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			client := &recordingClient{MockNode: driver.NewMockNode(ctrl)}
			client.EXPECT().GetValidatorId().Return(test.validatorId).AnyTimes()

			net := driver.NewMockNetwork(ctrl)
			// An empty network leaves nothing to catch up with after the restart.
			net.EXPECT().DialRandomRpc().Return(nil, driver.ErrEmptyNetwork)
			net.EXPECT().ReconnectNode(gomock.Any(), client).Return(nil)
			net.EXPECT().ResumeNode(client)

			env := steps.NewEnv(net, nil, nil)
			env.Nodes["frail"] = client
			step := &StartNode{Node: steps.Node{Node: "frail"}}
			require.NoError(t, step.Run(t.Context(), env))
			require.Equal(t, test.wantStart, client.started)
		})
	}
}

// A node expected to fail may never catch up with the network, so a restart
// in place does not wait for it to, whether the step or the node says so. A
// DialRpc call to wait on the node would be unexpected on this mock.
func TestStartNode_RestartInPlace_DoesNotWaitForCatchUp_WhenTheNodeIsExpectedToFail(t *testing.T) {
	tests := map[string]struct {
		stepFailing, nodeFailing bool
	}{
		"step": {stepFailing: true},
		"node": {nodeFailing: true},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			client := &recordingClient{MockNode: driver.NewMockNode(ctrl)}
			client.EXPECT().GetValidatorId().Return(nil).AnyTimes()
			client.EXPECT().IsExpectedFailure().Return(test.nodeFailing).AnyTimes()

			head := rpc.NewMockClient(ctrl)
			head.EXPECT().BlockNumber(gomock.Any()).Return(uint64(10), nil)
			head.EXPECT().Close()
			net := driver.NewMockNetwork(ctrl)
			net.EXPECT().DialRandomRpc().Return(head, nil)
			net.EXPECT().ReconnectNode(gomock.Any(), client).Return(nil)
			net.EXPECT().ResumeNode(client)

			env := steps.NewEnv(net, nil, nil)
			env.Nodes["frail"] = client
			step := &StartNode{Node: steps.Node{Node: "frail"}, Failing: test.stepFailing}
			require.NoError(t, step.Run(t.Context(), env))
		})
	}
}

// recordingClient is a node a restart in place can drive, recording which
// start it was given.
type recordingClient struct {
	*driver.MockNode
	started string
}

func (c *recordingClient) StartSonicd(context.Context) error {
	c.started = "validator"
	return nil
}

func (c *recordingClient) StartSonicdAsObserver(context.Context) error {
	c.started = "observer"
	return nil
}

func (c *recordingClient) WaitForSync(context.Context) error {
	return nil
}
