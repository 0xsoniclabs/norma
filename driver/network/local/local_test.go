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

package local

import (
	"bufio"
	"fmt"
	"github.com/0xsoniclabs/norma/driver/parser"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/node"
	"go.uber.org/mock/gomock"
)

func TestLocalNetworkIsNetwork(t *testing.T) {
	var net LocalNetwork
	var _ driver.Network = &net
}

func TestLocalNetwork_CanStartNodesAndShutThemDown(t *testing.T) {
	t.Parallel()
	config := driver.NetworkConfig{Validators: driver.DefaultValidators}
	for _, N := range []int{1, 3} {
		N := N
		t.Run(fmt.Sprintf("num_nodes=%d", N), func(t *testing.T) {
			t.Parallel()
			net, err := NewLocalNetwork(&config)
			if err != nil {
				t.Fatalf("failed to create new local network: %v", err)
			}
			t.Cleanup(func() {
				_ = net.Shutdown()
			})

			nodes := []driver.Node{}
			for i := 0; i < N; i++ {
				node, err := net.CreateNode(&driver.NodeConfig{
					Image: driver.DefaultClientDockerImageName,
					Name:  fmt.Sprintf("T-%d", i),
				})
				if err != nil {
					t.Errorf("failed to create node: %v", err)
					continue
				}
				nodes = append(nodes, node)
			}

			for _, node := range nodes {
				if err := node.Stop(); err != nil {
					t.Errorf("failed to stop node: %v", err)
				}
			}

			for _, node := range nodes {
				if err := node.Cleanup(); err != nil {
					t.Errorf("failed to cleanup node: %v", err)
				}
			}
		})
	}
}

func TestLocalNetwork_CanEnforceNetworkLatency(t *testing.T) {
	t.Parallel()
	for _, rtt := range []time.Duration{0, 100 * time.Millisecond, 200 * time.Millisecond} {
		rtt := rtt
		t.Run(fmt.Sprintf("rtt=%v", rtt), func(t *testing.T) {
			t.Parallel()
			config := driver.NetworkConfig{
				Validators:    driver.NewDefaultValidators(2),
				RoundTripTime: rtt,
			}
			net, err := NewLocalNetwork(&config)
			if err != nil {
				t.Fatalf("failed to create new local network: %v", err)
			}
			t.Cleanup(func() {
				_ = net.Shutdown()
			})

			// measure latency between nodes
			nodes := net.GetActiveNodes()
			if got, want := len(nodes), 2; got != want {
				t.Fatalf("invalid number of active nodes, got %d, want %d", got, want)
			}
			got, err := nodes[0].(*node.OperaNode).GetRoundTripTime(nodes[1].Hostname())
			if err != nil {
				t.Errorf("failed to measure network delay: %v", err)
			}
			if got < rtt-10*time.Millisecond {
				t.Errorf("network RTT is too low: %v < %v", got, rtt)
			}
			if got > rtt+10*time.Millisecond {
				t.Errorf("network RTT is too high: %v > %v", got, rtt)
			}
		})
	}
}

func TestLocalNetwork_CanStartApplicationsAndShutThemDown(t *testing.T) {
	t.Parallel()
	config := driver.NetworkConfig{Validators: driver.DefaultValidators}
	for _, N := range []int{1, 3} {
		N := N
		t.Run(fmt.Sprintf("num_nodes=%d", N), func(t *testing.T) {
			t.Parallel()

			net, err := NewLocalNetwork(&config)
			if err != nil {
				t.Fatalf("failed to create new local network: %v", err)
			}
			t.Cleanup(func() {
				_ = net.Shutdown()
			})

			apps := []driver.Application{}
			for i := 0; i < N; i++ {
				app, err := net.CreateApplication(&driver.ApplicationConfig{
					Name: fmt.Sprintf("T-%d", i),
				})
				if err != nil {
					t.Errorf("failed to create app: %v", err)
					continue
				}

				if got, want := app.Config().Name, fmt.Sprintf("T-%d", i); got != want {
					t.Errorf("app configurion not propagated: %v != %v", got, want)
				}

				apps = append(apps, app)
			}

			for _, app := range apps {
				if err := app.Start(); err != nil {
					t.Errorf("failed to start app: %v", err)
				}
			}

			for _, app := range apps {
				if err := app.Stop(); err != nil {
					t.Errorf("failed to stop app: %v", err)
				}
			}
		})
	}
}

func TestLocalNetwork_CanPerformNetworkShutdown(t *testing.T) {
	t.Parallel()
	N := 2
	config := driver.NetworkConfig{Validators: driver.DefaultValidators}

	net, err := NewLocalNetwork(&config)
	if err != nil {
		t.Fatalf("failed to create new local network: %v", err)
	}
	t.Cleanup(func() {
		_ = net.Shutdown()
	})

	for i := 0; i < N; i++ {
		_, err := net.CreateNode(&driver.NodeConfig{
			Name:  fmt.Sprintf("T-%d", i),
			Image: driver.DefaultClientDockerImageName,
		})
		if err != nil {
			t.Errorf("failed to create node: %v", err)
		}
	}

	for i := 0; i < N; i++ {
		_, err := net.CreateApplication(&driver.ApplicationConfig{
			Name: fmt.Sprintf("T-%d", i),
		})
		if err != nil {
			t.Errorf("failed to create app: %v", err)
		}
	}

	if err := net.Shutdown(); err != nil {
		t.Errorf("failed to shut down network: %v", err)
	}
}

func TestLocalNetwork_Shutdown_Graceful(t *testing.T) {
	t.Parallel()
	N := 3
	config := driver.NetworkConfig{Validators: driver.DefaultValidators}

	net, err := NewLocalNetwork(&config)
	if err != nil {
		t.Fatalf("failed to create new local network: %v", err)
	}

	done := make(chan bool, N)

	ctrl := gomock.NewController(t)
	listener := driver.NewMockNetworkListener(ctrl)
	listener.EXPECT().AfterNodeCreation(gomock.Any()).DoAndReturn(func(node driver.Node) {
		reader, err := node.StreamLog()
		if err != nil {
			t.Errorf("error: %v", err)
		}
		t.Cleanup(func() {
			if err := reader.Close(); err != nil {
				t.Errorf("cannot close: %v", err)
			}
		})

		go func() {
			scanner := bufio.NewScanner(reader)
			for scanner.Scan() {
				line := scanner.Text()
				if strings.Contains(line, "State DB closed") {
					done <- true
				}
			}
		}()
	}).Times(N)
	net.RegisterListener(listener)

	for i := 0; i < N; i++ {
		_, err := net.CreateNode(&driver.NodeConfig{
			Name:  fmt.Sprintf("T-%d", i),
			Image: driver.DefaultClientDockerImageName,
		})
		if err != nil {
			t.Errorf("failed to create node: %v", err)
		}
	}

	if err := net.Shutdown(); err != nil {
		t.Errorf("failed to shut down network: %v", err)
	}

	// N containers must stop gracefully
	for i := 0; i < N; i++ {
		select {
		case <-done:
			// one container done successfully
		case <-time.After(180 * time.Second):
			t.Errorf("container did not stop gracefully")
		}
	}
}

func TestLocalNetwork_CanRunWithMultipleValidators(t *testing.T) {
	t.Parallel()
	for _, N := range []int{1, 3} {
		N := N
		config := driver.NetworkConfig{Validators: driver.NewDefaultValidators(N)}
		t.Run(fmt.Sprintf("num_validators=%d", N), func(t *testing.T) {
			t.Parallel()
			net, err := NewLocalNetwork(&config)
			if err != nil {
				t.Fatalf("failed to create new local network: %v", err)
			}
			t.Cleanup(func() {
				_ = net.Shutdown()
			})

			app, err := net.CreateApplication(&driver.ApplicationConfig{
				Name: "TestApp",
			})
			if err != nil {
				t.Fatalf("failed to create app: %v", err)
			}

			if err := app.Start(); err != nil {
				t.Errorf("failed to start app: %v", err)
			}

			if err := app.Stop(); err != nil {
				t.Errorf("failed to stop app: %v", err)
			}
		})
	}
}

func TestLocalNetwork_CanRunWithVariousValidators(t *testing.T) {
	t.Parallel()

	var one = 1
	var two = 2
	var three = 3

	validators := driver.NewValidators([]parser.Validator{
		{},
		{Name: "validator1", Instances: &three, ImageName: "sonic:v2.0.0"},
		{Name: "validator2", Instances: &two, ImageName: "sonic:v2.0.1"},
		{Name: "validator3", Instances: &one, ImageName: "sonic:v2.0.2"},
		{Name: "validator4", ImageName: "sonic"},
	})

	config := driver.NetworkConfig{Validators: validators}
	net, err := NewLocalNetwork(&config)
	if err != nil {
		t.Fatalf("failed to create new local network: %v", err)
	}
	t.Cleanup(func() {
		if err := net.Shutdown(); err != nil {
			t.Fatalf("failed to shut down network: %v", err)
		}
	})

	if got := net.GetActiveNodes(); len(got) != 8 {
		t.Errorf("invalid number of active nodes, got %d, want 6", len(got))
	}

}

func TestLocalNetwork_NotifiesListenersOnNodeStartup(t *testing.T) {
	t.Parallel()
	config := driver.NetworkConfig{Validators: driver.NewDefaultValidators(2)}
	ctrl := gomock.NewController(t)
	listener := driver.NewMockNetworkListener(ctrl)

	net, err := NewLocalNetwork(&config)
	if err != nil {
		t.Fatalf("failed to create new local network: %v", err)
	}
	t.Cleanup(func() {
		_ = net.Shutdown()
	})

	activeNodes := net.GetActiveNodes()
	if got, want := len(activeNodes), config.Validators.GetNumValidators(); got != want {
		t.Errorf("invalid number of active nodes, got %d, want %d", got, want)
	}

	net.RegisterListener(listener)
	listener.EXPECT().AfterNodeCreation(gomock.Any())

	net.CreateNode(&driver.NodeConfig{
		Name:  "Test",
		Image: driver.DefaultClientDockerImageName,
	})

	activeNodes = net.GetActiveNodes()
	if got, want := len(activeNodes), config.Validators.GetNumValidators()+1; got != want {
		t.Errorf("invalid number of active nodes, got %d, want %d", got, want)
	}

}

func TestLocalNetwork_NotifiesListenersOnAppStartup(t *testing.T) {
	t.Parallel()
	config := driver.NetworkConfig{Validators: driver.DefaultValidators}
	ctrl := gomock.NewController(t)
	listener := driver.NewMockNetworkListener(ctrl)

	net, err := NewLocalNetwork(&config)
	if err != nil {
		t.Fatalf("failed to create new local network: %v", err)
	}
	t.Cleanup(func() {
		_ = net.Shutdown()
	})

	net.RegisterListener(listener)
	listener.EXPECT().AfterApplicationCreation(gomock.Any())

	_, err = net.CreateApplication(&driver.ApplicationConfig{
		Name: "TestApp",
	})
	if err != nil {
		t.Errorf("creation of app failed: %v", err)
	}
}

func TestLocalNetwork_CanRemoveNode(t *testing.T) {
	t.Parallel()
	config := driver.NetworkConfig{Validators: driver.DefaultValidators}
	for _, N := range []int{1, 3} {
		N := N
		t.Run(fmt.Sprintf("num_nodes=%d", N), func(t *testing.T) {
			t.Parallel()
			net, err := NewLocalNetwork(&config)
			ctrl := gomock.NewController(t)
			listener := driver.NewMockNetworkListener(ctrl)
			listener.EXPECT().AfterNodeCreation(gomock.Any()).Times(N)
			listener.EXPECT().AfterNodeRemoval(gomock.Any()).Times(N)
			net.RegisterListener(listener)

			if err != nil {
				t.Fatalf("failed to create new local network: %v", err)
			}
			t.Cleanup(func() {
				_ = net.Shutdown()
			})

			nodes := make([]driver.Node, 0, N)
			for i := 0; i < N; i++ {
				node, err := net.CreateNode(&driver.NodeConfig{
					Name:  fmt.Sprintf("T-%d", i),
					Image: driver.DefaultClientDockerImageName,
				})
				if err != nil {
					t.Errorf("failed to create node: %s", err)
				}
				nodes = append(nodes, node)
			}

			// remove nodes one by one
			for _, node := range nodes {
				if err := net.RemoveNode(node); err != nil {
					t.Errorf("cannot remove node: %s", err)
				}

				id, err := node.GetNodeID()
				if err != nil {
					t.Errorf("cannot get node ID: %s", err)
				}

				_, exists := net.nodes[id]
				if exists {
					t.Errorf("node %s was not removed", id)
				}
			}

			// removed nodes are only detached from the network, but still running - i.e. they can be turned off
			for _, node := range nodes {
				if err := node.Stop(); err != nil {
					t.Errorf("failed to stop node: %v", err)
				}
				if err := node.Cleanup(); err != nil {
					t.Errorf("failed to cleanup node: %v", err)
				}
			}
		})
	}
}

func TestLocalNetwork_Num_Validators_Started(t *testing.T) {
	t.Parallel()
	for i := 1; i < 3; i++ {
		i := i
		t.Run(fmt.Sprintf("num_validators=%d", i), func(t *testing.T) {
			t.Parallel()
			config := driver.NetworkConfig{Validators: driver.NewDefaultValidators(i)}
			net, err := NewLocalNetwork(&config)
			if err != nil {
				t.Fatalf("failed to create new local network: %v", err)
			}
			t.Cleanup(func() {
				if err := net.Shutdown(); err != nil {
					t.Fatalf("failed to shut down network: %v", err)
				}
			})

			if got, want := len(net.GetActiveNodes()), config.Validators.GetNumValidators(); got != want {
				t.Errorf("invalid number of active nodes, got %d, want %d", got, want)
			}
		})
	}
}

func TestLocalNetwork_Can_Run_Multiple_Client_Images(t *testing.T) {
	t.Parallel()
	config := driver.NetworkConfig{Validators: driver.DefaultValidators}

	net, err := NewLocalNetwork(&config)
	if err != nil {
		t.Fatalf("failed to create new local network: %v", err)
	}
	t.Cleanup(func() {
		_ = net.Shutdown()
	})

	images := []string{"sonic", "sonic:local", "sonic:v2.0.2", "sonic:v2.0.1", "sonic:v2.0.0"}
	checksum := make(chan string)

	for i, image := range images {
		node, err := net.CreateNode(&driver.NodeConfig{
			Name:  fmt.Sprintf("T-%d", i),
			Image: image,
		})
		if err != nil {
			t.Errorf("failed to create node: %v", err)
		}

		// read logs and check the image name is correct
		reader, err := node.StreamLog()
		if err != nil {
			t.Fatalf("cannot read logs: %e", err)
		}
		t.Cleanup(func() {
			_ = reader.Close()
		})

		go func() {
			scanner := bufio.NewScanner(reader)
			for scanner.Scan() {
				line := scanner.Text()
				if strings.Contains(line, "Sonic binary checksum:") {
					parts := strings.SplitN(line, ":", 2)
					if len(parts) == 2 {
						checksum <- strings.TrimSpace(parts[1])
					}
				}
			}
		}()
	}

	gotChecksums := make(map[string]struct{})
	for len(gotChecksums) < len(images) {
		select {
		case val := <-checksum:
			gotChecksums[val] = struct{}{}
		case <-time.After(180 * time.Second):
			t.Fatalf("timeout while waiting for checksums")
		}
	}

	if got, want := len(gotChecksums), len(images); got != want {
		t.Errorf("invalid number of checksum, got: %d, want %d", got, want)
	}

	if err := net.Shutdown(); err != nil {
		t.Errorf("failed to shut down network: %v", err)
	}
}

func TestLocalNetworkApplyNetworkRules_Success(t *testing.T) {
	t.Parallel()
	config := driver.NetworkConfig{Validators: driver.DefaultValidators}
	net, err := NewLocalNetwork(&config)
	if err != nil {
		t.Fatalf("failed to create new local network: %v", err)
	}
	t.Cleanup(func() {
		if err := net.Shutdown(); err != nil {
			t.Fatalf("failed to shut down network, %v", err)
		}
	})

	// fetch the base fee via RPC
	client, err := net.DialRandomRpc()
	if err != nil {
		t.Fatalf("failed to dial random RPC: %v", err)
	}
	defer client.Close()

	type rulesType struct {
		Economy struct {
			MinBaseFee *big.Int
		}
	}

	var originalRules rulesType
	if err := client.Call(&originalRules, "eth_getRules", "latest"); err != nil {
		t.Fatalf("failed to call eth_getRules: %v", err)
	}

	rules := driver.NetworkRules{}
	wantFee := originalRules.Economy.MinBaseFee.Int64() + 123
	rules["MIN_BASE_FEE"] = fmt.Sprintf("%d", wantFee)

	if err := net.ApplyNetworkRules(rules); err != nil {
		t.Errorf("failed to apply network rules: %v", err)
	}

	var result rulesType
	if err := client.Call(&result, "eth_getRules", "latest"); err != nil {
		t.Fatalf("failed to call eth_getRules: %v", err)
	}

	if got, want := result.Economy.MinBaseFee.Int64(), wantFee; got != want {
		t.Errorf("invalid base fee, got %d, want %d", got, want)
	}
}

func TestLocalNetwork_FailingFlagPropagated(t *testing.T) {
	t.Parallel()
	config := driver.NetworkConfig{Validators: []driver.Validator{
		{Name: "validator", Failing: true, Instances: 1, ImageName: driver.DefaultClientDockerImageName}},
	}
	net, err := NewLocalNetwork(&config)
	if err != nil {
		t.Fatalf("failed to create new local network: %v", err)
	}
	t.Cleanup(func() {
		if err := net.Shutdown(); err != nil {
			t.Fatalf("failed to shut down network: %v", err)
		}
	})

	if _, err := net.CreateNode(&driver.NodeConfig{
		Name:    "node",
		Failing: true,
		Image:   driver.DefaultClientDockerImageName,
	}); err != nil {
		t.Fatalf("failed to create node: %v", err)
	}

	for _, node := range net.GetActiveNodes() {
		if !node.IsExpectedFailure() {
			t.Errorf("node is not failing: %s", node.GetLabel())
		}
	}

}

func TestLocalNetwork_MountDataDir_Can_Be_Reused(t *testing.T) {
	t.Parallel()

	// jenkins uses different access privileges for docker
	// i.e. we need to create a temporary directory in /tmp for docker mount
	// as the test cleanup cannot delete the directory if the mount is in the subdirectory of this test.
	temp, err := os.MkdirTemp("/tmp", fmt.Sprintf("%s-docker-volume-*", t.Name()))
	if err != nil {
		t.Fatalf("failed to create temporary directory: %v", err)
	}
	defer func() {
		os.RemoveAll(temp)
	}()

	config := driver.NetworkConfig{Validators: driver.DefaultValidators, OutputDir: temp}
	net, err := NewLocalNetwork(&config)
	if err != nil {
		t.Fatalf("failed to create new local network: %v", err)
	}
	t.Cleanup(func() {
		if err := net.Shutdown(); err != nil {
			t.Fatalf("failed to shut down network: %v", err)
		}
	})

	dataVolume := "abcd"
	node, err := net.CreateNode(&driver.NodeConfig{
		Name:       "node",
		DataVolume: &dataVolume,
		Image:      driver.DefaultClientDockerImageName,
	})
	if err != nil {
		t.Fatalf("failed to create node: %v", err)
	}

	getModificationTime := func() (*time.Time, []string, error) {
		var carmenModTime *time.Time
		var visitedDirs []string
		localDirBinding := fmt.Sprintf("%s/%s", temp, dataVolume)
		err := filepath.Walk(localDirBinding, func(path string, info os.FileInfo, err error) error {
			visitedDirs = append(visitedDirs, path)
			if strings.HasSuffix(path, "transactions.rlp") {
				carmenModTime = new(time.Time)
				*carmenModTime = info.ModTime()
			}
			return nil
		})
		return carmenModTime, visitedDirs, err
	}

	// save modification time of the database lock
	prevModTime, visitedDirs, err := getModificationTime()
	if err != nil {
		t.Fatalf("failed to get modification time: %v", err)
	}
	if prevModTime == nil {
		t.Fatalf("directory does not contain database files: %v", visitedDirs)
	}

	// stop the node
	if err := net.RemoveNode(node); err != nil {
		t.Fatalf("failed to remove node: %v", err)
	}
	if err := node.Stop(); err != nil {
		t.Fatalf("failed to stop node: %v", err)
	}
	if err := node.Cleanup(); err != nil {
		t.Fatalf("failed to cleanup node: %v", err)
	}

	// re-run another node on the same data volume
	if _, err := net.CreateNode(&driver.NodeConfig{
		Name:       "node2",
		DataVolume: &dataVolume,
		Image:      driver.DefaultClientDockerImageName,
	}); err != nil {
		t.Fatalf("failed to create node: %v", err)
	}

	// the database lock should have been updated
	currModTime, visitedDirs, err := getModificationTime()
	if err != nil {
		t.Fatalf("failed to get modification time: %v", err)
	}
	if got, want := *currModTime, *prevModTime; got.Equal(want) {
		t.Errorf("got modification time %v, wanted modification time %v", got, want)
	}

}
