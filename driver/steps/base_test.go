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

package steps

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsInstanceLabel_AcceptsOnlyLabelsStartNodeGives(t *testing.T) {
	tests := map[string]bool{
		"val-0":      true,
		"val-12":     true,
		"val":        false, // the base name itself is not an instance
		"val-extra":  false,
		"val--1":     false,
		"val-01":     false,
		"val-+1":     false,
		"val-1-2":    false,
		"val-1.5":    false,
		"other-0":    false,
		"value-0":    false,
		"val-0-real": false,
	}
	for label, want := range tests {
		require.Equal(t, want, IsInstanceLabel(label, "val"), label)
	}
}

func TestInstanceLabel_IsRecognisedAsAnInstanceOfItsBase(t *testing.T) {
	for _, instance := range []int{0, 1, 9, 10, 100} {
		label := InstanceLabel("val", instance)
		require.True(t, IsInstanceLabel(label, "val"), label)
	}
}
