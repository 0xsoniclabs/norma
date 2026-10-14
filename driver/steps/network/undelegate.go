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
	"context"
	"errors"
	"fmt"

	"github.com/0xsoniclabs/norma/driver/steps"
	"gopkg.in/yaml.v3"
)

// Undelegate undelegates stake from one or more validator nodes. A target
// naming a delegator undelegates as that external delegator; otherwise the
// validator's self-stake is undelegated with its own key.
type Undelegate struct {
	Targets []UndelegateTarget
}

// UndelegateTarget is a single validator node to undelegate from. Stake is
// optional; if omitted, the full stake of the effective delegator is
// undelegated. Delegator names an account created by a prior delegate step.
type UndelegateTarget struct {
	Node      string  `yaml:"node"`
	Stake     *uint64 `yaml:"stake,omitempty"`
	Delegator string  `yaml:"delegator,omitempty"`
}

func (*Undelegate) Name() string { return "undelegate" }

func (*Undelegate) Help() string {
	return `Undelegate stake from one or more validator nodes.
    Each target in the list has the fields:
      node:      (required) name of the validator node to undelegate from.
      delegator: (optional) name of the delegator account to undelegate as.
                 When omitted, the validator's self-stake is undelegated
                 using its own key (existing behavior).
      stake:     (optional) amount in S to undelegate; if omitted, the full
                 stake for the effective delegator is queried on-chain
                 and undelegated.
    Shorthand: a bare node name may be used instead of a list when
    undelegating a single node with its full self-stake.
    Example:
      - undelegate:
        - node: val-1
        - node: val-2
          stake: 1000000
        - node: val-3
          delegator: alice
          stake: 500000`
}

func (s *Undelegate) DecodeValue(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		if steps.IsNull(value) {
			return errors.New("undelegate requires at least one target")
		}
		s.Targets = []UndelegateTarget{{Node: value.Value}}
	case yaml.SequenceNode:
		for i, item := range value.Content {
			var target UndelegateTarget
			if err := steps.DecodeStrict(item, &target, "an undelegate target"); err != nil {
				return fmt.Errorf("undelegate target %d: %w", i+1, err)
			}
			s.Targets = append(s.Targets, target)
		}
	default:
		return errors.New("undelegate value must be a node name or a list of targets")
	}
	return nil
}

func (s *Undelegate) Validate() error {
	if len(s.Targets) == 0 {
		return errors.New("undelegate requires at least one target")
	}
	for i, t := range s.Targets {
		if t.Node == "" {
			return fmt.Errorf("undelegate target %d: missing required 'node' field", i+1)
		}
		if err := matchName("node", t.Node); err != nil {
			return fmt.Errorf("undelegate target %d: %w", i+1, err)
		}
		if t.Delegator != "" {
			if err := matchName("delegator", t.Delegator); err != nil {
				return fmt.Errorf("undelegate target %d: %w", i+1, err)
			}
		}
	}
	return nil
}

// UpdateModel undelegates from validators; a delegator can only take back
// stake it delegated to that validator, all of it when no stake is given.
// The self-stake of a validator is not modelled.
func (s *Undelegate) UpdateModel(m *steps.Model) error {
	for _, target := range s.Targets {
		label, err := m.Validator(target.Node)
		if err != nil {
			return err
		}
		if target.Delegator == "" {
			continue
		}
		if !m.Delegators[target.Delegator] {
			return fmt.Errorf("undelegate: delegator %q was never used in a prior delegate step", target.Delegator)
		}
		key := steps.Delegation{Delegator: target.Delegator, Node: label}
		delegated, ok := m.Delegations[key]
		if !ok {
			return fmt.Errorf("delegator %q has no stake on node %q", target.Delegator, target.Node)
		}
		stake := delegated
		if target.Stake != nil {
			stake = *target.Stake
		}
		if stake > delegated {
			return fmt.Errorf("delegator %q has %d S on node %q, cannot undelegate %d S",
				target.Delegator, delegated, target.Node, stake)
		}
		m.Delegations[key] = delegated - stake
	}
	return nil
}

func (s *Undelegate) Run(ctx context.Context, env *steps.Env) error {
	for _, target := range s.Targets {
		id, err := resolveValidatorId(env, target.Node)
		if err != nil {
			return err
		}
		var stakeAmount uint64
		if target.Stake != nil {
			stakeAmount = *target.Stake
		}

		if target.Delegator == "" {
			// stakeAmount=0 → UnregisterValidatorNode queries self-stake on-chain
			if err := env.Validators.UnregisterValidator(ctx, id, stakeAmount); err != nil {
				return fmt.Errorf("failed to unregister validator %s: %w", target.Node, err)
			}
			continue
		}

		delegator, ok := env.Stakes.Delegators[target.Delegator]
		if !ok {
			return fmt.Errorf(
				"undelegate: delegator %q was never used in a prior delegate step",
				target.Delegator,
			)
		}

		// If no explicit amount, undelegate the full expected stake.
		effectiveStake := stakeAmount
		key := steps.StakeKey{Delegator: target.Delegator, ValidatorId: id}
		if effectiveStake == 0 {
			effectiveStake = env.Stakes.Expected[key]
		}

		if err := env.Validators.UndelegateAs(
			ctx, id, stakeAmount, delegator.PrivateKey,
		); err != nil {
			return fmt.Errorf(
				"failed to undelegate from validator %s as %s: %w",
				target.Node, target.Delegator, err,
			)
		}

		// Keep fully undelegated pairs at 0 so verifyStakes asserts the
		// stake is gone on-chain.
		if remaining, ok := env.Stakes.Expected[key]; ok {
			env.Stakes.Expected[key] = remaining - min(effectiveStake, remaining)
		}
	}
	return nil
}
