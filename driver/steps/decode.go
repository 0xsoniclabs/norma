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
	"fmt"
	"reflect"
	"strings"

	"gopkg.in/yaml.v3"
)

// Decode decodes a function written in a scenario into a new instance of the
// matching kind. It is either a bare name, as in `- advanceEpoch`, or a
// mapping holding the name as one key, whose value goes to DecodeValue, and
// the parameters as sibling keys. what names the kind of function in errors.
func Decode[T Function](node *yaml.Node, kinds []func() T, what string) (T, error) {
	var zero T
	switch node.Kind {
	case yaml.ScalarNode:
		fn, ok := lookup(kinds, node.Value)
		if !ok {
			return zero, fmt.Errorf("line %d: unknown %s function: %q", node.Line, what, node.Value)
		}
		return fn, nil
	case yaml.MappingNode:
	default:
		return zero, fmt.Errorf("line %d: %s must be a string or mapping", node.Line, what)
	}

	var fn T
	found := false
	var params []*yaml.Node
	for i := 0; i < len(node.Content); i += 2 {
		key, value := node.Content[i], node.Content[i+1]
		next, ok := lookup(kinds, key.Value)
		if !ok {
			params = append(params, key, value)
			continue
		}
		if found {
			return zero, fmt.Errorf("line %d: %s contains multiple function keys: %q and %q",
				key.Line, what, fn.Name(), next.Name())
		}
		fn, found = next, true
		if err := decodeValue(fn, value, what); err != nil {
			return zero, err
		}
	}
	if !found {
		return zero, fmt.Errorf("line %d: no known %s function found in mapping", node.Line, what)
	}
	return fn, decodeParams(fn, params, what)
}

// IsNull reports whether value is left empty, as in `- stopNode:`.
func IsNull(value *yaml.Node) bool {
	return value.Kind == yaml.ScalarNode && (value.Tag == "!!null" || value.Value == "")
}

// Param is a parameter of a function, as listed by scenario-help.
type Param struct {
	Key, Help string
}

// Params lists the parameters of fn in declaration order.
func Params(fn Function) []Param {
	var params []Param
	visitParams(reflect.ValueOf(fn).Elem(), func(key, help string, _ reflect.Value) {
		params = append(params, Param{key, help})
	})
	return params
}

func lookup[T Function](kinds []func() T, name string) (T, bool) {
	for _, kind := range kinds {
		if fn := kind(); fn.Name() == name {
			return fn, true
		}
	}
	var zero T
	return zero, false
}

func decodeValue(fn Function, value *yaml.Node, what string) error {
	if decoder, ok := fn.(ValueDecoder); ok {
		if err := decoder.DecodeValue(value); err != nil {
			return fmt.Errorf("line %d: %w", value.Line, err)
		}
		return nil
	}
	if value.Kind == yaml.MappingNode {
		return decodeParams(fn, value.Content, what)
	}
	if IsNull(value) {
		return nil
	}
	return fmt.Errorf("line %d: %s does not take a value, got %q", value.Line, fn.Name(), value.Value)
}

// decodeParams decodes key/value pairs into the parameter fields of fn,
// rejecting keys it does not declare.
func decodeParams(fn Function, pairs []*yaml.Node, what string) error {
	fields := map[string]reflect.Value{}
	visitParams(reflect.ValueOf(fn).Elem(), func(key, _ string, field reflect.Value) {
		fields[key] = field
	})
	for i := 0; i < len(pairs); i += 2 {
		key, value := pairs[i], pairs[i+1]
		field, ok := fields[key.Value]
		if !ok {
			return fmt.Errorf("line %d: parameter %q is not valid for %s %s",
				key.Line, key.Value, what, fn.Name())
		}
		if err := value.Decode(field.Addr().Interface()); err != nil {
			return fmt.Errorf("line %d: invalid %s value: %w", key.Line, key.Value, err)
		}
	}
	return nil
}

// visitParams calls visit for every field of v tagged `yaml`, descending into
// embedded structs.
func visitParams(v reflect.Value, visit func(key, help string, field reflect.Value)) {
	t := v.Type()
	for i := range t.NumField() {
		f := t.Field(i)
		key, _, _ := strings.Cut(f.Tag.Get("yaml"), ",")
		switch {
		case key != "" && key != "-":
			visit(key, f.Tag.Get("help"), v.Field(i))
		case f.Anonymous && f.Type.Kind() == reflect.Struct:
			visitParams(v.Field(i), visit)
		}
	}
}
