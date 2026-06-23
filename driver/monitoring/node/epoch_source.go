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
