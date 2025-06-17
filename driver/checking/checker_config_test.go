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

package checking

import (
	"strings"
	"testing"
)

func TestCheckerConfig_IsSupportedChecker(t *testing.T) {
	tests := []struct {
		typ      string
		expected bool
	}{
		{"block_height_checker", true},
		{"blocks_height_checker", false},
		{"block_hashes_checker", false},
		{"blocks_hashes_checker", true},
		{"block_rolling_checker", false},
		{"blocks_rolling_checker", true},
	}

	for _, test := range tests {
		t.Run(test.typ, func(t *testing.T) {
			res := IsSupportedChecker(test.typ)
			if res != test.expected {
				t.Errorf("IsSupportedChecker(%s) = %v, want %v", test.typ, res, test.expected)
			}
		})
	}
}

func TestCheckerConfig_IsCorrectlyConfigured(t *testing.T) {
	tests := []struct {
		typ      string
		config   map[string]string
		expected bool
		err      string
	}{
		{"block_height_checker", map[string]string{}, true, ""},
		{"block_height_checker", map[string]string{"slack": "5"}, true, ""},
		{"block_height_checker", map[string]string{"slack": "-1"}, false, "error parsing slack"},
		{"block_height_checker", map[string]string{"slack": "abcd"}, false, "error parsing slack"},
		{"blocks_hashes_checker", map[string]string{}, true, ""},
		{"blocks_hashes_checker", map[string]string{"random": "config"}, true, ""},
		{"blocks_rolling_checker", map[string]string{}, true, ""},
		{"blocks_rolling_checker", map[string]string{"tolerance": "10"}, true, ""},
		{"blocks_rolling_checker", map[string]string{"tolerance": "0"}, false, "minimum tolerance sample size is 5"},
		{"blocks_rolling_checker", map[string]string{"tolerance": "abcd"}, false, "error parsing slack"},
	}

	for _, test := range tests {
		t.Run(test.typ, func(t *testing.T) {
			if !IsSupportedChecker(test.typ) {
				t.Errorf("%s not supported", test.typ)
			}

			err := RegisterNetworkCheck("test", test.typ, test.config)
			if test.expected && err != nil {
				t.Errorf("%s should pass when configured with %v; got %v", test.typ, test.config, err)
			}
			if !test.expected && (err == nil || !strings.Contains(err.Error(), test.err)) {
				t.Errorf("expected error of type %s, got: %v", test.err, err)
			}

		})
	}
}
