package checking

import (
	"math"
	"strings"
	"testing"

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
			"blocks": map[string]any{
				"max_block_gas": 20500000000,
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
	node.EXPECT().DialRpc().Return(rpcClient, nil)
	rpcClient.EXPECT().GetNetworkRules("latest").Return(current, nil)
	rpcClient.EXPECT().Close()

	checker := (&networkRulesChecker{net: net}).Configure(config)
	if err := checker.Check(); err != nil {
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
	node.EXPECT().DialRpc().Return(rpcClient, nil)
	node.EXPECT().GetLabel().AnyTimes().Return("node-1")
	rpcClient.EXPECT().GetNetworkRules("latest").Return(current, nil)
	rpcClient.EXPECT().Close()

	checker := (&networkRulesChecker{
		net: net,
		rulesPatch: genesis.NetworkRulesPatch{
			Blocks: &genesis.BlocksPatch{MaxBlockGas: new(uint64(20500000000))},
		},
	})

	err := checker.Check()
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
	node.EXPECT().DialRpc().Return(rpcClient, nil)
	node.EXPECT().GetLabel().AnyTimes().Return("node-1")
	rpcClient.EXPECT().GetNetworkRules("latest").Return(current, nil)
	rpcClient.EXPECT().Close()

	checker := (&networkRulesChecker{
		net: net,
		rulesPatch: genesis.NetworkRulesPatch{
			Blocks: &genesis.BlocksPatch{MaxBlockGas: new(uint64(math.MaxInt64))},
		},
	})

	err := checker.Check()
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
	rpcClient2 := rpc.NewMockClient(ctrl)

	current := opera.FakeNetRules(opera.GetSonicUpgrades())
	patched := current
	if err := genesis.ApplyNetworkRulesPatch(&patched, genesis.NetworkRulesPatch{
		Blocks: &genesis.BlocksPatch{MaxBlockGas: new(uint64(20500000000))},
	}); err != nil {
		t.Fatalf("failed to prepare patched rules: %v", err)
	}

	net.EXPECT().GetActiveNodes().Return([]driver.Node{node1, node2})

	node1.EXPECT().IsExpectedFailure().AnyTimes().Return(false)
	node1.EXPECT().DialRpc().Return(rpcClient1, nil)
	node1.EXPECT().GetLabel().AnyTimes().Return("node-1")
	rpcClient1.EXPECT().GetNetworkRules("latest").Return(patched, nil)
	rpcClient1.EXPECT().Close()

	node2.EXPECT().IsExpectedFailure().AnyTimes().Return(true)
	node2.EXPECT().DialRpc().Return(rpcClient2, nil)
	node2.EXPECT().GetLabel().AnyTimes().Return("node-2")
	rpcClient2.EXPECT().GetNetworkRules("latest").Return(current, nil)
	rpcClient2.EXPECT().Close()

	checker := &networkRulesChecker{
		net: net,
		rulesPatch: genesis.NetworkRulesPatch{
			Blocks: &genesis.BlocksPatch{MaxBlockGas: new(uint64(20500000000))},
		},
	}

	if err := checker.Check(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNetworkRulesChecker_Configure_WithInvalidRulesConfig(t *testing.T) {
	checker := (&networkRulesChecker{}).Configure(CheckerConfig{"rules": []any{"invalid"}})

	err := checker.Check()
	if err == nil || !strings.Contains(err.Error(), "failed to decode rules patch") {
		t.Fatalf("expected configure error, got: %v", err)
	}
}

func TestNetworkRulesChecker_Check_FailsOnExpectedFailureSetMismatch(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)
	node := driver.NewMockNode(ctrl)
	rpcClient := rpc.NewMockClient(ctrl)

	current := opera.FakeNetRules(opera.GetSonicUpgrades())
	if err := genesis.ApplyNetworkRulesPatch(&current, genesis.NetworkRulesPatch{
		Blocks: &genesis.BlocksPatch{MaxBlockGas: new(uint64(20500000000))},
	}); err != nil {
		t.Fatalf("failed to prepare current rules: %v", err)
	}

	net.EXPECT().GetActiveNodes().Return([]driver.Node{node})
	node.EXPECT().IsExpectedFailure().AnyTimes().Return(true)
	node.EXPECT().GetLabel().AnyTimes().Return("node-1")
	node.EXPECT().DialRpc().Return(rpcClient, nil)
	rpcClient.EXPECT().GetNetworkRules("latest").Return(current, nil)
	rpcClient.EXPECT().Close()

	checker := &networkRulesChecker{net: net, rulesPatch: genesis.NetworkRulesPatch{Blocks: &genesis.BlocksPatch{MaxBlockGas: new(uint64(20500000000))}}}
	if err := checker.Check(); err == nil || !strings.Contains(err.Error(), "unexpected failure set") {
		t.Fatalf("expected failure-set mismatch, got: %v", err)
	}
}
