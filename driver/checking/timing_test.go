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
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"
)

func TestSleep_WaitsForTheFullDuration(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		start := time.Now()
		if err := sleep(t.Context(), time.Hour); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if elapsed := time.Since(start); elapsed != time.Hour {
			t.Errorf("slept %v, want an hour", elapsed)
		}
	})
}

func TestSleep_ReturnsWhenTheContextEnds(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		start := time.Now()
		if err := sleep(ctx, time.Hour); !errors.Is(err, context.Canceled) {
			t.Errorf("got %v, want a context error", err)
		}
		if elapsed := time.Since(start); elapsed != 0 {
			t.Errorf("waited %v on an ended context, want no wait", elapsed)
		}
	})
}

func TestSleep_ReturnsWhenTheContextEndsMidWait(t *testing.T) {
	// The wait has to be given up as soon as the scenario is cancelled, rather
	// than at the end of the requested duration.
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		go func() {
			time.Sleep(time.Minute)
			cancel()
		}()

		start := time.Now()
		if err := sleep(ctx, time.Hour); !errors.Is(err, context.Canceled) {
			t.Errorf("got %v, want a context error", err)
		}
		if elapsed := time.Since(start); elapsed != time.Minute {
			t.Errorf("waited %v, want to stop after a minute", elapsed)
		}
	})
}

func TestSleep_DoesNotWaitForNonPositiveDurations(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		for _, d := range []time.Duration{0, -time.Second} {
			start := time.Now()
			if err := sleep(t.Context(), d); err != nil {
				t.Errorf("unexpected error for duration %v: %v", d, err)
			}
			if elapsed := time.Since(start); elapsed != 0 {
				t.Errorf("waited %v for duration %v", elapsed, d)
			}
		}
	})
}

func TestObservationWindow_PrefersAnExplicitDuration(t *testing.T) {
	got, err := observationWindow(42*time.Second, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := 42 * time.Second; got != want {
		t.Errorf("window is %v, want the explicit %v", got, want)
	}
}

func TestObservationWindow_DerivesFromToleranceSamples(t *testing.T) {
	got, err := observationWindow(0, 7)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := 7 * blockSampleInterval; got != want {
		t.Errorf("window is %v, want %v", got, want)
	}
}

func TestObservationWindow_RejectsUnusableInputs(t *testing.T) {
	tests := map[string]struct {
		duration  time.Duration
		tolerance int
	}{
		"no duration and no tolerance": {0, 0},
		"negative tolerance":           {0, -1},
		"duration below two samples":   {blockSampleInterval, 0},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := observationWindow(
				test.duration, test.tolerance,
			); err == nil {
				t.Errorf("expected an error, got nil")
			}
		})
	}
}
