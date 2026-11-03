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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/monitoring"
	"github.com/0xsoniclabs/norma/driver/rpc"
	"github.com/0xsoniclabs/sonic/opera/contracts/sfc"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

const (
	// historicalStateSamples is how many blocks, spread evenly up to the
	// common height, the account state is compared at.
	historicalStateSamples = 16
	// historicalStateAccounts caps the accounts compared at each sample.
	historicalStateAccounts = 16
	// historicalStateSlots is how many leading storage slots of each contract
	// are compared.
	historicalStateSlots = 3
	// unindexedLogsRange is the widest block range Sonic serves eth_getLogs
	// for without an address or topic filter.
	unindexedLogsRange = 100
)

func init() {
	RegisterNetworkCheck("historicalState",
		func(net driver.Network, monitor *monitoring.Monitor) Checker {
			return &historicalStateChecker{net: net}
		})
}

// historicalStateChecker checks that all nodes serve the same past: the logs
// of every block, read from the receipts and through the log index, and the
// balance, nonce, code and leading storage slots of the SFC, the contracts
// that emitted logs and the senders of sampled blocks, at those blocks. A node
// must also serve the same logs from its index as from its receipts. Nodes
// expected to fail are left out.
type historicalStateChecker struct {
	net driver.Network
}

// Configure returns itself since there is nothing to configure
func (c *historicalStateChecker) Configure(config CheckerConfig) Checker {
	return c
}

func (c *historicalStateChecker) Check(ctx context.Context) error {
	var nodes []driver.Node
	var clients []rpc.Client
	defer func() {
		for _, client := range clients {
			client.Close()
		}
	}()
	for _, n := range c.net.GetActiveNodes() {
		if n.IsExpectedFailure() {
			continue
		}
		client, err := n.DialRpc(ctx)
		if err != nil {
			return fmt.Errorf("failed to dial RPC for node %s; %w", n.GetLabel(), err)
		}
		nodes = append(nodes, n)
		clients = append(clients, client)
	}

	limit, err := commonHeight(ctx, nodes, clients)
	if err != nil {
		return err
	}

	queries, want, emitters, err := logQueries(ctx, nodes[0].GetLabel(), clients[0], limit)
	if err != nil {
		return err
	}
	samples := sampleBlocks(limit)
	senders, err := sampleSenders(ctx, clients[0], samples)
	if err != nil {
		return err
	}
	queries = append(queries, stateQueries(samples, emitters, senders)...)
	want = append(want, make([]json.RawMessage, len(queries)-len(want))...)
	slog.Info("comparing historical state", "up_to_block", limit, "queries", len(queries), "nodes", len(nodes))

	// ponytail: nodes are queried one after another, in parallel if the
	// check ever becomes the long pole of a scenario.
	for i, n := range nodes {
		for j, q := range queries {
			if i == 0 && want[j] != nil {
				continue
			}
			got, err := q.run(ctx, clients[i])
			if isTooManyResults(err) {
				// The reference node served the same range within the limit.
				return assertionf("nodes %s and %s disagree on %s: %s serves too many logs",
					nodes[0].GetLabel(), n.GetLabel(), q, n.GetLabel())
			}
			if err != nil {
				return fmt.Errorf("failed to query %s at node %s; %w", q, n.GetLabel(), err)
			}
			if i == 0 {
				want[j] = got
			} else if !bytes.Equal(got, want[j]) {
				return assertionf("nodes %s and %s disagree on %s: %.200s vs %.200s",
					nodes[0].GetLabel(), n.GetLabel(), q, want[j], got)
			}
		}
	}
	return nil
}

// historyQuery is one RPC call whose result every node must agree on.
type historyQuery struct {
	method string
	args   []any
}

func (q historyQuery) String() string {
	return fmt.Sprintf("%s%v", q.method, q.args)
}

// run returns the result of the query, with logs reduced to the fields of
// logEntry so that client versions serving them differently still compare
// equal.
func (q historyQuery) run(ctx context.Context, client rpc.Client) (json.RawMessage, error) {
	var result json.RawMessage
	if err := client.CallContext(ctx, &result, q.method, q.args...); err != nil {
		return nil, err
	}
	if q.method != "eth_getLogs" {
		return result, nil
	}
	var logs []logEntry
	if err := json.Unmarshal(result, &logs); err != nil {
		return nil, fmt.Errorf("failed to decode logs; %w", err)
	}
	return json.Marshal(logs)
}

func logsQuery(from, to uint64, addresses []common.Address) historyQuery {
	filter := map[string]any{
		"fromBlock": hexutil.EncodeUint64(from),
		"toBlock":   hexutil.EncodeUint64(to),
	}
	if len(addresses) > 0 {
		filter["address"] = addresses
	}
	return historyQuery{"eth_getLogs", []any{filter}}
}

// logEntry is a log as eth_getLogs serves it, without the block timestamp,
// which not every client version and query path fills in.
type logEntry struct {
	Address          common.Address  `json:"address"`
	Topics           json.RawMessage `json:"topics"`
	Data             json.RawMessage `json:"data"`
	BlockNumber      json.RawMessage `json:"blockNumber"`
	TransactionHash  json.RawMessage `json:"transactionHash"`
	TransactionIndex json.RawMessage `json:"transactionIndex"`
	BlockHash        json.RawMessage `json:"blockHash"`
	LogIndex         json.RawMessage `json:"logIndex"`
	Removed          json.RawMessage `json:"removed"`
}

// logQueries splits the blocks up to limit into ranges the reference node
// serves the logs of, and returns a query reading each range from the receipts
// plus, for a range holding logs, one reading it through the log index, which
// the reference node must answer with the same logs. It also returns the
// reference node's result of each query and every address that emitted a log.
func logQueries(
	ctx context.Context, label string, reference rpc.Client, limit uint64,
) ([]historyQuery, []json.RawMessage, []common.Address, error) {
	var queries []historyQuery
	var results []json.RawMessage
	var emitters []common.Address
	width := uint64(unindexedLogsRange)
	for from := uint64(0); from <= limit; {
		to := min(from+width, limit)
		fromReceipts := logsQuery(from, to, nil)
		receipts, err := fromReceipts.run(ctx, reference)
		if isTooManyResults(err) && to > from {
			width = (to - from) / 2
			continue
		}
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to query %s at node %s; %w", fromReceipts, label, err)
		}
		queries = append(queries, fromReceipts)
		results = append(results, receipts)

		var receiptLogs []logEntry
		if err := json.Unmarshal(receipts, &receiptLogs); err != nil {
			return nil, nil, nil, err
		}
		var addresses []common.Address
		for _, l := range receiptLogs {
			if !slices.Contains(addresses, l.Address) {
				addresses = append(addresses, l.Address)
			}
		}
		if len(addresses) > 0 {
			fromIndex := logsQuery(from, to, addresses)
			index, err := fromIndex.run(ctx, reference)
			if isTooManyResults(err) {
				// Both paths share the limit, which the receipts stayed within.
				return nil, nil, nil, assertionf(
					"node %s serves too many logs from its log index for blocks %d-%d, "+
						"but %d from its receipts", label, from, to, len(receiptLogs))
			}
			if err != nil {
				return nil, nil, nil, fmt.Errorf("failed to query %s at node %s; %w", fromIndex, label, err)
			}
			if !bytes.Equal(index, receipts) {
				var indexLogs []logEntry
				_ = json.Unmarshal(index, &indexLogs)
				return nil, nil, nil, assertionf(
					"node %s serves %d logs from its log index for blocks %d-%d, "+
						"but %d different ones from its receipts",
					label, len(indexLogs), from, to, len(receiptLogs))
			}
			queries = append(queries, fromIndex)
			results = append(results, index)
		}
		for _, a := range addresses {
			if !slices.Contains(emitters, a) {
				emitters = append(emitters, a)
			}
		}

		from = to + 1
		width = unindexedLogsRange
	}
	return queries, results, emitters, nil
}

// isTooManyResults reports whether err is Sonic refusing a log query for
// matching more logs than its result limit.
func isTooManyResults(err error) bool {
	return err != nil && strings.Contains(err.Error(), "too many results")
}

// sampleBlocks spreads historicalStateSamples blocks evenly over 1..limit,
// ending at limit.
func sampleBlocks(limit uint64) []uint64 {
	var samples []uint64
	for k := uint64(1); k <= historicalStateSamples; k++ {
		block := max(limit*k/historicalStateSamples, 1)
		if !slices.Contains(samples, block) {
			samples = append(samples, block)
		}
	}
	return samples
}

// sampleSenders returns the senders of the transactions in the sampled blocks.
func sampleSenders(ctx context.Context, reference rpc.Client, samples []uint64) ([]common.Address, error) {
	var senders []common.Address
	for _, block := range samples {
		var result struct {
			Transactions []struct {
				From common.Address `json:"from"`
			} `json:"transactions"`
		}
		err := reference.CallContext(ctx, &result, "eth_getBlockByNumber", hexutil.EncodeUint64(block), true)
		if err != nil {
			return nil, fmt.Errorf("failed to get the transactions of block %d; %w", block, err)
		}
		for _, tx := range result.Transactions {
			if !slices.Contains(senders, tx.From) {
				senders = append(senders, tx.From)
			}
		}
	}
	return senders, nil
}

// stateQueries reads, at every sampled block, the balance, nonce and code of
// the SFC, the emitters and the senders, up to historicalStateAccounts of them
// in that order, and the leading storage slots of the SFC and the emitters.
func stateQueries(samples []uint64, emitters, senders []common.Address) []historyQuery {
	contracts := []common.Address{sfc.ContractAddress}
	for _, a := range emitters {
		if !slices.Contains(contracts, a) {
			contracts = append(contracts, a)
		}
	}
	contracts = contracts[:min(len(contracts), historicalStateAccounts)]
	accounts := slices.Clone(contracts)
	for _, a := range senders {
		if len(accounts) < historicalStateAccounts && !slices.Contains(accounts, a) {
			accounts = append(accounts, a)
		}
	}

	var queries []historyQuery
	for _, block := range samples {
		at := hexutil.EncodeUint64(block)
		for _, a := range accounts {
			queries = append(queries,
				historyQuery{"eth_getBalance", []any{a, at}},
				historyQuery{"eth_getTransactionCount", []any{a, at}},
				historyQuery{"eth_getCode", []any{a, at}},
			)
		}
		for _, a := range contracts {
			for slot := range uint64(historicalStateSlots) {
				queries = append(queries,
					historyQuery{"eth_getStorageAt", []any{a, hexutil.EncodeUint64(slot), at}})
			}
		}
	}
	return queries
}
