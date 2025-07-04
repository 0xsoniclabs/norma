package checking

import (
	"github.com/0xsoniclabs/norma/driver/monitoring"
	netmon "github.com/0xsoniclabs/norma/driver/monitoring/network"
	nodemon "github.com/0xsoniclabs/norma/driver/monitoring/node"
)

//go:generate mockgen -source monitoring_data.go -destination monitoring_data_mock.go -package checking

// MonitoringData is an interface that defines a method to get monitoring data related to this checker.
type MonitoringData interface {
	// GetNodes returns the nodes that are being monitored.
	GetNodes() []monitoring.Node
	// GetBlockStatus returns the monitoring data for a specific node.
	GetBlockStatus(monitoring.Node) monitoring.Series[monitoring.Time, monitoring.BlockStatus]
	// GetBlockGasRate returns the block gas rate for the network.
	GetBlockGasRate() monitoring.Series[monitoring.BlockNumber, float64]
}

// MonitoringDataAdapter is an adapter that implements the MonitoringData interface
type monitoringDataAdapter struct {
	monitor *monitoring.Monitor
}

func (m *monitoringDataAdapter) GetNodes() []monitoring.Node {
	return monitoring.GetSubjects(m.monitor, nodemon.NodeBlockStatus)
}

func (m *monitoringDataAdapter) GetBlockStatus(node monitoring.Node) monitoring.Series[monitoring.Time, monitoring.BlockStatus] {
	data, _ := monitoring.GetData(m.monitor, node, nodemon.NodeBlockStatus)
	return data
}

func (m *monitoringDataAdapter) GetBlockGasRate() monitoring.Series[monitoring.BlockNumber, float64] {
	data, _ := monitoring.GetData(m.monitor, monitoring.Network{}, netmon.BlockGasRate)
	return data
}
