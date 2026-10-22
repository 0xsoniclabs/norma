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
	"fmt"
	"slices"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/monitoring"
	"github.com/0xsoniclabs/norma/driver/rpc"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

func init() {
	RegisterNetworkCheck("receiptGasUsed",
		func(net driver.Network, monitor *monitoring.Monitor) Checker {
			return &receiptGasUsedChecker{net: net}
		})
}

// receiptGasUsedChecker checks the GasUsed nodes report for receipts after a
// failed bare-root bundle, whose gas leaks into their cumulative gas. The
// amended nodes, which correct the leak, must agree on every receipt, as must
// the leaked nodes, which do not; and the two groups must disagree on at least
// one in a block before the export epoch, the epoch the exporter of a g-file
// stopped at, so a leak was corrected in the g-file rather than in blocks the
// importers executed themselves. Blocks are compared up to the lowest head of
// all of them.
type receiptGasUsedChecker struct {
	net             driver.Network
	amended, leaked []string
	exportEpoch     uint64
}

func (c *receiptGasUsedChecker) Configure(config CheckerConfig) Checker {
	amended, _ := config["amended"].([]string)
	leaked, _ := config["leaked"].([]string)
	exportEpoch, _ := config["exportEpoch"].(uint64)
	return &receiptGasUsedChecker{net: c.net, amended: amended, leaked: leaked, exportEpoch: exportEpoch}
}

// labeledClient is the RPC client of the node with the given label.
type labeledClient struct {
	label  string
	client rpc.Client
}

func (c *receiptGasUsedChecker) Check(ctx context.Context) error {
	amended, err := c.dial(ctx, c.amended)
	defer closeClients(amended)
	if err != nil {
		return err
	}
	leaked, err := c.dial(ctx, c.leaked)
	defer closeClients(leaked)
	if err != nil {
		return err
	}

	limit, err := lowestHead(ctx, append(slices.Clone(amended), leaked...))
	if err != nil {
		return err
	}

	differ := false
	for block := uint64(1); block <= limit; block++ {
		want, err := commonGasUsed(amended, block)
		if err != nil {
			return err
		}
		got, err := commonGasUsed(leaked, block)
		if err != nil {
			return err
		}
		if len(want) != len(got) {
			return fmt.Errorf("block %d has %d receipts on %v but %d on %v",
				block, len(want), c.amended, len(got), c.leaked)
		}
		if !differ && !slices.Equal(want, got) {
			differ, err = c.exported(amended[0], block)
			if err != nil {
				return err
			}
		}
	}
	if !differ {
		return fmt.Errorf("no receipt in a block before epoch %d, up to block %d, reports a different GasUsed on %v than on %v",
			c.exportEpoch, limit, c.amended, c.leaked)
	}
	return nil
}

// dial returns the RPC clients of the active nodes with the given labels, and
// those dialled before an error, so the caller can close them.
func (c *receiptGasUsedChecker) dial(ctx context.Context, labels []string) ([]labeledClient, error) {
	if len(labels) == 0 {
		return nil, fmt.Errorf("receiptGasUsed requires amended and leaked nodes")
	}
	nodes := map[string]driver.Node{}
	for _, n := range c.net.GetActiveNodes() {
		nodes[n.GetLabel()] = n
	}
	clients := make([]labeledClient, 0, len(labels))
	for _, label := range labels {
		n, ok := nodes[label]
		if !ok {
			return clients, fmt.Errorf("node %s is not active", label)
		}
		client, err := n.DialRpc(ctx)
		if err != nil {
			return clients, fmt.Errorf("failed to dial RPC for node %s; %w", label, err)
		}
		clients = append(clients, labeledClient{label, client})
	}
	return clients, nil
}

func closeClients(clients []labeledClient) {
	for _, c := range clients {
		c.client.Close()
	}
}

func lowestHead(ctx context.Context, clients []labeledClient) (uint64, error) {
	limit := uint64(0)
	for i, c := range clients {
		head, err := c.client.BlockNumber(ctx)
		if err != nil {
			return 0, fmt.Errorf("failed to get block height of node %s; %w", c.label, err)
		}
		if i == 0 || head < limit {
			limit = head
		}
	}
	return limit, nil
}

// commonGasUsed returns the GasUsed of the receipts of a block, which all
// clients must report alike.
func commonGasUsed(clients []labeledClient, block uint64) ([]uint64, error) {
	var common []uint64
	for i, c := range clients {
		gasUsed, err := getGasUsed(c.client, block)
		if err != nil {
			return nil, fmt.Errorf("failed to get receipts of block %d from node %s; %w", block, c.label, err)
		}
		if i == 0 {
			common = gasUsed
			continue
		}
		if !slices.Equal(common, gasUsed) {
			return nil, fmt.Errorf("receipts of block %d report GasUsed %v on node %s but %v on node %s",
				block, common, clients[0].label, gasUsed, c.label)
		}
	}
	return common, nil
}

// exported reports whether the block is of an epoch before the export epoch.
func (c *receiptGasUsedChecker) exported(node labeledClient, block uint64) (bool, error) {
	var header *struct {
		Epoch hexutil.Uint64 `json:"epoch"`
	}
	err := node.client.Call(&header, "eth_getBlockByNumber", hexutil.EncodeUint64(block), false)
	if err == nil && header == nil {
		err = fmt.Errorf("block not found")
	}
	if err != nil {
		return false, fmt.Errorf("failed to get block %d from node %s; %w", block, node.label, err)
	}
	return uint64(header.Epoch) < c.exportEpoch, nil
}

func getGasUsed(client rpc.Client, block uint64) ([]uint64, error) {
	var receipts []struct {
		GasUsed hexutil.Uint64 `json:"gasUsed"`
	}
	if err := client.Call(&receipts, "eth_getBlockReceipts", hexutil.EncodeUint64(block)); err != nil {
		return nil, err
	}
	gasUsed := make([]uint64, len(receipts))
	for i, r := range receipts {
		gasUsed[i] = uint64(r.GasUsed)
	}
	return gasUsed, nil
}
