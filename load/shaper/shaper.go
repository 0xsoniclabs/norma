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

package shaper

import (
	"fmt"
	"time"
)

//go:generate mockgen -source shaper.go -destination shaper_mock.go -package shaper

// Shaper defines the shape of traffic to be produced by an application.
type Shaper interface {
	// Start notifies the shaper that processing is started at the given time.
	Start(time.Time)

	// GetNumMessagesInInterval provides the number of messages to be produced
	// in the given time interval. The result is expected to be >= 0.
	GetNumMessagesInInterval(start time.Time, duration time.Duration) float64
}

// ParseRate builds the shaper a scenario rate describes.
func ParseRate(rate *Rate) (Shaper, error) {
	// return default constant shaper if rate is not specified
	if rate == nil {
		return NewConstantShaper(0), nil
	}

	if rate.Constant != nil {
		return NewConstantShaper(float64(*rate.Constant)), nil
	}
	if rate.Slope != nil {
		return NewSlopeShaper(float64(rate.Slope.Start), float64(rate.Slope.Increment)), nil
	}
	if rate.Saturate {
		return Saturate{}, nil
	}

	return nil, fmt.Errorf("unknown rate type")
}

// Saturate is the shape of an application submitting transactions as fast as
// the network accepts them. It produces no messages on its own; the controller
// recognizes it and lets its users submit back to back.
type Saturate struct{}

func (Saturate) Start(time.Time) {}

func (Saturate) GetNumMessagesInInterval(time.Time, time.Duration) float64 {
	return 0
}
