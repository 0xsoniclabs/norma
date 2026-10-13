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
	"bytes"
	"errors"
	"fmt"
	"reflect"
	"regexp"
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
		nested, err := decodeValue(fn, value)
		if err != nil {
			return zero, err
		}
		params = append(params, nested...)
	}
	if !found {
		return zero, fmt.Errorf("line %d: no known %s function found in mapping", node.Line, what)
	}
	return fn, decodeFields(reflect.ValueOf(fn).Elem(), params, what+" "+fn.Name())
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

// decodeValue decodes the value under the function key, returning the
// parameters it holds when it is a mapping of them.
func decodeValue(fn Function, value *yaml.Node) ([]*yaml.Node, error) {
	if decoder, ok := fn.(ValueDecoder); ok {
		if err := decoder.DecodeValue(value); err != nil {
			return nil, fmt.Errorf("line %d: %w", value.Line, err)
		}
		return nil, nil
	}
	if value.Kind == yaml.MappingNode {
		return value.Content, nil
	}
	if IsNull(value) {
		return nil, nil
	}
	return nil, fmt.Errorf("line %d: %s does not take a value, got %q", value.Line, fn.Name(), value.Value)
}

// DecodeStrict decodes a mapping into the fields of out tagged `yaml`.
// Unlike yaml.Node.Decode it rejects keys out does not declare and keys given
// twice, as the rest of a scenario file does; owner names out in errors.
func DecodeStrict(value *yaml.Node, out any, owner string) error {
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("line %d: %s must be a mapping", value.Line, owner)
	}
	return decodeFields(reflect.ValueOf(out).Elem(), value.Content, owner)
}

// decodeFields decodes key/value pairs into the fields of v tagged `yaml`,
// rejecting keys v does not declare and keys given twice.
func decodeFields(v reflect.Value, pairs []*yaml.Node, owner string) error {
	fields := map[string]reflect.Value{}
	visitParams(v, func(key, _ string, field reflect.Value) {
		fields[key] = field
	})
	seen := map[string]bool{}
	for i := 0; i < len(pairs); i += 2 {
		key, value := pairs[i], pairs[i+1]
		field, ok := fields[key.Value]
		if !ok {
			return fmt.Errorf("line %d: parameter %q is not valid for %s", key.Line, key.Value, owner)
		}
		if seen[key.Value] {
			return fmt.Errorf("line %d: parameter %q is given twice", key.Line, key.Value)
		}
		seen[key.Value] = true
		if err := DecodeKnown(value, field.Addr().Interface()); err != nil {
			return fmt.Errorf("line %d: invalid %s value: %w", key.Line, key.Value, err)
		}
	}
	return nil
}

// DecodeKnown decodes value into out like yaml.Node.Decode, but rejects
// mapping keys out does not declare at any depth, which yaml.Node.Decode
// silently ignores.
func DecodeKnown(value *yaml.Node, out any) error {
	data, err := yaml.Marshal(value)
	if err != nil {
		return err
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	err = decoder.Decode(out)
	// The lines yaml reports are relative to value, so they are dropped in
	// favour of the caller's.
	var typeErr *yaml.TypeError
	if errors.As(err, &typeErr) {
		messages := make([]string, len(typeErr.Errors))
		for i, message := range typeErr.Errors {
			messages[i] = relativeLine.ReplaceAllString(message, "")
		}
		return errors.New(strings.Join(messages, "; "))
	}
	return err
}

var relativeLine = regexp.MustCompile(`^line \d+: `)

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
