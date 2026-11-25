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

package docker

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/0xsoniclabs/norma/driver/network"
	"github.com/stretchr/testify/require"
)

func TestImplements(t *testing.T) {
	var inst Container
	var _ network.Host = &inst

}

func TestRunAndStopOneContainer(t *testing.T) {
	cli, cont := startContainer(t)

	if !containerExists(t, cli, cont.id) {
		t.Errorf("container %s is not running", cont.id)
	}

	if err := cont.Stop(t.Context()); err != nil {
		t.Fatalf("error: %v", err)
	}
}

func TestContainer_StopCanBeCalledMoreThanOnce(t *testing.T) {
	_, cont := startContainer(t)

	if !cont.IsRunning() {
		t.Errorf("started container is not running")
	}

	if err := cont.Stop(t.Context()); err != nil {
		t.Fatalf("error stopping container: %v", err)
	}

	if cont.IsRunning() {
		t.Errorf("stopped container is still running")
	}

	if err := cont.Stop(t.Context()); err != nil {
		t.Fatalf("error calling Stop() on stopped container: %v", err)
	}
}

func TestContainer_Cleanup(t *testing.T) {
	cli, cont := startContainer(t)

	if err := cont.Cleanup(context.Background()); err != nil {
		t.Fatalf("error: %v", err)
	}

	if containerExists(t, cli, cont.id) {
		t.Errorf("container should not exist: %s", cont.id)
	}
}

func TestContainer_CleanupCanBeCalledMoreThanOnce(t *testing.T) {
	cli, cont := startContainer(t)

	if !cont.IsRunning() {
		t.Errorf("started container is not running")
	}

	if err := cont.Cleanup(context.Background()); err != nil {
		t.Fatalf("error cleaning up container: %v", err)
	}

	if containerExists(t, cli, cont.id) {
		t.Errorf("container should no longer exist: %s", cont.id)
	}

	if cont.IsRunning() {
		t.Errorf("cleaned up container is still running")
	}

	if err := cont.Cleanup(context.Background()); err != nil {
		t.Fatalf("error calling Cleanup() on cleared container: %v", err)
	}
}

func TestContainer_SaveLogTo(t *testing.T) {
	_, cont := startContainer(t)

	tmp := t.TempDir()
	if err := cont.SaveLogTo(t.Context(), tmp); err != nil {
		t.Fatalf("cannot save logs: %e", err)
	}

	files, err := os.ReadDir(tmp)
	if err != nil {
		t.Fatal(err)
	}

	var numOfFiles int
	for _, file := range files {
		if !file.IsDir() {
			numOfFiles++
		}
	}

	if numOfFiles == 0 {
		t.Errorf("no log files were obtained")
	}
}

func TestContainer_StreamLog(t *testing.T) {
	_, cont := startContainer(t)

	reader, err := cont.StreamLog(t.Context())
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
			if strings.Contains(line, "Hello from Docker!") {
				done <- true
			}
		}
	}()

	var containerStarted bool
	select {
	case containerStarted = <-done:
	case <-time.After(10 * time.Second):
	}

	if !containerStarted {
		t.Errorf("expected log not found")
	}
}

func TestContainer_StreamManyReaders(t *testing.T) {
	_, cont := startContainer(t)

	reader1, err := cont.StreamLog(t.Context())
	if err != nil {
		t.Fatalf("cannot read logs: %e", err)
	}
	reader2, err := cont.StreamLog(t.Context())
	if err != nil {
		t.Fatalf("cannot read logs: %e", err)
	}
	reader3, err := cont.StreamLog(t.Context())
	if err != nil {
		t.Fatalf("cannot read logs: %e", err)
	}

	t.Cleanup(func() {
		_ = reader1.Close()
		_ = reader2.Close()
		_ = reader3.Close()
	})

	done := make(chan int)

	go func() {
		defer close(done)
		var count int
		for _, reader := range []io.Reader{reader1, reader2, reader3} {
			scanner := bufio.NewScanner(reader)
			for scanner.Scan() {
				line := scanner.Text()
				if strings.Contains(line, "Hello from Docker!") {
					count++
				}
			}
		}
		done <- count
	}()

	var count int
	select {
	case count = <-done:
	case <-time.After(10 * time.Second):
	}

	if count != 3 {
		t.Errorf("not all readers got data: %d", count)
	}
}

func TestContainer_Exec(t *testing.T) {
	_, cont := startRunningContainer(t, nil)
	testString := "Hello world!"
	out, err := cont.Exec(t.Context(), []string{"sh", "-c", "echo " + testString})
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if !strings.Contains(out, testString) {
		t.Errorf("expected %s, got %s", testString, out)
	}
}

func TestContainer_SendSignal(t *testing.T) {
	cli, cont := startRunningContainer(t, nil)
	if err := cont.SendSignal(t.Context(), SigKill); err != nil {
		t.Fatalf("error: %v", err)
	}
	// check the container is stopped
	info, err := cli.cli.ContainerInspect(t.Context(), cont.id)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if info.State.Running {
		t.Errorf("expected container to be Killed")
	}
}

func TestNetwork_Cleanup(t *testing.T) {
	cli, net := createNetwork(t)

	if err := net.Cleanup(context.Background()); err != nil {
		t.Fatalf("error: %v", err)
	}

	if networkExists(t, cli, net.id) {
		t.Errorf("network should not exist: %s", net.id)
	}
}

func TestNetwork_CleanupCanBeCalledMoreThanOnce(t *testing.T) {
	cli, net := createNetwork(t)

	if err := net.Cleanup(context.Background()); err != nil {
		t.Fatalf("error cleaning up network: %v", err)
	}

	if networkExists(t, cli, net.id) {
		t.Fatalf("network should no longer exist: %s", net.id)
	}

	if err := net.Cleanup(context.Background()); err != nil {
		t.Fatalf("error calling Cleanup() on cleared network: %v", err)
	}
}

func TestContainerCanJoinNetwork(t *testing.T) {
	cli, net := createNetwork(t)
	_, cont := startRunningContainer(t, net)

	containers, err := cli.listContainers(t.Context())
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	for _, c := range containers {
		if c.ID != cont.id {
			continue
		}
		for _, cn := range c.NetworkSettings.Networks {
			if cn.NetworkID == net.id {
				return
			}
		}
	}
	t.Fatalf("container is not connected to network: %s", net.id)
}

func containerExists(t *testing.T, cli *Client, id string) bool {
	// test the container exists
	var exists bool
	containers, err := cli.listContainers(t.Context())
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	for _, c := range containers {
		if c.ID == id {
			exists = true
			break
		}
	}

	return exists
}

func networkExists(t *testing.T, cli *Client, id string) bool {
	// test the network exists
	var exists bool
	networks, err := cli.listNetworks(t.Context())
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	for _, n := range networks {
		if n.ID == id {
			exists = true
			break
		}
	}

	return exists
}

// A container that fails to start is known to no one but Start, so Start
// removes it; left behind, nothing would ever clean it up.
func TestClient_Start_RemovesTheContainer_WhenStartingItFails(t *testing.T) {
	cli, err := NewClient()
	require.NoError(t, err)
	t.Cleanup(func() { _ = cli.Close() })

	// The container is created, but cannot join a network that does not
	// exist. Depending on the daemon, connecting fails, or every start does
	// until the deadline ends the retries, as a scenario's would; either way
	// after the container exists.
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	name := strings.ReplaceAll(t.Name(), "/", "-")
	timeout := time.Second
	started, err := cli.Start(ctx, &ContainerConfig{
		Hostname:        name,
		ImageName:       "hello-world",
		ShutdownTimeout: &timeout,
		Network:         &Network{id: "no-such-network", name: "no-such-network"},
	})
	if started != nil {
		t.Cleanup(func() { _ = started.Cleanup(context.Background()) })
	}
	require.Error(t, err)

	// Listed with the stopped ones, as a container that never started is.
	containers, err := cli.listContainers(t.Context())
	require.NoError(t, err)
	for _, container := range containers {
		require.NotContains(t, container.Names, "/"+name,
			"the container that failed to start was left behind")
	}
}

func startContainer(t *testing.T) (*Client, *Container) {
	cli, err := NewClient()
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	timeout := time.Second
	cont, err := cli.Start(t.Context(), &ContainerConfig{
		Hostname:        t.Name(),
		ImageName:       "hello-world",
		ShutdownTimeout: &timeout,
	})
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	t.Cleanup(func() {
		_ = cont.Cleanup(context.Background())
		_ = cli.Close()
	})

	return cli, cont
}

func startRunningContainer(t *testing.T, dn *Network) (*Client, *Container) {
	cli, err := NewClient()
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	timeout := time.Second
	cont, err := cli.Start(t.Context(), &ContainerConfig{
		Hostname:        t.Name(),
		ImageName:       "alpine",                            // use minimal linux image
		Entrypoint:      []string{"tail", "-f", "/dev/null"}, // keep container running
		ShutdownTimeout: &timeout,
		Network:         dn,
	})
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	t.Cleanup(func() {
		_ = cont.Cleanup(context.Background())
		_ = cli.Close()
	})

	return cli, cont
}

func createNetwork(t *testing.T) (*Client, *Network) {
	cli, err := NewClient()
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	net, err := cli.CreateBridgeNetwork(t.Context())
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	t.Cleanup(func() {
		_ = net.Cleanup(context.Background())
		_ = cli.Close()
	})

	return cli, net
}

// Files reach the node's container and come back through the Docker API, so
// the daemon need not share the driver's filesystem.
func TestContainer_CopyToAndFrom_MoveFilesThroughTheApi_WhenNestedAndLarge(t *testing.T) {
	cli, err := NewClient()
	require.NoError(t, err)
	t.Cleanup(func() { _ = cli.Close() })
	timeout := time.Second
	cont, err := cli.Start(t.Context(), &ContainerConfig{
		Hostname:        t.Name(),
		ImageName:       "alpine",
		Entrypoint:      []string{"tail", "-f", "/dev/null"},
		ShutdownTimeout: &timeout,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = cont.Cleanup(context.Background()) })

	hostDir := t.TempDir()
	tree := filepath.Join(hostDir, "keystore")
	require.NoError(t, os.MkdirAll(filepath.Join(tree, "validator"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(tree, "validator", "key"), []byte("secret"), 0o600))
	large := filepath.Join(hostDir, "large.g")
	require.NoError(t, os.WriteFile(large, bytes.Repeat([]byte("g"), 8<<20), 0o644))

	require.NoError(t, cont.CopyTo(t.Context(), tree, "/datadir/keystore"))
	require.NoError(t, cont.CopyTo(t.Context(), large, "/shared/in.g"))
	output, err := cont.Exec(t.Context(), []string{"sh", "-c",
		"cat /datadir/keystore/validator/key && wc -c < /shared/in.g && cp /shared/in.g /shared/out.g"})
	require.NoError(t, err, output)
	require.Equal(t, "secret8388608", strings.ReplaceAll(strings.TrimSpace(output), "\n", ""))

	back := filepath.Join(hostDir, "back.g")
	require.NoError(t, cont.CopyFrom(t.Context(), "/shared/out.g", back))
	got, err := os.ReadFile(back)
	require.NoError(t, err)
	want, err := os.ReadFile(large)
	require.NoError(t, err)
	require.Equal(t, want, got)

	require.Error(t, cont.CopyFrom(t.Context(), "/shared/missing.g", back))
	require.ErrorContains(t, cont.CopyFrom(t.Context(), "/datadir", back), "not a file")
	got, err = os.ReadFile(back)
	require.NoError(t, err)
	require.Equal(t, want, got)
}

func TestContainer_CopyTo_KeepsSymlinks_WhenTheRootOrAnEntryIsOne(t *testing.T) {
	cli, err := NewClient()
	require.NoError(t, err)
	t.Cleanup(func() { _ = cli.Close() })
	timeout := time.Second
	cont, err := cli.Start(t.Context(), &ContainerConfig{
		Hostname:        t.Name(),
		ImageName:       "alpine",
		Entrypoint:      []string{"tail", "-f", "/dev/null"},
		ShutdownTimeout: &timeout,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = cont.Cleanup(context.Background()) })

	hostDir := t.TempDir()
	genesis := filepath.Join(hostDir, "genesis.json")
	require.NoError(t, os.WriteFile(genesis, []byte("{}"), 0o644))
	linkedGenesis := filepath.Join(hostDir, "linked.json")
	require.NoError(t, os.Symlink(genesis, linkedGenesis))
	tree := filepath.Join(hostDir, "tree")
	require.NoError(t, os.MkdirAll(tree, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(tree, "file"), []byte("content"), 0o644))
	require.NoError(t, os.Symlink("file", filepath.Join(tree, "link")))

	require.NoError(t, cont.CopyTo(t.Context(), linkedGenesis, "/genesis.json"))
	require.NoError(t, cont.CopyTo(t.Context(), tree, "/tree"))
	output, err := cont.Exec(t.Context(), []string{"sh", "-c",
		"cat /genesis.json && readlink /tree/link && cat /tree/link"})
	require.NoError(t, err, output)
	require.Equal(t, "{}filecontent", strings.ReplaceAll(strings.TrimSpace(output), "\n", ""))
}
