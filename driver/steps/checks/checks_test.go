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
	"errors"
	"testing"
	"time"

	"github.com/0xsoniclabs/norma/driver/checking"
	"github.com/0xsoniclabs/norma/driver/steps"
	"github.com/0xsoniclabs/norma/genesis"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func durationOf(d time.Duration) *steps.Duration {
	v := steps.Duration(d)
	return &v
}

// Every check translates its scenario parameters into the configuration of a
// named checker; a wrong name or key would silently check something else.
func TestChecks_RunTheirCheckerWithTheParametersOfTheScenario(t *testing.T) {
	five, three := 5, 3
	ceiling := 16_500_000.0
	maxBlockGas := uint64(30_000_000_000)
	rules := genesis.NetworkRulesPatch{Blocks: &genesis.BlocksPatch{MaxBlockGas: &maxBlockGas}}

	tests := map[string]struct {
		check   Check
		checker string
		// config is what the checker is configured with, nil when it is
		// used as registered.
		config checking.CheckerConfig
	}{
		"blocksProduced without parameters": {
			check:   &BlocksProduced{},
			checker: "blocksRolling",
		},
		"blocksProduced with a window": {
			check:   &BlocksProduced{Window: Window{Tolerance: &five, Duration: durationOf(30 * time.Second)}},
			checker: "blocksRolling",
			config:  checking.CheckerConfig{"tolerance": 5, "duration": int64(30 * time.Second)},
		},
		"blocksHalted expected to fail": {
			check:   &BlocksHalted{Failing: steps.Failing{Failing: true}},
			checker: "blocksHalted",
			config:  checking.CheckerConfig{"failing": true},
		},
		"blockGasRate with a ceiling": {
			check:   &BlockGasRate{Ceiling: &ceiling, Window: Window{Tolerance: &three}},
			checker: "blockGasRate",
			config:  checking.CheckerConfig{"ceiling": 16_500_000, "tolerance": 3},
		},
		"blockHeights passes its tolerance as slack": {
			check:   &BlockHeights{Tolerance: &five, Convergence: Convergence{Duration: durationOf(0)}},
			checker: "blockHeight",
			config:  checking.CheckerConfig{"slack": 5, "duration": int64(0)},
		},
		"blockHashes expected to fail": {
			check:   &BlockHashes{Failing: steps.Failing{Failing: true}},
			checker: "blocksHashes",
			config:  checking.CheckerConfig{"failing": true},
		},
		"networkRules with expected rules": {
			check:   &NetworkRules{Rules: rules, Convergence: Convergence{Duration: durationOf(10 * time.Second)}},
			checker: "networkRules",
			config:  checking.CheckerConfig{"rules": rules, "duration": int64(10 * time.Second)},
		},
		"eventThrottled with its nodes": {
			check:   &EventThrottled{ThrottledNodes: []string{"a", "b"}},
			checker: "eventThrottled",
			config:  checking.CheckerConfig{"throttledNodes": []string{"a", "b"}},
		},
		"validatorsActive": {
			check:   &ValidatorsActive{},
			checker: "validatorsActive",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			registered := checking.NewMockChecker(ctrl)
			checks := checking.Checks{test.checker: registered}

			if test.config == nil {
				registered.EXPECT().Check(gomock.Any()).Return(nil)
			} else {
				configured := checking.NewMockChecker(ctrl)
				registered.EXPECT().Configure(test.config).
					DoAndReturn(func(config checking.CheckerConfig) checking.Checker {
						require.NoError(t, config.Check(), "the checker would reject its configuration")
						return configured
					})
				// A check expected to fail passes when its checker fails.
				var checkErr error
				if failing, _ := test.config["failing"].(bool); failing {
					checkErr = errors.New("observed failure")
				}
				configured.EXPECT().Check(gomock.Any()).Return(checkErr)
			}

			require.NoError(t, test.check.Run(t.Context(), checks))
		})
	}
}

func TestChecks_FailingCheckFails_WhenItsCheckerPasses(t *testing.T) {
	ctrl := gomock.NewController(t)
	registered := checking.NewMockChecker(ctrl)
	configured := checking.NewMockChecker(ctrl)
	registered.EXPECT().Configure(gomock.Any()).Return(configured)
	configured.EXPECT().Check(gomock.Any()).Return(nil)

	check := &BlocksHalted{Failing: steps.Failing{Failing: true}}
	err := check.Run(t.Context(), checking.Checks{"blocksHalted": registered})
	require.ErrorContains(t, err, "failure expected")
}

func TestChecks_ReportAMissingChecker(t *testing.T) {
	err := (&BlockHashes{}).Run(t.Context(), checking.Checks{})
	require.ErrorContains(t, err, `checker "blocksHashes" not found`)
}

func TestChecks_AreSkipped_WhenNoCheckerIsConfigured(t *testing.T) {
	require.NoError(t, (&BlockHashes{}).Run(t.Context(), nil))
}

func TestChecksStep_NamesTheCheckThatFailed(t *testing.T) {
	ctrl := gomock.NewController(t)
	hashes := checking.NewMockChecker(ctrl)
	heights := checking.NewMockChecker(ctrl)
	hashes.EXPECT().Check(gomock.Any()).Return(nil)
	heights.EXPECT().Check(gomock.Any()).Return(errors.New("nodes disagree"))

	env := steps.NewEnv(nil, checking.Checks{"blocksHashes": hashes, "blockHeight": heights}, nil)
	step := &Checks{Items: []Check{&BlockHashes{}, &BlockHeights{}}}
	err := step.Run(t.Context(), env)
	require.ErrorContains(t, err, "check 2 (blockHeights): nodes disagree")
}

func TestNetworkRules_RejectsRulesTheNetworkWouldRefuse(t *testing.T) {
	// Rules from Allegro on are validated; a single parent cannot form a DAG.
	maxParents, allegro := uint64(1), true
	check := &NetworkRules{Rules: genesis.NetworkRulesPatch{
		Dag:      &genesis.DagPatch{MaxParents: &maxParents},
		Upgrades: &genesis.UpgradesPatch{Allegro: &allegro},
	}}
	require.ErrorContains(t, check.Validate(), "invalid rules")
	require.NoError(t, (&NetworkRules{}).Validate())
}

func TestBlockGasRate_RejectsANegativeCeiling(t *testing.T) {
	negative, zero := -1.0, 0.0
	require.ErrorContains(t, (&BlockGasRate{Ceiling: &negative}).Validate(), "ceiling must be non-negative")
	require.NoError(t, (&BlockGasRate{Ceiling: &zero}).Validate())
	require.NoError(t, (&BlockGasRate{}).Validate())

	// The window floor still applies alongside it.
	short := 1
	require.ErrorContains(t, (&BlockGasRate{Window: Window{Tolerance: &short}}).Validate(), "observation window")
}

func TestBlockHeights_RejectsANegativeTolerance(t *testing.T) {
	negative, zero := -1, 0
	require.ErrorContains(t, (&BlockHeights{Tolerance: &negative}).Validate(), "tolerance must be non-negative")
	require.NoError(t, (&BlockHeights{Tolerance: &zero}).Validate())
	require.NoError(t, (&BlockHeights{}).Validate())
}
