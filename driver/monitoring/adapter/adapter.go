package adapter

import (
	"github.com/0xsoniclabs/norma/driver/monitoring"
	nodemon "github.com/0xsoniclabs/norma/driver/monitoring/node"
)

//go:generate mockgen -source adapter.go -destination monitor_data_mock.go -package adapter

type MonitoringData interface {
	// GetNodes returns the nodes that are being monitored.
	GetNodes() []monitoring.Node
	// GetData returns the monitoring data for a specific node.
	GetData(monitoring.Node) monitoring.Series[monitoring.Time, monitoring.BlockStatus]
}

// MonitoringDataAdapter is an adapter that implements the MonitoringData interface
type monitoringDataAdapter struct {
	monitor *monitoring.Monitor
}

func NewMonitoringData(monitor *monitoring.Monitor) *monitoringDataAdapter {
	return &monitoringDataAdapter{monitor: monitor}
}

func (m *monitoringDataAdapter) GetNodes() []monitoring.Node {
	return monitoring.GetSubjects(m.monitor, nodemon.NodeBlockStatus)
}

func (m *monitoringDataAdapter) GetData(node monitoring.Node) monitoring.Series[monitoring.Time, monitoring.BlockStatus] {
	data, _ := monitoring.GetData(m.monitor, node, nodemon.NodeBlockStatus)
	return data
}
