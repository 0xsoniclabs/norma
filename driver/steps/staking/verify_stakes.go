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
)

// VerifyStakes asserts that the on-chain stake of every tracked
// (delegator, validator) pair matches the ledger.
type VerifyStakes struct{}

func (*VerifyStakes) Name() string { return "verifyStakes" }

func (*VerifyStakes) Help() string {
	return `Verify that on-chain stake state matches the executor's
    internal bookkeeping for every delegator that participated in the
    scenario so far. Fails if any tracked (delegator, validator) pair
    has an on-chain stake that differs from the expected value.`
}

func (*VerifyStakes) Validate() error { return nil }

func (*VerifyStakes) Run(_ context.Context, env *steps.Env) error {
	var errs []error
	for key, expected := range env.Stakes.Expected {
		delegator, ok := env.Stakes.Delegators[key.Delegator]
		if !ok {
			errs = append(errs, fmt.Errorf(
				"verifyStakes: unknown delegator %q in tracked state", key.Delegator,
			))
			continue
		}
		actual, err := env.Validators.GetDelegatorStake(delegator.Address, key.ValidatorId)
		if err != nil {
			errs = append(errs, fmt.Errorf(
				"verifyStakes: query failed for %s@%d: %w", key.Delegator, key.ValidatorId, err,
			))
			continue
		}
		if actual != expected {
			errs = append(errs, fmt.Errorf(
				"verifyStakes: mismatch for delegator %s on validator %d: expected %d S, got %d S",
				key.Delegator, key.ValidatorId, expected, actual,
			))
		}
	}
	return errors.Join(errs...)
}
