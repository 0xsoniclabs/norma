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
	"crypto/ecdsa"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/steps"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"go.uber.org/mock/gomock"
)

func TestDelegate_FundsAndDelegatesForEachTarget(t *testing.T) {
	ctrl := gomock.NewController(t)
	registry := steps.NewMockValidatorRegistry(ctrl)

	const validatorId = 2
	gomock.InOrder(
		registry.EXPECT().FundDelegator(gomock.Any(), gomock.Any(), uint64(2_000_000)+delegatorGasBudget).Return(nil),
		registry.EXPECT().Delegate(gomock.Any(), validatorId, uint64(2_000_000), gomock.Any()).Return(nil),
		registry.EXPECT().FundDelegator(gomock.Any(), gomock.Any(), uint64(1_000_000)+delegatorGasBudget).Return(nil),
		registry.EXPECT().Delegate(gomock.Any(), validatorId, uint64(1_000_000), gomock.Any()).Return(nil),
	)

	env := newTestEnv(registry, map[string]driver.Node{
		"heavy": validatorNode(ctrl, validatorId),
	})
	step := &Delegate{Targets: []DelegateTarget{
		{Node: "heavy", Delegator: "alice", Stake: 2_000_000},
		{Node: "heavy", Delegator: "bob", Stake: 1_000_000},
	}}
	if err := step.Run(t.Context(), env); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetOrCreateDelegator_IsDeterministicPerName(t *testing.T) {
	ledger := &steps.NewEnv(nil, nil, nil).Stakes
	a1, err := getOrCreateDelegator(ledger, "alice")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	a2, err := getOrCreateDelegator(ledger, "alice")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a1 != a2 {
		t.Fatalf("expected same account instance on repeated lookup, got different pointers")
	}

	bob, err := getOrCreateDelegator(ledger, "bob")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a1.Address == bob.Address {
		t.Fatalf("expected distinct addresses for distinct names, got same %s", a1.Address.Hex())
	}
}

// The tests below cover scenarios that are set up wrongly: they target nodes
// that do not exist, are not validators, or hold no stake for the delegator
// performing the operation. Each of them must fail with a message that names
// the offending step, and must not leave the stake bookkeeping in a state
// that a later verifyStakes would silently accept.

func TestDelegate_UnknownNodeIsReported(t *testing.T) {
	ctrl := gomock.NewController(t)
	registry := steps.NewMockValidatorRegistry(ctrl)
	// No delegation call is expected for a node that does not exist.

	env := newTestEnv(registry, nil)
	err := delegateStep("ghost", "alice", 1_000).Run(t.Context(), env)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "not found in active nodes") {
		t.Errorf("unexpected error: %v", err)
	}
	if len(env.Stakes.Expected) != 0 {
		t.Errorf("expected no tracked stake, got %v", env.Stakes.Expected)
	}
}

func TestUndelegate_UnknownNodeIsReported(t *testing.T) {
	tests := map[string]UndelegateTarget{
		"self undelegation":      {Node: "ghost"},
		"delegator undelegation": {Node: "ghost", Delegator: "alice"},
	}
	for name, target := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			registry := steps.NewMockValidatorRegistry(ctrl)
			// Neither undelegation path may be entered.

			env := newTestEnv(registry, nil)
			step := &Undelegate{Targets: []UndelegateTarget{target}}
			err := step.Run(t.Context(), env)
			if err == nil {
				t.Fatal("expected an error, got nil")
			}
			if !strings.Contains(err.Error(), "not found in active nodes") {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestDelegation_NonValidatorNodeIsReported(t *testing.T) {
	ctrl := gomock.NewController(t)
	registry := steps.NewMockValidatorRegistry(ctrl)
	observer := driver.NewMockNode(ctrl)
	observer.EXPECT().GetValidatorId().Return(nil).AnyTimes()

	env := newTestEnv(registry, map[string]driver.Node{"watcher": observer})

	err := delegateStep("watcher", "alice", 1_000).Run(t.Context(), env)
	if err == nil || !strings.Contains(err.Error(), "is not a validator") {
		t.Errorf("expected a non-validator error for delegate, got: %v", err)
	}

	err = undelegateStep("watcher", "", nil).Run(t.Context(), env)
	if err == nil || !strings.Contains(err.Error(), "is not a validator") {
		t.Errorf("expected a non-validator error for undelegate, got: %v", err)
	}
}

// A base name matching several numbered instances is ambiguous: picking the
// first one would stake on an arbitrary validator.
func TestDelegation_AmbiguousNodeNameIsReported(t *testing.T) {
	ctrl := gomock.NewController(t)
	registry := steps.NewMockValidatorRegistry(ctrl)

	env := newTestEnv(registry, map[string]driver.Node{
		"heavy-0": validatorNode(ctrl, 2),
		"heavy-1": validatorNode(ctrl, 3),
	})

	err := delegateStep("heavy", "alice", 1_000).Run(t.Context(), env)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "multiple instances") {
		t.Errorf("unexpected error: %v", err)
	}
}

// Funding precedes delegation; when the treasury transfer fails the delegation
// must not be attempted, because the account cannot pay for it.
func TestDelegate_FundingFailureSkipsTheDelegation(t *testing.T) {
	ctrl := gomock.NewController(t)
	registry := steps.NewMockValidatorRegistry(ctrl)

	registry.EXPECT().
		FundDelegator(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(fmt.Errorf("insufficient funds for gas * price + value"))
	// Delegate must not be called.

	env := newTestEnv(registry, map[string]driver.Node{
		"heavy": validatorNode(ctrl, 2),
	})

	err := delegateStep("heavy", "alice", 1_000).Run(t.Context(), env)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to fund delegator") {
		t.Errorf("unexpected error: %v", err)
	}
	if len(env.Stakes.Expected) != 0 {
		t.Errorf("expected no tracked stake, got %v", env.Stakes.Expected)
	}
}

// A rejected delegation must not be recorded as expected stake, otherwise the
// next verifyStakes would report a mismatch instead of the real failure.
func TestDelegate_FailedDelegationIsNotTracked(t *testing.T) {
	ctrl := gomock.NewController(t)
	registry := steps.NewMockValidatorRegistry(ctrl)

	registry.EXPECT().FundDelegator(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
	registry.EXPECT().
		Delegate(gomock.Any(), 2, uint64(1_000), gomock.Any()).
		Return(fmt.Errorf("delegate transaction reverted"))

	env := newTestEnv(registry, map[string]driver.Node{
		"heavy": validatorNode(ctrl, 2),
	})

	if err := delegateStep("heavy", "alice", 1_000).Run(t.Context(), env); err == nil {
		t.Fatal("expected an error, got nil")
	}
	if len(env.Stakes.Expected) != 0 {
		t.Errorf("expected no tracked stake, got %v", env.Stakes.Expected)
	}
}

// Undelegating a known delegator from a validator it never staked on reaches
// the chain, where the SFC query finds no stake. The step must surface that
// rejection rather than treat itself as a no-op.
func TestUndelegate_DelegatorWithoutStakeOnValidatorIsReported(t *testing.T) {
	ctrl := gomock.NewController(t)
	registry := steps.NewMockValidatorRegistry(ctrl)

	registry.EXPECT().FundDelegator(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
	registry.EXPECT().Delegate(gomock.Any(), 2, uint64(1_000), gomock.Any()).Return(nil)
	// The undelegation targets the other validator, where alice has nothing.
	registry.EXPECT().
		UndelegateAs(gomock.Any(), 3, uint64(0), gomock.Any()).
		Return(fmt.Errorf("delegator has no stake on validator 3"))

	env := newTestEnv(registry, map[string]driver.Node{
		"heavy": validatorNode(ctrl, 2),
		"light": validatorNode(ctrl, 3),
	})

	if err := delegateStep("heavy", "alice", 1_000).Run(t.Context(), env); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err := undelegateStep("light", "alice", nil).Run(t.Context(), env)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "no stake on validator 3") {
		t.Errorf("unexpected error: %v", err)
	}
	// The stake on the validator alice did delegate to stays untouched.
	if got := env.Stakes.Expected[steps.StakeKey{Delegator: "alice", ValidatorId: 2}]; got != 1_000 {
		t.Errorf("expected the unrelated stake to remain 1000 S, got %d S", got)
	}
}

// Self-undelegation from a validator without stake is rejected by the registry;
// the error must name the node so the scenario step can be identified.
func TestUndelegate_SelfUndelegationFailureIsReported(t *testing.T) {
	ctrl := gomock.NewController(t)
	registry := steps.NewMockValidatorRegistry(ctrl)

	registry.EXPECT().
		UnregisterValidator(gomock.Any(), 2, uint64(0)).
		Return(fmt.Errorf("validator has no self-stake"))

	env := newTestEnv(registry, map[string]driver.Node{
		"heavy": validatorNode(ctrl, 2),
	})

	err := undelegateStep("heavy", "", nil).Run(t.Context(), env)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "heavy") {
		t.Errorf("expected the node name in the error, got: %v", err)
	}
}

// Undelegating more than was delegated is rejected on-chain. The local
// bookkeeping must not underflow into a huge expected stake when it happens.
func TestUndelegate_MoreThanDelegatedDoesNotUnderflowTheBookkeeping(t *testing.T) {
	ctrl := gomock.NewController(t)
	registry := steps.NewMockValidatorRegistry(ctrl)

	registry.EXPECT().FundDelegator(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
	registry.EXPECT().Delegate(gomock.Any(), 2, uint64(1_000), gomock.Any()).Return(nil)
	registry.EXPECT().UndelegateAs(gomock.Any(), 2, uint64(5_000), gomock.Any()).Return(nil)

	env := newTestEnv(registry, map[string]driver.Node{
		"heavy": validatorNode(ctrl, 2),
	})

	if err := delegateStep("heavy", "alice", 1_000).Run(t.Context(), env); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tooMuch := uint64(5_000)
	if err := undelegateStep("heavy", "alice", &tooMuch).Run(t.Context(), env); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := env.Stakes.Expected[steps.StakeKey{Delegator: "alice", ValidatorId: 2}]; got != 0 {
		t.Errorf("expected the tracked stake to be clamped to 0, got %d S", got)
	}
}

func TestVerifyStakes_StakeQueryFailureIsReported(t *testing.T) {
	ctrl := gomock.NewController(t)
	registry := steps.NewMockValidatorRegistry(ctrl)

	registry.EXPECT().FundDelegator(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
	registry.EXPECT().Delegate(gomock.Any(), 2, uint64(1_000), gomock.Any()).Return(nil)
	registry.EXPECT().
		GetDelegatorStake(gomock.Any(), 2).
		Return(uint64(0), fmt.Errorf("connection reset"))

	env := newTestEnv(registry, map[string]driver.Node{
		"heavy": validatorNode(ctrl, 2),
	})
	if err := delegateStep("heavy", "alice", 1_000).Run(t.Context(), env); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err := (&VerifyStakes{}).Run(t.Context(), env)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "query failed") {
		t.Errorf("unexpected error: %v", err)
	}
}

// A single account may stake on the same validator repeatedly: topping up an
// existing stake, reducing it in several steps, draining it and staking again
// from zero. All of it must go through one delegator account, and the tracked
// stake must follow the running total.
func TestDelegation_SingleAccountRepeatedlyDelegatesAndUndelegates(t *testing.T) {
	ctrl := gomock.NewController(t)
	registry := steps.NewMockValidatorRegistry(ctrl)

	const validatorId = 2
	var calls []string
	var funded []common.Address
	var signers []*ecdsa.PrivateKey

	registry.EXPECT().
		FundDelegator(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, address common.Address, amount uint64) error {
			calls = append(calls, fmt.Sprintf("fund %d", amount))
			funded = append(funded, address)
			return nil
		}).AnyTimes()
	registry.EXPECT().
		Delegate(gomock.Any(), validatorId, gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ int, stake uint64, key *ecdsa.PrivateKey) error {
			calls = append(calls, fmt.Sprintf("delegate %d", stake))
			signers = append(signers, key)
			return nil
		}).AnyTimes()
	registry.EXPECT().
		UndelegateAs(gomock.Any(), validatorId, gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ int, stake uint64, key *ecdsa.PrivateKey) error {
			calls = append(calls, fmt.Sprintf("undelegate %d", stake))
			signers = append(signers, key)
			return nil
		}).AnyTimes()

	env := newTestEnv(registry, map[string]driver.Node{
		"heavy": validatorNode(ctrl, validatorId),
	})

	// The sequence of scenarios/examples/repeated_delegation.yml.
	partial := func(stake uint64) *uint64 { return &stake }
	sequence := []struct {
		name      string
		step      steps.Step
		wantStake uint64
	}{
		{"initial stake", delegateStep("heavy", "cycler", 1_000_000), 1_000_000},
		{"top up", delegateStep("heavy", "cycler", 500_000), 1_500_000},
		{"first partial", undelegateStep("heavy", "cycler", partial(400_000)), 1_100_000},
		{"second partial", undelegateStep("heavy", "cycler", partial(100_000)), 1_000_000},
		{"drain", undelegateStep("heavy", "cycler", nil), 0},
		{"stake again from zero", delegateStep("heavy", "cycler", 750_000), 750_000},
		{"drain again", undelegateStep("heavy", "cycler", nil), 0},
	}

	for _, test := range sequence {
		if err := test.step.Run(t.Context(), env); err != nil {
			t.Fatalf("%s: unexpected error: %v", test.name, err)
		}
		got := env.Stakes.Expected[steps.StakeKey{Delegator: "cycler", ValidatorId: validatorId}]
		if got != test.wantStake {
			t.Errorf("%s: expected a tracked stake of %d S, got %d S", test.name, test.wantStake, got)
		}
	}

	// A full undelegation passes 0 down, letting the chain report the amount.
	want := []string{
		"fund " + fmt.Sprint(1_000_000+delegatorGasBudget), "delegate 1000000",
		"fund " + fmt.Sprint(500_000+delegatorGasBudget), "delegate 500000",
		"undelegate 400000",
		"undelegate 100000",
		"undelegate 0",
		"fund " + fmt.Sprint(750_000+delegatorGasBudget), "delegate 750000",
		"undelegate 0",
	}
	if !slices.Equal(calls, want) {
		t.Errorf("unexpected call sequence:\n got %v\nwant %v", calls, want)
	}

	// Every operation belongs to the same account: one tracked delegator,
	// one funded address, one signing key.
	if len(env.Stakes.Delegators) != 1 {
		t.Errorf("expected a single delegator account, got %d", len(env.Stakes.Delegators))
	}
	address := env.Stakes.Delegators["cycler"].Address
	for _, got := range funded {
		if got != address {
			t.Errorf("expected all funding to go to %s, got %s", address.Hex(), got.Hex())
		}
	}
	for _, key := range signers {
		if crypto.PubkeyToAddress(key.PublicKey) != address {
			t.Errorf("expected all operations to be signed by %s, got %s",
				address.Hex(), crypto.PubkeyToAddress(key.PublicKey).Hex())
		}
	}

	// After the last drain the chain must agree that nothing is staked.
	registry.EXPECT().GetDelegatorStake(address, validatorId).Return(uint64(0), nil)
	if err := (&VerifyStakes{}).Run(t.Context(), env); err != nil {
		t.Errorf("unexpected verification error: %v", err)
	}
}

// newTestEnv builds the minimal environment the staking steps operate on.
func newTestEnv(registry steps.ValidatorRegistry, nodes map[string]driver.Node) *steps.Env {
	env := steps.NewEnv(nil, nil, registry)
	for name, node := range nodes {
		env.Nodes[name] = node
	}
	return env
}

// validatorNode returns a node mock reporting the given validator id.
func validatorNode(ctrl *gomock.Controller, id int) driver.Node {
	node := driver.NewMockNode(ctrl)
	node.EXPECT().GetValidatorId().Return(&id).AnyTimes()
	return node
}

func delegateStep(node, delegator string, stake uint64) *Delegate {
	return &Delegate{Targets: []DelegateTarget{
		{Node: node, Delegator: delegator, Stake: stake},
	}}
}

func undelegateStep(node, delegator string, stake *uint64) *Undelegate {
	return &Undelegate{Targets: []UndelegateTarget{
		{Node: node, Delegator: delegator, Stake: stake},
	}}
}
