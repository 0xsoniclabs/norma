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
	"crypto/ecdsa"
	"crypto/elliptic"
	crand "crypto/rand"
	"fmt"
	"math/big"
	"sync/atomic"

	"github.com/0xsoniclabs/norma/driver/rpc"
	contract "github.com/0xsoniclabs/norma/load/contracts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// NewEcdsaApplication generates a P-256 key pair, deploys an EcdsaCounter
// contract with the corresponding public key, and returns an Application that
// exercises the P256Verify precompile (EIP-7951).
func NewEcdsaApplication(ctxt AppContext, feederId, appId uint32) (Application, error) {
	client := ctxt.GetClient()
	chainId, err := client.ChainID(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to get chain ID; %w", err)
	}

	// Generate a P-256 (secp256r1) key pair used for all transactions of this app.
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), crand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate P-256 key; %w", err)
	}
	ecdhKey, err := privateKey.ECDH()
	if err != nil {
		return nil, fmt.Errorf("failed to convert P-256 key to ECDH; %w", err)
	}
	pub := ecdhKey.PublicKey().Bytes() // 65 bytes: 0x04 + X + Y
	var pubKeyX, pubKeyY [32]byte
	copy(pubKeyX[:], pub[1:33])
	copy(pubKeyY[:], pub[33:65])

	_, receipt, err := DeployContract(ctxt, func(opts *bind.TransactOpts, backend bind.ContractBackend) (common.Address, *types.Transaction, *contract.EcdsaCounter, error) {
		return contract.DeployEcdsaCounter(opts, backend, pubKeyX, pubKeyY)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to deploy EcdsaCounter contract; %w", err)
	}

	accountFactory, err := NewAccountFactory(chainId, feederId, appId)
	if err != nil {
		return nil, err
	}

	parsedAbi, err := contract.EcdsaCounterMetaData.GetAbi()
	if err != nil {
		return nil, err
	}

	return &EcdsaApplication{
		abi:             parsedAbi,
		contractAddress: receipt.ContractAddress,
		accountFactory:  accountFactory,
		privateKey:      privateKey,
	}, nil
}

// EcdsaApplication represents a deployed EcdsaCounter contract.
type EcdsaApplication struct {
	abi             *abi.ABI
	contractAddress common.Address
	accountFactory  *AccountFactory
	privateKey      *ecdsa.PrivateKey
}

func (f *EcdsaApplication) CreateUsers(appContext AppContext, numUsers int) ([]User, error) {
	users := make([]User, numUsers)
	addresses := make([]common.Address, numUsers)
	for i := 0; i < numUsers; i++ {
		workerAccount, err := f.accountFactory.CreateAccount(appContext.GetClient())
		if err != nil {
			return nil, err
		}
		users[i] = &EcdsaUser{
			abi:        f.abi,
			sender:     workerAccount,
			contract:   f.contractAddress,
			privateKey: f.privateKey,
		}
		addresses[i] = workerAccount.address
	}

	fundsPerUser := new(big.Int).Mul(big.NewInt(1_000), big.NewInt(1_000_000_000_000_000_000))
	return users, appContext.FundAccounts(addresses, fundsPerUser)
}

func (f *EcdsaApplication) GetReceivedTransactions(rpcClient rpc.Client) (uint64, error) {
	c, err := contract.NewEcdsaCounter(f.contractAddress, rpcClient)
	if err != nil {
		return 0, fmt.Errorf("failed to get EcdsaCounter contract; %w", err)
	}
	count, err := c.GetCount(nil)
	if err != nil {
		return 0, err
	}
	return count.Uint64(), nil
}

// EcdsaUser sends incrementCounter transactions to the EcdsaCounter contract.
type EcdsaUser struct {
	abi        *abi.ABI
	sender     *Account
	contract   common.Address
	privateKey *ecdsa.PrivateKey
	sentTxs    atomic.Uint64
}

func (g *EcdsaUser) GenerateTx() (*types.Transaction, error) {
	// Generate a random 32-byte hash to sign.
	var hashBytes [32]byte
	if _, err := crand.Read(hashBytes[:]); err != nil {
		return nil, fmt.Errorf("failed to generate random hash; %w", err)
	}

	// Sign the hash with the shared P-256 private key.
	sigR, sigS, err := ecdsa.Sign(crand.Reader, g.privateKey, hashBytes[:])
	if err != nil {
		return nil, fmt.Errorf("failed to sign with P-256 key; %w", err)
	}

	data, err := g.abi.Pack("incrementCounter", hashBytes, bigIntTo32Bytes(sigR), bigIntTo32Bytes(sigS))
	if err != nil {
		return nil, fmt.Errorf("failed to pack incrementCounter calldata; %w", err)
	}

	const gasLimit = 50_000
	tx, err := createTx(g.sender, g.contract, big.NewInt(0), data, gasLimit)
	if err == nil {
		g.sentTxs.Add(1)
	}
	return tx, err
}

func (g *EcdsaUser) GetSentTransactions() uint64 {
	return g.sentTxs.Load()
}

// bigIntTo32Bytes encodes v as a 32-byte big-endian array, zero-padded on the left.
func bigIntTo32Bytes(v *big.Int) [32]byte {
	var b [32]byte
	v.FillBytes(b[:])
	return b
}
