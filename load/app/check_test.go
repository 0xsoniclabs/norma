// Copyright 2026 Fantom Foundation
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

package app

import (
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// callWithGasLimit builds a call of the given outcome whose transaction has the
// given gas limit.
func callWithGasLimit(outcome Outcome, gasLimit uint64) Call {
	to := common.Address{1}
	return Call{
		Tx:      types.NewTx(&types.DynamicFeeTx{Gas: gasLimit, To: &to}),
		Outcome: outcome,
	}
}

func receipt(status uint64, gasUsed uint64) *types.Receipt {
	return &types.Receipt{
		Status:      status,
		GasUsed:     gasUsed,
		BlockNumber: big.NewInt(7),
	}
}

func TestCheckOutcome_AcceptsMatchingOutcomes(t *testing.T) {
	tests := map[string]struct {
		call    Call
		receipt *types.Receipt
	}{
		"successful transaction": {
			call:    callWithGasLimit(Success, 100_000),
			receipt: receipt(types.ReceiptStatusSuccessful, 50_000),
		},
		"reverted transaction refunds the unused gas": {
			call:    callWithGasLimit(Reverted, 100_000),
			receipt: receipt(types.ReceiptStatusFailed, 30_000),
		},
		"failed transaction consumes the full limit": {
			call:    callWithGasLimit(Failed, 100_000),
			receipt: receipt(types.ReceiptStatusFailed, 100_000),
		},
		"rejected transaction has no receipt": {
			call:    callWithGasLimit(Rejected, 100_000),
			receipt: nil,
		},
		"indeterminate transaction without a receipt": {
			call:    callWithGasLimit(Indeterminate, 100_000),
			receipt: nil,
		},
		"indeterminate transaction with a receipt": {
			call:    callWithGasLimit(Indeterminate, 100_000),
			receipt: receipt(types.ReceiptStatusFailed, 100_000),
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if err := CheckOutcome(nil, test.call, test.receipt); err != nil {
				t.Errorf("expected the outcome to be accepted, got %v", err)
			}
		})
	}
}

func TestCheckOutcome_RejectsDivergingOutcomes(t *testing.T) {
	tests := map[string]struct {
		call    Call
		receipt *types.Receipt
	}{
		"successful transaction was aborted": {
			call:    callWithGasLimit(Success, 100_000),
			receipt: receipt(types.ReceiptStatusFailed, 100_000),
		},
		"successful transaction never reached a block": {
			call:    callWithGasLimit(Success, 100_000),
			receipt: nil,
		},
		"reverting transaction succeeded": {
			call:    callWithGasLimit(Reverted, 100_000),
			receipt: receipt(types.ReceiptStatusSuccessful, 30_000),
		},
		"reverting transaction consumed all its gas": {
			call:    callWithGasLimit(Reverted, 100_000),
			receipt: receipt(types.ReceiptStatusFailed, 100_000),
		},
		"failing transaction succeeded": {
			call:    callWithGasLimit(Failed, 100_000),
			receipt: receipt(types.ReceiptStatusSuccessful, 100_000),
		},
		"failing transaction reverted instead": {
			call:    callWithGasLimit(Failed, 100_000),
			receipt: receipt(types.ReceiptStatusFailed, 30_000),
		},
		"failing transaction never reached a block": {
			call:    callWithGasLimit(Failed, 100_000),
			receipt: nil,
		},
		"rejected transaction was included": {
			call:    callWithGasLimit(Rejected, 100_000),
			receipt: receipt(types.ReceiptStatusSuccessful, 30_000),
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if err := CheckOutcome(nil, test.call, test.receipt); err == nil {
				t.Error("expected the diverging outcome to be reported")
			}
		})
	}
}

func TestRevertReason_DecodesRevertData(t *testing.T) {
	tests := map[string]struct {
		data string
		want string
	}{
		// Error("some reason")
		"string reason": {
			data: "0x08c379a0" +
				"0000000000000000000000000000000000000000000000000000000000000020" +
				"000000000000000000000000000000000000000000000000000000000000000b" +
				"736f6d6520726561736f6e00000000000000000000000000000000000000000000",
			want: "some reason",
		},
		// Panic(0x11)
		"arithmetic panic": {
			data: "0x4e487b71" +
				"0000000000000000000000000000000000000000000000000000000000000011",
			want: "arithmetic underflow or overflow",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got, ok := revertReason(&revertError{data: test.data})
			if !ok {
				t.Fatal("expected the revert data to be decoded")
			}
			if got != test.want {
				t.Errorf("expected reason %q, got %q", test.want, got)
			}
		})
	}
}

func TestRevertReason_IgnoresErrorsWithoutRevertData(t *testing.T) {
	if _, ok := revertReason(errors.New("connection refused")); ok {
		t.Error("expected an error without revert data to be reported as undecodable")
	}
	if _, ok := revertReason(&revertError{data: "not hex"}); ok {
		t.Error("expected malformed revert data to be reported as undecodable")
	}
}

// revertError carries revert data the way the RPC client reports it.
type revertError struct {
	data string
}

func (e *revertError) Error() string          { return "execution reverted" }
func (e *revertError) ErrorData() interface{} { return e.data }
