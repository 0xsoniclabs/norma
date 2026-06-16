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
	"fmt"
	"math/big"
	"sync"
	"sync/atomic"

	"github.com/0xsoniclabs/norma/driver/rpc"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// NewBls12AddApplication creates a new application that sends transactions to
// the BLS12-381 G1 addition precompile (address 0x0b). The precompile performs
// elliptic curve point addition on the BLS12-381 curve.
// This precompile has been introduced in Prague and is available in Sonic starting with Allegro.
func NewBls12AddApplication(ctxt AppContext, feederId, appId uint32) (Application, error) {
	client := ctxt.GetClient()
	chainId, err := client.ChainID(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to get chain ID; %w", err)
	}

	accountFactory, err := NewAccountFactory(chainId, feederId, appId)
	if err != nil {
		return nil, err
	}

	return &Bls12AddApplication{
		contractAddress: common.HexToAddress("0x0b"),
		accountFactory:  accountFactory,
	}, nil
}

// Bls12AddApplication exercises the BLS12-381 G1 addition precompile at address 0x0b.
type Bls12AddApplication struct {
	contractAddress common.Address
	accountFactory  *AccountFactory
	mu              sync.Mutex       // guards userAddresses
	userAddresses   []common.Address // tracks accounts to query nonces from
}

// CreateUsers creates the specified number of users, each with a unique funded
// account, to send BLS12 addition transactions concurrently.
func (app *Bls12AddApplication) CreateUsers(appContext AppContext, numUsers int) ([]User, error) {
	fundsPerUser := big.NewInt(1_000)
	fundsPerUser = new(big.Int).Mul(fundsPerUser, big.NewInt(1_000_000_000_000_000_000)) // to wei
	workerAccounts, err := appContext.AllocateAccounts(numUsers, fundsPerUser)
	if err != nil {
		return nil, err
	}

	users := make([]User, numUsers)
	addresses := make([]common.Address, numUsers)
	for i := 0; i < numUsers; i++ {
		workerAccount := workerAccounts[i]
		users[i] = &Bls12AddUser{
			sender:   workerAccount,
			contract: app.contractAddress,
		}
		addresses[i] = workerAccount.address
	}

	app.mu.Lock()
	app.userAddresses = append(app.userAddresses, addresses...)
	app.mu.Unlock()

	return users, nil
}

// GetReceivedTransactions returns the total number of transactions processed
// by summing the on-chain nonces of all user accounts. Each successful
// transaction increments the sender's nonce, so the sum across all users
// equals the total number of received transactions.
func (app *Bls12AddApplication) GetReceivedTransactions(rpcClient rpc.Client) (uint64, error) {
	app.mu.Lock()
	addresses := make([]common.Address, len(app.userAddresses))
	copy(addresses, app.userAddresses)
	app.mu.Unlock()

	var total uint64
	for _, addr := range addresses {
		nonce, err := rpcClient.NonceAt(context.Background(), addr, nil)
		if err != nil {
			return 0, fmt.Errorf("failed to get nonce for %v; %w", addr, err)
		}
		total += nonce
	}
	return total, nil
}

// Bls12AddUser generates transactions that invoke the BLS12-381 G1 addition
// precompile with a valid pair of curve points.
type Bls12AddUser struct {
	sender   *Account
	contract common.Address
	sentTxs  atomic.Uint64
}

// GenerateTx creates a transaction calling the BLS12-381 G1 addition
// precompile with two valid G1 points encoded as ABI-style calldata.
func (g *Bls12AddUser) GenerateTx() (*types.Transaction, error) {
	// Valid example input for bls12 addition
	data := common.FromHex("0x0000000000000000000000000000000017f1d3a73197d7942695638c4fa9ac0fc3688c4f9774b905a14e3a3f171bac586c55e83ff97a1aeffb3af00adb22c6bb0000000000000000000000000000000008b3f481e3aaa0f1a09e30ed741d8ae4fcf5e095d5d00af600db18cb2c04b3edd03cc744a2888ae40caa232946c5e7e100000000000000000000000000000000112b98340eee2777cc3c14163dea3ec97977ac3dc5c70da32e6e87578f44912e902ccef9efe28d4a78b8999dfbca942600000000000000000000000000000000186b28d92356c4dfec4b5201ad099dbdede3781f8998ddf929b4cd7756192185ca7b8f4ef7088f813270ac3d48868a21")

	const gasLimit = 45_000 // add extra gas for data floor gas
	tx, err := createTx(g.sender, g.contract, big.NewInt(0), data, gasLimit)
	if err == nil {
		g.sentTxs.Add(1)
	}
	return tx, err
}

// GetSentTransactions returns the number of transactions this user has sent.
func (g *Bls12AddUser) GetSentTransactions() uint64 {
	return g.sentTxs.Load()
}
