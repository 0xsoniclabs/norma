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

package checking

import (
	"context"
	"fmt"
	"testing"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/rpc"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestVerifyAllActive_Passes_WhenEveryValidatorIsInTheSet(t *testing.T) {
	err := verifyAllActive(
		map[int]string{1: "alpha", 2: "beta"},
		[]int{1, 2, 3},
	)
	require.NoError(t, err)
}

func TestVerifyAllActive_Fails_WhenAValidatorIsNotInTheSet(t *testing.T) {
	err := verifyAllActive(
		map[int]string{1: "genesis-validator", 2: "joining-validator"},
		[]int{1},
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "1 of 2 running validator nodes")
	require.Contains(t, err.Error(), "joining-validator (validator 2)")
	require.Contains(t, err.Error(), "active set is [1]")
	require.NotContains(t, err.Error(), "genesis-validator")
}

func TestVerifyAllActive_Fails_WhenTheSetIsEmpty(t *testing.T) {
	err := verifyAllActive(map[int]string{1: "alpha", 2: "beta"}, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "2 of 2 running validator nodes")
}

func TestVerifyAllActive_ListsMissingValidators_InStableOrder(t *testing.T) {
	expected := map[int]string{1: "alpha", 2: "beta", 3: "gamma"}
	for range 10 {
		err := verifyAllActive(expected, []int{1})
		require.Error(t, err)
		require.Contains(t, err.Error(),
			"beta (validator 2), gamma (validator 3)")
	}
}

// A validator emitting rarely — because it just joined, or because the event
// throttler suppresses its empty events — is still a full member.
func TestVerifyAllActive_Passes_ForAMemberThatEmitsRarely(t *testing.T) {
	require.NoError(t, verifyAllActive(
		map[int]string{1: "alpha", 4: "joining-pair-1"},
		[]int{1, 2, 3, 4},
	))
}

func TestExpectedValidatorLabels_SkipsNonValidatorsAndExpectedFailures(t *testing.T) {
	ctrl := gomock.NewController(t)
	validator := driver.NewMockNode(ctrl)
	observer := driver.NewMockNode(ctrl)
	failing := driver.NewMockNode(ctrl)

	id := 1
	validator.EXPECT().IsExpectedFailure().Return(false)
	validator.EXPECT().GetValidatorId().Return(&id)
	validator.EXPECT().GetLabel().Return("alpha")

	observer.EXPECT().IsExpectedFailure().Return(false)
	observer.EXPECT().GetValidatorId().Return(nil)

	// An expected failure may legitimately have left the set, so it must not
	// be consulted for a validator id at all.
	failing.EXPECT().IsExpectedFailure().Return(true)

	labels := expectedValidatorLabels(
		[]driver.Node{validator, observer, failing},
	)
	require.Equal(t, map[int]string{1: "alpha"}, labels)
}

func TestValidatorsActiveChecker_Fails_WhenNetworkHasNoNodes(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)
	net.EXPECT().GetActiveNodes().Return(nil)

	err := newValidatorsActiveChecker(net).Check(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "no active nodes")
}

func TestValidatorsActiveChecker_Fails_WhenNetworkHasNoValidators(t *testing.T) {
	ctrl := gomock.NewController(t)
	observer := driver.NewMockNode(ctrl)
	observer.EXPECT().IsExpectedFailure().Return(false)
	observer.EXPECT().GetValidatorId().Return(nil)

	net := driver.NewMockNetwork(ctrl)
	net.EXPECT().GetActiveNodes().Return([]driver.Node{observer})

	err := newValidatorsActiveChecker(net).Check(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "no active validator nodes")
}

func TestValidatorsActiveChecker_ReportsValidatorMissingFromTheSet(t *testing.T) {
	ctrl := gomock.NewController(t)
	checker, client := checkerWithNodes(ctrl, map[int]string{
		1: "genesis-validator",
		2: "joining-validator",
	})
	client.EXPECT().Close()
	checker.getActiveValidators = func(rpc.Client) ([]int, error) {
		return []int{1}, nil
	}

	err := checker.Check(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "joining-validator (validator 2)")
}

func TestValidatorsActiveChecker_Passes_WhenAllValidatorsAreInTheSet(t *testing.T) {
	ctrl := gomock.NewController(t)
	checker, client := checkerWithNodes(ctrl, map[int]string{
		1: "alpha", 2: "beta",
	})
	client.EXPECT().Close()
	checker.getActiveValidators = func(rpc.Client) ([]int, error) {
		return []int{1, 2}, nil
	}

	require.NoError(t, checker.Check(context.Background()))
}

func TestValidatorsActiveChecker_PropagatesValidatorSetReadFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	checker, client := checkerWithNodes(ctrl, map[int]string{1: "alpha"})
	client.EXPECT().Close()
	checker.getActiveValidators = func(rpc.Client) ([]int, error) {
		return nil, fmt.Errorf("SFC unreachable")
	}

	err := checker.Check(context.Background())
	require.ErrorContains(t, err, "SFC unreachable")
}

// checkerWithNodes builds a checker over a network of validator nodes with the
// given validator id to label mapping, all reachable over the returned client.
func checkerWithNodes(
	ctrl *gomock.Controller, validators map[int]string,
) (*validatorsActiveChecker, *rpc.MockClient) {
	client := rpc.NewMockClient(ctrl)

	nodes := make([]driver.Node, 0, len(validators))
	for id, label := range validators {
		id := id
		node := driver.NewMockNode(ctrl)
		node.EXPECT().IsExpectedFailure().Return(false).AnyTimes()
		node.EXPECT().GetValidatorId().Return(&id)
		node.EXPECT().GetLabel().Return(label)
		// dialFirstReachable stops at the first node that answers, and the node
		// order follows map iteration, so let any of them serve the dial.
		node.EXPECT().DialRpc(gomock.Any()).Return(client, nil).AnyTimes()
		nodes = append(nodes, node)
	}

	net := driver.NewMockNetwork(ctrl)
	net.EXPECT().GetActiveNodes().Return(nodes)

	checker := newValidatorsActiveChecker(net)
	return checker, client
}
