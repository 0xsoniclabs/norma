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

package txmon

import (
	"slices"
	"time"
)

// maxSamplesPerSet bounds how many measurements of one duration of one
// application are retained. A run produces one measurement per transaction,
// which is more than a report can show and more than a distribution needs; the
// limit keeps the exported data proportional to what is read from it.
const maxSamplesPerSet = 10_000

// sampleSet is a bounded selection of measurements. Once it is full, every
// second retained sample is dropped and only every second following sample is
// accepted, repeatedly - so the retained samples stay spread over the whole run
// instead of describing only its beginning.
type sampleSet struct {
	samples []Sample
	// seen counts the offered samples, stride how many of them are currently
	// worth one retained sample.
	seen   int
	stride int
}

// add offers a measurement to the set.
func (s *sampleSet) add(at time.Time, duration time.Duration) {
	if s.stride == 0 {
		s.stride = 1
	}
	s.seen++
	if s.seen%s.stride != 0 {
		return
	}

	s.samples = append(s.samples, Sample{At: at, Duration: duration})
	if len(s.samples) < maxSamplesPerSet {
		return
	}

	// Halve the retained samples and accept half as many from now on, which
	// leaves the set with a uniform selection of everything offered so far.
	kept := s.samples[:0]
	for i, sample := range s.samples {
		if i%2 == 0 {
			kept = append(kept, sample)
		}
	}
	s.samples = kept
	s.stride *= 2
}

// sorted returns the retained samples ordered by the moment they were measured
// from. Measurements are offered in the order transactions reach a phase, which
// is not the order they were submitted in.
func (s *sampleSet) sorted() []Sample {
	sorted := slices.Clone(s.samples)
	slices.SortFunc(sorted, func(a, b Sample) int {
		return a.At.Compare(b.At)
	})
	return sorted
}
