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

	mon "github.com/0xsoniclabs/norma/driver/monitoring"
)

// NodeEpochStatus is a metric derived from NodeBlockStatus that exports
// the epoch number of each node over time.
var NodeEpochStatus = mon.Metric[mon.Node, mon.Series[mon.Time, int]]{
	Name:        "NodeEpochStatus",
	Description: "The epoch number of nodes at various times.",
}

func init() {
	if err := mon.RegisterSource(NodeEpochStatus, newNodeEpochStatusSource); err != nil {
		panic(fmt.Sprintf("failed to register metric source: %v", err))
	}
}

// nodeEpochStatusSource wraps NodeBlockStatus and extracts only the epoch.
type nodeEpochStatusSource struct {
	monitor *mon.Monitor
}

func newNodeEpochStatusSource(monitor *mon.Monitor) mon.Source[mon.Node, mon.Series[mon.Time, int]] {
	return &nodeEpochStatusSource{monitor: monitor}
}

func (s *nodeEpochStatusSource) GetMetric() mon.Metric[mon.Node, mon.Series[mon.Time, int]] {
	return NodeEpochStatus
}

func (s *nodeEpochStatusSource) GetSubjects() []mon.Node {
	return mon.GetSubjects(s.monitor, NodeBlockStatus)
}

func (s *nodeEpochStatusSource) GetData(node mon.Node) (mon.Series[mon.Time, int], bool) {
	return nil, false
}

func (s *nodeEpochStatusSource) Shutdown() error {
	return nil
}

func (s *nodeEpochStatusSource) ForEachRecord(consumer func(r mon.Record)) {
	subjects := mon.GetSubjects(s.monitor, NodeBlockStatus)
	for _, subject := range subjects {
		series, exists := mon.GetData(s.monitor, subject, NodeBlockStatus)
		if !exists {
			continue
		}

		latest := series.GetLatest()
		if latest == nil {
			continue
		}

		var first mon.Time
		allData := series.GetRange(first, latest.Position)

		r := mon.Record{}
		r.SetSubject(subject)
		for _, point := range allData {
			r.SetPosition(point.Position)
			r.Value = fmt.Sprintf("%d", point.Value.Epoch)
			consumer(r)
		}
		r.SetPosition(latest.Position)
		r.Value = fmt.Sprintf("%d", latest.Value.Epoch)
		consumer(r)
	}
}
