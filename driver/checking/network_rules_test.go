package checking

import (
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/rpc"
	"github.com/0xsoniclabs/norma/genesis"
	"github.com/0xsoniclabs/sonic/opera"
	"go.uber.org/mock/gomock"
)

func TestNetworkRulesChecker_ConfigureAndCheck_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)
	node := driver.NewMockNode(ctrl)
	rpcClient := rpc.NewMockClient(ctrl)

	config := CheckerConfig{
		"rules": map[string]any{
			"Blocks": map[string]any{
				"MaxBlockGas": 20500000000,
			},
		},
	}

	current := opera.FakeNetRules(opera.GetSonicUpgrades())
	if err := genesis.ApplyNetworkRulesPatch(&current, genesis.NetworkRulesPatch{
		Blocks: &genesis.BlocksPatch{MaxBlockGas: new(uint64(20500000000))},
	}); err != nil {
		t.Fatalf("failed to prepare current rules: %v", err)
	}

	net.EXPECT().GetActiveNodes().Return([]driver.Node{node})
	node.EXPECT().IsExpectedFailure().AnyTimes().Return(false)
	node.EXPECT().DialRpc(gomock.Any()).Return(rpcClient, nil)
	rpcClient.EXPECT().GetNetworkRules("latest").Return(current, nil)
	rpcClient.EXPECT().Close()

	checker := (&networkRulesChecker{net: net}).Configure(config)
	if err := checker.Check(t.Context()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNetworkRulesChecker_Check_FailsOnMismatch(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)
	node := driver.NewMockNode(ctrl)
	rpcClient := rpc.NewMockClient(ctrl)

	current := opera.FakeNetRules(opera.GetSonicUpgrades())

	net.EXPECT().GetActiveNodes().Return([]driver.Node{node})
	node.EXPECT().IsExpectedFailure().AnyTimes().Return(false)
	node.EXPECT().DialRpc(gomock.Any()).Return(rpcClient, nil)
	node.EXPECT().GetLabel().AnyTimes().Return("node-1")
	rpcClient.EXPECT().GetNetworkRules("latest").Return(current, nil)
	rpcClient.EXPECT().Close()

	checker := (&networkRulesChecker{
		net: net,
		rulesPatch: genesis.NetworkRulesPatch{
			Blocks: &genesis.BlocksPatch{MaxBlockGas: new(uint64(20500000000))},
		},
	})

	err := checker.Check(t.Context())
	if err == nil || !strings.Contains(err.Error(), "applied network rules mismatch") {
		t.Fatalf("expected mismatch error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "Blocks.MaxBlockGas") {
		t.Fatalf("expected mismatch details to include field path, got: %v", err)
	}
}

func TestNetworkRulesChecker_Check_FailsOnLargeUint64Mismatch(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)
	node := driver.NewMockNode(ctrl)
	rpcClient := rpc.NewMockClient(ctrl)

	current := opera.FakeNetRules(opera.GetSonicUpgrades())

	net.EXPECT().GetActiveNodes().Return([]driver.Node{node})
	node.EXPECT().IsExpectedFailure().AnyTimes().Return(false)
	node.EXPECT().DialRpc(gomock.Any()).Return(rpcClient, nil)
	node.EXPECT().GetLabel().AnyTimes().Return("node-1")
	rpcClient.EXPECT().GetNetworkRules("latest").Return(current, nil)
	rpcClient.EXPECT().Close()

	checker := (&networkRulesChecker{
		net: net,
		rulesPatch: genesis.NetworkRulesPatch{
			Blocks: &genesis.BlocksPatch{MaxBlockGas: new(uint64(math.MaxInt64))},
		},
	})

	err := checker.Check(t.Context())
	if err == nil || !strings.Contains(err.Error(), "applied network rules mismatch") {
		t.Fatalf("expected mismatch error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "Blocks.MaxBlockGas") {
		t.Fatalf("expected mismatch details to include field path, got: %v", err)
	}
	if strings.Contains(err.Error(), "unable to extract differing fields") {
		t.Fatalf("expected concrete mismatch details, got: %v", err)
	}
}

func TestNetworkRulesChecker_Check_ExpectedFailingNode(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)
	node1 := driver.NewMockNode(ctrl)
	node2 := driver.NewMockNode(ctrl)
	rpcClient1 := rpc.NewMockClient(ctrl)

	current := opera.FakeNetRules(opera.GetSonicUpgrades())
	patched := current
	if err := genesis.ApplyNetworkRulesPatch(&patched, genesis.NetworkRulesPatch{
		Blocks: &genesis.BlocksPatch{MaxBlockGas: new(uint64(20500000000))},
	}); err != nil {
		t.Fatalf("failed to prepare patched rules: %v", err)
	}

	net.EXPECT().GetActiveNodes().Return([]driver.Node{node1, node2})

	node1.EXPECT().IsExpectedFailure().AnyTimes().Return(false)
	node1.EXPECT().DialRpc(gomock.Any()).Return(rpcClient1, nil)
	node1.EXPECT().GetLabel().AnyTimes().Return("node-1")
	rpcClient1.EXPECT().GetNetworkRules("latest").Return(patched, nil)
	rpcClient1.EXPECT().Close()

	// node2 is expected to fail and is never dialed.
	node2.EXPECT().IsExpectedFailure().AnyTimes().Return(true)
	node2.EXPECT().GetLabel().AnyTimes().Return("node-2")

	checker := &networkRulesChecker{
		net: net,
		rulesPatch: genesis.NetworkRulesPatch{
			Blocks: &genesis.BlocksPatch{MaxBlockGas: new(uint64(20500000000))},
		},
	}

	if err := checker.Check(t.Context()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNetworkRulesChecker_Check_ConvergesAfterLag(t *testing.T) {
	prev := networkRulesPollInterval
	networkRulesPollInterval = time.Millisecond
	defer func() { networkRulesPollInterval = prev }()

	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)
	node := driver.NewMockNode(ctrl)
	rpcClient := rpc.NewMockClient(ctrl)

	old := opera.FakeNetRules(opera.GetSonicUpgrades())
	updated := old
	patch := genesis.NetworkRulesPatch{Blocks: &genesis.BlocksPatch{MaxBlockGas: new(uint64(20500000000))}}
	if err := genesis.ApplyNetworkRulesPatch(&updated, patch); err != nil {
		t.Fatalf("failed to prepare updated rules: %v", err)
	}

	net.EXPECT().GetActiveNodes().Return([]driver.Node{node}).AnyTimes()
	node.EXPECT().IsExpectedFailure().AnyTimes().Return(false)
	node.EXPECT().GetLabel().AnyTimes().Return("node-1")
	node.EXPECT().DialRpc(gomock.Any()).Return(rpcClient, nil).AnyTimes()
	rpcClient.EXPECT().Close().AnyTimes()
	// The node reports the old rules first, then converges to the new ones.
	gomock.InOrder(
		rpcClient.EXPECT().GetNetworkRules("latest").Return(old, nil),
		rpcClient.EXPECT().GetNetworkRules("latest").Return(updated, nil),
	)

	checker := &networkRulesChecker{net: net, rulesPatch: patch, timeout: 5 * time.Second}
	if err := checker.Check(t.Context()); err != nil {
		t.Fatalf("expected convergence, got: %v", err)
	}
}

func TestNetworkRulesChecker_Check_ReturnsMismatchAfterTimeout(t *testing.T) {
	prev := networkRulesPollInterval
	networkRulesPollInterval = time.Millisecond
	defer func() { networkRulesPollInterval = prev }()

	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)
	node := driver.NewMockNode(ctrl)
	rpcClient := rpc.NewMockClient(ctrl)

	old := opera.FakeNetRules(opera.GetSonicUpgrades())

	net.EXPECT().GetActiveNodes().Return([]driver.Node{node}).AnyTimes()
	node.EXPECT().IsExpectedFailure().AnyTimes().Return(false)
	node.EXPECT().GetLabel().AnyTimes().Return("node-1")
	node.EXPECT().DialRpc(gomock.Any()).Return(rpcClient, nil).AnyTimes()
	rpcClient.EXPECT().GetNetworkRules("latest").Return(old, nil).AnyTimes()
	rpcClient.EXPECT().Close().AnyTimes()

	checker := &networkRulesChecker{
		net:        net,
		rulesPatch: genesis.NetworkRulesPatch{Blocks: &genesis.BlocksPatch{MaxBlockGas: new(uint64(20500000000))}},
		timeout:    20 * time.Millisecond,
	}
	err := checker.Check(t.Context())
	if err == nil || !strings.Contains(err.Error(), "applied network rules mismatch") {
		t.Fatalf("expected mismatch error after timeout, got: %v", err)
	}
}

func TestNetworkRulesChecker_Configure_WithInvalidRulesConfig(t *testing.T) {
	checker := (&networkRulesChecker{}).Configure(CheckerConfig{"rules": []any{"invalid"}})

	err := checker.Check(t.Context())
	if err == nil || !strings.Contains(err.Error(), "failed to decode rules patch") {
		t.Fatalf("expected configure error, got: %v", err)
	}
}

func TestNetworkRulesChecker_Check_SkipsFailingNodeWithStaleRules(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)
	node := driver.NewMockNode(ctrl)

	// The only node is expected to fail and is never dialed, so the check passes.
	net.EXPECT().GetActiveNodes().Return([]driver.Node{node})
	node.EXPECT().IsExpectedFailure().AnyTimes().Return(true)
	node.EXPECT().GetLabel().AnyTimes().Return("node-1")

	checker := &networkRulesChecker{net: net, rulesPatch: genesis.NetworkRulesPatch{Blocks: &genesis.BlocksPatch{MaxBlockGas: new(uint64(20500000000))}}}
	if err := checker.Check(t.Context()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNetworkRulesChecker_Check_EmptyPatchReturnsNil(t *testing.T) {
	checker := &networkRulesChecker{
		rulesPatch: genesis.NetworkRulesPatch{},
	}
	if err := checker.Check(t.Context()); err != nil {
		t.Fatalf("expected nil error for empty rules patch, got: %v", err)
	}
}

func TestNetworkRulesChecker_Check_DialRpcFailsOnNonFailingNode(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)
	node := driver.NewMockNode(ctrl)

	net.EXPECT().GetActiveNodes().Return([]driver.Node{node})
	node.EXPECT().IsExpectedFailure().AnyTimes().Return(false)
	node.EXPECT().GetLabel().AnyTimes().Return("node-1")
	node.EXPECT().DialRpc(gomock.Any()).Return(nil, fmt.Errorf("connection refused"))

	checker := &networkRulesChecker{
		net: net,
		rulesPatch: genesis.NetworkRulesPatch{
			Blocks: &genesis.BlocksPatch{MaxBlockGas: new(uint64(20500000000))},
		},
	}

	err := checker.Check(t.Context())
	if err == nil || !strings.Contains(err.Error(), "failed to dial node RPC") {
		t.Fatalf("expected dial error, got: %v", err)
	}
}
