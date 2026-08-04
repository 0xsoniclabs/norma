// Copyright 2026 Fantom Foundation
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

package app

import (
	"fmt"
	"strings"
	"testing"
)

func TestNewGenerators_WithoutATypeYieldsTheFullMix(t *testing.T) {
	instances, err := NewGenerators("", 0, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := len(instances), len(generators); got != want {
		t.Errorf("expected %d generators, got %d", want, got)
	}
	for _, instance := range instances {
		if instance.Generator == nil {
			t.Errorf("generator %q was not created", instance.Name)
		}
		if instance.Weight <= 0 {
			t.Errorf("generator %q has a non-positive weight %d", instance.Name, instance.Weight)
		}
	}
}

func TestNewGenerators_WithATypeYieldsThatGeneratorOnly(t *testing.T) {
	for _, name := range LoadTypes() {
		t.Run(name, func(t *testing.T) {
			instances, err := NewGenerators(name, 0, 0)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(instances) != 1 {
				t.Fatalf("expected exactly one generator, got %d", len(instances))
			}
			if instances[0].Name != name {
				t.Errorf("expected generator %q, got %q", name, instances[0].Name)
			}
		})
	}
}

func TestNewGenerators_TypeIsCaseInsensitive(t *testing.T) {
	instances, err := NewGenerators("CounTer", 0, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if instances[0].Name != "counter" {
		t.Errorf("expected the counter generator, got %q", instances[0].Name)
	}
}

func TestNewGenerators_RejectsUnknownTypes(t *testing.T) {
	_, err := NewGenerators("does-not-exist", 0, 0)
	if err == nil {
		t.Fatal("expected an error for an unknown type")
	}
	if !strings.Contains(err.Error(), "does-not-exist") {
		t.Errorf("expected the error to name the type, got %v", err)
	}
}

func TestNewGenerators_GivesEveryGeneratorItsOwnAccounts(t *testing.T) {
	// Loads running side by side must not have their generators share an account,
	// or their nonce sequences would collide.
	owner := map[uint32]string{}
	for _, appId := range []uint32{0, 1, 2, 17} {
		instances, err := NewGenerators("", 0, appId)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		for _, instance := range instances {
			label := fmt.Sprintf("%s of load %d", instance.Name, appId)
			if previous, taken := owner[instance.AppId]; taken {
				t.Errorf("%s shares the account namespace %d with %s",
					label, instance.AppId, previous)
			}
			owner[instance.AppId] = label
		}
	}
}

func TestNewGenerators_SelectedGeneratorKeepsItsNamespace(t *testing.T) {
	// Selecting one generator must place its accounts where the full mix would,
	// so that a targeted load and a mixed load never collide.
	mixed, err := NewGenerators("", 0, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, instance := range mixed {
		selected, err := NewGenerators(instance.Name, 0, 3)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if selected[0].AppId != instance.AppId {
			t.Errorf("generator %q has namespace %d when selected alone and %d within the mix",
				instance.Name, selected[0].AppId, instance.AppId)
		}
	}
}

func TestIsSupportedLoadType(t *testing.T) {
	if !IsSupportedLoadType("") {
		t.Error("expected the empty type standing for the full mix to be supported")
	}
	for _, name := range LoadTypes() {
		if !IsSupportedLoadType(name) {
			t.Errorf("expected %q to be supported", name)
		}
	}
	if IsSupportedLoadType("does-not-exist") {
		t.Error("expected an unknown type to be unsupported")
	}
}
