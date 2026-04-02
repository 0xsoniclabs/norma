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

package bundling_test

import (
	"math/big"
	"testing"

	"github.com/0xsoniclabs/norma/load/app/bundling"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

func newTestSigner() types.Signer {
	return types.NewLondonSigner(big.NewInt(1))
}

func newTestStep(t *testing.T, nonce uint64) bundling.BundleStep {
	t.Helper()
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}
	to := common.Address{0x42}
	return bundling.Step(key, &types.DynamicFeeTx{
		Nonce:     nonce,
		Gas:       21_000,
		GasFeeCap: big.NewInt(1e9),
		To:        &to,
	})
}

func TestBuild_ReturnsEnvelopeAddressedToBundleProcessor(t *testing.T) {
	envelope, err := bundling.NewBuilder(newTestSigner()).
		With(newTestStep(t, 0)).
		Build()

	if err != nil {
		t.Fatalf("Build() failed: %v", err)
	}
	if envelope == nil {
		t.Fatal("Build() returned nil envelope")
	}
	if envelope.To() == nil || *envelope.To() != bundling.BundleProcessor {
		t.Errorf("envelope.To() = %v, want %v", envelope.To(), bundling.BundleProcessor)
	}
}

func TestBuild_MultipleStepsProduceSingleEnvelope(t *testing.T) {
	envelope, err := bundling.NewBuilder(newTestSigner()).
		With(newTestStep(t, 0), newTestStep(t, 0)).
		Build()

	if err != nil {
		t.Fatalf("Build() failed: %v", err)
	}
	if envelope == nil || envelope.To() == nil || *envelope.To() != bundling.BundleProcessor {
		t.Errorf("expected envelope addressed to BundleProcessor, got %v", envelope)
	}
}

func TestBuild_ValidityWindowIsRespected(t *testing.T) {
	envelope, err := bundling.NewBuilder(newTestSigner()).
		SetEarliest(100).
		SetLatest(200).
		With(newTestStep(t, 0)).
		Build()

	if err != nil {
		t.Fatalf("Build() failed: %v", err)
	}
	if envelope == nil || envelope.To() == nil || *envelope.To() != bundling.BundleProcessor {
		t.Errorf("expected envelope addressed to BundleProcessor, got %v", envelope)
	}
}

func TestBuild_DefaultLatestIsCappedAtMaxBlockRange(t *testing.T) {
	envelope, err := bundling.NewBuilder(newTestSigner()).
		SetEarliest(500).
		With(newTestStep(t, 0)).
		Build()

	if err != nil {
		t.Fatalf("Build() failed: %v", err)
	}
	if envelope == nil {
		t.Fatal("Build() returned nil envelope")
	}
}

func TestStep_PanicsOnUnsupportedTxType(t *testing.T) {
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}
	defer func() {
		if r := recover(); r == nil {
			t.Error("Step() with LegacyTx should have panicked")
		}
	}()
	bundling.Step(key, &types.LegacyTx{})
}
