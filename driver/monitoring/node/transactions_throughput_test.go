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

package nodemon

import (
	"fmt"
	"math/rand"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/monitoring"
	"go.uber.org/mock/gomock"
)

func TestTransactionsThroughputSource(t *testing.T) {
	ctrl := gomock.NewController(t)

	net := driver.NewMockNetwork(ctrl)
	net.EXPECT().RegisterListener(gomock.Any()).AnyTimes()
	net.EXPECT().GetActiveNodes().AnyTimes().Return([]driver.Node{})

	monitor, err := monitoring.NewMonitor(net, monitoring.MonitorConfig{OutputDir: t.TempDir()})
	if err != nil {
		t.Fatalf("failed to initiate monitor: %v", err)
	}
	source := NewTransactionsThroughputSource(monitor)

	now := time.Now()
	seconds := now.Unix()
	loops := 100
	nodes := []monitoring.Node{"A", "B", "C"}

	expected := make(map[monitoring.Node][]float32, len(nodes))
	for _, node := range nodes {
		timeGrow := rand.Intn(10) + 1
		expectedTxsList := make([]float32, 0, loops)
		// insert certain transactions in the same controlled delay between each
		for i := 0; i < loops; i++ {
			// progressively growing time
			timeStamp := time.Unix(seconds+int64(i*timeGrow), 0)
			txs := rand.Intn(1000)
			expectedTxs := float32(txs) / float32(int64(i*timeGrow)-int64((i-1)*timeGrow))
			expectedTxsList = append(expectedTxsList, expectedTxs)

			b := monitoring.Block{Height: i, Time: timeStamp, Txs: txs}
			source.OnBlock(node, b)
		}
		expected[node] = expectedTxsList
	}

	for node, txs := range expected {
		t.Run(fmt.Sprintf("node-%s", node), func(t *testing.T) {
			series, exists := source.GetData(node)
			if !exists {
				t.Errorf("data should exist")
			}

			// skip first block which is off
			for i := 1; i < loops; i++ {
				if got, want := series.GetRange(monitoring.BlockNumber(i), monitoring.BlockNumber(i+1))[0].Value, txs[i]; got != want {
					t.Errorf("transaction throughput incorrect: %3.2f != %3.2f", got, want)
				}
			}
		})
	}

}

func TestTransactionsTimeDiffBelowSec(t *testing.T) {
	ctrl := gomock.NewController(t)

	net := driver.NewMockNetwork(ctrl)
	net.EXPECT().RegisterListener(gomock.Any()).AnyTimes()
	net.EXPECT().GetActiveNodes().AnyTimes().Return([]driver.Node{})

	monitor, err := monitoring.NewMonitor(net, monitoring.MonitorConfig{OutputDir: t.TempDir()})
	if err != nil {
		t.Fatalf("failed to initiate monitor: %v", err)
	}
	source := NewTransactionsThroughputSource(monitor)

	seconds := time.Now().Unix()
	nsDiff := int64(50)
	secDif := 50 / 1e9

	// time diff only 50ns
	source.OnBlock("A", monitoring.Block{Height: 10, Time: time.Unix(seconds, 0), Txs: 10})
	source.OnBlock("A", monitoring.Block{Height: 11, Time: time.Unix(seconds, nsDiff), Txs: 10})

	series, exists := source.GetData("A")
	if !exists {
		t.Errorf("data should exist")
	}

	if got, want := series.GetLatest().Value, float32(10)/float32(secDif); got != want {
		t.Errorf("transaction throughput incorrect: %3.2f != %3.2f", got, want)
	}
}

func TestTransactionsCsvExport(t *testing.T) {
	ctrl := gomock.NewController(t)

	net := driver.NewMockNetwork(ctrl)
	net.EXPECT().RegisterListener(gomock.Any()).AnyTimes()
	net.EXPECT().GetActiveNodes().AnyTimes().Return([]driver.Node{})

	config := monitoring.MonitorConfig{OutputDir: t.TempDir()}
	monitor, err := monitoring.NewMonitor(net, config)
	if err != nil {
		t.Fatalf("failed to initiate monitor: %v", err)
	}
	source := NewTransactionsThroughputSource(monitor)
	factory := sourceFactory[monitoring.Node, monitoring.Series[monitoring.BlockNumber, float32]]{TransactionsThroughput, source}
	if err := monitoring.InstallSource[monitoring.Node, monitoring.Series[monitoring.BlockNumber, float32]](monitor, &factory); err != nil {
		t.Fatalf("failed to install source: %v", err)
	}

	seconds := time.Now().Unix()

	// time diff only 50ns
	source.OnBlock("A", monitoring.Block{Height: 10, Time: time.Unix(seconds, 0), Txs: 10})
	source.OnBlock("A", monitoring.Block{Height: 11, Time: time.Unix(seconds+1, 0), Txs: 10})

	if err := monitor.Shutdown(); err != nil {
		t.Fatalf("failed to shut down monitoring: %v", err)
	}

	content, _ := os.ReadFile(monitor.GetMeasurementFileName())
	if got, want := string(content), "TransactionsThroughput, network, A, , , 11, , 10\n"; !strings.Contains(got, want) {
		t.Errorf("unexpected export: %v != %v", got, want)
	}
}

func TestTransactionsBellowMeasurableDiff(t *testing.T) {
	ctrl := gomock.NewController(t)

	net := driver.NewMockNetwork(ctrl)
	net.EXPECT().RegisterListener(gomock.Any()).AnyTimes()
	net.EXPECT().GetActiveNodes().AnyTimes().Return([]driver.Node{})

	config := monitoring.MonitorConfig{OutputDir: t.TempDir()}
	monitor, err := monitoring.NewMonitor(net, config)
	if err != nil {
		t.Fatalf("failed to initiate monitor: %v", err)
	}
	source := NewTransactionsThroughputSource(monitor)

	seconds := time.Now().Unix()

	// time diff bellow measurable diff
	source.OnBlock("A", monitoring.Block{Height: 10, Time: time.Unix(seconds, 0), Txs: 10})
	source.OnBlock("A", monitoring.Block{Height: 11, Time: time.Unix(seconds, 0), Txs: 10})

	series, _ := source.GetData("A")
	if got := series.GetLatest(); got != nil {
		t.Errorf("there should be no value")
	}
}

func TestTransactionsZeroTransactionsBellowMeasurableDiff(t *testing.T) {
	ctrl := gomock.NewController(t)

	net := driver.NewMockNetwork(ctrl)
	net.EXPECT().RegisterListener(gomock.Any()).AnyTimes()
	net.EXPECT().GetActiveNodes().AnyTimes().Return([]driver.Node{})

	config := monitoring.MonitorConfig{OutputDir: t.TempDir()}
	monitor, err := monitoring.NewMonitor(net, config)
	if err != nil {
		t.Fatalf("failed to initiate monitor: %v", err)
	}
	source := NewTransactionsThroughputSource(monitor)

	seconds := time.Now().Unix()

	// time diff bellow measurable diff
	source.OnBlock("A", monitoring.Block{Height: 10, Time: time.Unix(seconds, 0), Txs: 0})
	source.OnBlock("A", monitoring.Block{Height: 11, Time: time.Unix(seconds, 0), Txs: 0})

	series, _ := source.GetData("A")
	if got := series.GetLatest(); got != nil {
		t.Errorf("there should be no value")
	}
}

func TestTransactionsThroughputSource_SuppressesConflictWhileNodeIsSyncing(t *testing.T) {
	ctrl := gomock.NewController(t)

	net := driver.NewMockNetwork(ctrl)
	net.EXPECT().RegisterListener(gomock.Any()).AnyTimes()
	net.EXPECT().GetActiveNodes().AnyTimes().Return([]driver.Node{})

	monitor, err := monitoring.NewMonitor(net, monitoring.MonitorConfig{OutputDir: t.TempDir()})
	if err != nil {
		t.Fatalf("failed to initiate monitor: %v", err)
	}
	source := NewTransactionsThroughputSource(monitor)

	node := monitoring.Node("A")
	series := source.GetOrAddSubject(node)
	if err := series.Append(monitoring.BlockNumber(11), 1); err != nil {
		t.Fatalf("failed to seed series: %v", err)
	}

	baseTime := time.Unix(100, 0)
	source.lastTimes[node] = baseTime
	source.OnBlock(node, monitoring.Block{Height: 11, Time: baseTime.Add(time.Second), Txs: 1})

	if !source.isNodeSyncing(node) {
		t.Fatalf("node syncing state should remain enabled after suppressed conflict")
	}

	if got, want := len(series.GetRange(monitoring.BlockNumber(0), monitoring.BlockNumber(100))), 1; got != want {
		t.Fatalf("unexpected number of points: %d != %d", got, want)
	}
}

func TestTransactionsThroughputSource_DisablesSyncingAfterFirstSuccessfulAppend(t *testing.T) {
	ctrl := gomock.NewController(t)

	net := driver.NewMockNetwork(ctrl)
	net.EXPECT().RegisterListener(gomock.Any()).AnyTimes()
	net.EXPECT().GetActiveNodes().AnyTimes().Return([]driver.Node{})

	monitor, err := monitoring.NewMonitor(net, monitoring.MonitorConfig{OutputDir: t.TempDir()})
	if err != nil {
		t.Fatalf("failed to initiate monitor: %v", err)
	}
	source := NewTransactionsThroughputSource(monitor)

	node := monitoring.Node("A")
	baseTime := time.Unix(100, 0)
	source.OnBlock(node, monitoring.Block{Height: 10, Time: baseTime, Txs: 1})
	source.OnBlock(node, monitoring.Block{Height: 11, Time: baseTime.Add(time.Second), Txs: 1})

	if source.isNodeSyncing(node) {
		t.Fatalf("node syncing state should be disabled after first successful append")
	}
}

func TestTransactionsThroughputSource_SuppressesAfterResync(t *testing.T) {
	ctrl := gomock.NewController(t)

	net := driver.NewMockNetwork(ctrl)
	net.EXPECT().RegisterListener(gomock.Any()).AnyTimes()
	net.EXPECT().GetActiveNodes().AnyTimes().Return([]driver.Node{})

	monitor, err := monitoring.NewMonitor(net, monitoring.MonitorConfig{OutputDir: t.TempDir()})
	if err != nil {
		t.Fatalf("failed to initiate monitor: %v", err)
	}
	source := NewTransactionsThroughputSource(monitor)

	node := monitoring.Node("A")
	baseTime := time.Unix(100, 0)
	source.OnBlock(node, monitoring.Block{Height: 10, Time: baseTime, Txs: 1})
	source.OnBlock(node, monitoring.Block{Height: 11, Time: baseTime.Add(time.Second), Txs: 1})

	// After successful appends the node is synced. An out-of-order
	// error at this point means the node restarted, so suppress it.
	if !source.shouldSuppressAppendConflict(node, monitoring.ErrOutOfOrderAppend) {
		t.Fatalf("out-of-order conflict should be suppressed (node restarted)")
	}
	if !source.isNodeSyncing(node) {
		t.Fatalf("node should be re-marked as syncing after out-of-order conflict")
	}
}
