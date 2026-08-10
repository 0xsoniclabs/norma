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

package txmon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/0xsoniclabs/norma/driver/monitoring"
	"github.com/0xsoniclabs/norma/driver/rpc"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// The responses below are the ones a Sonic client sends; the parsing of these
// shapes is what connects this package to the client, and a mismatch would show
// up as metrics that are quietly empty.

func TestBlockTransactions_ReadsTheTransactionsAndTheirGasFromTheReceipts(t *testing.T) {
	require := require.New(t)
	client := newFakeClient(t, map[string]string{
		"eth_getBlockReceipts": `[
			{
				"blockNumber": "0x2a",
				"transactionHash": "0x1111111111111111111111111111111111111111111111111111111111111111",
				"gasUsed": "0x5208",
				"status": "0x1"
			},
			{
				"blockNumber": "0x2a",
				"transactionHash": "0x2222222222222222222222222222222222222222222222222222222222222222",
				"gasUsed": "0x6d90",
				"status": "0x1"
			}
		]`,
	})

	txs, err := blockTransactions(client, 42)
	require.NoError(err)
	require.Equal([]IncludedTransaction{
		{
			Hash:    common.HexToHash("0x1111111111111111111111111111111111111111111111111111111111111111"),
			GasUsed: 21_000,
		},
		{
			Hash:    common.HexToHash("0x2222222222222222222222222222222222222222222222222222222222222222"),
			GasUsed: 28_048,
		},
	}, txs)
	require.Equal([]any{"0x2a"}, client.args("eth_getBlockReceipts"),
		"the receipts are requested for the block as a whole")
}

func TestBlockTransactions_ReportsAnUnknownBlock(t *testing.T) {
	client := newFakeClient(t, map[string]string{"eth_getBlockReceipts": `null`})
	_, err := blockTransactions(client, 42)
	require.ErrorContains(t, err, "not found")
}

func TestBlockTransactions_AcceptsAnEmptyBlock(t *testing.T) {
	client := newFakeClient(t, map[string]string{"eth_getBlockReceipts": `[]`})
	txs, err := blockTransactions(client, 42)
	require.NoError(t, err)
	require.Empty(t, txs)
}

func TestBlockGasLimit_ReadsTheLimitFromTheHeader(t *testing.T) {
	require := require.New(t)
	client := newFakeClient(t, map[string]string{
		"eth_getBlockByNumber": `{
			"number": "0x2a",
			"gasUsed": "0x5208",
			"gasLimit": "0x12a05f200"
		}`,
	})

	limit, err := blockGasLimit(client, 42)
	require.NoError(err)
	require.Equal(uint64(5_000_000_000), limit)
	require.Equal([]any{"0x2a", false}, client.args("eth_getBlockByNumber"),
		"the header is enough, the transactions of the block are read from the receipts")
}

func TestBlockGasLimit_ReportsAnUnknownBlock(t *testing.T) {
	client := newFakeClient(t, map[string]string{"eth_getBlockByNumber": `null`})
	_, err := blockGasLimit(client, 42)
	require.ErrorContains(t, err, "not found")
}

func TestEventPayload_ReadsTheTransactionsAndParentsOfAnEvent(t *testing.T) {
	require := require.New(t)
	client := newFakeClient(t, map[string]string{
		"dag_getEventPayload": `{
			"epoch": "0x5",
			"creator": "0x1",
			"creationTime": "0x17b1f0e3a0000000",
			"parents": ["0x3333333333333333333333333333333333333333333333333333333333333333"],
			"transactions": ["0x4444444444444444444444444444444444444444444444444444444444444444"]
		}`,
	})

	id := common.HexToHash("0x55")
	event, err := eventPayload(client, id)
	require.NoError(err)
	require.Equal(int64(0x17b1f0e3a0000000), int64(event.CreationTime),
		"the creation time is nanoseconds since the Unix epoch")
	require.Len(event.Parents, 1)
	require.Len(event.Transactions, 1)
	require.Equal([]any{id.Hex(), true}, client.args("dag_getEventPayload"),
		"the payload must be requested with its transactions")
}

func TestEmissionObserver_MarksTheTransactionsOfEveryEventOnce(t *testing.T) {
	require := require.New(t)

	// A DAG of three events, the head referring to two parents, one of which is
	// referred to twice.
	head := common.HexToHash("0xaa")
	left := common.HexToHash("0xbb")
	right := common.HexToHash("0xcc")
	txs := []common.Hash{{0x01}, {0x02}, {0x03}}
	events := map[common.Hash]string{
		head: fmt.Sprintf(`{"creationTime": "0x1", "parents": [%q, %q],
			"transactions": [%q]}`, left, right, txs[0]),
		left: fmt.Sprintf(
			`{"creationTime": "0x1", "parents": [%q], "transactions": [%q]}`, right, txs[1]),
		right: fmt.Sprintf(
			`{"creationTime": "0x1", "parents": [], "transactions": [%q]}`, txs[2]),
	}

	reads := map[common.Hash]int{}
	client := newFakeClient(t, nil)
	client.handler = func(method string, args ...any) (string, error) {
		switch method {
		case "eth_currentEpoch":
			return `"0x5"`, nil
		case "dag_getHeads":
			require.Equal("0x5", args[0], "the heads of the current epoch are requested")
			return fmt.Sprintf(`[%q]`, head), nil
		case "dag_getEventPayload":
			id := common.HexToHash(args[0].(string))
			reads[id]++
			return events[id], nil
		}
		return "", fmt.Errorf("unexpected method %s", method)
	}

	observer := newEmissionObserver(NewTracker(), nil)
	require.NoError(observer.poll(t.Context(), client))
	require.Equal(3, observer.events)
	for id, count := range reads {
		require.Equal(1, count, "event %s was read more than once", id)
	}

	// A second poll finds nothing new, so no event is read again.
	require.NoError(observer.poll(t.Context(), client))
	require.Equal(3, observer.events)
	for id, count := range reads {
		require.Equal(1, count, "event %s was read again in a later poll", id)
	}
}

func TestEmissionObserver_MarksTransactionsWithTheEventsCreationTime(t *testing.T) {
	require := require.New(t)
	tracker := NewTracker()
	tx := newAccount(t, 1).transaction(t, 0)
	tracker.OnTransactionSubmitted(source("app", 0), tx, epoch, nil)

	emittedAt := epoch.Add(750 * time.Millisecond)
	head := common.HexToHash("0xaa")
	client := newFakeClient(t, nil)
	client.handler = func(method string, args ...any) (string, error) {
		switch method {
		case "eth_currentEpoch":
			return `"0x1"`, nil
		case "dag_getHeads":
			return fmt.Sprintf(`[%q]`, head), nil
		case "dag_getEventPayload":
			return fmt.Sprintf(
				`{"creationTime": %q, "parents": [], "transactions": [%q]}`,
				fmt.Sprintf("0x%x", emittedAt.UnixNano()), tx.Hash(),
			), nil
		}
		return "", fmt.Errorf("unexpected method %s", method)
	}

	observer := newEmissionObserver(tracker, nil)
	require.NoError(observer.poll(t.Context(), client))

	require.Equal(Counts{Emitted: 1}, tracker.Counts("app"))
	require.Equal(
		[]Sample{{At: epoch, Duration: 750 * time.Millisecond}},
		tracker.Samples("app", TimeToEmit),
	)
}

func TestEmissionObserver_ReportsAFailedRead(t *testing.T) {
	client := newFakeClient(t, nil)
	client.handler = func(string, ...any) (string, error) {
		return "", errors.New("the dag namespace is not enabled")
	}

	observer := newEmissionObserver(NewTracker(), nil)
	err := observer.poll(t.Context(), client)
	require.ErrorContains(t, err, "the dag namespace is not enabled")
}

func TestInclusionObserver_ReadsEveryBlockOnce(t *testing.T) {
	require := require.New(t)
	tracker := NewTracker()
	tx := newAccount(t, 1).transaction(t, 0)
	tracker.OnTransactionSubmitted(source("app", 0), tx, epoch, nil)

	includedAt := epoch.Add(2 * time.Second)
	reads := 0
	client := newFakeClient(t, nil)
	client.handler = func(method string, args ...any) (string, error) {
		if method == "eth_getBlockByNumber" {
			return `{"gasLimit": "0x12a05f200"}`, nil
		}
		require.Equal("eth_getBlockReceipts", method)
		reads++
		return fmt.Sprintf(
			`[{"transactionHash": %q, "gasUsed": "0x5208"}]`, tx.Hash()), nil
	}

	observer := newInclusionObserver(tracker, fixedClient{client})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	go observer.run(ctx)

	// Every node of the network announces the same block; it must be read once.
	block := monitoring.Block{Height: 7, Time: includedAt}
	observer.OnBlock("validator-0", block)
	observer.OnBlock("validator-1", block)

	require.Eventually(func() bool {
		return tracker.Counts("app").Included == 1
	}, 5*time.Second, 10*time.Millisecond)
	require.Equal(1, reads)
	require.Equal(
		[]Sample{{At: epoch, Duration: 2 * time.Second}},
		tracker.Samples("app", TimeToInclude),
	)
	require.Equal(
		[]BlockLimit{{Block: 7, GasLimit: 5_000_000_000}},
		tracker.BlockGasLimits(),
		"the limit of the block is recorded along with its transactions",
	)
}

func TestReadBlock_RetriesANodeThatIsBehind(t *testing.T) {
	require := require.New(t)

	attempts := 0
	client := newFakeClient(t, nil)
	client.handler = func(method string, args ...any) (string, error) {
		if method == "eth_getBlockByNumber" {
			return `{"gasLimit": "0x12a05f200"}`, nil
		}
		attempts++
		if attempts < blockReadAttempts {
			// The queried node has not processed the block yet.
			return `null`, nil
		}
		return fmt.Sprintf(
			`[{"transactionHash": %q, "gasUsed": "0x5208"}]`, common.Hash{0x01}), nil
	}

	contents, err := readBlock(t.Context(), client, 12)
	require.NoError(err)
	require.Len(contents.transactions, 1)
	require.Equal(uint64(5_000_000_000), contents.gasLimit)
	require.Equal(blockReadAttempts, attempts)
}

func TestReadBlock_GivesUpOnABlockThatStaysUnknown(t *testing.T) {
	client := newFakeClient(t, nil)
	client.handler = func(string, ...any) (string, error) { return `null`, nil }

	_, err := readBlock(t.Context(), client, 12)
	require.ErrorContains(t, err, "not found")
}

// fixedClient hands out the same client for every request.
type fixedClient struct {
	client rpc.Client
}

func (c fixedClient) dial(context.Context) (rpc.Client, error) {
	return c.client, nil
}

// fakeClient answers RPC calls with canned JSON, decoded into the result the
// caller passed - the same path the real client takes.
type fakeClient struct {
	*rpc.MockClient
	mu        sync.Mutex
	responses map[string]string
	handler   func(method string, args ...any) (string, error)
	seen      map[string][]any
}

func newFakeClient(t *testing.T, responses map[string]string) *fakeClient {
	client := &fakeClient{
		MockClient: rpc.NewMockClient(gomock.NewController(t)),
		responses:  responses,
		seen:       map[string][]any{},
	}
	client.MockClient.EXPECT().
		Call(gomock.Any(), gomock.Any(), gomock.Any()).
		AnyTimes().
		DoAndReturn(client.call)
	client.MockClient.EXPECT().Close().AnyTimes()
	return client
}

func (c *fakeClient) call(result any, method string, args ...any) error {
	c.mu.Lock()
	c.seen[method] = args
	handler, response := c.handler, c.responses[method]
	c.mu.Unlock()

	if handler != nil {
		var err error
		if response, err = handler(method, args...); err != nil {
			return err
		}
	}
	if response == "" {
		return fmt.Errorf("no response configured for %s", method)
	}
	return json.Unmarshal([]byte(response), result)
}

// args returns the arguments the given method was last called with.
func (c *fakeClient) args(method string) []any {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.seen[method]
}
