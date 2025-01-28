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

package controller

import (
	"log"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/load/app"
)

func runGeneratorLoop(user app.User, trigger <-chan struct{}, network driver.Network) {
	for range trigger {
		// retrieve gas price for each tx individually as the gasPrice can change by amount of time passed(larger db size),
		// or by new validator registration, etc.
		rpcClient, err := network.DialRandomRpc()
		if err != nil {
			log.Printf("generator loop; failed to dial random rpc; %v", err)
			continue
		}
		currentRegularGasPrice, err := app.GetGasPrice(rpcClient)
		if err != nil {
			log.Printf("generator loop; failed to get gas price; %v", err)
			continue
		}

		tx, err := user.GenerateTx(currentRegularGasPrice)
		if err != nil {
			log.Printf("failed to generate tx; %v", err)
		} else {
			network.SendTransaction(tx)
		}
	}
}
