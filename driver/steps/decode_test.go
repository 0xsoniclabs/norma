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
	"gopkg.in/yaml.v3"
)

var testKinds = []func() Step{
	func() Step { return &StopNode{} },
	func() Step { return &CheckDb{} },
	func() Step { return &AdvanceEpoch{} },
}

func decodeStep(t *testing.T, input string) (Step, error) {
	t.Helper()
	var node yaml.Node
	require.NoError(t, yaml.Unmarshal([]byte(input), &node))
	return Decode(node.Content[0], testKinds, "step")
}

func TestDecode_ReadsValueAndParams(t *testing.T) {
	step, err := decodeStep(t, "checkDb: node-A\nmode: archive\n")
	require.NoError(t, err)
	require.Equal(t, &CheckDb{Node: Node{Node: "node-A"}, Mode: "archive"}, step)
}

func TestDecode_ReadsParamsListedBeforeTheFunctionKey(t *testing.T) {
	step, err := decodeStep(t, "mode: live\ncheckDb: node-A\n")
	require.NoError(t, err)
	require.Equal(t, &CheckDb{Node: Node{Node: "node-A"}, Mode: "live"}, step)
}

func TestDecode_ReadsParamsNestedUnderAFunctionWithoutValue(t *testing.T) {
	step, err := decodeStep(t, "advanceEpoch: {}\n")
	require.NoError(t, err)
	require.Equal(t, &AdvanceEpoch{}, step)
}

func TestDecode_AcceptsABareName(t *testing.T) {
	step, err := decodeStep(t, "advanceEpoch\n")
	require.NoError(t, err)
	require.Equal(t, &AdvanceEpoch{}, step)
}

func TestDecode_ReportsInvalidInputWithItsLine(t *testing.T) {
	tests := map[string]struct {
		input     string
		wantError string
	}{
		"unknown name":           {"bogus\n", `line 1: unknown step function: "bogus"`},
		"no function key":        {"mode: live\n", "line 1: no known step function"},
		"two function keys":      {"stopNode: a\ncheckDb: a\n", `line 2: step contains multiple function keys: "stopNode" and "checkDb"`},
		"unknown parameter":      {"checkDb: a\n\nfile: x\n", `line 3: parameter "file" is not valid for step checkDb`},
		"value on a bare step":   {"advanceEpoch: now\n", `line 1: advanceEpoch does not take a value, got "now"`},
		"parameter of bad type":  {"checkDb: a\nmode: [live]\n", "line 2: invalid mode value"},
		"name that is not a str": {"stopNode: [a, b]\n", "line 1: value must be a name"},
		"sequence as a step":     {"- stopNode\n", "line 1: step must be a string or mapping"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := decodeStep(t, test.input)
			require.ErrorContains(t, err, test.wantError)
		})
	}
}

func TestParams_ListsEmbeddedParamsInDeclarationOrder(t *testing.T) {
	params := Params(&ExportGenesis{})
	require.Len(t, params, 1)
	require.Equal(t, "file", params[0].Key)
	require.NotEmpty(t, params[0].Help)

	require.Empty(t, Params(&StopNode{}))
}
