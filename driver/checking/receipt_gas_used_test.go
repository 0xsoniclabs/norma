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
// given GasUsed for the single receipt of every block.
func receiptsNetwork(t *testing.T, gasUsed map[string]uint64) driver.Network {
	ctrl := gomock.NewController(t)
	nodes := []driver.Node{}
	for label, gas := range gasUsed {
		client := rpc.NewMockClient(ctrl)
		client.EXPECT().Close().AnyTimes()
		client.EXPECT().BlockNumber(gomock.Any()).AnyTimes().Return(uint64(2), nil)
		client.EXPECT().Call(gomock.Any(), "eth_getBlockReceipts", gomock.Any()).AnyTimes().
			DoAndReturn(func(result any, _ string, _ ...any) error {
				receipts := fmt.Sprintf(`[{"gasUsed":%q}]`, hexutil.EncodeUint64(gas))
				return json.Unmarshal([]byte(receipts), result)
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
		gasUsed   map[string]uint64
		wantError string
	}{
		"amended corrects the leak": {
			gasUsed: map[string]uint64{"a1": 21_000, "a2": 21_000, "l1": 50_090},
		},
		"amended nodes disagree": {
			gasUsed:   map[string]uint64{"a1": 21_000, "a2": 50_090, "l1": 50_090},
			wantError: "receipts of block 1 report GasUsed",
		},
		"no leak corrected": {
			gasUsed:   map[string]uint64{"a1": 21_000, "a2": 21_000, "l1": 21_000},
			wantError: "no receipt up to block 2",
		},
		"node not active": {
			gasUsed:   map[string]uint64{"a1": 21_000, "l1": 50_090},
			wantError: "node a2 is not active",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			checker := (&receiptGasUsedChecker{net: receiptsNetwork(t, test.gasUsed)}).
				Configure(CheckerConfig{"amended": []string{"a1", "a2"}, "leaked": []string{"l1"}})
			err := checker.Check(t.Context())
			if test.wantError == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, test.wantError)
		})
	}
}
