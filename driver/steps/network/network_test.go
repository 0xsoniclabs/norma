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

package network

import (
	"errors"
	"math"
	"testing"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/steps"
	"github.com/0xsoniclabs/norma/genesis"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"gopkg.in/yaml.v3"
)

func decode(t *testing.T, input string) (steps.Step, error) {
	t.Helper()
	var node yaml.Node
	require.NoError(t, yaml.Unmarshal([]byte(input), &node))
	kinds := []func() steps.Step{
		func() steps.Step { return &UpdateRules{} },
		func() steps.Step { return &Delegate{} },
		func() steps.Step { return &Undelegate{} },
	}
	return steps.Decode(node.Content[0], kinds, "step")
}

func TestUpdateRules_TakesOnlyARulesMapping(t *testing.T) {
	_, err := decode(t, "updateRules: fast\n")
	require.ErrorContains(t, err, "updateRules value must be a mapping")

	_, err = decode(t, "updateRules: [1]\n")
	require.ErrorContains(t, err, "updateRules value must be a mapping")

	// Left empty it parses, and is then rejected as changing nothing.
	step, err := decode(t, "updateRules:\n")
	require.NoError(t, err)
	require.ErrorContains(t, step.Validate(), "at least one rule")
}

func TestUndelegate_TakesANodeNameOrAListOfTargets(t *testing.T) {
	step, err := decode(t, "undelegate: val-1\n")
	require.NoError(t, err)
	require.Equal(t, &Undelegate{Targets: []UndelegateTarget{{Node: "val-1"}}}, step)

	_, err = decode(t, "undelegate:\n")
	require.ErrorContains(t, err, "at least one target")

	_, err = decode(t, "undelegate: {node: val-1}\n")
	require.ErrorContains(t, err, "a node name or a list of targets")
}

func TestUpdateRules_ReportsRulesTheNetworkRejects(t *testing.T) {
	net := driver.NewMockNetwork(gomock.NewController(t))
	net.EXPECT().ApplyNetworkRules(gomock.Any(), gomock.Any()).Return(errors.New("not a validator"))

	maxBlockGas := uint64(1_000)
	step := &UpdateRules{Rules: genesis.NetworkRulesPatch{
		Blocks: &genesis.BlocksPatch{MaxBlockGas: &maxBlockGas},
	}}
	err := step.Run(t.Context(), steps.NewEnv(net, nil, nil))
	require.ErrorContains(t, err, "failed to apply network rules: not a validator")
}

func TestAdvanceEpoch_ReportsAFailedSeal(t *testing.T) {
	net := driver.NewMockNetwork(gomock.NewController(t))
	// Without a node to dial, the surrounding block waits are skipped.
	net.EXPECT().DialRandomRpc().Return(nil, driver.ErrEmptyNetwork).AnyTimes()
	net.EXPECT().AdvanceEpoch(gomock.Any(), 1).Return(errors.New("seal reverted"))

	err := (&AdvanceEpoch{}).Run(t.Context(), steps.NewEnv(net, nil, nil))
	require.ErrorContains(t, err, "seal reverted")
}

// A misspelled optional field must not be ignored: an undelegate target
// without a stake withdraws everything, so `stkae: 100` would undelegate the
// full stake instead of 100 S.
func TestUndelegate_RejectsUnknownTargetFields(t *testing.T) {
	_, err := decode(t, "undelegate:\n  - node: val-1\n    stkae: 100\n")
	require.ErrorContains(t, err, `parameter "stkae" is not valid for an undelegate target`)

	step, err := decode(t, "undelegate:\n  - node: val-1\n    stake: 100\n    delegator: alice\n")
	require.NoError(t, err)
	stake := uint64(100)
	require.Equal(t, &Undelegate{Targets: []UndelegateTarget{
		{Node: "val-1", Stake: &stake, Delegator: "alice"},
	}}, step)
}

func TestDelegate_RejectsUnknownTargetFields(t *testing.T) {
	_, err := decode(t, "delegate:\n  - node: val-1\n    delegator: alice\n    stake: 100\n    bogus: 1\n")
	require.ErrorContains(t, err, `parameter "bogus" is not valid for a delegate target`)

	_, err = decode(t, "delegate:\n  - node: val-1\n    node: val-2\n")
	require.ErrorContains(t, err, `parameter "node" is given twice`)
}

// Unknown rules are rejected rather than dropped, which would apply the rest
// of the patch without them.
func TestUpdateRules_RejectsUnknownRules(t *testing.T) {
	_, err := decode(t, "updateRules:\n  Blocks:\n    MaxBlockGas: 100000\n    MaxEmptyBlockSkipPerod: 3s\n")
	require.ErrorContains(t, err, "field MaxEmptyBlockSkipPerod not found")
}

// The delegator is funded with the stake plus a gas budget; a stake that
// would wrap that sum around is rejected.
func TestDelegate_RejectsAStakeTheFundingCannotCover(t *testing.T) {
	limit := uint64(math.MaxUint64) - delegatorGasBudget
	step := &Delegate{Targets: []DelegateTarget{{Node: "val", Delegator: "alice", Stake: limit + 1}}}
	require.ErrorContains(t, step.Validate(), "stake must be at most")

	step.Targets[0].Stake = limit
	require.NoError(t, step.Validate())
}
