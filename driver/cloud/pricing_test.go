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

package cloud

import (
	"testing"
	"time"

	"github.com/0xsoniclabs/norma/driver/parser"
	"github.com/stretchr/testify/require"
)

func TestNewPlan_SizesAndPricesTheRun_WhenGivenAnEstimate(t *testing.T) {
	tests := map[string]struct {
		peakNodes   int
		duration    time.Duration
		machineType string
		timeout     time.Duration
		wantMachine string
		wantTimeout time.Duration
	}{
		"a small scenario gets the smallest machine and the minimum timeout": {
			peakNodes:   1,
			duration:    2 * time.Minute,
			wantMachine: "n2-standard-4",
			wantTimeout: minTimeout,
		},
		"a larger scenario gets a machine that fits and twice its duration": {
			peakNodes:   10,
			duration:    20 * time.Minute,
			wantMachine: "n2-standard-16",
			wantTimeout: 40 * time.Minute,
		},
		"the given machine and timeout are kept": {
			peakNodes:   1,
			duration:    time.Minute,
			machineType: "n2-standard-32",
			timeout:     15 * time.Minute,
			wantMachine: "n2-standard-32",
			wantTimeout: 15 * time.Minute,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			est := parser.Estimate{Duration: test.duration, PeakNodes: test.peakNodes}

			plan, err := NewPlan(est, test.machineType, test.timeout)

			require.NoError(t, err)
			require.Equal(t, test.wantMachine, plan.Machine.Name)
			require.Equal(t, test.wantTimeout, plan.Timeout)
			require.Equal(t, test.wantTimeout+ttlMargin, plan.TTL)
			require.Greater(t, plan.WorstCaseCost, plan.ExpectedCost)
		})
	}
}

func TestNewPlan_Fails_WhenNoMachineFits(t *testing.T) {
	tests := map[string]struct {
		peakNodes   int
		machineType string
		wantErr     string
	}{
		"too many nodes for one machine": {peakNodes: 500, wantErr: "needs more than one host"},
		"a machine type without a price": {peakNodes: 1, machineType: "e2-micro", wantErr: `no price known for machine type "e2-micro"`},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := NewPlan(parser.Estimate{PeakNodes: test.peakNodes}, test.machineType, 0)
			require.ErrorContains(t, err, test.wantErr)
		})
	}
}
