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

package app

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestLoadParameter_OnlyPriorityLanesCarryAnotherLoad(t *testing.T) {
	// A lane is not a kind of load, it is a property of one, so it is the only
	// type asking which load it should carry.
	require.True(t, AcceptsLoadParameter("priority"))
	require.True(t, AcceptsLoadParameter("PRIORITY"), "types are case-insensitive")
	for _, appType := range []string{"counter", "uniswap", "largecontract", "mix", ""} {
		require.False(t, AcceptsLoadParameter(appType),
			"the %s load is its own, so it takes no load parameter", appType)
	}
}

func TestNewApplication_RejectsALoadTheTypeCannotCarry(t *testing.T) {
	ctrl := gomock.NewController(t)
	ctxt := NewMockAppContext(ctrl)

	// Nothing is created, so the context is never used; the parameters alone
	// decide these.
	_, err := NewApplication("counter", "uniswap", ctxt, 0, 0)
	require.ErrorContains(t, err, "generates its own load")

	_, err = NewApplication("priority", "nonesuch", ctxt, 0, 0)
	require.ErrorContains(t, err, "unknown load")

	_, err = NewApplication("priority", "priority", ctxt, 0, 0)
	require.ErrorContains(t, err, "cannot be nested")
}

func TestPriorityApplication_RegistersTheAccountsOfEveryUser(t *testing.T) {
	first, second := newTestAccount(t, 1), newTestAccount(t, 2)
	users := []User{
		&CounterUser{sender: first},
		&CounterUser{sender: second},
	}

	// The registration itself talks to the chain, so this test stops at the
	// accounts the application collected to register.
	app := &PriorityApplication{load: "counter"}
	collected, err := app.signingAccounts(users)
	require.NoError(t, err)
	require.Equal(t, []*Account{first, second}, collected)
}

func TestPriorityApplication_RefusesALoadThatDoesNotDiscloseItsAccounts(t *testing.T) {
	ctrl := gomock.NewController(t)

	// A user that signs with accounts it creates while running - a bundle or a
	// composed load - cannot be registered up front, and prioritizing a part of
	// its traffic would quietly measure something else than it claims to.
	opaque := NewMockUser(ctrl)
	app := &PriorityApplication{load: "mix"}

	_, err := app.signingAccounts([]User{opaque})
	require.ErrorContains(t, err, "cannot be prioritized")
	require.ErrorContains(t, err, "mix", "the error should name the load that was asked for")
}

func newTestAccount(t *testing.T, id int) *Account {
	t.Helper()
	const fakeNetworkID = 0xfa3
	factory, err := NewAccountFactory(big.NewInt(fakeNetworkID), 0, uint32(id))
	require.NoError(t, err)
	account, err := factory.CreateAccount(nil)
	require.NoError(t, err)
	return account
}
