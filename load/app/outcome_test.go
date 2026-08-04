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

package app

import (
	"testing"

	"github.com/0xsoniclabs/sonic/opera"
	"github.com/ethereum/go-ethereum/common"
)

func TestPickOutcome_OnlyDrawsSupportedOutcomes(t *testing.T) {
	supported := []Outcome{Success, Rejected}
	allowed := map[Outcome]bool{Success: true, Rejected: true}

	drawn := map[Outcome]int{}
	for i := 0; i < 1000; i++ {
		outcome := pickOutcome(supported)
		if !allowed[outcome] {
			t.Fatalf("drew unsupported outcome %v", outcome)
		}
		drawn[outcome]++
	}

	for _, outcome := range supported {
		if drawn[outcome] == 0 {
			t.Errorf("outcome %v was never drawn", outcome)
		}
	}
}

func TestPickOutcome_FavoursSuccess(t *testing.T) {
	const draws = 10_000
	successes := 0
	for i := 0; i < draws; i++ {
		if pickOutcome([]Outcome{Success, Reverted, Failed, Rejected}) == Success {
			successes++
		}
	}
	// The weights put success at 85 of 100; well over half is enough to show the
	// load stays dominated by transactions that go through.
	if successes < draws/2 {
		t.Errorf("expected successful transactions to dominate, got %d of %d", successes, draws)
	}
}

func TestPickOutcome_WithoutSupportedOutcomesDrawsSuccess(t *testing.T) {
	if got := pickOutcome(nil); got != Success {
		t.Errorf("expected success, got %v", got)
	}
}

func TestOutOfGasLimit_CoversTheIntrinsicCostAndNoMore(t *testing.T) {
	target := common.Address{1}
	tests := map[string]txPayload{
		"bare call":              {to: &target},
		"call with a selector":   {to: &target, data: []byte{1, 2, 3, 4}},
		"call with zero padding": {to: &target, data: make([]byte, 100)},
		"contract creation":      {data: []byte{1, 2, 3, 4}},
		"call delegating accounts": {
			to:         &target,
			data:       []byte{1, 2, 3, 4},
			delegators: []*Account{{}, {}, {}},
		},
	}

	for name, payload := range tests {
		t.Run(name, func(t *testing.T) {
			minimum, err := minimumGas(payload)
			if err != nil {
				t.Fatalf("failed to compute the minimum gas: %v", err)
			}
			limit, err := outOfGasLimit(payload)
			if err != nil {
				t.Fatalf("failed to compute the out-of-gas limit: %v", err)
			}
			// The transaction pool needs the limit to reach the minimum, and the
			// execution needs to run out of gas right after.
			if limit != minimum+1 {
				t.Errorf("limit %d leaves %d gas to execute with, expected 1", limit, limit-minimum)
			}
		})
	}
}

func TestMinimumGas_AccountsForDelegatedAccounts(t *testing.T) {
	// The authorizations of a set-code transaction are part of its intrinsic cost.
	// Leaving them out yields a limit the transaction pool refuses outright instead
	// of one the execution runs out of.
	target := common.Address{1}
	plain := txPayload{to: &target, data: []byte{1, 2, 3, 4}}
	delegating := plain
	delegating.delegators = []*Account{{}, {}, {}}

	withoutAuthorizations, err := minimumGas(plain)
	if err != nil {
		t.Fatalf("failed to compute the minimum gas: %v", err)
	}
	withAuthorizations, err := minimumGas(delegating)
	if err != nil {
		t.Fatalf("failed to compute the minimum gas: %v", err)
	}
	if withAuthorizations <= withoutAuthorizations {
		t.Errorf("delegating three accounts did not raise the minimum gas above %d, got %d",
			withoutAuthorizations, withAuthorizations)
	}
}

func TestRejectedGasLimit_ExceedsWhatThePoolAccepts(t *testing.T) {
	rules := opera.FakeNetRules(opera.GetSonicUpgrades())
	if got, want := rejectedGasLimit(rules), poolGasLimit(rules); got <= want {
		t.Errorf("gas limit %d does not exceed the pool limit %d", got, want)
	}
}

func TestPoolGasLimit_IsZeroWhenTheEventOverheadExceedsTheEventGas(t *testing.T) {
	rules := opera.FakeNetRules(opera.GetSonicUpgrades())
	rules.Economy.Gas.MaxEventGas = 0
	if got := poolGasLimit(rules); got != 0 {
		t.Errorf("expected a pool limit of 0, got %d", got)
	}
}
