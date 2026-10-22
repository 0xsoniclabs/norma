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

package parser

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDryRun_ReportsTheStep_WhenItCannotRun(t *testing.T) {
	tests := map[string]struct {
		steps     string
		wantError string
	}{
		"heal a running node": {
			steps:     "  - healDb: val\n",
			wantError: `step 2 (healDb val): cannot heal node "val" while its client is running`,
		},
		"start a killed node": {
			steps:     "  - killSonic: val\n  - startNode: val\n",
			wantError: `step 3 (startNode val): cannot start node "val" while its client is killed`,
		},
		"start a running node": {
			steps:     "  - startNode: val\n",
			wantError: `cannot start node "val" while its client is running`,
		},
		"start a name next to its active instances": {
			steps:     "  - startNode: obs\n    instances: 2\n  - startNode: obs\n",
			wantError: `node "obs" still has active instances`,
		},
		"restart in place with several instances": {
			steps:     "  - stopSonic: val\n  - startNode: val\n    instances: 2\n",
			wantError: `restarting node "val" in place cannot start 2 instances`,
		},
		"stop a stopped client": {
			steps:     "  - stopSonic: val\n  - stopSonic: val\n",
			wantError: `cannot stop the client of node "val" while its client is stopped`,
		},
		"restart in place as another role": {
			steps:     "  - startNode: obs\n  - stopSonic: obs\n  - startNode: obs\n    type: validator\n",
			wantError: `node "obs" was started as an observer; restarting it in place as a validator is not supported`,
		},
		"export from a running client": {
			steps:     "  - exportGenesis: val\n    file: a.g\n",
			wantError: `cannot export a g-file from node "val" while its client is running`,
		},
		"import a file no step exported": {
			steps:     "  - stopSonic: val\n  - importEvents: val\n    file: missing.events\n",
			wantError: `file "missing.events" is not in the shared directory`,
		},
		"wait for a client that does not stop by itself": {
			steps:     "  - waitForSonicExit: val\n",
			wantError: "was not started with --exitwhensynced",
		},
		"act on an unknown node": {
			steps:     "  - killSonic: ghost\n",
			wantError: `node "ghost" not found in active nodes`,
		},
		"stop a node twice": {
			steps:     "  - stopNode: val\n  - stopNode: val\n",
			wantError: `step 3 (stopNode val): node "val" not found in active nodes`,
		},
		"rejoin an observer as a validator": {
			steps:     "  - startNode: obs\n  - stopNode: obs\n  - startNode: obs\n    type: validator\n",
			wantError: "validator obs is rejoining but has no preserved validator ID",
		},
		"run an app through an unknown node": {
			steps:     "  - runApp: load\n    type: counter\n    rate:\n      constant: 1\n    nodes: [ghost]\n",
			wantError: `step 2 (runApp load): node "ghost" not found in active nodes`,
		},
		"run an app through a killed node": {
			steps:     "  - startNode: obs\n  - killSonic: obs\n  - runApp: load\n    type: counter\n    rate:\n      constant: 1\n    nodes: [obs]\n",
			wantError: `step 4 (runApp load): node "obs" has no running client to submit through`,
		},
		"stop an app that is not running": {
			steps:     "  - stopApp: load\n",
			wantError: `application "load" not found in active apps`,
		},
		"delegate to an observer": {
			steps:     "  - startNode: obs\n  - delegate:\n    - node: obs\n      delegator: alice\n      stake: 100\n",
			wantError: `node "obs" is not a validator`,
		},
		"undelegate as an unused delegator": {
			steps:     "  - undelegate:\n    - node: val\n      delegator: alice\n",
			wantError: `delegator "alice" was never used in a prior delegate step`,
		},
		"undelegate a drained delegation": {
			steps: "  - delegate:\n    - node: val\n      delegator: alice\n      stake: 100\n" +
				"  - undelegate:\n    - node: val\n      delegator: alice\n" +
				"  - undelegate:\n    - node: val\n      delegator: alice\n",
			wantError: `step 4 (undelegate): delegator "alice" has no stake on node "val"`,
		},
		// As in a run, an explicit stake of 0 undelegates all of it.
		"undelegate after a zero stake drained it": {
			steps: "  - delegate:\n    - node: val\n      delegator: alice\n      stake: 100\n" +
				"  - undelegate:\n    - node: val\n      delegator: alice\n      stake: 0\n" +
				"  - undelegate:\n    - node: val\n      delegator: alice\n      stake: 1\n",
			wantError: `step 4 (undelegate): delegator "alice" has no stake on node "val"`,
		},
		"wait for a client whose exit flag is only part of another argument": {
			steps: "  - startNode: obs\n    extraArguments: \"--datadir /tmp/--exitwhensynced.epoch\"\n" +
				"  - waitForSonicExit: obs\n",
			wantError: "was not started with --exitwhensynced",
		},
		// The node itself accepts a killed client only for a self-exit seen
		// first; a scenario's killed client was killed and must be healed.
		"wait for the exit of a killed client": {
			steps: "  - startNode: obs\n    extraArguments: \"--exitwhensynced.epoch 3\"\n" +
				"  - killSonic: obs\n  - waitForSonicExit: obs\n",
			wantError: `cannot wait for the exit of node "obs" while its client is killed`,
		},
		"prepare an existing node": {
			steps:     "  - prepareNode: val\n    file: a.g\n",
			wantError: `node "val" already exists`,
		},
		"prepare from a file never exported": {
			steps:     "  - prepareNode: fresh\n    file: a.g\n",
			wantError: `file "a.g" is not in the shared directory`,
		},
		"prepare a name next to its active instances": {
			steps: "  - startNode: obs\n    instances: 2\n  - stopSonic: val\n" +
				"  - exportGenesis: val\n    file: a.g\n  - prepareNode: obs\n    file: a.g\n",
			wantError: `node "obs" still has active instances`,
		},
		"undelegate more than was delegated": {
			steps: "  - delegate:\n    - node: val\n      delegator: alice\n      stake: 100\n" +
				"  - undelegate:\n    - node: val\n      delegator: alice\n      stake: 500\n",
			wantError: `step 3 (undelegate): delegator "alice" has 100 S on node "val", cannot undelegate 500 S`,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			require.ErrorContains(t, dryRun(t, test.steps), test.wantError)
		})
	}
}

func TestDryRun_Succeeds_WhenEveryStepCanRun(t *testing.T) {
	tests := map[string]string{
		"kill, heal and restart keeping the role": "  - killSonic: val\n  - healDb: val\n  - startNode: val\n",
		"restart with the same role":              "  - stopSonic: val\n  - startNode: val\n    type: validator\n",
		"move a file between nodes": "  - startNode: obs\n  - stopSonic: val\n  - stopSonic: obs\n" +
			"  - exportEvents: val\n    file: a.events\n  - importEvents: obs\n    file: a.events\n",
		"wait for a client told to stop": "  - startNode: obs\n    extraArguments: \"--exitwhensynced.epoch 3\"\n" +
			"  - waitForSonicExit: obs\n  - checkDb: obs\n",
		"bootstrap a node from a g-file": "  - stopSonic: val\n  - exportGenesis: val\n    file: a.g\n" +
			"  - prepareNode: fresh\n    file: a.g\n  - checkDb: fresh\n  - startNode: fresh\n",
		"rejoin a validator": "  - stopNode: val\n  - startNode: val\n    type: validator\n",
		"drain a delegation": "  - delegate:\n    - node: val\n      delegator: alice\n      stake: 100\n" +
			"  - undelegate:\n    - node: val\n      delegator: alice\n",
		"stop the instances of a name": "  - startNode: obs\n    instances: 2\n  - stopNode: obs\n",
		"delegate to the only remaining instance": "  - startNode: heavy\n    type: validator\n    instances: 2\n" +
			"  - stopNode: heavy-0\n  - delegate:\n    - node: heavy\n      delegator: alice\n      stake: 100\n",
		"start a name again once its instances stopped": "  - startNode: obs\n    instances: 2\n" +
			"  - stopNode: obs\n  - startNode: obs\n",
		"run and stop an app": "  - runApp: load\n    type: counter\n    rate:\n      constant: 1\n" +
			"  - stopApp: load\n",
		"run an app through the instances of a name": "  - startNode: rpc\n    instances: 2\n" +
			"  - runApp: load\n    type: counter\n    rate:\n      constant: 1\n    nodes: [val, rpc]\n",
	}
	for name, steps := range tests {
		t.Run(name, func(t *testing.T) {
			require.NoError(t, dryRun(t, steps))
		})
	}
}

func TestDryRun_Fails_WhenTheFirstStepDoesNotStartAValidator(t *testing.T) {
	for name, input := range map[string]string{
		"an observer":    "Name: T\nDescription: t\nScenario:\n  - startNode: obs\n",
		"another step":   "Name: T\nDescription: t\nScenario:\n  - advanceEpoch\n",
		"no step at all": "Name: T\nDescription: t\nDisableEndChecks: true\n",
	} {
		t.Run(name, func(t *testing.T) {
			scenario, err := ParseBytes([]byte(input))
			require.NoError(t, err)
			require.Error(t, scenario.DryRun())
		})
	}
}

// dryRun parses a scenario whose first step starts the validator val, followed
// by the given steps, and dry-runs it.
func dryRun(t *testing.T, steps string) error {
	t.Helper()
	scenario, err := ParseBytes([]byte(`
Name: Test
Description: Test
DisableEndChecks: true
Scenario:
  - startNode: val
    type: validator
` + steps))
	require.NoError(t, err)
	require.NoError(t, scenario.Check())
	return scenario.DryRun()
}
