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
	"crypto/ecdsa"
	"crypto/elliptic"
	crand "crypto/rand"
	"fmt"
	"math/big"

	contract "github.com/0xsoniclabs/norma/load/contracts/abi"
	"github.com/0xsoniclabs/sonic/opera"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// newEcdsaGenerator creates a generator exercising the P256Verify precompile
// (EIP-7951) through a contract that only increments its counter for a valid
// P-256 signature. Its reverting variant submits a signature the precompile
// rejects.
func newEcdsaGenerator(feederId, appId uint32) Generator {
	g := &ecdsaGenerator{txGenerator: newTxGenerator(feederId, appId)}
	g.supports = func(rules opera.Rules) bool { return rules.Upgrades.Brio }
	g.onDeploy = g.deploy
	g.onSuccess = g.incrementWithValidSignature
	g.onRevert = g.incrementWithInvalidSignature
	return g
}

type ecdsaGenerator struct {
	txGenerator
	abi     *abi.ABI
	address common.Address
	// privateKey matches the public key the contract was deployed with.
	privateKey *ecdsa.PrivateKey
}

func (g *ecdsaGenerator) deploy(ctxt AppContext) error {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), crand.Reader)
	if err != nil {
		return fmt.Errorf("failed to generate a P-256 key; %w", err)
	}
	g.privateKey = privateKey

	ecdhKey, err := privateKey.ECDH()
	if err != nil {
		return fmt.Errorf("failed to convert the P-256 key to ECDH; %w", err)
	}
	publicKey := ecdhKey.PublicKey().Bytes() // < 65 bytes: 0x04 + X + Y
	var publicKeyX, publicKeyY [32]byte
	copy(publicKeyX[:], publicKey[1:33])
	copy(publicKeyY[:], publicKey[33:65])

	_, receipt, err := DeployContract(ctxt, func(opts *bind.TransactOpts, backend bind.ContractBackend) (common.Address, *types.Transaction, *contract.EcdsaCounter, error) {
		return contract.DeployEcdsaCounter(opts, backend, publicKeyX, publicKeyY)
	})
	if err != nil {
		return fmt.Errorf("failed to deploy the EcdsaCounter contract; %w", err)
	}
	g.address = receipt.ContractAddress

	g.abi, err = contract.EcdsaCounterMetaData.GetAbi()
	return err
}

func (g *ecdsaGenerator) incrementWithValidSignature(int, *Account) (txPayload, error) {
	var hash [32]byte
	if _, err := crand.Read(hash[:]); err != nil {
		return txPayload{}, fmt.Errorf("failed to generate a random hash; %w", err)
	}
	r, s, err := ecdsa.Sign(crand.Reader, g.privateKey, hash[:])
	if err != nil {
		return txPayload{}, fmt.Errorf("failed to sign with the P-256 key; %w", err)
	}
	return g.incrementPayload(hash, bigIntTo32Bytes(r), bigIntTo32Bytes(s), "")
}

func (g *ecdsaGenerator) incrementWithInvalidSignature(int, *Account) (txPayload, error) {
	var hash, r, s [32]byte
	if _, err := crand.Read(hash[:]); err != nil {
		return txPayload{}, fmt.Errorf("failed to generate a random hash; %w", err)
	}
	if _, err := crand.Read(r[:]); err != nil {
		return txPayload{}, fmt.Errorf("failed to generate a random signature; %w", err)
	}
	if _, err := crand.Read(s[:]); err != nil {
		return txPayload{}, fmt.Errorf("failed to generate a random signature; %w", err)
	}
	return g.incrementPayload(hash, r, s, "invalid P-256 signature")
}

func (g *ecdsaGenerator) incrementPayload(hash, r, s [32]byte, reason string) (txPayload, error) {
	data, err := g.abi.Pack("incrementCounter", hash, r, s)
	if err != nil {
		return txPayload{}, fmt.Errorf("failed to pack incrementCounter; %w", err)
	}
	return txPayload{to: &g.address, data: data, gasLimit: 50_000, reason: reason}, nil
}

func bigIntTo32Bytes(v *big.Int) [32]byte {
	var b [32]byte
	v.FillBytes(b[:])
	return b
}
