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

package network

import (
	"fmt"

	"github.com/0xsoniclabs/norma/driver/node"
	"github.com/0xsoniclabs/norma/driver/steps"
	"github.com/ethereum/go-ethereum/crypto"
)

// delegatorGasBudget is the extra amount (in S) transferred to a delegator
// account on top of the delegation stake. Delegate and undelegate
// transactions reserve their fixed 1e6 gas limit times a fee cap derived
// from the current base fee, a small fraction of 1 S at fakenet base fees,
// so 1 S leaves headroom for repeated operations.
const delegatorGasBudget uint64 = 1

// getOrCreateDelegator returns the delegator account for the given name,
// creating it deterministically on first use. The private key is derived
// from keccak256("norma-delegator:" + name), which keeps accounts
// reproducible across runs and avoids colliding with the fakenet validator
// key range used by evmcore.FakeKey.
func getOrCreateDelegator(ledger *steps.Ledger, name string) (*steps.Delegator, error) {
	if existing, ok := ledger.Delegators[name]; ok {
		return existing, nil
	}
	seed := crypto.Keccak256([]byte("norma-delegator:" + name))
	key, err := crypto.ToECDSA(seed)
	if err != nil {
		return nil, fmt.Errorf("failed to derive delegator key: %w", err)
	}
	account := &steps.Delegator{
		Name:       name,
		PrivateKey: key,
		Address:    crypto.PubkeyToAddress(key.PublicKey),
	}
	ledger.Delegators[name] = account
	return account, nil
}

// resolveValidatorId maps a scenario-level node name to its validator ID,
// falling back to the single numbered instance "<name>-0".
func resolveValidatorId(env *steps.Env, name string) (int, error) {
	label, err := steps.ResolveLabel(env.Nodes, name)
	if err != nil {
		return 0, err
	}
	id := env.Nodes[label].GetValidatorId()
	if id == nil {
		return 0, fmt.Errorf("node %q is not a validator", name)
	}
	return *id, nil
}

func matchName(field, name string) error {
	if !node.NamePattern.MatchString(name) {
		return fmt.Errorf("%s name must match %v, got %v", field, node.NamePattern, name)
	}
	return nil
}
