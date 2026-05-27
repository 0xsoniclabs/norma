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
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/monitoring"
	"github.com/0xsoniclabs/norma/driver/monitoring/utils"
	opera "github.com/0xsoniclabs/norma/driver/node"
	"github.com/ethereum/go-ethereum/rpc"
)

// NodeBlockStatus collects a per-node time series of its current block height.
var NodeBlockStatus = monitoring.Metric[monitoring.Node, monitoring.Series[monitoring.Time, monitoring.BlockStatus]]{
	Name:        "NodeBlockStatus",
	Description: "The epoch number and block height of nodes at various times.",
}

func init() {
	if err := monitoring.RegisterSource(NodeBlockStatus, NewNodeBlockStatusSource); err != nil {
		panic(fmt.Sprintf("failed to register metric source: %v", err))
	}
}

// NewNodeBlockStatusSource creates a new data source periodically collecting data on
// the block height at various nodes over time.
func NewNodeBlockStatusSource(monitor *monitoring.Monitor) monitoring.Source[monitoring.Node, monitoring.Series[monitoring.Time, monitoring.BlockStatus]] {
	return newNodeBlockStatusSource(monitor, time.Second)
}

func newNodeBlockStatusSource(monitor *monitoring.Monitor, period time.Duration) monitoring.Source[monitoring.Node, monitoring.Series[monitoring.Time, monitoring.BlockStatus]] {
	return newPeriodicNodeDataSource[monitoring.BlockStatus](NodeBlockStatus, monitor, period, &blockProgressSensorFactory{})
}

type blockProgressSensorFactory struct{}

func (f *blockProgressSensorFactory) CreateSensor(node driver.Node) (utils.Sensor[monitoring.BlockStatus], error) {
	url := node.GetServiceUrl(&opera.OperaRpcService)
	if url == nil {
		return nil, fmt.Errorf("node does not export an RPC server")
	}
	// current version of eth in sonic doesn't allow access to inner client
	rpcClient, err := rpc.DialContext(context.Background(), string(*url))
	if err != nil {
		return nil, err
	}
	return &blockProgressSensor{rpcClient}, nil
}

type blockProgressSensor struct {
	rpcClient *rpc.Client
}

func (s *blockProgressSensor) ReadValue() (monitoring.BlockStatus, error) {
	var raw map[string]interface{}
	err := s.rpcClient.Call(&raw, "eth_getBlockByNumber", "latest", false)
	if err != nil {
		return monitoring.BlockStatus{}, err
	}

	epoch, err := strconv.ParseUint(raw["epoch"].(string), 0, 64)
	if err != nil {
		return monitoring.BlockStatus{}, err
	}

	number, err := strconv.ParseUint(raw["number"].(string), 0, 64)
	if err != nil {
		return monitoring.BlockStatus{}, err
	}

	return monitoring.BlockStatus{
		Epoch:       epoch,
		BlockHeight: number,
	}, nil
}
