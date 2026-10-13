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

package staking

import (
	"context"
	"errors"
	"fmt"

	"github.com/0xsoniclabs/norma/driver/steps"
	"gopkg.in/yaml.v3"
)

// Delegate delegates stake from named external delegator accounts to
// validator nodes. Accounts are created on first use; every delegation is
// funded from the treasury with its stake plus a gas budget.
type Delegate struct {
	Targets []DelegateTarget
}

// DelegateTarget is a single delegation from a named external delegator
// account to a validator node. Unlike undelegate, delegate never derives
// values from on-chain state, so every field is required.
type DelegateTarget struct {
	Node      string `yaml:"node"`
	Stake     uint64 `yaml:"stake"`
	Delegator string `yaml:"delegator"`
}

func (*Delegate) Name() string { return "delegate" }

func (*Delegate) Help() string {
	return `Delegate stake from one or more named external delegator accounts
    to validator nodes.
    Each target in the list has three required fields:
      node:      name of the validator node receiving the delegation.
      delegator: name of the delegator account (created on first use, funded
                 automatically from the treasury).
      stake:     amount in S to delegate.
    Example:
      - delegate:
        - node: val-1
          delegator: alice
          stake: 2000000
        - node: val-1
          delegator: bob
          stake: 1000000`
}

func (s *Delegate) DecodeValue(value *yaml.Node) error {
	// There is no scalar shorthand: stake and delegator must be explicit.
	if value.Kind != yaml.SequenceNode {
		return errors.New("delegate value must be a list of targets")
	}
	for i, item := range value.Content {
		var target DelegateTarget
		if err := item.Decode(&target); err != nil {
			return fmt.Errorf("delegate target %d: %w", i+1, err)
		}
		s.Targets = append(s.Targets, target)
	}
	return nil
}

func (s *Delegate) Validate() error {
	if len(s.Targets) == 0 {
		return errors.New("delegate requires at least one target")
	}
	for i, t := range s.Targets {
		if t.Node == "" {
			return fmt.Errorf("delegate target %d: missing required 'node' field", i+1)
		}
		if err := matchName("node", t.Node); err != nil {
			return fmt.Errorf("delegate target %d: %w", i+1, err)
		}
		if t.Delegator == "" {
			return fmt.Errorf("delegate target %d: missing required 'delegator' field", i+1)
		}
		if err := matchName("delegator", t.Delegator); err != nil {
			return fmt.Errorf("delegate target %d: %w", i+1, err)
		}
		if t.Stake == 0 {
			return fmt.Errorf("delegate target %d: stake must be greater than zero", i+1)
		}
	}
	return nil
}

func (s *Delegate) Run(ctx context.Context, env *steps.Env) error {
	for _, target := range s.Targets {
		id, err := resolveValidatorId(env, target.Node)
		if err != nil {
			return err
		}
		delegator, err := getOrCreateDelegator(&env.Stakes, target.Delegator)
		if err != nil {
			return fmt.Errorf("failed to derive delegator %q: %w", target.Delegator, err)
		}

		fundAmount := target.Stake + delegatorGasBudget
		if err := env.Validators.FundDelegator(ctx, delegator.Address, fundAmount); err != nil {
			return fmt.Errorf(
				"failed to fund delegator %q: %w", target.Delegator, err,
			)
		}

		if err := env.Validators.Delegate(ctx, id, target.Stake, delegator.PrivateKey); err != nil {
			return fmt.Errorf(
				"failed to delegate from %s to %s: %w",
				target.Delegator, target.Node, err,
			)
		}

		env.Stakes.Expected[steps.StakeKey{Delegator: target.Delegator, ValidatorId: id}] += target.Stake
	}
	return nil
}
