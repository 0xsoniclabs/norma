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

func TestResolveLabel_FindsTheNodeAStepMeans_WhenItIsUnambiguous(t *testing.T) {
	tests := map[string]struct {
		nodes     []string
		want      string
		wantError string
	}{
		"the name itself":             {nodes: []string{"val", "val-0"}, want: "val"},
		"the only instance":           {nodes: []string{"val-0"}, want: "val-0"},
		"the only remaining instance": {nodes: []string{"val-1", "val-extra"}, want: "val-1"},
		"no node":                     {nodes: []string{"val-extra", "other-0"}, wantError: `node "val" not found`},
		"several instances": {
			nodes:     []string{"val-2", "val-1"},
			wantError: `node "val" has multiple instances; use an explicit instance name (e.g. "val-1")`,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			nodes := map[string]bool{}
			for _, label := range test.nodes {
				nodes[label] = true
			}
			got, err := ResolveLabel(nodes, "val")
			if test.wantError != "" {
				require.ErrorContains(t, err, test.wantError)
				return
			}
			require.NoError(t, err)
			require.Equal(t, test.want, got)
		})
	}
}

func TestInstanceLabel_IsRecognisedAsAnInstanceOfItsBase(t *testing.T) {
	for _, instance := range []int{0, 1, 9, 10, 100} {
		label := InstanceLabel("val", instance)
		require.True(t, IsInstanceLabel(label, "val"), label)
	}
}
