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

package executor

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/network"
	"github.com/0xsoniclabs/sonic/evmcore"
	"github.com/ethereum/go-ethereum/common"
)

//go:generate mockgen -source validator_registry.go -destination validator_registry_mock.go -package executor

// validatorActivationTimeout bounds how long ensureValidatorsActive waits for a
// sealed epoch to propagate to the node it queries. Var so tests can shorten it.
var validatorActivationTimeout = 30 * time.Second

// validatorRegistry abstracts how an executor registers, activates and
// unregisters validator nodes with the network, and how it moves stake
// between external delegator accounts and those validators.
type validatorRegistry interface {
	registerNewValidator(ctx context.Context, stake uint64) (int, error)
	ensureValidatorsActive(ctx context.Context, validatorIds []int) error
	unregisterValidator(ctx context.Context, validatorId int, stake uint64) error
	// delegate delegates `stake` from the given external delegator key to
	// the validator identified by validatorId.
	delegate(ctx context.Context, validatorId int, stake uint64, delegator *ecdsa.PrivateKey) error
	// undelegateAs undelegates `stake` from the validator, signing the
	// transaction with the given delegator key. If stake is 0, the full
	// on-chain stake for the delegator on that validator is undelegated.
	undelegateAs(ctx context.Context, validatorId int, stake uint64, delegator *ecdsa.PrivateKey) error
	// fundDelegator transfers `amount` (in S) from the treasury account to
	// the given delegator address so it can pay for delegation + gas.
	fundDelegator(ctx context.Context, delegator common.Address, amount uint64) error
	// getDelegatorStake returns the current on-chain stake (in S) held by
	// the given delegator on the given validator.
	getDelegatorStake(delegator common.Address, validatorId int) (uint64, error)
}

// netBasedValidatorRegistry is the production implementation of
// validatorRegistry: it registers and unregisters validators against a live
// network via RPC.
type netBasedValidatorRegistry struct {
	net driver.Network
}

func (a netBasedValidatorRegistry) registerNewValidator(ctx context.Context, stake uint64) (int, error) {
	rpcClient, err := a.net.DialRandomRpc()
	if err != nil {
		return 0, fmt.Errorf("failed to connect to RPC; %v", err)
	}
	defer rpcClient.Close()
	id, err := network.RegisterValidatorNode(ctx, rpcClient, stake)
	if err != nil {
		return 0, fmt.Errorf("failed to register validator node; %v", err)
	}
	return id, nil
}

// ensureValidatorsActive makes the given validators take part in consensus. It
// seals the current epoch when any of them is missing from the validator set,
// and returns once all of them are reported in it.
//
// Registering a validator through the SFC contract is not enough to make it
// part of consensus: validator sets are per-epoch, so a validator created
// mid-epoch emits nothing and carries no stake weight until that epoch is
// sealed. Sealing here is what makes a node started as "type: validator"
// actually be a validator.
//
// Membership is read rather than inferred from what was just registered, so
// that the ones a caller only expects to be members are held to it too. A
// rejoining node's validator is normally still in the set and then costs no
// more than that read, but one whose stake was undelegated, or one registered
// by a step that was allowed to fail, is not — and is admitted here instead of
// silently running as an observer.
func (a netBasedValidatorRegistry) ensureValidatorsActive(ctx context.Context, validatorIds []int) error {
	if len(validatorIds) == 0 {
		return nil
	}

	rpcClient, err := a.net.DialRandomRpc()
	if err != nil {
		return fmt.Errorf("failed to connect to RPC; %v", err)
	}
	defer rpcClient.Close()

	if active, err := network.GetActiveValidatorIDs(rpcClient); err == nil &&
		len(missingValidators(validatorIds, active)) == 0 {
		slog.Info("validators are already in the validator set",
			"validators", validatorIds,
			"active_validators", active,
		)
		return nil
	}

	// Sealing an epoch means landing a transaction, which a network that is not
	// producing blocks cannot do. Wait for a block first, as the advanceEpoch
	// step does, so that a stalled network is reported as one rather than as a
	// transaction that never got its receipt.
	if err := waitForBlockProduction(ctx, a.net); err != nil {
		return fmt.Errorf(
			"no block produced to seal an epoch on for validators %v; %w",
			validatorIds, err,
		)
	}

	if err := a.net.AdvanceEpoch(ctx, 1); err != nil {
		return fmt.Errorf(
			"failed to seal epoch to activate validators %v; %w",
			validatorIds, err,
		)
	}

	// AdvanceEpoch confirms the seal on the node it dialed itself, which need
	// not be the node queried here, so poll until the set has caught up. A
	// failed read is treated like a set that has not caught up yet, since a
	// node that is restarting or briefly unresponsive recovers well within the
	// timeout; it is only reported once the deadline expires.
	deadline := time.Now().Add(validatorActivationTimeout)
	for {
		active, readErr := network.GetActiveValidatorIDs(rpcClient)
		missing := missingValidators(validatorIds, active)
		if readErr == nil && len(missing) == 0 {
			slog.Info("validators joined the validator set",
				"validators", validatorIds,
				"active_validators", active,
			)
			return nil
		}

		if time.Now().After(deadline) {
			if readErr != nil {
				return fmt.Errorf(
					"failed to read the validator set while waiting %s for "+
						"validators %v to join; %w",
					validatorActivationTimeout, validatorIds, readErr,
				)
			}
			return fmt.Errorf(
				"validators %v did not join the validator set within %s of "+
					"sealing the epoch; active validators are %v",
				missing, validatorActivationTimeout, active,
			)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
}

// missingValidators returns the ids from want that the given validator set does
// not contain, preserving the order they were asked for in.
func missingValidators(want, active []int) []int {
	missing := make([]int, 0, len(want))
	for _, id := range want {
		if !slices.Contains(active, id) {
			missing = append(missing, id)
		}
	}
	return missing
}

func (a netBasedValidatorRegistry) unregisterValidator(ctx context.Context, validatorId int, stake uint64) error {
	rpcClient, err := a.net.DialRandomRpc()
	if err != nil {
		return fmt.Errorf("failed to connect to RPC; %v", err)
	}
	defer rpcClient.Close()
	err = network.UnregisterValidatorNode(ctx, rpcClient, validatorId, stake)
	if err != nil {
		return fmt.Errorf("failed to unregister validator node; %v", err)
	}
	return nil
}

// treasuryKey is the key holding the initial fakenet balance. This is the
// same key used by fakenet validator 1 and by load/app treasury operations.
// Delegator accounts created for scenarios are funded from this account.
func treasuryKey() *ecdsa.PrivateKey {
	return evmcore.FakeKey(1)
}

func (a netBasedValidatorRegistry) delegate(
	ctx context.Context, validatorId int, stake uint64, delegator *ecdsa.PrivateKey,
) error {
	rpcClient, err := a.net.DialRandomRpc()
	if err != nil {
		return fmt.Errorf("failed to connect to RPC; %v", err)
	}
	defer rpcClient.Close()
	if err := network.DelegateToValidator(ctx, rpcClient, validatorId, stake, delegator); err != nil {
		return fmt.Errorf("failed to delegate to validator %d; %v", validatorId, err)
	}
	return nil
}

func (a netBasedValidatorRegistry) undelegateAs(
	ctx context.Context, validatorId int, stake uint64, delegator *ecdsa.PrivateKey,
) error {
	rpcClient, err := a.net.DialRandomRpc()
	if err != nil {
		return fmt.Errorf("failed to connect to RPC; %v", err)
	}
	defer rpcClient.Close()
	if err := network.UndelegateFromValidator(ctx, rpcClient, validatorId, stake, delegator); err != nil {
		return fmt.Errorf("failed to undelegate from validator %d; %v", validatorId, err)
	}
	return nil
}

func (a netBasedValidatorRegistry) fundDelegator(
	ctx context.Context, delegator common.Address, amount uint64,
) error {
	rpcClient, err := a.net.DialRandomRpc()
	if err != nil {
		return fmt.Errorf("failed to connect to RPC; %v", err)
	}
	defer rpcClient.Close()
	if err := network.FundAccount(ctx, rpcClient, treasuryKey(), delegator, amount); err != nil {
		return fmt.Errorf("failed to fund delegator %s; %v", delegator.Hex(), err)
	}
	return nil
}

func (a netBasedValidatorRegistry) getDelegatorStake(
	delegator common.Address, validatorId int,
) (uint64, error) {
	rpcClient, err := a.net.DialRandomRpc()
	if err != nil {
		return 0, fmt.Errorf("failed to connect to RPC; %v", err)
	}
	defer rpcClient.Close()
	return network.GetDelegatorStake(rpcClient, delegator, validatorId)
}
