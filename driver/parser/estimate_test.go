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
	"time"

	"github.com/stretchr/testify/require"
)

func TestEstimate_SumsStepTimesAndTracksPeaks_WhenTheScenarioIsValid(t *testing.T) {
	scenario, err := ParseBytes([]byte(`
Name: estimate
DisableEndChecks: true
InitialNetworkRules:
  Epochs:
    MaxEpochDuration: 20s
Scenario:
  - startNode: val
    type: validator
    instances: 3
  - waitFor: 30s
  - waitForEpoch
  - stopNode: val-0
  - startNode: obs
  - waitForBlocks: 10
  - stopSonic: obs
  - checkDb: obs
`))
	require.NoError(t, err)

	got, err := scenario.Estimate()
	require.NoError(t, err)

	want := stepTimes["startNode"] + 30*time.Second + 20*time.Second +
		stepTimes["stopNode"] + stepTimes["startNode"] + 10*blockTime + 2*defaultStepTime
	require.Equal(t, want, got.Duration)
	require.Equal(t, 3, got.PeakNodes)
	require.Equal(t, 0, got.PeakApps)
	require.Equal(t, []string{"checkDb", "stopSonic", "waitForBlocks"}, got.Uncalibrated)
}

func TestEstimate_CountsTheHostsNodesTakePlacedOnePerMachine_WhenNodesComeAndGo(t *testing.T) {
	tests := map[string]struct {
		steps string
		want  int
	}{
		"instances take a host each": {
			steps: "  - startNode: val\n    type: validator\n    instances: 3\n",
			want:  3,
		},
		"a node started again keeps its host": {
			steps: "  - startNode: val\n    type: validator\n  - startNode: obs\n" +
				"  - stopNode: obs\n  - startNode: obs\n",
			want: 2,
		},
		"a restarted client stays in its container": {
			steps: "  - startNode: val\n    type: validator\n  - startNode: obs\n" +
				"  - stopSonic: obs\n  - startNode: obs\n",
			want: 2,
		},
		"nodes continuing from one data volume share its host": {
			steps: "  - startNode: val\n    type: validator\n  - startNode: a\n    dataVolume: v\n" +
				"  - stopNode: a\n  - startNode: b\n    dataVolume: v\n",
			want: 2,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			scenario, err := ParseBytes([]byte("Name: n\nDescription: d\nDisableEndChecks: true\nScenario:\n" + test.steps))
			require.NoError(t, err)

			got, err := scenario.Estimate()

			require.NoError(t, err)
			require.Equal(t, test.want, got.Hosts)
		})
	}
}

// The hosts these scenarios took when run with `norma run --hosts`.
func TestEstimate_CountsTheHostsRealRunsTook_WhenGivenRepositoryScenarios(t *testing.T) {
	tests := map[string]int{
		"../../scenarios/examples/validators_are_really_validators.yml":                   4,
		"../../scenarios/release_testing/features/genesis/genesis_self_compatibility.yml": 3,
		"../../scenarios/examples/rejoin_upgraded_with_data_volume.yml":                   4,
	}
	for path, want := range tests {
		t.Run(path, func(t *testing.T) {
			scenario, err := ParseFile(path)
			require.NoError(t, err)

			got, err := scenario.Estimate()

			require.NoError(t, err)
			require.Equal(t, want, got.Hosts)
		})
	}
}
