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
	"testing"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/monitoring"
	"go.uber.org/mock/gomock"
)

func TestChecker_DefaultChecker_Success(t *testing.T) {
	tmpDir := t.TempDir()

	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)
	net.EXPECT().RegisterListener(gomock.Any()).AnyTimes()
	net.EXPECT().GetActiveNodes().AnyTimes()
	monitor, err := monitoring.NewMonitor(net, monitoring.MonitorConfig{
		EvaluationLabel: "test",
		OutputDir:       tmpDir,
	})
	if err != nil {
		t.Errorf("failed to start monitor; %v", err)
	}
	t.Cleanup(func() {
		_ = monitor.Shutdown()
	})

	registrations = make(registry)
	checkers := InitNetworkChecks(net, monitor)

	if len(checkers) != len(defaultRegistrations) {
		t.Errorf("Expected %d checkers, got %d", len(defaultRegistrations), len(checkers))
	}
}

func TestChecker_CustomChecker_Success(t *testing.T) {
	type check struct {
		name   string
		typ    string
		config map[string]string
	}
	type testcase struct {
		checks []check
	}
	tests := []testcase{
		{[]check{
			{"test1", "block_height_checker", map[string]string{"slack": "5"}},
		}},
		{[]check{
			{"test2", "blocks_hashes_checker", map[string]string{}},
		}},
		{[]check{
			{"test3", "blocks_hashes_checker", map[string]string{"random": "ignored"}},
		}},
		{[]check{
			{"test4", "blocks_rolling_checker", map[string]string{"tolerance": "10"}},
		}},
		{[]check{
			{"test5", "block_height_checker", map[string]string{"slack": "5"}},
			{"test6", "blocks_hashes_checker", map[string]string{}},
			{"test7", "blocks_rolling_checker", map[string]string{"tolerance": "10"}},
		}},
	}

	for _, test := range tests {
		tmpDir := t.TempDir()

		ctrl := gomock.NewController(t)
		net := driver.NewMockNetwork(ctrl)
		net.EXPECT().RegisterListener(gomock.Any()).AnyTimes()
		net.EXPECT().GetActiveNodes().AnyTimes()

		monitor, err := monitoring.NewMonitor(net, monitoring.MonitorConfig{
			EvaluationLabel: "test",
			OutputDir:       tmpDir,
		})
		if err != nil {
			t.Errorf("failed to start monitor; %v", err)
		}
		t.Cleanup(func() {
			_ = monitor.Shutdown()
		})

		registrations = make(registry)
		for _, chk := range test.checks {
			err := RegisterNetworkCheck(chk.name, chk.typ, chk.config)
			if err != nil {
				t.Errorf("Should succeed when registering %+v; got %v", chk, err)
			}
		}

		checkers := InitNetworkChecks(net, monitor)
		if len(checkers) != len(test.checks) {
			t.Errorf("Expected %d checkers, got %d", len(test.checks), len(checkers))
		}

	}
}
