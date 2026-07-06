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

package netmon

import (
	"fmt"
	"io"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/monitoring"
	"go.uber.org/mock/gomock"
)

func TestValidatorStakeSource_CollectsDataPeriodically(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)
	net.EXPECT().RegisterListener(gomock.Any()).AnyTimes()
	net.EXPECT().GetActiveNodes().AnyTimes().DoAndReturn(func() []driver.Node {
		node := driver.NewMockNode(ctrl)
		node.EXPECT().GetLabel().AnyTimes().Return("node-a")
		node.EXPECT().StreamLog(gomock.Any()).AnyTimes().Return(io.NopCloser(strings.NewReader(monitoring.Node1TestLog)), nil)
		url := driver.URL("node-a")
		node.EXPECT().GetServiceUrl(gomock.Any()).AnyTimes().Return(&url, nil)
		return []driver.Node{node}
	})

	monitor, err := monitoring.NewMonitor(net, monitoring.MonitorConfig{OutputDir: t.TempDir()})
	if err != nil {
		t.Fatalf("failed to initialize monitor: %v", err)
	}

	ticks := 0
	source := newValidatorStakeSourceWithCollector(monitor, 20*time.Millisecond, func() (map[int]string, error) {
		ticks++
		return map[int]string{
			1: "5000000000000000000000000",
			2: "5000000000000000000000000",
		}, nil
	})

	time.Sleep(90 * time.Millisecond)
	if err := source.Shutdown(); err != nil {
		t.Fatalf("failed to shutdown source: %v", err)
	}

	series1, exists := source.GetData(monitoring.Node("validator-1"))
	if !exists || series1 == nil {
		t.Fatalf("missing series for validator-1")
	}
	data1 := series1.GetRange(monitoring.Time(0), monitoring.Time(math.MaxInt64))
	if len(data1) == 0 {
		t.Fatalf("expected data points for validator-1")
	}
	for _, point := range data1 {
		if got, want := point.Value, "5000000000000000000000000"; got != want {
			t.Fatalf("unexpected stake for validator-1, got %s want %s", got, want)
		}
	}

	series2, exists := source.GetData(monitoring.Node("validator-2"))
	if !exists || series2 == nil {
		t.Fatalf("missing series for validator-2")
	}
	if got := len(series2.GetRange(monitoring.Time(0), monitoring.Time(math.MaxInt64))); got == 0 {
		t.Fatalf("expected data points for validator-2")
	}
	if ticks < 2 {
		t.Fatalf("expected at least two collection ticks, got %d", ticks)
	}
}

func TestFetchValidatorStakes_ReturnsEmptyWhenNetworkIsEmpty(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)
	net.EXPECT().DialRandomRpc().Return(nil, driver.ErrEmptyNetwork)

	stakes, err := fetchValidatorStakes(net)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(stakes) != 0 {
		t.Fatalf("expected empty stakes, got %d entries", len(stakes))
	}
}

func TestFetchValidatorStakes_ReturnsErrorForDialFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)
	net.EXPECT().DialRandomRpc().Return(nil, fmt.Errorf("boom"))

	stakes, err := fetchValidatorStakes(net)
	if err == nil {
		t.Fatalf("expected an error")
	}
	if stakes != nil {
		t.Fatalf("expected nil stakes on error")
	}
}
