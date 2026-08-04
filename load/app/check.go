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
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
)

// CheckOutcome verifies that the given call produced the outcome it was created
// for. The receipt is the one the network produced for the call, or nil if the
// transaction never reached a block.
//
// Reverted and Failed are told apart by the gas the transaction consumed: a
// REVERT refunds the gas the call did not use, while an abort without revert
// data consumes the full limit.
func CheckOutcome(ctxt AppContext, call Call, receipt *types.Receipt) error {
	hash := call.Tx.Hash()

	if call.Outcome == Indeterminate {
		return nil
	}

	if call.Outcome == Rejected {
		if receipt != nil {
			return fmt.Errorf(
				"transaction %v was expected to be rejected but was included in block %v",
				hash, receipt.BlockNumber)
		}
		return nil
	}

	if receipt == nil {
		return fmt.Errorf("transaction %v was expected to be %s but never reached a block",
			hash, call.Outcome)
	}

	switch call.Outcome {
	case Success:
		if receipt.Status != types.ReceiptStatusSuccessful {
			return fmt.Errorf("transaction %v was expected to succeed but was aborted after %d of %d gas",
				hash, receipt.GasUsed, call.Tx.Gas())
		}
		return nil

	case Failed:
		if receipt.Status == types.ReceiptStatusSuccessful {
			return fmt.Errorf("transaction %v was expected to run out of gas but succeeded", hash)
		}
		if receipt.GasUsed < call.Tx.Gas() {
			return fmt.Errorf(
				"transaction %v was expected to run out of gas but consumed only %d of %d gas, which means it reverted",
				hash, receipt.GasUsed, call.Tx.Gas())
		}
		return nil

	case Reverted:
		if receipt.Status == types.ReceiptStatusSuccessful {
			return fmt.Errorf("transaction %v was expected to revert but succeeded", hash)
		}
		if receipt.GasUsed >= call.Tx.Gas() {
			return fmt.Errorf(
				"transaction %v was expected to revert but consumed all %d gas, which means it was aborted without reverting",
				hash, call.Tx.Gas())
		}
		if call.Reason == "" {
			return nil
		}
		return checkRevertReason(ctxt, call, receipt)
	}

	return fmt.Errorf("transaction %v has unknown expected outcome %v", hash, call.Outcome)
}

// checkRevertReason replays the call against the state preceding the block it was
// included in and compares the revert reason the node reports with the expected
// one. The state of the parent block does not always reproduce the revert - other
// transactions of the same block may have changed the state the call depends on -
// so a replay that does not revert is reported as inconclusive rather than as a
// mismatch.
func checkRevertReason(ctxt AppContext, call Call, receipt *types.Receipt) error {
	tx := call.Tx
	sender, err := types.Sender(types.LatestSignerForChainID(tx.ChainId()), tx)
	if err != nil {
		return fmt.Errorf("failed to recover the sender of %v; %w", tx.Hash(), err)
	}

	parent := new(big.Int).Sub(receipt.BlockNumber, big.NewInt(1))
	_, err = ctxt.GetClient().CallContract(context.Background(), ethereum.CallMsg{
		From:     sender,
		To:       tx.To(),
		Gas:      tx.Gas(),
		GasPrice: tx.GasFeeCap(),
		Value:    tx.Value(),
		Data:     tx.Data(),
	}, parent)

	if err == nil {
		slog.Debug("revert reason check inconclusive: replay did not revert",
			"tx", tx.Hash(), "expected", call.Reason)
		return nil
	}

	reason, ok := revertReason(err)
	if !ok {
		slog.Debug("revert reason check inconclusive: replay returned no revert data",
			"tx", tx.Hash(), "expected", call.Reason, "error", err)
		return nil
	}
	if !strings.Contains(reason, call.Reason) {
		return fmt.Errorf("transaction %v reverted with %q, expected %q",
			tx.Hash(), reason, call.Reason)
	}
	return nil
}

// revertReason decodes the revert data an RPC error carries into the reason
// string or panic description it encodes. It reports false if the error does not
// carry revert data.
func revertReason(err error) (string, bool) {
	var withData interface{ ErrorData() interface{} }
	if !errors.As(err, &withData) {
		return "", false
	}
	encoded, ok := withData.ErrorData().(string)
	if !ok {
		return "", false
	}
	data, decodeErr := hexutil.Decode(encoded)
	if decodeErr != nil {
		return "", false
	}
	reason, unpackErr := abi.UnpackRevert(data)
	if unpackErr != nil {
		return "", false
	}
	return reason, true
}
