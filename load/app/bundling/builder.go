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

package bundling

import (
	"crypto/ecdsa"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

// Step pairs a private key with unsigned transaction data to be included as
// one step in a bundle. Supported types: *types.DynamicFeeTx, *types.AccessListTx.
func Step(key *ecdsa.PrivateKey, tx types.TxData) BundleStep {
	switch tx.(type) {
	case *types.DynamicFeeTx, *types.AccessListTx:
		return BundleStep{key: key, tx: tx}
	default:
		panic(fmt.Sprintf("bundling: unsupported TxData type %T (only DynamicFeeTx and AccessListTx are supported)", tx))
	}
}

// BundleStep is a (key, unsigned-tx-data) pair that becomes one signed
// transaction inside the bundle.
type BundleStep struct {
	key *ecdsa.PrivateKey
	tx  types.TxData
}

// Builder constructs a bundle envelope transaction.
type Builder struct {
	signer   types.Signer
	flags    ExecutionFlags
	earliest *uint64
	latest   *uint64
	steps    []BundleStep
}

// NewBuilder returns a Builder that uses signer for all signing operations.
// The default execution flags are EF_AllOf.
func NewBuilder(signer types.Signer) *Builder {
	return &Builder{signer: signer, flags: EF_AllOf}
}

// SetFlags overrides the execution flags. Defaults to EF_AllOf if not called.
func (b *Builder) SetFlags(flags ExecutionFlags) *Builder {
	b.flags = flags
	return b
}

// SetEarliest sets the first block number in which the bundle is valid.
// Defaults to 0 if not set.
func (b *Builder) SetEarliest(earliest uint64) *Builder {
	b.earliest = &earliest
	return b
}

// SetLatest sets the last block number (inclusive) in which the bundle is valid.
// Defaults to earliest+MaxBlockRange-1 if not set.
func (b *Builder) SetLatest(latest uint64) *Builder {
	b.latest = &latest
	return b
}

// With appends one or more steps to the bundle.
func (b *Builder) With(steps ...BundleStep) *Builder {
	b.steps = append(b.steps, steps...)
	return b
}

// Build returns a signed envelope transaction ready to be sent via
// eth_sendRawTransaction. The envelope carries the RLP-encoded bundle in its
// data field and is addressed to BundleProcessor.
func (b *Builder) Build() (*types.Transaction, error) {
	earliest := uint64(0)
	latest := MaxBlockRange - 1
	if b.earliest != nil {
		earliest = *b.earliest
		latest = earliest + MaxBlockRange - 1
	}
	if b.latest != nil {
		latest = *b.latest
	}

	signer := b.signer
	if signer == nil {
		signer = types.LatestSignerForChainID(big.NewInt(1))
	}

	// 1. Build the execution plan from the unsigned tx data (before adding the marker).
	plan := executionPlan{
		Steps: make([]executionStep, len(b.steps)),
		Flags: b.flags,
		Range: blockRange{Earliest: earliest, Latest: latest},
	}
	for i, step := range b.steps {
		plan.Steps[i] = executionStep{
			From: crypto.PubkeyToAddress(step.key.PublicKey),
			Hash: signer.Hash(types.NewTx(step.tx)),
		}
	}

	// 2. Annotate each tx with the BundleOnly marker + plan hash.
	planHash := plan.hash()
	marker := types.AccessTuple{
		Address:     BundleOnly,
		StorageKeys: []common.Hash{planHash},
	}
	for _, step := range b.steps {
		switch data := step.tx.(type) {
		case *types.DynamicFeeTx:
			data.AccessList = append(data.AccessList, marker)
		case *types.AccessListTx:
			data.AccessList = append(data.AccessList, marker)
		default:
			return nil, fmt.Errorf("bundling: unsupported TxData type %T", step.tx)
		}
	}

	// 3. Sign the annotated transactions.
	txs := make([]*types.Transaction, len(b.steps))
	for i, step := range b.steps {
		signed, err := types.SignNewTx(step.key, signer, step.tx)
		if err != nil {
			return nil, fmt.Errorf("bundling: failed to sign step %d: %w", i, err)
		}
		txs[i] = signed
	}

	tb := &transactionBundle{
		Transactions: txs,
		Flags:        b.flags,
		Range:        blockRange{Earliest: earliest, Latest: latest},
	}

	// 4. Wrap in an envelope transaction signed by a throwaway key.
	envelopeKey, err := crypto.GenerateKey()
	if err != nil {
		return nil, fmt.Errorf("bundling: failed to generate envelope key: %w", err)
	}
	return newEnvelope(signer, envelopeKey, tb)
}

// newEnvelope wraps a bundle in a signed AccessListTx addressed to BundleProcessor.
func newEnvelope(signer types.Signer, key *ecdsa.PrivateKey, tb *transactionBundle) (*types.Transaction, error) {
	payload := tb.encode()

	intrinsic, err := core.IntrinsicGas(
		payload,
		nil,   // no access list on envelope
		nil,   // no auth list
		false, // not a contract creation
		true,  // homestead
		true,  // istanbul (EIP-2028)
		true,  // shanghai (EIP-3860)
	)
	if err != nil {
		return nil, fmt.Errorf("bundling: failed to compute intrinsic gas: %w", err)
	}

	floorDataGas, err := core.FloorDataGas(payload)
	if err != nil {
		return nil, fmt.Errorf("bundling: failed to compute floor data gas: %w", err)
	}

	txGasSum := uint64(0)
	for _, tx := range tb.Transactions {
		txGasSum += tx.Gas()
	}

	gasLimit := max(intrinsic, floorDataGas, txGasSum)

	chainId := big.NewInt(1)
	if len(tb.Transactions) > 0 {
		chainId = tb.Transactions[0].ChainId()
	}

	return types.SignNewTx(key, signer, &types.AccessListTx{
		ChainID: chainId,
		To:      &BundleProcessor,
		Data:    payload,
		Gas:     gasLimit,
	})
}
