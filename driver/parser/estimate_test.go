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
