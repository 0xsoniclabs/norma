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

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/network"
)

//go:generate mockgen -source validator_registry.go -destination validator_registry_mock.go -package executor

// validatorRegistry abstracts how an executor registers and unregisters
// validator nodes with the network.
type validatorRegistry interface {
	registerNewValidator(ctx context.Context, stake uint64) (int, error)
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
