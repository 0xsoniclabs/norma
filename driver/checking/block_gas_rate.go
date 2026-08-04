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
	"log/slog"
	"math"
	"time"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/monitoring"
)

const defaultCeiling float64 = math.MaxFloat64

func init() {
	RegisterNetworkCheck("blockGasRate",
		func(net driver.Network, monitor *monitoring.Monitor) Checker {
			return &blockGasRateChecker{
				monitor:          &monitoringDataAdapter{monitor},
				ceiling:          defaultCeiling,
				toleranceSamples: defaultToleranceSamples,
			}
		})
}

// blockGasRateChecker is a Checker verifying that blocks stay below a gas rate
// ceiling. It observes the network forward in time: it notes the current chain
// head, waits for an observation window, and only inspects the blocks produced
// while it waited. Blocks from before the check are not re-examined, so a gas
// rate spike caused by an earlier transition - a rules update, or a restarted
// node catching up - cannot fail a check placed after a later step, nor every
// check for the rest of the scenario.
type blockGasRateChecker struct {
	monitor          MonitoringData
	ceiling          float64
	toleranceSamples int
	// duration overrides the observation window; when 0 it is derived from
	// toleranceSamples.
	duration time.Duration
}

// Configure returns a deep copy of the original checker.
// If the config doesn't provide any replacement value, copy from the value of the original.
// If the config is invalid, return error instead.
// If the config is nil, return original checker.
func (c *blockGasRateChecker) Configure(config CheckerConfig) Checker {
	if config == nil {
		return c
	}

	ceiling := c.ceiling
	if val, exist := config["ceiling"]; exist {
		ceiling = float64(val.(int))
	}

	tolerance := c.toleranceSamples
	if val, exist := config["tolerance"]; exist {
		tolerance = val.(int)
	}

	duration := c.duration
	if val, exist := config["duration"]; exist {
		duration = time.Duration(val.(int64))
	}

	return &blockGasRateChecker{
		monitor:          c.monitor,
		ceiling:          ceiling,
		toleranceSamples: tolerance,
		duration:         duration,
	}
}

// Check observes the network for the observation window and verifies that every
// block produced during it stayed at or below the gas rate ceiling.
func (c *blockGasRateChecker) Check(ctx context.Context) error {
	window, err := observationWindow(c.duration, c.toleranceSamples)
	if err != nil {
		return err
	}

	series := c.monitor.GetBlockGasRate()

	// The first block of interest is the one after the current head.
	firstBlock := monitoring.BlockNumber(0)
	if head := series.GetLatest(); head != nil {
		firstBlock = head.Position + 1
	}

	if err := sleep(ctx, window); err != nil {
		return err
	}

	head := series.GetLatest()
	if head == nil || head.Position < firstBlock {
		slog.Warn(
			"blockGasRate: no block was produced during the observation "+
				"window; there was no gas rate to check",
			"window", window,
			"from_block", firstBlock,
		)
		return nil
	}

	for _, point := range series.GetRange(firstBlock, head.Position+1) {
		if point.Value > c.ceiling {
			return fmt.Errorf(
				"exceeded gas ceiling; Block %d has gas rate of %f > %f",
				point.Position, point.Value, c.ceiling,
			)
		}
	}

	return nil
}
