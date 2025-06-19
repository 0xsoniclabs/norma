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
	"strings"
	"testing"
)

func TestParseEmpty(t *testing.T) {
	_, err := ParseBytes([]byte{})
	if err == nil {
		t.Fatal("parsing of empty input should have failed")
	}
}

var minimalExample = `
name: Minimal Example
`

func TestParseMinimalExample(t *testing.T) {
	_, err := ParseBytes([]byte(minimalExample))
	if err != nil {
		t.Fatalf("parsing of the minimal example should have worked, got %v", err)
	}
}

var unknownKeyExample = minimalExample + `
some_other_key: with a value
`

func TestParseFailsOnUnknownKey(t *testing.T) {
	_, err := ParseBytes([]byte(unknownKeyExample))
	if err == nil {
		t.Fatalf("parsing of the example with unknown key should have failed")
	}
	if !strings.Contains(err.Error(), "some_other_key") {
		t.Errorf("error message should have named the invalid key")
	}
}

// smallExample defines a small scenario including instances of most
// configuration options.
var smallExample = `
name: Small Test

# Initial validator nodes in the network.
validators:
  - name: validator-1
  - name: validator-2
    imagename: "sonic:v2.0.2"
  - name: validator-3
    instances: 2
    imagename: "sonic:v2.0.1"
  - name: validator-4
    instances: 3
    imagename: "sonic"

nodes:
  - name: A
    instances: 10
    start: 5
    end: 7.5

applications:
  - name: lottery
    instances: 10
    start: 7
    end: 10
    rate:
      constant: 8

  - name: my_coin
    rate:
      slope:
        start: 5
        increment: 1

  - name: game
    rate:
      wave:
        min: 10
        max: 20
        period: 120
`

func TestParseSmallExampleWorks(t *testing.T) {
	_, err := ParseBytes([]byte(smallExample))
	if err != nil {
		t.Fatalf("parsing of input failed: %v", err)
	}
}

// withClientType defines an example with client specification
var withClientType = `
name: Small Test
validators:
  - name: validator-1
nodes:
  - name: A
    instances: 10
    start: 5
    end: 7.5
    client:
      imagename: main
      type: validator
      data_volume: abcd 	

applications:
  - name: lottery
    instances: 10
    start: 7
    end: 10
    rate:
      constant: 8

  - name: my_coin
    rate:
      slope:
        start: 5
        increment: 1

  - name: game
    rate:
      wave:
        min: 10
        max: 20
        period: 120
`

func TestParseWithClientTypeWorks(t *testing.T) {
	scenario, err := ParseBytes([]byte(withClientType))
	if err != nil {
		t.Fatalf("parsing of input failed: %v", err)
	}

	if got, want := scenario.Nodes[0].Client.ImageName, "main"; got != want {
		t.Errorf("unexpected value: got: %v, want: %v", got, want)
	}
	if got, want := scenario.Nodes[0].Client.Type, "validator"; got != want {
		t.Errorf("unexpected value: got: %v, want: %v", got, want)
	}
	if got, want := *scenario.Nodes[0].Client.DataVolume, "abcd"; got != want {
		t.Errorf("unexpected value: got: %v, want: %v", got, want)
	}
}

var withCheats = smallExample + `

cheats:
  - name: hello
    start: 8
`

func TestParseExampleWithCheats(t *testing.T) {
	_, err := ParseBytes([]byte(withCheats))
	if err != nil {
		t.Fatalf("parsing of input failed: %v", err)
	}
}

// for advance_epoch, epochs can be omitted and will default to 1
var withAdvanceEpoch = smallExample + `

advance_epoch:
  - time: 25
    epochs: 2
  - time: 50 
`

func TestParseExampleWithAdvanceEpoch(t *testing.T) {
	_, err := ParseBytes([]byte(withAdvanceEpoch))
	if err != nil {
		t.Fatalf("parsing of input failed: %v", err)
	}
}

var withChecks = smallExample + `

checks:
  - time: 25
    check: block_rolling
  - time: 50 
    check: block_rolling
`

func TestParseExampleWithChecks(t *testing.T) {
	_, err := ParseBytes([]byte(withChecks))
	if err != nil {
		t.Fatalf("parsing of input failed: %v", err)
	}
}

func TestNetwork_Rules(t *testing.T) {
	scenario, err := ParseBytes([]byte(networkRulesPayload))
	if err != nil {
		t.Fatalf("parsing of input failed: %v", err)
	}

	if got, want := scenario.NetworkRules.Genesis["MAX_BLOCK_GAS"], "20500000000"; got != want {
		t.Errorf("unexpected value: got: %v, want: %v", got, want)
	}

	if got, want := scenario.NetworkRules.Genesis["MAX_EPOCH_GAS"], "1500000000000"; got != want {
		t.Errorf("unexpected value: got: %v, want: %v", got, want)
	}

	if got, want := scenario.NetworkRules.Updates[0].Time, float32(10); got != want {
		t.Errorf("unexpected value: got: %v, want: %v", got, want)
	}

	if got, want := scenario.NetworkRules.Updates[0].Rules["MAX_BLOCK_GAS"], "20500000001"; got != want {
		t.Errorf("unexpected value: got: %v, want: %v", got, want)
	}

	if got, want := scenario.NetworkRules.Updates[1].Time, float32(30); got != want {
		t.Errorf("unexpected value: got: %v, want: %v", got, want)
	}

	if got, want := scenario.NetworkRules.Updates[1].Rules["MAX_EPOCH_GAS"], "1500000000002"; got != want {
		t.Errorf("unexpected value: got: %v, want: %v", got, want)
	}

	if got, want := scenario.NetworkRules.Updates[1].Rules["MAX_EPOCH_DURATION"], "10s"; got != want {
		t.Errorf("unexpected value: got: %v, want: %v", got, want)
	}
}

var networkRulesPayload = `
name: Network Rules Example

network_rules:
  genesis:
      MAX_BLOCK_GAS: 20500000000
      MAX_EPOCH_GAS: 1500000000000
      YET_ANOTHER_RULE: abcd
  updates:
      - time: 10
        rules:
          MAX_BLOCK_GAS: 20500000001
          YET_ANOTHER_RULE: abcde
      - time: 30
        rules:
          MAX_EPOCH_GAS: 1500000000002
          MAX_EPOCH_DURATION: 10s
          YET_ANOTHER_RULE: abcdef

`
