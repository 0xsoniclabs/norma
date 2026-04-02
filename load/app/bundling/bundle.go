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

// Package bundling provides helpers for creating Sonic bundled transactions
// (Brio hard-fork). A bundle is a set of transactions that are executed
// atomically within the same block, wrapped in a single envelope transaction
// sent to BundleProcessor.
//
// Typical usage:
//
//	signer := types.LatestSignerForChainID(chainId)
//	envelope, err := bundling.NewBuilder(signer).
//	    SetEarliest(blockNumber).
//	    SetLatest(blockNumber + 100).
//	    With(
//	        bundling.Step(key1, &types.DynamicFeeTx{...}),
//	        bundling.Step(key2, &types.DynamicFeeTx{...}),
//	    ).
//	    Build()
//	// envelope is a *types.Transaction ready to send via eth_sendRawTransaction
package bundling

import (
	"bytes"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
)

var (
	// BundleOnly is placed in the access list of every bundled transaction to
	// mark it as bundle-only (must not be included in a block standalone).
	BundleOnly = common.HexToAddress("0x00000000000000000000000000000000000B0D1E")

	// BundleProcessor is the address to which the envelope transaction is sent.
	// Its data field carries the RLP-encoded bundle.
	BundleProcessor = common.HexToAddress("0x00000000000000000000000000000000B0D1EADD")

	// MaxBlockRange is the maximum allowed width of the validity block range.
	MaxBlockRange = uint64(1024)
)

// ExecutionFlags controls how failed or invalid transactions within a bundle
// are handled. The zero value is equivalent to EF_AllOf.
type ExecutionFlags uint8

const (
	EF_AllOf           ExecutionFlags = 0b000 // revert whole bundle on first failure (default)
	EF_TolerateInvalid ExecutionFlags = 0b001 // treat invalid txs as successful
	EF_TolerateFailed  ExecutionFlags = 0b010 // treat reverted txs as successful
	EF_OneOf           ExecutionFlags = 0b100 // stop after first successful tx
)

// blockRange is a closed interval [Earliest, Latest] of block numbers.
type blockRange struct {
	Earliest uint64
	Latest   uint64
}

// executionStep is one entry in the execution plan: sender address and signing
// hash of the transaction (computed before the BundleOnly marker is added).
type executionStep struct {
	From common.Address
	Hash common.Hash
}

// executionPlan is the agreed-upon description of a bundle, hashed and stored
// as a storage key in the BundleOnly access-list entry of every inner tx.
type executionPlan struct {
	Steps []executionStep
	Flags ExecutionFlags
	Range blockRange
}

func (e *executionPlan) hash() common.Hash {
	hasher := crypto.NewKeccakState()
	_ = rlp.Encode(hasher, e)
	return common.BytesToHash(hasher.Sum(nil))
}

// transactionBundle is the in-memory bundle before it is wrapped in an
// envelope transaction.
type transactionBundle struct {
	Transactions types.Transactions
	Flags        ExecutionFlags
	Range        blockRange
}

// --- encoding ---

const bundleEncodingVersion byte = 1

type bundleEncodingV1 struct {
	Bundle   types.Transactions
	Flags    ExecutionFlags
	Earliest uint64
	Latest   uint64
}

func (tb *transactionBundle) encode() []byte {
	buf := bytes.Buffer{}
	_ = rlp.Encode(&buf, bundleEncodingVersion)
	_ = rlp.Encode(&buf, bundleEncodingV1{
		tb.Transactions,
		tb.Flags,
		tb.Range.Earliest,
		tb.Range.Latest,
	})
	return buf.Bytes()
}
