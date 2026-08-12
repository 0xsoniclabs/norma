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
	"github.com/0xsoniclabs/norma/driver/rpc"
	"github.com/ethereum/go-ethereum/core/types"
)

//go:generate mockgen -source app.go -destination app_mock.go -package app

type Application interface {
	// CreateUsers creates a number of users for this application that
	// can generate transactions for this application.
	CreateUsers(context AppContext, numUsers int) ([]User, error)

	// GetReceivedTransactions returns the total number of transactions
	// received by this application up to the current point in time.
	GetReceivedTransactions(rpcClient rpc.Client) (uint64, error)
}

// User produces a stream of transactions to Generate traffic on the chain.
// Implementations are not required to be thread-safe.
type User interface {
	GenerateTx() (*types.Transaction, error)
	GetSentTransactions() uint64
}

// PrioritizableUser is a User that discloses the accounts it signs its
// transactions with.
//
// Sonic keys the priority of a transaction on its sender, so a load can only be
// placed in a priority lane if the accounts signing it are known before it
// starts - see PriorityApplication. Users that create senders while they run, or
// that delegate to other users, deliberately do not implement this: registering
// the accounts they happen to hold at setup time would prioritize a part of
// their traffic and silently leave the rest behind.
type PrioritizableUser interface {
	User

	// SigningAccounts returns the accounts this user signs its transactions
	// with, all of which it will use for the whole run.
	SigningAccounts() []*Account
}
