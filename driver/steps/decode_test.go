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

// paramStep takes no value, only parameters, flat or nested under its key.
type paramStep struct {
	Mode string `yaml:"mode" help:"A mode."`
}

func (*paramStep) Name() string                    { return "paramStep" }
func (*paramStep) Help() string                    { return "A step with parameters only." }
func (*paramStep) Validate() error                 { return nil }
func (*paramStep) Run(context.Context, *Env) error { return nil }

var testKinds = []func() Step{
	func() Step { return &nodeStep{} },
	func() Step { return &bareStep{} },
	func() Step { return &paramStep{} },
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
		"parameter given twice":  {"nodeStep: a\nmode: x\nmode: y\n", `line 3: parameter "mode" is given twice`},
		"nested and flat":        {"paramStep: {mode: x}\nmode: y\n", `line 2: parameter "mode" is given twice`},
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

func TestDecode_ReadsParamsNestedUnderTheFunctionKey(t *testing.T) {
	step, err := decodeStep(t, "paramStep:\n  mode: x\n")
	require.NoError(t, err)
	require.Equal(t, &paramStep{Mode: "x"}, step)
}

type target struct {
	Node  string  `yaml:"node"`
	Stake *uint64 `yaml:"stake,omitempty"`
}

func decodeTarget(t *testing.T, input string) (target, error) {
	t.Helper()
	var node yaml.Node
	require.NoError(t, yaml.Unmarshal([]byte(input), &node))
	var out target
	err := DecodeStrict(node.Content[0], &out, "a target")
	return out, err
}

func TestDecodeStrict_DecodesDeclaredFields(t *testing.T) {
	got, err := decodeTarget(t, "node: val\nstake: 100\n")
	require.NoError(t, err)
	require.Equal(t, "val", got.Node)
	require.Equal(t, uint64(100), *got.Stake)
}

func TestDecodeStrict_RejectsWhatYamlWouldIgnore(t *testing.T) {
	tests := map[string]struct {
		input     string
		wantError string
	}{
		"misspelled key": {"node: val\nstkae: 100\n", `line 2: parameter "stkae" is not valid for a target`},
		"key twice":      {"node: a\nnode: b\n", `line 2: parameter "node" is given twice`},
		"not a mapping":  {"val\n", "line 1: a target must be a mapping"},
		"bad value":      {"node: val\nstake: lots\n", "line 2: invalid stake value"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := decodeTarget(t, test.input)
			require.ErrorContains(t, err, test.wantError)
		})
	}
}
