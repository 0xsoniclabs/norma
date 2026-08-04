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
	"fmt"
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

// minObservationSamples is the smallest number of monitoring samples an
// observation window must be able to hold. Deciding whether a block height
// changed takes two samples, so a shorter window could only ever report that
// nothing was observed.
const minObservationSamples = 2

// observationWindow returns how long a checker should observe the network. An
// explicit duration wins; otherwise the window is derived from a tolerance
// expressed in monitoring samples. The result is never so short that the
// monitor could not take enough samples to see a change.
func observationWindow(
	duration time.Duration, toleranceSamples int,
) (time.Duration, error) {
	window := duration
	if window <= 0 {
		if toleranceSamples <= 0 {
			return 0, fmt.Errorf(
				"tolerance must be > 0, got %d", toleranceSamples,
			)
		}
		window = time.Duration(toleranceSamples) * blockSampleInterval
	}

	if minimum := minObservationSamples * blockSampleInterval; window < minimum {
		return 0, fmt.Errorf(
			"observation window of %s is shorter than the %s needed to "+
				"collect %d monitoring samples",
			window, minimum, minObservationSamples,
		)
	}
	return window, nil
}
