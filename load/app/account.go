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

package app

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"math/big"
	"sync/atomic"

	"github.com/Fantom-foundation/Norma/driver/rpc"
	contract "github.com/Fantom-foundation/Norma/load/contracts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

// AccountFactory use one mnemonic phrase to generate any amount accounts, by BIP-39 standard.
// Any factory using the same mnemonic, feederId and appId produce the same sequence of accounts,
// which can be used to reuse existing accounts from previous runs.
type AccountFactory struct {
	keyGenerator *KeyGenerator
	chainID      *big.Int
	numAccounts  int64
}

// NewAccountFactory creates a new AccountFactory, generating accounts for given feeder and app.
// Re-creating a factory using the same feederId and appId will produce the same sequence of accounts.
func NewAccountFactory(chainID *big.Int, feederId, appId uint32) (*AccountFactory, error) {
	keyGenerator, err := NewKeyGenerator(Mnemonic, feederId, appId)
	if err != nil {
		return nil, err
	}
	return &AccountFactory{
		keyGenerator: keyGenerator,
		chainID:      chainID,
		numAccounts:  0,
	}, nil
}

// CreateAccount generates the next account in the sequence generated by the AccountFactory.
func (f *AccountFactory) CreateAccount(rpcClient rpc.RpcClient) (*Account, error) {
	id := atomic.AddInt64(&f.numAccounts, 1)
	privateKey, err := f.keyGenerator.GeneratePrivateKey(uint32(id))
	if err != nil {
		return nil, err
	}
	address := crypto.PubkeyToAddress(privateKey.PublicKey)

	nonce, err := rpcClient.NonceAt(context.Background(), address, nil) // nonce at latest block
	if err != nil {
		return nil, fmt.Errorf("failed to get address nonce; %v", err)
	}

	return &Account{
		privateKey: privateKey,
		address:    address,
		chainID:    f.chainID,
		nonce:      nonce,
	}, nil
}

// Account represents an account from which we can send transactions.
// It sustains the nonce value - it allows multiple generators which use one Account
// to produce multiple txs in one block.
type Account struct {
	id         int
	privateKey *ecdsa.PrivateKey
	address    common.Address
	chainID    *big.Int
	nonce      uint64
	publicKey  []byte
}

// NewAccount creates an Account instance from the provided private key
func NewAccount(id int, privateKeyHex string, publicKey []byte, chainID int64) (*Account, error) {
	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		return nil, err
	}
	address := crypto.PubkeyToAddress(privateKey.PublicKey)
	return &Account{
		id:         id,
		privateKey: privateKey,
		address:    address,
		chainID:    big.NewInt(chainID),
		nonce:      0,
		publicKey:  publicKey,
	}, nil
}

// Fund transfers finances to given account for covering txs fees if its balance is lower than required endowment
func (a *Account) Fund(fundingAccount *Account, rpcClient rpc.RpcClient, regularGasPrice *big.Int, endowment int64) error {
	balance, err := rpcClient.BalanceAt(context.Background(), a.address, nil)
	if err != nil {
		return fmt.Errorf("failed to get balance before funding; %v", err)
	}

	value := big.NewInt(0).Mul(big.NewInt(endowment), big.NewInt(1_000_000_000_000_000_000)) // FTM to wei
	value.Sub(value, balance)
	if value.Sign() <= 0 {
		return nil // already funded
	}

	priorityGasPrice := getPriorityGasPrice(regularGasPrice)
	if err := transferValue(rpcClient, fundingAccount, a.address, value, priorityGasPrice); err != nil {
		return fmt.Errorf("failed to transfer (value: %s, gasPrice: %s): %v", value, priorityGasPrice, err)
	}
	return nil
}

// getNextNonce provides a nonce to be used for next transactions sent using this account
func (a *Account) getNextNonce() uint64 {
	current := atomic.AddUint64(&a.nonce, 1)
	return current - 1
}

func (a *Account) getCurrentNonce() uint64 {
	return atomic.LoadUint64(&a.nonce)
}

// CreateValidator creates non genesis validator trough createValidator sfc call
func (a *Account) CreateValidator(SFCContract *contract.SFC, rpcClient rpc.RpcClient) (*types.Transaction, error) {
	// get price of gas from the network
	regularGasPrice, err := getGasPrice(rpcClient)
	if err != nil {
		return nil, err
	}

	txOpts, err := bind.NewKeyedTransactorWithChainID(a.privateKey, a.chainID)
	if err != nil {
		return nil, fmt.Errorf("failed to create txOpts; %v", err)
	}
	txOpts.GasPrice = getPriorityGasPrice(regularGasPrice)
	txOpts.Nonce = big.NewInt(int64(a.getNextNonce()))
	txOpts.Value = big.NewInt(0).Mul(big.NewInt(5_000_000), big.NewInt(1_000_000_000_000_000_000)) // 5_000_000 FTM
	return SFCContract.CreateValidator(txOpts, a.publicKey)
}
