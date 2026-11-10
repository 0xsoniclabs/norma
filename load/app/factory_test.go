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
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRegisterApplication_TypeIsSupported_WhenRegistered(t *testing.T) {
	require.False(t, IsSupportedApplicationType("registered-test"))
	errFactory := errors.New("factory called")
	RegisterApplication("Registered-Test", func(AppContext, uint32, uint32) (Application, error) {
		return nil, errFactory
	})
	t.Cleanup(func() { delete(registeredFactories, "registered-test") })

	require.True(t, IsSupportedApplicationType("REGISTERED-TEST"))
	require.True(t, IsSupportedApplicationType("counter"))
	_, err := NewApplication("registered-test", nil, 0, 0)
	require.ErrorIs(t, err, errFactory)
}
