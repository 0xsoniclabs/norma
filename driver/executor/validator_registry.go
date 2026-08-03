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
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/network"
)

//go:generate mockgen -source validator_registry.go -destination validator_registry_mock.go -package executor

// validatorActivationTimeout bounds how long activateValidators waits for a
// sealed epoch to propagate to the node it queries. Var so tests can shorten it.
var validatorActivationTimeout = 30 * time.Second

// validatorRegistry abstracts how an executor registers, activates and
// unregisters validator nodes with the network.
type validatorRegistry interface {
	registerNewValidator(ctx context.Context, stake uint64) (int, error)
	activateValidators(ctx context.Context, validatorIds []int) error
	unregisterValidator(ctx context.Context, validatorId int, stake uint64) error
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

// activateValidators seals the current epoch so that the given freshly
// registered validators join the validator set, then waits until they are
// reported as active.
//
// Registering a validator through the SFC contract is not enough to make it
// part of consensus: validator sets are per-epoch, so a validator created
// mid-epoch emits nothing and carries no stake weight until that epoch is
// sealed. Sealing here is what makes a node started as "type: validator"
// actually be a validator.
func (a netBasedValidatorRegistry) activateValidators(ctx context.Context, validatorIds []int) error {
	if len(validatorIds) == 0 {
		return nil
	}

	if err := a.net.AdvanceEpoch(ctx, 1); err != nil {
		return fmt.Errorf(
			"failed to seal epoch to activate validators %v; %w",
			validatorIds, err,
		)
	}

	rpcClient, err := a.net.DialRandomRpc()
	if err != nil {
		return fmt.Errorf("failed to connect to RPC; %v", err)
	}
	defer rpcClient.Close()

	// AdvanceEpoch confirms the seal on the node it dialed itself, which need
	// not be the node queried here, so poll until the set has caught up. A
	// failed read is treated like a set that has not caught up yet, since a
	// node that is restarting or briefly unresponsive recovers well within the
	// timeout; it is only reported once the deadline expires.
	deadline := time.Now().Add(validatorActivationTimeout)
	for {
		active, readErr := network.GetActiveValidatorIDs(rpcClient)

		missing := make([]int, 0, len(validatorIds))
		for _, id := range validatorIds {
			if !slices.Contains(active, id) {
				missing = append(missing, id)
			}
		}
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
