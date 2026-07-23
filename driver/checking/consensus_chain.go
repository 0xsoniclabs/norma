package checking

import (
	"context"
	"fmt"
	"maps"
	"time"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/monitoring"
	"github.com/0xsoniclabs/sonic/gossip/blockproc/netrules"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
)

// defaultConsensusChainTimeout bounds how long the check waits for the
// useConsensusChain flag to read as active on every node. The flag is served
// through the asynchronously-written archive state, so a read right after the
// hand-over can transiently see the pre-flip value.
const defaultConsensusChainTimeout = 30 * time.Second

// consensusChainPollInterval is the delay between convergence polls. Var so
// tests can shorten it.
var consensusChainPollInterval = 500 * time.Millisecond

func init() {
	RegisterNetworkCheck("consensusChainActive", func(net driver.Network, monitor *monitoring.Monitor) Checker {
		return &consensusChainChecker{net: net, timeout: defaultConsensusChainTimeout}
	})
}

// consensusChainChecker verifies that the Sonic consensus engine is the
// canonical block producer, i.e. the on-chain useConsensusChain flag reads as
// active on every non-failing node.
type consensusChainChecker struct {
	net driver.Network
	// timeout bounds convergence polling. Zero means a single attempt.
	timeout time.Duration
}

func (c *consensusChainChecker) Configure(config CheckerConfig) Checker {
	return c
}

func (c *consensusChainChecker) Check(ctx context.Context) error {
	if c.timeout <= 0 {
		return c.checkOnce(ctx)
	}

	deadline := time.Now().Add(c.timeout)
	ticker := time.NewTicker(consensusChainPollInterval)
	defer ticker.Stop()

	for {
		err := c.checkOnce(ctx)
		if err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (c *consensusChainChecker) checkOnce(ctx context.Context) error {
	nodes := c.net.GetActiveNodes()

	expectedFailures := make(map[string]struct{})
	gotFailures := make(map[string]struct{})
	for _, node := range nodes {
		if node.IsExpectedFailure() {
			expectedFailures[node.GetLabel()] = struct{}{}
		}

		rpcClient, err := node.DialRpc(ctx)
		if err != nil {
			if node.IsExpectedFailure() {
				gotFailures[node.GetLabel()] = struct{}{}
				continue
			}
			return fmt.Errorf("failed to dial node RPC %s: %w", node.GetLabel(), err)
		}

		caller, err := netrules.NewNetworkRulesCaller(netrules.GetAddress(), rpcClient)
		if err != nil {
			rpcClient.Close()
			return fmt.Errorf("failed to get network rules caller on node %s: %w", node.GetLabel(), err)
		}

		active, err := caller.GetUseConsensusChain(&bind.CallOpts{Context: ctx})
		rpcClient.Close()
		if err != nil {
			if node.IsExpectedFailure() {
				gotFailures[node.GetLabel()] = struct{}{}
				continue
			}
			return fmt.Errorf("failed to read useConsensusChain on node %s: %w", node.GetLabel(), err)
		}
		if !active {
			if node.IsExpectedFailure() {
				gotFailures[node.GetLabel()] = struct{}{}
				continue
			}
			return fmt.Errorf("consensus chain is not canonical on node %s: useConsensusChain is false", node.GetLabel())
		}
	}

	if got, want := gotFailures, expectedFailures; !maps.Equal(got, want) {
		return fmt.Errorf("unexpected failure set for consensus-chain check, got %v, want %v", got, want)
	}

	return nil
}
