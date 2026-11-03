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

package checking

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/rpc"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"go.uber.org/mock/gomock"
)

// historyNode describes one node of a simulated chain for the historical
// state check. Every block from 1 on holds one transaction from sender, which
// makes emitter emit one log.
type historyNode struct {
	label   string
	height  uint64
	failing bool
	// stateFork is the first block from which the node reports different
	// balances; zero means it agrees everywhere.
	stateFork uint64
	// duplicateIndexedLogs makes the log index serve every log twice.
	duplicateIndexedLogs bool
	// maxLogs makes a multi-block log query fail once it matches more logs.
	maxLogs int
}

var (
	historyEmitter = common.Address{0xE1}
	historySender  = common.Address{0x5E}
)

func historyNetwork(t *testing.T, nodes ...historyNode) (driver.Network, map[string]int) {
	t.Helper()
	ctrl := gomock.NewController(t)
	calls := map[string]int{}

	mocked := make([]driver.Node, 0, len(nodes))
	for _, spec := range nodes {
		node := driver.NewMockNode(ctrl)
		node.EXPECT().GetLabel().AnyTimes().Return(spec.label)
		node.EXPECT().IsExpectedFailure().AnyTimes().Return(spec.failing)

		client := rpc.NewMockClient(ctrl)
		client.EXPECT().Close().AnyTimes()
		client.EXPECT().BlockNumber(gomock.Any()).AnyTimes().Return(spec.height, nil)
		client.EXPECT().CallContext(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().
			DoAndReturn(func(_ context.Context, result any, method string, args ...any) error {
				calls[method]++
				response, err := spec.answer(method, args)
				if err != nil {
					return err
				}
				encoded, err := json.Marshal(response)
				if err != nil {
					return err
				}
				return json.Unmarshal(encoded, result)
			})

		node.EXPECT().DialRpc(gomock.Any()).AnyTimes().Return(client, nil)
		mocked = append(mocked, node)
	}

	net := driver.NewMockNetwork(ctrl)
	net.EXPECT().GetActiveNodes().AnyTimes().Return(mocked)
	return net, calls
}

func (n historyNode) answer(method string, args []any) (any, error) {
	block := func(arg any) uint64 {
		number, _ := hexutil.DecodeUint64(arg.(string))
		return number
	}
	switch method {
	case "eth_getLogs":
		filter := args[0].(map[string]any)
		from, to := block(filter["fromBlock"]), block(filter["toBlock"])
		_, indexed := filter["address"]
		logs := []map[string]any{}
		for b := max(from, 1); b <= min(to, n.height); b++ {
			entry := map[string]any{
				"address":          historyEmitter,
				"topics":           []string{},
				"data":             "0x",
				"blockNumber":      hexutil.EncodeUint64(b),
				"transactionHash":  common.Hash{byte(b)},
				"transactionIndex": "0x0",
				"blockHash":        common.Hash{0xB, byte(b)},
				"logIndex":         "0x0",
				"removed":          false,
			}
			if indexed {
				entry["blockTimestamp"] = hexutil.EncodeUint64(b)
				logs = append(logs, entry)
				if n.duplicateIndexedLogs {
					logs = append(logs, entry)
				}
			} else {
				logs = append(logs, entry)
			}
		}
		if n.maxLogs > 0 && to > from && len(logs) > n.maxLogs {
			return nil, fmt.Errorf("too many results, consider narrowing your query criteria, the limit is %d", n.maxLogs)
		}
		return logs, nil
	case "eth_getBlockByNumber":
		return map[string]any{"transactions": []map[string]any{{"from": historySender}}}, nil
	case "eth_getBalance":
		at := block(args[1])
		if n.stateFork > 0 && at >= n.stateFork {
			at += 1000
		}
		return hexutil.EncodeUint64(at), nil
	case "eth_getTransactionCount", "eth_getCode":
		return hexutil.EncodeUint64(block(args[1])), nil
	case "eth_getStorageAt":
		return common.Hash{byte(block(args[2]))}, nil
	}
	return nil, errors.New("unexpected method " + method)
}

func TestHistoricalState_PassesWhenAllNodesAgree(t *testing.T) {
	net, calls := historyNetwork(t,
		historyNode{label: "node1", height: 250},
		historyNode{label: "node2", height: 250},
	)

	c := historicalStateChecker{net: net}
	if err := c.Check(t.Context()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, method := range []string{"eth_getLogs", "eth_getBalance", "eth_getTransactionCount", "eth_getCode", "eth_getStorageAt"} {
		if calls[method] == 0 {
			t.Errorf("%s was never compared", method)
		}
	}
}

func TestHistoricalState_FailsOnDivergingState(t *testing.T) {
	net, _ := historyNetwork(t,
		historyNode{label: "node1", height: 250},
		historyNode{label: "node2", height: 250, stateFork: 200},
	)

	c := historicalStateChecker{net: net}
	err := c.Check(t.Context())
	if !errors.Is(err, ErrAssertion) || !strings.Contains(err.Error(), "eth_getBalance") {
		t.Fatalf("got %v, want an assertion on eth_getBalance", err)
	}
	if !strings.Contains(err.Error(), "node1") || !strings.Contains(err.Error(), "node2") {
		t.Errorf("error does not name the disagreeing nodes: %v", err)
	}
}

func TestHistoricalState_FailsWhenTheLogIndexDisagreesWithTheReceipts(t *testing.T) {
	net, _ := historyNetwork(t,
		historyNode{label: "node1", height: 250, duplicateIndexedLogs: true},
	)

	c := historicalStateChecker{net: net}
	err := c.Check(t.Context())
	if !errors.Is(err, ErrAssertion) || !strings.Contains(err.Error(), "log index") {
		t.Fatalf("got %v, want an assertion on the log index", err)
	}
}

func TestHistoricalState_FailsWhenAnotherNodeDuplicatesIndexedLogs(t *testing.T) {
	net, _ := historyNetwork(t,
		historyNode{label: "node1", height: 250},
		historyNode{label: "node2", height: 250, duplicateIndexedLogs: true},
	)

	c := historicalStateChecker{net: net}
	err := c.Check(t.Context())
	if !errors.Is(err, ErrAssertion) || !strings.Contains(err.Error(), "eth_getLogs") {
		t.Fatalf("got %v, want an assertion on eth_getLogs", err)
	}
}

func TestHistoricalState_NarrowsLogQueriesThatMatchTooManyLogs(t *testing.T) {
	net, _ := historyNetwork(t,
		historyNode{label: "node1", height: 250, maxLogs: 30},
		historyNode{label: "node2", height: 250, maxLogs: 30},
	)

	c := historicalStateChecker{net: net}
	if err := c.Check(t.Context()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHistoricalState_IgnoresNodesExpectedToFail(t *testing.T) {
	net, _ := historyNetwork(t,
		historyNode{label: "node1", height: 250},
		historyNode{label: "node2", height: 250, failing: true, stateFork: 10},
	)

	c := historicalStateChecker{net: net}
	if err := c.Check(t.Context()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHistoricalState_ComparesOnlyBlocksEveryNodeHas(t *testing.T) {
	net, _ := historyNetwork(t,
		historyNode{label: "ahead", height: 300, stateFork: 260},
		historyNode{label: "behind", height: 250},
	)

	c := historicalStateChecker{net: net}
	if err := c.Check(t.Context()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSampleBlocks_EndsAtTheLimitWithoutDuplicates(t *testing.T) {
	for _, limit := range []uint64{2, 5, 16, 1000} {
		samples := sampleBlocks(limit)
		if samples[len(samples)-1] != limit {
			t.Errorf("limit %d: samples %v do not end at the limit", limit, samples)
		}
		for i := 1; i < len(samples); i++ {
			if samples[i] <= samples[i-1] {
				t.Errorf("limit %d: samples %v are not strictly increasing", limit, samples)
			}
		}
	}
}
