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

package app_test

import (
	"math/big"
	"strings"
	"testing"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/network/local"
	"github.com/0xsoniclabs/norma/load/app"
	contract "github.com/0xsoniclabs/norma/load/contracts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// TestLargeContractDeploymentFailsWithoutBrio verifies that deploying
// LargeContractCounter and LargeContract on a network without the Brio upgrade
// fails due to the contract code size exceeding the pre-Brio limits.
func TestLargeContractDeploymentFailsWithoutBrio(t *testing.T) {
	net, err := local.NewLocalNetwork(&driver.NetworkConfig{
		Validators: driver.DefaultValidators,
		NetworkRules: map[string]string{
			"UPGRADES_SONIC":   "true",
			"UPGRADES_ALLEGRO": "true",
			// UPGRADES_BRIO omitted intentionally - deployment of large contracts should fail
		},
	})
	if err != nil {
		t.Fatalf("failed to create local network: %v", err)
	}
	t.Cleanup(func() {
		if err := net.Shutdown(); err != nil {
			t.Fatalf("failed to shutdown network: %v", err)
		}
	})

	primaryAccount, err := app.NewAccount(0, PrivateKey, nil, FakeNetworkID)
	if err != nil {
		t.Fatal(err)
	}

	ctxt, err := app.NewContext(net, primaryAccount)
	if err != nil {
		t.Fatal(err)
	}
	defer ctxt.Close()

	t.Run("LargeContractCounter", func(t *testing.T) {
		_, _, err := app.DeployContract(ctxt, contract.DeployLargeContractCounter)
		if err == nil {
			t.Fatal("expected deployment to fail due to contract size, but it succeeded")
		}
		if !strings.Contains(err.Error(), "max initcode size exceeded") {
			t.Errorf("expected a max initcode size error, got: %v", err)
		}
	})

	t.Run("LargeContract", func(t *testing.T) {
		deployer := func(opts *bind.TransactOpts, backend bind.ContractBackend) (common.Address, *types.Transaction, *contract.LargeContract, error) {
			return contract.DeployLargeContract(opts, backend, common.Address{}, big.NewInt(0))
		}
		_, _, err := app.DeployContract(ctxt, deployer)
		if err == nil {
			t.Fatal("expected deployment to fail due to contract size, but it succeeded")
		}
		if !strings.Contains(err.Error(), "max initcode size exceeded") {
			t.Errorf("expected a max initcode size error, got: %v", err)
		}
	})
}
