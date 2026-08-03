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
	"time"
)

// sleep waits for d, or until ctx is done, whichever happens first. It returns
// ctx.Err() when the context ended before d elapsed, and nil when the full
// duration was waited. A non-positive d does not wait.
//
// Checkers that observe the network forward in time wait here rather than in
// time.Sleep, so that a cancelled scenario does not have to sit out the rest of
// an observation window. Unit tests run these waits inside a testing/synctest
// bubble, where the timer below is driven by the bubble's fake clock and a
// window of any length costs no wall-clock time.
func sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
