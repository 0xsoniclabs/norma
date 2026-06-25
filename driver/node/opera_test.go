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

func (cleanupHostStub) CheckRunning() error { return nil }

func (cleanupHostStub) GetAddressForService(*network.ServiceDescription) *network.AddressPort {
	return nil
}

func (cleanupHostStub) Stop() error { return nil }

func (cleanupHostStub) SaveLogTo(string) error { return nil }

func (cleanupHostStub) StreamLog() (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}

func (cleanupHostStub) Cleanup() error { return nil }

func TestImplements(t *testing.T) {
	var inst OperaNode
	var _ driver.Node = &inst

}

func TestOperaNode_StartAndStop(t *testing.T) {
	docker, err := docker.NewClient()
	if err != nil {
		t.Fatalf("failed to create a docker client: %v", err)
	}
	t.Cleanup(func() {
		_ = docker.Close()
	})
	node, err := StartOperaDockerNode(t.Context(), docker, nil, &OperaNodeConfig{
		Label:         t.Name(),
		Image:         driver.DefaultClientDockerImageName,
		NetworkConfig: &driver.NetworkConfig{Validators: driver.DefaultValidators(t.Name())},
	})
	if err != nil {
		t.Fatalf("failed to create an Opera node on Docker: %v", err)
	}
	t.Cleanup(func() {
		if err := node.Cleanup(); err != nil {
			t.Errorf("failed to cleanup node: %v", err)
		}
	})
	if err = node.host.Stop(); err != nil {
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

	if err := node.Cleanup(); err != nil {
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
	node, err := StartOperaDockerNode(t.Context(), docker, nil, &OperaNodeConfig{
		Label:         t.Name(),
		Image:         driver.DefaultClientDockerImageName,
		NetworkConfig: &driver.NetworkConfig{Validators: driver.DefaultValidators(t.Name())},
	})
	if err != nil {
		t.Fatalf("failed to create an Opera node on Docker: %v", err)
	}
	t.Cleanup(func() {
		if err := node.Cleanup(); err != nil {
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

	node, err := StartOperaDockerNode(t.Context(), docker, nil, &OperaNodeConfig{
		Label:         t.Name(),
		Image:         driver.DefaultClientDockerImageName,
		NetworkConfig: &driver.NetworkConfig{Validators: driver.DefaultValidators(t.Name())},
	})
	if err != nil {
		t.Fatalf("failed to create an Opera node on Docker: %v", err)
	}
	t.Cleanup(func() {
		if err := node.Cleanup(); err != nil {
			t.Errorf("failed to cleanup node: %v", err)
		}
	})

	reader, err := node.StreamLog()
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

	node, err := StartOperaDockerNode(t.Context(), docker, nil, &OperaNodeConfig{
		Label:         t.Name(),
		Image:         driver.DefaultClientDockerImageName,
		NetworkConfig: &driver.NetworkConfig{Validators: driver.DefaultValidators(t.Name())},
	})
	if err != nil {
		t.Fatalf("failed to create an Opera node on Docker: %v", err)
	}
	t.Cleanup(func() {
		if err := node.Cleanup(); err != nil {
			t.Errorf("failed to cleanup node: %v", err)
		}
	})

	url := node.GetServiceUrl(&OperaDebugService)

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

	node, err := StartOperaDockerNode(t.Context(), client, nil, &OperaNodeConfig{
		Label:         t.Name(),
		Image:         driver.DefaultClientDockerImageName,
		NetworkConfig: &driver.NetworkConfig{Validators: driver.DefaultValidators(t.Name())},
	})
	if err != nil {
		t.Fatalf("failed to create client node: %v", err)
	}
	defer func() {
		if err := node.Cleanup(); err != nil {
			t.Errorf("cannot cleanup: %v", err)
		}
	}()

	reader, err := node.StreamLog()
	if err != nil {
		t.Errorf("error: %v", err)
	}
	defer func() {
		if err := reader.Close(); err != nil {
			t.Errorf("cannot close: %v", err)
		}
	}()

	done := make(chan bool, 1)
	go func() {
		scanner := bufio.NewScanner(reader)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.Contains(line, "State DB closed") {
				done <- true
			}
		}
	}()

	if err := node.Stop(); err != nil {
		t.Errorf("cannot stop client node: %v", err)
	}

	select {
	case <-done:
		// container stopped gracefully
	case <-time.After(180 * time.Second):
		t.Errorf("container did not stop gracefully")
	}
}

func TestCheckBlockProducing_SucceedsWithTwoIncreasingBlocks(t *testing.T) {
	t.Parallel()
	logContent := "INFO [05-04|09:34:15.537] Starting node\n" +
		"INFO [05-04|09:34:16.000] New block index=1 gas_used=0 txs=0 t=1ms\n" +
		"INFO [05-04|09:34:17.000] New block index=2 gas_used=0 txs=0 t=1ms\n"

	node := &OperaNode{
		host:   &fakeLogHost{logContent: logContent},
		config: &OperaNodeConfig{Label: "test-node"},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := node.CheckBlockProducing(ctx); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestCheckBlockProducing_FailsWithOnlyOneBlock(t *testing.T) {
	t.Parallel()
	logContent := "INFO [05-04|09:34:15.537] Starting node\n" +
		"INFO [05-04|09:34:16.000] New block index=1 gas_used=0 txs=0 t=1ms\n"

	node := &OperaNode{
		host:   &fakeLogHost{logContent: logContent},
		config: &OperaNodeConfig{Label: "test-node"},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := node.CheckBlockProducing(ctx)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "without observing 2 increasing blocks") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestCheckBlockProducing_FailsWhenNoBlockBeforeTimeout(t *testing.T) {
	t.Parallel()

	node := &OperaNode{
		host:   &blockingLogHost{},
		config: &OperaNodeConfig{Label: "test-node"},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := node.CheckBlockProducing(ctx)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "did not produce 2 increasing blocks before timeout") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestCheckBlockProducing_FailsWhenLogStreamEndsWithoutBlock(t *testing.T) {
	t.Parallel()
	logContent := "INFO Starting node\nINFO IPC endpoint opened\n"

	node := &OperaNode{
		host:   &fakeLogHost{logContent: logContent},
		config: &OperaNodeConfig{Label: "test-node"},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := node.CheckBlockProducing(ctx)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "without observing 2 increasing blocks") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

// fakeLogHost is a test Host implementation that returns pre-configured log content.
type fakeLogHost struct {
	cleanupHostStub
	logContent string
}

func (h *fakeLogHost) StreamLog() (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader(h.logContent)), nil
}

// blockingLogHost is a test Host implementation whose StreamLog blocks until
// the reader is closed (simulating a running container with no block output).
type blockingLogHost struct {
	cleanupHostStub
}

func (h *blockingLogHost) StreamLog() (io.ReadCloser, error) {
	r, w := io.Pipe()
	// Write some non-block log lines, then keep the pipe open.
	go func() {
		_, _ = w.Write([]byte("INFO Starting node\nINFO IPC endpoint opened\n"))
		// Never close w — the reader blocks until r.Close() is called.
	}()
	return r, nil
}
