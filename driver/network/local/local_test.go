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
	"context"
	"fmt"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/0xsoniclabs/norma/driver/network"
	"github.com/0xsoniclabs/norma/genesis"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/node"
	"github.com/0xsoniclabs/norma/driver/rpc"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestLocalNetworkIsNetwork(t *testing.T) {
	var net LocalNetwork
	var _ driver.Network = &net
}

func TestNewLocalNetwork_CreatesEmptyNetwork(t *testing.T) {
	t.Parallel()
	config := driver.NetworkConfig{
		Validators: driver.NewDefaultTestValidators(t.Name(), 3),
	}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()

	net, err := NewLocalNetwork(ctx, &config)
	if err != nil {
		t.Fatalf("failed to create local network: %v", err)
	}
	t.Cleanup(func() { _ = net.Shutdown() })

	nodes := net.GetActiveNodes()
	if len(nodes) != 0 {
		t.Fatalf("expected no active nodes, got %d", len(nodes))
	}
}

func TestLocalNetwork_CanStartNodesAndShutThemDown(t *testing.T) {
	t.Parallel()
	config := driver.NetworkConfig{Validators: driver.DefaultValidators(t.Name())}
	for _, N := range []int{1, 3} {
		N := N
		t.Run(fmt.Sprintf("num-nodes-%d", N), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
			defer cancel()
			net, err := NewLocalLegacyNetwork(ctx, &config)
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
					Name:  fmt.Sprintf("N-%d-%s", i, t.Name()),
				})
				if err != nil {
					t.Fatalf("failed to create node: %v", err)
				}
				nodes = append(nodes, node)
			}

			for _, node := range nodes {
				if err := node.Stop(t.Context()); err != nil {
					t.Errorf("failed to stop node: %v", err)
				}
			}

			for _, node := range nodes {
				if err := node.Cleanup(context.Background()); err != nil {
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
		t.Run(fmt.Sprintf("rtt-%v", rtt), func(t *testing.T) {
			config := driver.NetworkConfig{
				Validators:    driver.NewDefaultTestValidators(t.Name(), 2),
				RoundTripTime: rtt,
			}
			net, err := NewLocalLegacyNetwork(t.Context(), &config)
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
	config := driver.NetworkConfig{Validators: driver.DefaultValidators(t.Name())}
	config.Validators[0].Name = fmt.Sprintf("validator-%s", t.Name())
	for _, N := range []int{1, 3} {
		N := N
		t.Run(fmt.Sprintf("num-nodes-%d", N), func(t *testing.T) {

			net, err := NewLocalLegacyNetwork(t.Context(), &config)
			if err != nil {
				t.Fatalf("failed to create new local network: %v", err)
			}
			t.Cleanup(func() {
				_ = net.Shutdown()
			})

			apps := []driver.Application{}
			for i := 0; i < N; i++ {
				app, err := net.CreateApplication(t.Context(), &driver.ApplicationConfig{
					Name: fmt.Sprintf("A-%d-%s", i, t.Name()),
				})
				if err != nil {
					t.Fatalf("failed to create app: %v", err)
				}

				if got, want := app.Config().Name, fmt.Sprintf("A-%d-%s", i, t.Name()); got != want {
					t.Errorf("app configurion not propagated: %v != %v", got, want)
				}

				apps = append(apps, app)
			}

			for _, app := range apps {
				if err := app.Start(t.Context()); err != nil {
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
	config := driver.NetworkConfig{Validators: driver.DefaultValidators(t.Name())}

	net, err := NewLocalLegacyNetwork(t.Context(), &config)
	if err != nil {
		t.Fatalf("failed to create new local network: %v", err)
	}
	t.Cleanup(func() {
		_ = net.Shutdown()
	})

	for i := 0; i < N; i++ {
		_, err := net.CreateNode(&driver.NodeConfig{
			Name:  fmt.Sprintf("N-%d-%s", i, t.Name()),
			Image: driver.DefaultClientDockerImageName,
		})
		if err != nil {
			t.Fatalf("failed to create node: %v", err)
		}
	}

	for i := 0; i < N; i++ {
		_, err := net.CreateApplication(t.Context(), &driver.ApplicationConfig{
			Name: fmt.Sprintf("A-%d-%s", i, t.Name()),
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
	config := driver.NetworkConfig{Validators: driver.DefaultValidators(t.Name())}

	net, err := NewLocalLegacyNetwork(t.Context(), &config)
	if err != nil {
		t.Fatalf("failed to create new local network: %v", err)
	}
	t.Cleanup(func() {
		_ = net.Shutdown()
	})

	createdNode, err := net.CreateNode(&driver.NodeConfig{
		Name:  fmt.Sprintf("N-%d-%s", 1, t.Name()),
		Image: driver.DefaultClientDockerImageName,
	})
	if err != nil {
		t.Fatalf("failed to create node: %v", err)
	}

	operaNode, ok := createdNode.(*node.OperaNode)
	if !ok {
		t.Fatalf("node is not an OperaNode")
	}

	// Stop sonicd gracefully (not the full network shutdown, which also
	// cleans up temp dirs before we can read the log).
	if err := operaNode.Stop(t.Context()); err != nil {
		t.Errorf("failed to stop node: %v", err)
	}

	// Wait for sonicd exec to finish so the log file is fully flushed.
	select {
	case <-operaNode.ExecDone():
	case <-time.After(60 * time.Second):
		t.Fatalf("sonicd exec did not finish in time")
	}

	// Read the complete exec log and check for graceful shutdown message.
	reader, err := operaNode.StreamExecLog()
	if err != nil {
		t.Fatalf("cannot read exec log: %v", err)
	}
	defer func() { _ = reader.Close() }()

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

func TestLocalNetwork_CanRunWithMultipleValidators(t *testing.T) {
	for _, N := range []int{1, 3} {
		N := N
		config := driver.NetworkConfig{Validators: driver.NewDefaultTestValidators(t.Name(), N)}
		t.Run(fmt.Sprintf("num-validators-%d", N), func(t *testing.T) {
			net, err := NewLocalLegacyNetwork(t.Context(), &config)
			if err != nil {
				t.Fatalf("failed to create new local network: %v", err)
			}
			t.Cleanup(func() {
				_ = net.Shutdown()
			})

			app, err := net.CreateApplication(t.Context(), &driver.ApplicationConfig{
				Name: "TestApp",
			})
			if err != nil {
				t.Fatalf("failed to create app: %v", err)
			}

			if err := app.Start(t.Context()); err != nil {
				t.Errorf("failed to start app: %v", err)
			}

			if err := app.Stop(); err != nil {
				t.Errorf("failed to stop app: %v", err)
			}
		})
	}
}

func TestLocalNetwork_CanRunWithVariousValidators(t *testing.T) {
	validators := driver.Validators{
		{Name: "validator", Instances: 1, ImageName: driver.DefaultClientDockerImageName, Stake: 5_000_000},
		{Name: "validator2", Instances: 1, ImageName: "sonic:v2.1.6", Stake: 5_000_000},
		{Name: "validator3", Instances: 1, ImageName: "sonic:local", Stake: 5_000_000},
	}

	config := driver.NetworkConfig{Validators: validators}
	net, err := NewLocalLegacyNetwork(t.Context(), &config)
	if err != nil {
		t.Fatalf("failed to create new local network: %v", err)
	}
	t.Cleanup(func() {
		if err := net.Shutdown(); err != nil {
			t.Fatalf("failed to shut down network: %v", err)
		}
	})

	if got := net.GetActiveNodes(); len(got) != 3 {
		t.Errorf("invalid number of active nodes, got %d, want 3", len(got))
	}

}

func TestLocalNetwork_NotifiesListenersOnNodeStartup(t *testing.T) {
	t.Parallel()
	config := driver.NetworkConfig{Validators: driver.NewDefaultTestValidators(t.Name(), 1)}
	ctrl := gomock.NewController(t)
	listener := driver.NewMockNetworkListener(ctrl)

	net, err := NewLocalLegacyNetwork(t.Context(), &config)
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

	_, err = net.CreateNode(&driver.NodeConfig{
		Name:  t.Name(),
		Image: driver.DefaultClientDockerImageName,
	})
	if err != nil {
		t.Fatalf("failed to create node: %v", err)
	}

	activeNodes = net.GetActiveNodes()
	if got, want := len(activeNodes), config.Validators.GetNumValidators()+1; got != want {
		t.Errorf("invalid number of active nodes, got %d, want %d", got, want)
	}

}

func TestLocalNetwork_NotifiesListenersOnAppStartup(t *testing.T) {
	t.Parallel()
	config := driver.NetworkConfig{Validators: driver.NewDefaultTestValidators(t.Name(), 1)}
	ctrl := gomock.NewController(t)
	listener := driver.NewMockNetworkListener(ctrl)

	net, err := NewLocalLegacyNetwork(t.Context(), &config)
	if err != nil {
		t.Fatalf("failed to create new local network: %v", err)
	}
	t.Cleanup(func() {
		_ = net.Shutdown()
	})

	net.RegisterListener(listener)
	listener.EXPECT().AfterApplicationCreation(gomock.Any())

	_, err = net.CreateApplication(t.Context(), &driver.ApplicationConfig{
		Name: "TestApp",
	})
	if err != nil {
		t.Errorf("creation of app failed: %v", err)
	}
}

func TestLocalNetwork_CanRemoveNode(t *testing.T) {
	t.Parallel()
	config := driver.NetworkConfig{Validators: driver.DefaultValidators(t.Name())}
	for _, N := range []int{1, 3} {
		N := N
		t.Run(fmt.Sprintf("num-nodes-%d", N), func(t *testing.T) {
			net, err := NewLocalLegacyNetwork(t.Context(), &config)
			if err != nil {
				t.Fatalf("failed to create new local network: %v", err)
			}
			t.Cleanup(func() {
				_ = net.Shutdown()
			})

			ctrl := gomock.NewController(t)
			listener := driver.NewMockNetworkListener(ctrl)
			listener.EXPECT().AfterNodeCreation(gomock.Any()).Times(N)
			listener.EXPECT().BeforeNodeRemoval(gomock.Any()).Times(N)
			net.RegisterListener(listener)

			nodes := make([]driver.Node, 0, N)
			for i := 0; i < N; i++ {
				node, err := net.CreateNode(&driver.NodeConfig{
					Name:  fmt.Sprintf("N-%d-%s", i, t.Name()),
					Image: driver.DefaultClientDockerImageName,
				})
				if err != nil {
					t.Fatalf("failed to create node: %s", err)
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
				if err := node.Stop(t.Context()); err != nil {
					t.Errorf("failed to stop node: %v", err)
				}
				if err := node.Cleanup(context.Background()); err != nil {
					t.Errorf("failed to cleanup node: %v", err)
				}
			}
		})
	}
}

func TestReconnectNodeAndRemoveNode_Succeed_WhenOnlyOtherNodeIsSuspended(t *testing.T) {
	t.Parallel()
	require := require.New(t)
	config := driver.NetworkConfig{Validators: driver.DefaultValidators(t.Name())}
	net, err := NewLocalLegacyNetwork(t.Context(), &config)
	require.NoError(err, "failed to create new local network")
	t.Cleanup(func() {
		require.NoError(net.Shutdown())
	})

	validators := net.GetActiveNodes()
	require.Len(validators, 1, "expected one genesis validator")
	validator := validators[0]

	observer, err := net.CreateNode(&driver.NodeConfig{
		Name:  "observer-" + t.Name(),
		Image: driver.DefaultClientDockerImageName,
	})
	require.NoError(err, "failed to create node")

	// Take the validator's client down the way a killSonic step does:
	// suspend first, then SIGKILL the client while the container lives on.
	opera, ok := validator.(*node.OperaNode)
	require.True(ok, "validator is not an OperaNode")
	net.SuspendNode(validator)
	require.NoError(opera.ForceStopSonicd(t.Context()),
		"failed to kill validator client")

	// The suspended validator is the only potential peer left. Both peer
	// management operations must skip its dead RPC endpoint and succeed
	// immediately, rather than retrying it for minutes and failing.
	require.NoError(net.ReconnectNode(t.Context(), observer),
		"reconnect stalled on suspended node")
	require.NoError(net.RemoveNode(observer),
		"remove stalled on suspended node")

	require.NoError(observer.Stop(t.Context()), "failed to stop node")
	require.NoError(observer.Cleanup(context.Background()), "failed to cleanup node")
}

func TestLocalNetwork_Num_Validators_Started(t *testing.T) {
	for i := 1; i < 3; i++ {
		t.Run(fmt.Sprintf("num-validators-%d", i), func(t *testing.T) {
			config := driver.NetworkConfig{Validators: driver.NewDefaultTestValidators(t.Name(), i)}
			net, err := NewLocalLegacyNetwork(t.Context(), &config)
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

// TestLocalNetwork_Can_Run_Multiple_Client_Images_LatestVersions checks if
// docker can create client through images called "sonic" and "sonic:local"
func TestLocalNetwork_Can_Run_Multiple_Client_Images_LatestVersions(t *testing.T) {
	config := driver.NetworkConfig{Validators: driver.DefaultValidators(t.Name())}

	net, err := NewLocalLegacyNetwork(t.Context(), &config)
	if err != nil {
		t.Fatalf("failed to create new local network: %v", err)
	}
	t.Cleanup(func() {
		_ = net.Shutdown()
	})

	images := []string{"sonic", "sonic:local"}
	checksum := make(chan string)
	gotChecksums := 0
	for _, image := range images {
		go func(img string) {
			cs, err := getChecksum(net, img)
			if err != nil {
				t.Errorf("failed to get checksum for %s; %v", img, err)
			}
			checksum <- cs
		}(image)
	}

	for gotChecksums < len(images) {
		select {
		case <-checksum:
			gotChecksums++
		case <-time.After(180 * time.Second):
			t.Fatalf("timeout while waiting for checksums; got: %d, want: %d", gotChecksums, len(images))
		}
	}

	if got, want := gotChecksums, len(images); got != want {
		t.Errorf("invalid number of checksum, got: %d, want %d", got, want)
	}
}

// TestLocalNetwork_Can_Run_Multiple_Client_Images_TaggedVersions checks if
// docker can create client through images called "sonic:<versions>"
// The checksum of each version must be unique.
func TestLocalNetwork_Can_Run_Multiple_Client_Images_TaggedVersions(t *testing.T) {
	config := driver.NetworkConfig{Validators: driver.DefaultValidators(t.Name())}

	net, err := NewLocalLegacyNetwork(t.Context(), &config)
	if err != nil {
		t.Fatalf("failed to create new local network: %v", err)
	}
	t.Cleanup(func() {
		_ = net.Shutdown()
	})

	images := []string{"sonic:v2.1.6", "sonic:local"}
	checksum := make(chan string)
	gotChecksums := make(map[string]struct{})
	for _, image := range images {
		go func(img string) {
			cs, err := getChecksum(net, img)
			if err != nil {
				t.Errorf("failed to get checksum for %s; %v", img, err)
			}
			checksum <- cs
		}(image)
	}

	for len(gotChecksums) < len(images) {
		select {
		case val := <-checksum:
			gotChecksums[val] = struct{}{}
		case <-time.After(180 * time.Second):
			t.Fatalf("timeout while waiting for checksums; got: %d, want: %d", len(gotChecksums), len(images))
		}
	}

	if got, want := len(gotChecksums), len(images); got != want {
		t.Errorf("invalid number of checksum, got: %d, want %d", got, want)
	}
}

// getChecksum creates a node of the provided image type on the provided network
// and extracts the sha256 checksum of the sonicd binary via docker exec.
func getChecksum(net *LocalNetwork, image string) (string, error) {
	n, err := net.CreateNode(&driver.NodeConfig{
		Name:  fmt.Sprintf("T-%s", strings.ReplaceAll(image, ":", "-")),
		Image: image,
	})
	if err != nil {
		return "", fmt.Errorf("failed to create node: %v", err)
	}

	operaNode, ok := n.(*node.OperaNode)
	if !ok {
		return "", fmt.Errorf("node is not an OperaNode")
	}

	output, err := operaNode.Exec(context.Background(), []string{"sha256sum", "./sonicd"})
	if err != nil {
		return "", fmt.Errorf("failed to exec sha256sum: %w", err)
	}

	fields := strings.Fields(strings.TrimSpace(output))
	if len(fields) == 0 {
		return "", fmt.Errorf("empty sha256sum output")
	}
	return fields[0], nil
}

// Every transaction spending the shared system account's nonce must go through
// the same node, so a lagging node cannot hand out a nonce already spent.
func TestLocalNetwork_DialSystemRpc_ReturnsSameNodeOnEveryCall(t *testing.T) {
	require := require.New(t)
	t.Parallel()

	config := driver.NetworkConfig{Validators: driver.DefaultValidators(t.Name())}
	net, err := NewLocalLegacyNetwork(t.Context(), &config)
	require.NoError(err)
	t.Cleanup(func() {
		require.NoError(net.Shutdown())
	})

	var first string
	for i := range 5 {
		client, err := net.DialSystemRpc()
		require.NoError(err, "call %d", i+1)
		client.Close()

		net.systemRpcMu.Lock()
		label := net.systemRpcLabel
		net.systemRpcMu.Unlock()

		require.NotEmpty(label, "call %d left no pinned node", i+1)
		if i == 0 {
			first = label
			continue
		}
		require.Equal(first, label, "system node moved on call %d", i+1)
	}
}

// A node can accept a connection while its RPC serves nothing. Dialling alone
// would then look like success, and the pinned node would never be re-chosen.
func TestLocalNetwork_ProbeRpc_FailsWhenNodeAcceptsButDoesNotAnswer(t *testing.T) {
	tests := map[string]struct {
		blockNumber func(m *rpc.MockClient)
		wantErr     bool
	}{
		"answers": {
			blockNumber: func(m *rpc.MockClient) {
				m.EXPECT().BlockNumber(gomock.Any()).Return(uint64(1), nil)
			},
			wantErr: false,
		},
		"accepts but does not answer": {
			blockNumber: func(m *rpc.MockClient) {
				m.EXPECT().BlockNumber(gomock.Any()).Return(uint64(0), context.DeadlineExceeded)
			},
			wantErr: true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			require := require.New(t)

			client := rpc.NewMockClient(gomock.NewController(t))
			test.blockNumber(client)

			err := probeRpc(client)
			if test.wantErr {
				require.Error(err)
				return
			}
			require.NoError(err)
		})
	}
}

func TestLocalNetworkApplyNetworkRules_Success(t *testing.T) {
	t.Parallel()
	config := driver.NetworkConfig{Validators: driver.DefaultValidators(t.Name())}
	net, err := NewLocalLegacyNetwork(t.Context(), &config)
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

	wantFee := originalRules.Economy.MinBaseFee.Int64() + 123
	baseFeePatch := genesis.BigIntValue(*big.NewInt(wantFee))
	rules := driver.NetworkRules{
		Economy: &genesis.EconomyPatch{
			MinBaseFee: &baseFeePatch,
		},
	}

	if err := net.ApplyNetworkRules(t.Context(), rules); err != nil {
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

func TestLocalNetworkAdvanceEpoch_Success(t *testing.T) {
	t.Parallel()
	config := driver.NetworkConfig{Validators: driver.DefaultValidators(t.Name())}
	net, err := NewLocalLegacyNetwork(t.Context(), &config)
	if err != nil {
		t.Fatalf("failed to create new local network: %v", err)
	}
	t.Cleanup(func() {
		if err := net.Shutdown(); err != nil {
			t.Fatalf("failed to shut down network, %v", err)
		}
	})

	client, err := net.DialRandomRpc()
	if err != nil {
		t.Fatalf("failed to dial random RPC: %v", err)
	}
	defer client.Close()

	// get original epoch
	originalEpoch, err := network.GetCurrentEpoch(client)
	if err != nil {
		t.Fatalf("failed to get current epoch: %v", err)
	}

	epochIncrement := 3 // takes ~5-6 seconds per increment
	if err := net.AdvanceEpoch(t.Context(), epochIncrement); err != nil {
		t.Errorf("failed to advance epoch: %v", err)
	}

	newEpoch, err := network.GetCurrentEpoch(client)
	if err != nil {
		t.Fatalf("failed to get new epoch: %v", err)
	}

	if got, want := newEpoch, originalEpoch+hexutil.Uint64(epochIncrement); got < want {
		t.Errorf("epoch did not advance correctly, got %d, want %d", got, want)
	}
}

// Two advances in a row are what the parser appends to every scenario, and used
// to fail the second one by reading a nonce the first had already spent.
func TestLocalNetwork_AdvanceEpoch_SucceedsWhenCalledTwiceInARow(t *testing.T) {
	require := require.New(t)
	t.Parallel()

	config := driver.NetworkConfig{Validators: driver.DefaultValidators(t.Name())}
	net, err := NewLocalLegacyNetwork(t.Context(), &config)
	require.NoError(err)
	t.Cleanup(func() {
		require.NoError(net.Shutdown())
	})

	for i := range 2 {
		require.NoError(net.AdvanceEpoch(t.Context(), 1), "advance %d of 2", i+1)
	}

	net.systemRpcMu.Lock()
	pinned := net.systemRpcLabel
	net.systemRpcMu.Unlock()
	require.NotEmpty(pinned, "no node was pinned for system transactions")

	active := make([]string, 0, len(net.GetActiveNodes()))
	for _, node := range net.GetActiveNodes() {
		active = append(active, string(node.GetLabel()))
	}
	require.Contains(active, pinned)
}

func TestLocalNetwork_FailingFlagPropagated(t *testing.T) {
	t.Parallel()
	config := driver.NetworkConfig{Validators: []driver.Validator{
		{Name: t.Name() + "-validator-ok", Failing: false, Instances: 1, ImageName: driver.DefaultClientDockerImageName},
		{Name: t.Name() + "-failing", Failing: true, Instances: 1, ImageName: driver.DefaultClientDockerImageName},
	}}
	net, err := NewLocalLegacyNetwork(t.Context(), &config)
	if err != nil {
		t.Fatalf("failed to create new local network: %v", err)
	}
	t.Cleanup(func() {
		if err := net.Shutdown(); err != nil {
			t.Fatalf("failed to shut down network: %v", err)
		}
	})

	if _, err := net.CreateNode(&driver.NodeConfig{
		Name:    t.Name() + "-failing-late",
		Failing: true,
		Image:   driver.DefaultClientDockerImageName,
	}); err != nil {
		t.Fatalf("failed to create node: %v", err)
	}

	var failingNodes int
	var nonFailingNodes int
	for _, node := range net.GetActiveNodes() {
		if node.IsExpectedFailure() {
			failingNodes++
		} else {
			nonFailingNodes++
		}
	}

	if got, want := failingNodes, 2; got < want {
		t.Errorf("insufficient failing nodes, got %d, want at least %d", got, want)
	}
	if got, want := nonFailingNodes, 1; got < want {
		t.Errorf("insufficient non-failing nodes, got %d, want at least %d", got, want)
	}

}

func TestLocalNetwork_MountDataDir_Can_Be_Reused(t *testing.T) {
	t.Parallel()
	temp := t.TempDir()

	config := driver.NetworkConfig{
		Validators: driver.DefaultValidators(t.Name() + "-1"),
		OutputDir:  temp,
	}
	net, err := NewLocalLegacyNetwork(t.Context(), &config)
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
		Name:       t.Name() + "-2",
		DataVolume: &dataVolume,
		Image:      driver.DefaultClientDockerImageName,
	})
	if err != nil {
		t.Fatalf("failed to create node: %v", err)
	}

	getModificationTime := func() (time.Time, []string, error) {
		var carmenModTime time.Time
		var found bool
		var visitedDirs []string
		localDirBinding := fmt.Sprintf("%s/%s", temp, dataVolume)
		err := filepath.Walk(localDirBinding, func(path string, info os.FileInfo, err error) error {
			visitedDirs = append(visitedDirs, path)
			if strings.HasSuffix(path, "transactions.rlp") {
				carmenModTime, found = info.ModTime(), true
			}
			return nil
		})
		if err == nil && !found {
			err = fmt.Errorf("directory contains no database files, visited %v", visitedDirs)
		}
		return carmenModTime, visitedDirs, err
	}

	// save modification time of the database lock
	prevModTime, prevVisitedDirs, err := getModificationTime()
	if err != nil {
		t.Fatalf("failed to get modification time: %v", err)
	}

	if !slices.ContainsFunc(
		prevVisitedDirs,
		func(s string) bool { return strings.Contains(s, temp) }) {
		t.Errorf(
			"expected at least one visited directory to contain %s, but visited %v",
			temp, prevVisitedDirs)
	}

	// stop the node
	if err := net.RemoveNode(node); err != nil {
		t.Fatalf("failed to remove node: %v", err)
	}
	if err := node.Stop(t.Context()); err != nil {
		t.Fatalf("failed to stop node: %v", err)
	}
	if err := node.Cleanup(context.Background()); err != nil {
		t.Fatalf("failed to cleanup node: %v", err)
	}

	// re-run another node on the same data volume
	if _, err := net.CreateNode(&driver.NodeConfig{
		Name:       t.Name() + "-3",
		DataVolume: &dataVolume,
		Image:      driver.DefaultClientDockerImageName,
	}); err != nil {
		t.Fatalf("failed to create node: %v", err)
	}

	// the database lock should have been updated
	currModTime, currVisitedDirs, err := getModificationTime()
	if err != nil {
		t.Fatalf("failed to get modification time: %v", err)
	}
	if got, want := currModTime, prevModTime; got.Equal(want) {
		t.Errorf("got modification time %v, wanted modification time %v", got, want)
	}
	if !slices.ContainsFunc(
		currVisitedDirs,
		func(s string) bool { return strings.Contains(s, temp) }) {
		t.Errorf(
			"expected at least one visited directory to contain %s, but visited %v",
			temp, currVisitedDirs)
	}
}
