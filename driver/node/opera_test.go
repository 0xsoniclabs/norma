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

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/docker"
	"github.com/0xsoniclabs/norma/driver/network"
)

type cleanupHostStub struct{}

func (cleanupHostStub) Hostname() string { return "" }

func (cleanupHostStub) IsRunning() bool { return false }

func (cleanupHostStub) CheckRunning(ctx context.Context) error { return nil }

func (cleanupHostStub) GetAddressForService(*network.ServiceDescription) (*network.AddressPort, error) {
	return nil, nil
}

func (cleanupHostStub) Stop(ctx context.Context) error { return nil }

func (cleanupHostStub) SaveLogTo(ctx context.Context, path string) error { return nil }

func (cleanupHostStub) StreamLog(context.Context) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}

func (cleanupHostStub) Cleanup(ctx context.Context) error { return nil }

func TestImplements(t *testing.T) {
	var inst OperaNode
	var _ driver.Node = &inst

}

func TestStartOperaDockerNode_ReturnsError_WhenNetworkIsNil(t *testing.T) {
	_, err := StartOperaDockerNode(t.Context(), nil, nil, &OperaNodeConfig{
		Label: t.Name(),
	})
	if err == nil {
		t.Fatal("expected error when network is nil")
	}
}

func TestOperaNode_StartAndStop(t *testing.T) {
	docker, err := docker.NewClient()
	if err != nil {
		t.Fatalf("failed to create a docker client: %v", err)
	}
	t.Cleanup(func() {
		_ = docker.Close()
	})
	dn := docker.CreateTestBridgeNetwork(t)
	node, err := StartOperaDockerNode(t.Context(), docker, dn, &OperaNodeConfig{
		Label:         t.Name(),
		Image:         driver.DefaultClientDockerImageName,
		NetworkConfig: &driver.NetworkConfig{Validators: driver.DefaultValidators(t.Name())},
	})
	if err != nil {
		t.Fatalf("failed to create an Opera node on Docker: %v", err)
	}
	t.Cleanup(func() {
		if err := node.Cleanup(context.Background()); err != nil {
			t.Errorf("failed to cleanup node: %v", err)
		}
	})
	if err = node.host.Stop(t.Context()); err != nil {
		t.Errorf("failed to stop Opera node: %v", err)
	}
}

func TestOperaNode_Cleanup_RemovesTempDirs(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "norma-opera-cleanup-*")
	if err != nil {
		t.Fatalf("failed to create temporary directory: %v", err)
	}

	node := &OperaNode{
		host:     cleanupHostStub{},
		config:   &OperaNodeConfig{Label: t.Name()},
		tempDirs: []string{tempDir},
	}

	if err := node.Cleanup(context.Background()); err != nil {
		t.Fatalf("cleanup failed: %v", err)
	}

	if _, err := os.Stat(tempDir); !os.IsNotExist(err) {
		t.Fatalf("temporary directory should be removed, stat error: %v", err)
	}

	if node.tempDirs != nil {
		t.Fatalf("tempDirs should be cleared after cleanup")
	}
}

func TestOperaNode_RpcServiceIsReadyAfterStartup(t *testing.T) {
	docker, err := docker.NewClient()
	if err != nil {
		t.Fatalf("failed to create a docker client: %v", err)
	}
	t.Cleanup(func() {
		_ = docker.Close()
	})
	dn := docker.CreateTestBridgeNetwork(t)
	node, err := StartOperaDockerNode(t.Context(), docker, dn, &OperaNodeConfig{
		Label:         t.Name(),
		Image:         driver.DefaultClientDockerImageName,
		NetworkConfig: &driver.NetworkConfig{Validators: driver.DefaultValidators(t.Name())},
	})
	if err != nil {
		t.Fatalf("failed to create an Opera node on Docker: %v", err)
	}
	t.Cleanup(func() {
		if err := node.Cleanup(context.Background()); err != nil {
			t.Errorf("failed to cleanup node: %v", err)
		}
	})
	if id, err := node.GetNodeID(); err != nil || len(id) == 0 {
		t.Errorf("failed to fetch NodeID from Opera node: '%v', err: %v", id, err)
	}
}

func TestOperaNode_StreamLog(t *testing.T) {
	docker, err := docker.NewClient()
	if err != nil {
		t.Fatalf("failed to create a docker client: %v", err)
	}
	t.Cleanup(func() {
		_ = docker.Close()
	})

	dn := docker.CreateTestBridgeNetwork(t)
	node, err := StartOperaDockerNode(t.Context(), docker, dn, &OperaNodeConfig{
		Label:         t.Name(),
		Image:         driver.DefaultClientDockerImageName,
		NetworkConfig: &driver.NetworkConfig{Validators: driver.DefaultValidators(t.Name())},
	})
	if err != nil {
		t.Fatalf("failed to create an Opera node on Docker: %v", err)
	}
	t.Cleanup(func() {
		if err := node.Cleanup(context.Background()); err != nil {
			t.Errorf("failed to cleanup node: %v", err)
		}
	})

	reader, err := node.StreamLog(t.Context())
	if err != nil {
		t.Fatalf("cannot read logs: %e", err)
	}

	t.Cleanup(func() {
		_ = reader.Close()
	})

	done := make(chan bool)

	go func() {
		defer close(done)
		scanner := bufio.NewScanner(reader)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.Contains(line, "IPC endpoint opened") {
				done <- true
			}
		}
	}()

	var started bool
	select {
	case started = <-done:
	case <-time.After(10 * time.Second):
	}

	if !started {
		t.Errorf("expected log not found")
	}
}

func TestOperaNode_MetricsExposed(t *testing.T) {
	docker, err := docker.NewClient()
	if err != nil {
		t.Fatalf("failed to create a docker client: %v", err)
	}
	t.Cleanup(func() {
		_ = docker.Close()
	})

	dn := docker.CreateTestBridgeNetwork(t)
	node, err := StartOperaDockerNode(t.Context(), docker, dn, &OperaNodeConfig{
		Label:         t.Name(),
		Image:         driver.DefaultClientDockerImageName,
		NetworkConfig: &driver.NetworkConfig{Validators: driver.DefaultValidators(t.Name())},
	})
	if err != nil {
		t.Fatalf("failed to create an Opera node on Docker: %v", err)
	}
	t.Cleanup(func() {
		if err := node.Cleanup(context.Background()); err != nil {
			t.Errorf("failed to cleanup node: %v", err)
		}
	})

	url, err := node.GetServiceUrl(&OperaDebugService)
	if err != nil {
		t.Fatalf("failed to get service URL: %v", err)
	}

	var apiWorks bool
	for i := 0; i < 100; i++ {
		resp, err := http.Get(fmt.Sprintf("%s/debug/metrics/prometheus", string(*url)))
		if err == nil {
			bodyBytes, err := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if err == nil && strings.Contains(string(bodyBytes), "# TYPE") {
				apiWorks = true
				break
			}
		}
		time.Sleep(100 * time.Millisecond)
	}

	if !apiWorks {
		t.Errorf("monitoring API has not been available")
	}
}

func TestClient_Stop_Graceful(t *testing.T) {
	t.Parallel()

	client, err := docker.NewClient()
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer func() {
		if err := client.Close(); err != nil {
			t.Errorf("cannot close: %v", err)
		}
	}()

	dn := client.CreateTestBridgeNetwork(t)
	node, err := StartOperaDockerNode(t.Context(), client, dn, &OperaNodeConfig{
		Label:         t.Name(),
		Image:         driver.DefaultClientDockerImageName,
		NetworkConfig: &driver.NetworkConfig{Validators: driver.DefaultValidators(t.Name())},
	})
	if err != nil {
		t.Fatalf("failed to create client node: %v", err)
	}
	defer func() {
		if err := node.Cleanup(context.Background()); err != nil {
			t.Errorf("cannot cleanup: %v", err)
		}
	}()

	if err := node.Stop(t.Context()); err != nil {
		t.Errorf("cannot stop client node: %v", err)
	}

	// Wait for the sonicd exec to finish so the log file is fully flushed.
	select {
	case <-node.sonicd.Done:
	case <-time.After(30 * time.Second):
		t.Fatalf("sonicd exec did not finish in time")
	}

	// Read the complete exec log and verify graceful shutdown message.
	reader, err := node.StreamExecLog()
	if err != nil {
		t.Fatalf("cannot read exec log: %v", err)
	}
	defer func() {
		if err := reader.Close(); err != nil {
			t.Errorf("cannot close: %v", err)
		}
	}()

	logBytes, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("failed to read exec log: %v", err)
	}

	if !strings.Contains(string(logBytes), "State DB closed") {
		t.Errorf("container did not stop gracefully: "+
			"\"State DB closed\" not found in exec log (%d bytes)",
			len(logBytes))
	}
}
