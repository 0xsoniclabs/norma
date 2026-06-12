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
	"testing"
	"time"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/network/local"
)

func TestCanCollectCpuProfileDateFromOperaNode(t *testing.T) {
	net, err := local.NewLocalNetwork(
		t.Context(),
		&driver.NetworkConfig{Validators: driver.DefaultValidators})
	if err != nil {
		t.Fatalf("failed to create local network: %v", err)
	}
	t.Cleanup(func() {
		if err := net.Shutdown(); err != nil {
			t.Fatalf("failed to shut down network: %v", err)
		}
	})
	node, err := net.CreateNode(&driver.NodeConfig{
		Name:  "test",
		Image: driver.DefaultClientDockerImageName,
	})
	if err != nil {
		t.Fatalf("failed to create node: %v", err)
	}
	data, err := GetPprofData(node, time.Second)
	if err != nil {
		t.Errorf("failed to collect pprof data from node: %v", err)
	}
	if len(data) == 0 {
		t.Errorf("fetched empty CPU profile")
	}
}
