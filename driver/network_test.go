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

package driver

import "testing"

func TestResolveClientImage(t *testing.T) {
	tests := []struct {
		imageName, goVersion, want string
	}{
		{"", "", "sonic:local"},
		{"sonic:v2.2.0", "", "sonic:v2.2.0"},
		{"", "1.27.0", "sonic:local_go1.27.0"},
		{"sonic:v2.2.0", "1.27rc2", "sonic:v2.2.0_go1.27rc2"},
		{"sonic", "1.27.0", "sonic:latest_go1.27.0"},
		{"alpine:latest", "", "alpine:latest"},
	}
	for _, tt := range tests {
		if got := ResolveClientImage(tt.imageName, tt.goVersion); got != tt.want {
			t.Errorf("ResolveClientImage(%q, %q) = %q, want %q",
				tt.imageName, tt.goVersion, got, tt.want)
		}
	}
}

func TestResolveClientImage_DefaultGoVersionAppliesToCandidateOnly(t *testing.T) {
	SetDefaultClientGoVersion("1.27.0")
	t.Cleanup(func() { SetDefaultClientGoVersion("") })

	if got, want := ResolveClientImage("", ""), "sonic:local_go1.27.0"; got != want {
		t.Errorf("candidate should be swept: got %q, want %q", got, want)
	}
	if got, want := ResolveClientImage("sonic:v2.2.0", ""), "sonic:v2.2.0"; got != want {
		t.Errorf("pinned image should be untouched: got %q, want %q", got, want)
	}
	if got, want := ResolveClientImage("", "1.28.0"), "sonic:local_go1.28.0"; got != want {
		t.Errorf("scenario goVersion should win over the flag: got %q, want %q", got, want)
	}
}
