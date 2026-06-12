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

package node

import "github.com/0xsoniclabs/norma/driver/network"

var OperaRpcService = network.ServiceDescription{
	Name:     "OperaRPC",
	Port:     18545,
	Protocol: "http",
}

var OperaWsService = network.ServiceDescription{
	Name:     "OperaWs",
	Port:     18546,
	Protocol: "ws",
}

var OperaDebugService = network.ServiceDescription{
	Name:     "OperaPprof",
	Port:     6060,
	Protocol: "http",
}

var OperaServices = network.ServiceGroup{}

func init() {
	if err := OperaServices.RegisterService(&OperaRpcService); err != nil {
		panic(err)
	}
	if err := OperaServices.RegisterService(&OperaWsService); err != nil {
		panic(err)
	}
	if err := OperaServices.RegisterService(&OperaDebugService); err != nil {
		panic(err)
	}
}
