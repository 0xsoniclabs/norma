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
	"encoding/json"
	"fmt"
	"testing"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/rpc"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// receiptsNetwork mocks a network of nodes at block 2, each reporting the
// given GasUsed for the receipts of every block, or no receipts for nil. Each
// block is of the epoch of its number.
func receiptsNetwork(t *testing.T, gasUsed map[string][]uint64) driver.Network {
	ctrl := gomock.NewController(t)
	nodes := []driver.Node{}
	for label, gas := range gasUsed {
		client := rpc.NewMockClient(ctrl)
		client.EXPECT().Close().AnyTimes()
		client.EXPECT().BlockNumber(gomock.Any()).AnyTimes().Return(uint64(2), nil)
		client.EXPECT().Call(gomock.Any(), "eth_getBlockReceipts", gomock.Any()).AnyTimes().
			DoAndReturn(func(result any, _ string, _ ...any) error {
				if gas == nil {
					return json.Unmarshal([]byte("null"), result)
				}
				receipts := []map[string]string{}
				for _, g := range gas {
					receipts = append(receipts, map[string]string{"gasUsed": hexutil.EncodeUint64(g)})
				}
				encoded, err := json.Marshal(receipts)
				if err != nil {
					return err
				}
				return json.Unmarshal(encoded, result)
			})
		client.EXPECT().Call(gomock.Any(), "eth_getBlockByNumber", gomock.Any()).AnyTimes().
			DoAndReturn(func(result any, _ string, args ...any) error {
				header := fmt.Sprintf(`{"epoch":%q}`, args[0])
				return json.Unmarshal([]byte(header), result)
			})

		node := driver.NewMockNode(ctrl)
		node.EXPECT().GetLabel().AnyTimes().Return(label)
		node.EXPECT().DialRpc(gomock.Any()).AnyTimes().Return(client, nil)
		nodes = append(nodes, node)
	}
	net := driver.NewMockNetwork(ctrl)
	net.EXPECT().GetActiveNodes().AnyTimes().Return(nodes)
	return net
}

func TestReceiptGasUsed_Check(t *testing.T) {
	tests := map[string]struct {
		gasUsed map[string][]uint64
		// exportEpoch is 2 unless set, so the export holds block 1.
		exportEpoch uint64
		wantError   string
	}{
		"amended corrects the leak": {
			gasUsed: map[string][]uint64{"a1": {21_000}, "a2": {21_000}, "l1": {50_090}},
		},
		"amended nodes disagree": {
			gasUsed:   map[string][]uint64{"a1": {21_000}, "a2": {50_090}, "l1": {50_090}},
			wantError: "receipts of block 1 report GasUsed",
		},
		"no leak corrected": {
			gasUsed:   map[string][]uint64{"a1": {21_000}, "a2": {21_000}, "l1": {21_000}},
			wantError: "no receipt in a block before epoch 2, up to block 2",
		},
		"leak corrected only from the export epoch on": {
			gasUsed:     map[string][]uint64{"a1": {21_000}, "a2": {21_000}, "l1": {50_090}},
			exportEpoch: 1,
			wantError:   "no receipt in a block before epoch 1, up to block 2",
		},
		"leaked receipts missing": {
			gasUsed:   map[string][]uint64{"a1": {21_000}, "a2": {21_000}, "l1": nil},
			wantError: "failed to get receipts of block 1 from node l1; block not found",
		},
		"blocks without receipts": {
			gasUsed:   map[string][]uint64{"a1": {}, "a2": {}, "l1": {}},
			wantError: "no receipt in a block before epoch 2, up to block 2",
		},
		"receipt counts differ": {
			gasUsed:   map[string][]uint64{"a1": {21_000}, "a2": {21_000}, "l1": {50_090, 21_000}},
			wantError: "block 1 has 1 receipts on [a1 a2] but 2 on [l1]",
		},
		"node not active": {
			gasUsed:   map[string][]uint64{"a1": {21_000}, "l1": {50_090}},
			wantError: "node a2 is not active",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			exportEpoch := test.exportEpoch
			if exportEpoch == 0 {
				exportEpoch = 2
			}
			checker := (&receiptGasUsedChecker{net: receiptsNetwork(t, test.gasUsed)}).
				Configure(CheckerConfig{"amended": []string{"a1", "a2"}, "leaked": []string{"l1"}, "exportEpoch": exportEpoch})
			err := checker.Check(t.Context())
			if test.wantError == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, test.wantError)
		})
	}
}
