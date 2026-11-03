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

package network

import (
	context "context"
	"fmt"
	"testing"
	"time"
)

func TestRegisterDuplicatedPortsForServices(t *testing.T) {
	service := ServiceDescription{
		Name:     "OperaPprof",
		Port:     6060,
		Protocol: "http",
	}

	serviceGroup := ServiceGroup{}

	if err := serviceGroup.RegisterService(&service); err != nil {
		t.Errorf("first registration must succeed")
	}

	if err := serviceGroup.RegisterService(&service); err == nil {
		t.Errorf("first registration must fail")
	}

}

func TestRetry(t *testing.T) {
	t.Parallel()

	var count int
	err := Retry(t.Context(), 5, 1*time.Millisecond,
		func(context.Context) error {
			count++
			if count >= 5 {
				return nil
			} else {
				return fmt.Errorf("no time to end yet")
			}
		})

	if err != nil {
		t.Errorf("Retry should success eventually")
	}

	if got, want := count, 5; got < want {
		t.Errorf("Retry finished early: %d < %d", got, want)
	}
}
