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
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// nodeStep takes a node name as its value and a mode parameter.
type nodeStep struct {
	Node
	Mode string `yaml:"mode" help:"A mode."`
}

func (*nodeStep) Name() string                    { return "nodeStep" }
func (*nodeStep) Help() string                    { return "A step on a node." }
func (*nodeStep) Run(context.Context, *Env) error { return nil }

// bareStep takes no value and no parameters.
type bareStep struct{}

func (*bareStep) Name() string                    { return "bareStep" }
func (*bareStep) Help() string                    { return "A step without arguments." }
func (*bareStep) Validate() error                 { return nil }
func (*bareStep) Run(context.Context, *Env) error { return nil }

var testKinds = []func() Step{
	func() Step { return &nodeStep{} },
	func() Step { return &bareStep{} },
}

func decodeStep(t *testing.T, input string) (Step, error) {
	t.Helper()
	var node yaml.Node
	require.NoError(t, yaml.Unmarshal([]byte(input), &node))
	return Decode(node.Content[0], testKinds, "step")
}

func TestDecode_ReadsValueAndParams(t *testing.T) {
	step, err := decodeStep(t, "nodeStep: node-A\nmode: archive\n")
	require.NoError(t, err)
	require.Equal(t, &nodeStep{Node: Node{Node: "node-A"}, Mode: "archive"}, step)
}

func TestDecode_ReadsParamsListedBeforeTheFunctionKey(t *testing.T) {
	step, err := decodeStep(t, "mode: live\nnodeStep: node-A\n")
	require.NoError(t, err)
	require.Equal(t, &nodeStep{Node: Node{Node: "node-A"}, Mode: "live"}, step)
}

func TestDecode_ReadsParamsNestedUnderAFunctionWithoutValue(t *testing.T) {
	step, err := decodeStep(t, "bareStep: {}\n")
	require.NoError(t, err)
	require.Equal(t, &bareStep{}, step)
}

func TestDecode_AcceptsABareName(t *testing.T) {
	step, err := decodeStep(t, "bareStep\n")
	require.NoError(t, err)
	require.Equal(t, &bareStep{}, step)
}

func TestDecode_ReportsInvalidInputWithItsLine(t *testing.T) {
	tests := map[string]struct {
		input     string
		wantError string
	}{
		"unknown name":           {"bogus\n", `line 1: unknown step function: "bogus"`},
		"no function key":        {"mode: live\n", "line 1: no known step function"},
		"two function keys":      {"nodeStep: a\nbareStep:\n", `line 2: step contains multiple function keys: "nodeStep" and "bareStep"`},
		"unknown parameter":      {"nodeStep: a\n\nfile: x\n", `line 3: parameter "file" is not valid for step nodeStep`},
		"value on a bare step":   {"bareStep: now\n", `line 1: bareStep does not take a value, got "now"`},
		"parameter of bad type":  {"nodeStep: a\nmode: [live]\n", "line 2: invalid mode value"},
		"name that is not a str": {"nodeStep: [a, b]\n", "line 1: value must be a name"},
		"sequence as a step":     {"- nodeStep\n", "line 1: step must be a string or mapping"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := decodeStep(t, test.input)
			require.ErrorContains(t, err, test.wantError)
		})
	}
}

func TestParams_ListsTaggedFieldsAndSkipsTheValue(t *testing.T) {
	require.Equal(t, []Param{{Key: "mode", Help: "A mode."}}, Params(&nodeStep{}))
	require.Empty(t, Params(&bareStep{}))
}
