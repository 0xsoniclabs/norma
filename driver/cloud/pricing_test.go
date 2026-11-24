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

func TestNewPlan_SizesAndPricesOneVMPerNode_WhenGivenAnEstimate(t *testing.T) {
	tests := map[string]struct {
		hosts                      int
		duration                   time.Duration
		nodeMachine, driverMachine string
		timeout                    time.Duration
		wantNode, wantDriver       string
		wantTimeout                time.Duration
	}{
		"a small scenario gets the defaults and the minimum timeout": {
			hosts:       1,
			duration:    2 * time.Minute,
			wantNode:    "n2-standard-2",
			wantDriver:  "n2-standard-4",
			wantTimeout: minTimeout,
		},
		"many nodes get a larger driver and twice the duration": {
			hosts:       40,
			duration:    20 * time.Minute,
			wantNode:    "n2-standard-2",
			wantDriver:  "n2-standard-8",
			wantTimeout: 40 * time.Minute,
		},
		"the given machines and timeout are kept": {
			hosts:         3,
			duration:      time.Minute,
			nodeMachine:   "n2-standard-4",
			driverMachine: "n2-standard-32",
			timeout:       15 * time.Minute,
			wantNode:      "n2-standard-4",
			wantDriver:    "n2-standard-32",
			wantTimeout:   15 * time.Minute,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			est := parser.Estimate{Duration: test.duration, Hosts: test.hosts}

			plan, err := NewPlan(est, test.nodeMachine, test.driverMachine, test.timeout)

			require.NoError(t, err)
			require.Equal(t, test.hosts, plan.Hosts)
			require.Equal(t, test.wantNode, plan.NodeMachine.Name)
			require.Equal(t, test.wantDriver, plan.DriverMachine.Name)
			require.Equal(t, test.wantTimeout, plan.Timeout)
			require.Equal(t, test.wantTimeout+ttlMargin, plan.TTL)
			require.Equal(t, plan.DriverMachine.VCPUs+test.hosts*plan.NodeMachine.VCPUs, plan.VCPUs())
			require.Equal(t, (test.hosts+1)*localSSDGB, plan.LocalSSDGB())
			hourly := plan.WorstCaseCost / plan.TTL.Hours()
			require.Greater(t, hourly, plan.DriverMachine.Hourly+float64(test.hosts)*plan.NodeMachine.Hourly)
			require.Greater(t, plan.WorstCaseCost, plan.ExpectedCost)
		})
	}
}

func TestNewPlan_Fails_WhenItCannotBePriced(t *testing.T) {
	tests := map[string]struct {
		hosts       int
		nodeMachine string
		wantErr     string
	}{
		"a scenario without nodes":       {hosts: 0, wantErr: "the scenario starts no nodes"},
		"a machine type without a price": {hosts: 1, nodeMachine: "e2-micro", wantErr: `no price known for machine type "e2-micro"`},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := NewPlan(parser.Estimate{Hosts: test.hosts}, test.nodeMachine, "", 0)
			require.ErrorContains(t, err, test.wantErr)
		})
	}
}
