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

	"github.com/0xsoniclabs/norma/driver/rpc"
	priority_registry "github.com/0xsoniclabs/sonic/gossip/blockproc/priorities/registry"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/core/types"
)

// The lane this application's users are registered in. A single level is enough
// to separate prioritized from ordinary traffic; the weight only orders
// transactions of the same level against each other and is therefore left at
// the same value for all users.
const (
	priorityLevel  = 1
	priorityWeight = 0
)

// priorityRegistryUpdateGasLimit covers a registry update, which writes one slot -
// the level, weight and entity of a sender share a single word. The limit is
// fixed to keep the setup from paying for a gas estimation per user.
const priorityRegistryUpdateGasLimit = 100_000

// The per-entity rate limits this application configures in the registry.
// They are deliberately generous: a load generator should be limited by the
// network, not by a lane budget it happens to share with itself. Lower them to
// observe the rate limiting itself.
var (
	priorityMaxGasPerEntityPerBlock          = big.NewInt(100_000_000)
	priorityMaxPiggybackTxsPerEntityPerEvent = big.NewInt(1_000)
)

// PriorityApplication generates traffic in a priority lane: the load itself is
// the one of the counter application - the same contract call, gas limit and
// gas price - and the only difference is that its users are registered in the
// on-chain priority registry. Running it next to a `counter` application in a
// congested network therefore isolates the effect of the priority lanes
// feature: the two applications differ in nothing else.
//
// All users of one application share one entity id, which is what the client
// rate-limits per block, so an application maps to what a production registry
// would consider one customer.
//
// Traffic is only prioritized while the network runs with the
// TransactionPriorities upgrade enabled; without it the registry is never
// queried and this application would be an ordinary counter application in
// disguise. It therefore refuses to start on a network that has the upgrade
// disabled rather than generating load that proves nothing.
type PriorityApplication struct {
	*CounterApplication
	registry *priority_registry.Registry
	entity   *big.Int
}

// NewPriorityApplication deploys the counter contract used to generate the
// traffic and configures the priority registry for it. It fails if the network
// does not currently apply transaction priorities.
func NewPriorityApplication(ctxt AppContext, feederId, appId uint32) (Application, error) {
	enabled, err := transactionPrioritiesEnabled(ctxt.GetClient())
	if err != nil {
		return nil, fmt.Errorf("failed to read the network's transaction priority state; %w", err)
	}
	if !enabled {
		return nil, fmt.Errorf(
			"transaction priorities are disabled on this network, so this " +
				"application's traffic would not be prioritized; enable the " +
				"TransactionPriorities upgrade in the scenario's network rules")
	}

	traffic, err := NewCounterApplication(ctxt, feederId, appId)
	if err != nil {
		return nil, fmt.Errorf("failed to create counter application for priority load; %w", err)
	}
	counter, ok := traffic.(*CounterApplication)
	if !ok {
		return nil, fmt.Errorf("unexpected application type %T for priority load", traffic)
	}

	registry, err := priority_registry.NewRegistry(priority_registry.GetAddress(), ctxt.GetClient())
	if err != nil {
		return nil, fmt.Errorf("failed to bind to priority registry contract; %w", err)
	}

	// The limits are global, so applications sharing a network overwrite each
	// other here - with the same values, since they are constants.
	if _, err := ctxt.Run(func(opts *bind.TransactOpts) (*types.Transaction, error) {
		return registry.SetConfig(opts, priorityMaxGasPerEntityPerBlock, priorityMaxPiggybackTxsPerEntityPerEvent)
	}); err != nil {
		return nil, fmt.Errorf("failed to configure priority registry limits; %w", err)
	}

	return &PriorityApplication{
		CounterApplication: counter,
		registry:           registry,
		entity:             new(big.Int).SetUint64(uint64(appId)),
	}, nil
}

// CreateUsers creates the users generating the load and registers them as
// members of this application's priority lane.
func (a *PriorityApplication) CreateUsers(ctxt AppContext, numUsers int) ([]User, error) {
	users, err := a.CounterApplication.CreateUsers(ctxt, numUsers)
	if err != nil {
		return nil, err
	}

	accounts := make([]*Account, 0, len(users))
	for _, user := range users {
		counterUser, ok := user.(*CounterUser)
		if !ok {
			return nil, fmt.Errorf("unexpected user type %T for priority load", user)
		}
		accounts = append(accounts, counterUser.sender)
	}

	if err := a.prioritize(ctxt, accounts); err != nil {
		return nil, fmt.Errorf("failed to register users in priority registry; %w", err)
	}
	return users, nil
}

// transactionPrioritiesEnabled reports whether the network currently applies
// transaction priorities.
func transactionPrioritiesEnabled(client rpc.Client) (bool, error) {
	rules, err := client.GetNetworkRules("latest")
	if err != nil {
		return false, fmt.Errorf("failed to get network rules; %w", err)
	}
	return rules.Upgrades.TransactionPriorities, nil
}

// prioritize has every account register itself in the priority registry. The
// registrations are submitted before any receipt is awaited, and each one comes
// from a different account, so a validator can emit all of them at once - it
// carries at most one transaction per sender per event. Registering them from a
// single account would instead cost one event per user, which on a congested
// network is a considerable part of a scenario.
func (a *PriorityApplication) prioritize(ctxt AppContext, accounts []*Account) error {
	gasPrice, err := ctxt.GetClient().SuggestGasPrice(context.Background())
	if err != nil {
		return fmt.Errorf("failed to get gas price suggestion; %w", err)
	}

	txs := make([]*types.Transaction, 0, len(accounts))
	for _, account := range accounts {
		opts, err := bind.NewKeyedTransactorWithChainID(account.privateKey, account.chainID)
		if err != nil {
			return fmt.Errorf("failed to create transactor; %w", err)
		}
		opts.GasLimit = priorityRegistryUpdateGasLimit
		opts.GasPrice = new(big.Int).Mul(gasPrice, big.NewInt(2))
		opts.Nonce = new(big.Int).SetUint64(account.getNextNonce())

		tx, err := a.registry.SetSenderPriority(opts, account.address, priorityLevel, priorityWeight, a.entity)
		if err != nil {
			return fmt.Errorf("failed to submit priority of %v; %w", account.address, err)
		}
		txs = append(txs, tx)
	}

	for _, tx := range txs {
		receipt, err := ctxt.GetReceipt(tx.Hash())
		if err != nil {
			return fmt.Errorf("failed to get receipt; %w", err)
		}
		if receipt.Status != types.ReceiptStatusSuccessful {
			return fmt.Errorf("priority registration reverted")
		}
	}
	return nil
}
