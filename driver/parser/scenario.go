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

package parser

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/0xsoniclabs/norma/driver/steps"
	"github.com/0xsoniclabs/norma/driver/steps/checks"
	"github.com/0xsoniclabs/norma/driver/steps/network"
	"github.com/0xsoniclabs/norma/genesis"
	"gopkg.in/yaml.v3"
)

// Scenario is the root element of a scenario description: an ordered list of
// blocking steps.
type Scenario struct {
	Name             string                    `yaml:"Name"`
	Description      string                    `yaml:"Description"`
	InitialRules     genesis.NetworkRulesPatch `yaml:"InitialNetworkRules"`
	DisableEndChecks bool                      `yaml:"DisableEndChecks,omitempty"`
	Timeouts         steps.Timeouts            `yaml:"Timeouts,omitempty"`
	Steps            Steps                     `yaml:"Scenario"`
}

// Steps is the ordered list of steps of a scenario.
type Steps []steps.Step

func (s *Steps) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.SequenceNode {
		return fmt.Errorf("line %d: Scenario must be a list of steps", value.Line)
	}
	for _, item := range value.Content {
		step, err := steps.Decode(item, stepKinds, "step")
		if err != nil {
			return err
		}
		*s = append(*s, step)
	}
	return nil
}

// Parse parses a scenario from the given reader.
func Parse(reader io.Reader) (Scenario, error) {
	var res Scenario
	decoder := yaml.NewDecoder(reader)
	decoder.KnownFields(true)
	err := decoder.Decode(&res)
	if err != nil {
		return Scenario{}, err
	}
	res.setDefaults()
	return res, nil
}

// ParseBytes parses a scenario from a byte slice.
func ParseBytes(data []byte) (Scenario, error) {
	return Parse(bytes.NewReader(data))
}

// ParseFile parses a scenario from a file.
func ParseFile(path string) (scenario Scenario, err error) {
	reader, err := os.Open(path)
	if err != nil {
		return Scenario{}, err
	}
	defer func() { err = errors.Join(err, reader.Close()) }()
	return Parse(reader)
}

// setDefaults sets default values on the scenario.
func (s *Scenario) setDefaults() {
	ensureDefaultEpochDuration(&s.InitialRules)

	if !s.DisableEndChecks {
		s.Steps = append(s.Steps,
			&network.AdvanceEpoch{},
			&network.AdvanceEpoch{},
			&checks.Checks{Items: []checks.Check{
				&checks.BlockHashes{},
				&checks.BlockHeights{},
			}},
		)
	}
}

// PrintHelp writes a formatted summary of all available scenario step
// functions, their descriptions, and accepted parameters to w. It returns
// the first write error encountered, if any.
func PrintHelp(w io.Writer) error {
	ew := &errWriter{w: w}
	ew.printf("Scenario step functions:\n\n")
	for _, step := range sortedByName(stepKinds) {
		params := steps.Params(step)
		ew.printf("  %-26s %s\n", step.Name(), step.Help())
		for _, p := range params {
			ew.printf("      %-22s %s\n", p.Key+":", p.Help)
		}
		_, isChecks := step.(*checks.Checks)
		if isChecks {
			ew.printf("\n    Available checks:\n")
			for _, check := range sortedByName(checks.Kinds) {
				ew.printf("      %-22s %s\n", check.Name(), check.Help())
				for _, p := range steps.Params(check) {
					ew.printf("          %-18s %s\n", p.Key+":", p.Help)
				}
			}
		}
		if len(params) > 0 || isChecks {
			ew.printf("\n")
		}
	}
	return ew.err
}

func sortedByName[T steps.Function](kinds []func() T) []T {
	fns := make([]T, 0, len(kinds))
	for _, kind := range kinds {
		fns = append(fns, kind())
	}
	slices.SortFunc(fns, func(a, b T) int {
		return strings.Compare(a.Name(), b.Name())
	})
	return fns
}

// errWriter wraps an io.Writer and records the first write error so that
// callers can chain multiple writes without checking each one individually.
type errWriter struct {
	w   io.Writer
	err error
}

func (ew *errWriter) printf(format string, args ...any) {
	if ew.err != nil {
		return
	}
	_, ew.err = fmt.Fprintf(ew.w, format, args...)
}
