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
	"fmt"
	"math"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/monitoring"
)

const defaultCeiling float64 = math.MaxFloat64

func init() {
	RegisterNetworkCheck("block_gas_rate", func(net driver.Network, monitor *monitoring.Monitor) Checker {
		return &blockGasRateChecker{
			monitor: &monitoringDataAdapter{monitor},
			ceiling: defaultCeiling,
		}
	})
}

// blockGasRateChecker is a Checker checking if each block has gas below the ceiling
type blockGasRateChecker struct {
	monitor MonitoringData
	ceiling float64
}

// Configure returns a deep copy of the original checker.
// If the config doesn't provide any replacement value, copy from the value of the original.
// If the config is invalid, return error instead.
// If the config is nil, return original checker.
func (c *blockGasRateChecker) Configure(config CheckerConfig) (Checker, error) {
	if config == nil {
		return c, nil
	}

	toFloat64 := func(val any) (float64, error) {
		switch v := val.(type) {
		case float64:
			return v, nil
		case int:
			return float64(v), nil
		case uint64:
			return float64(v), nil
		default:
			return 0, fmt.Errorf("invalid type; %T", val)
		}
	}

	ceiling := c.ceiling
	val, exist := config["ceiling"]
	if exist {
		cl, err := toFloat64(val)
		if err != nil {
			return nil, fmt.Errorf("failed to convert ceiling; %v", err)
		}
		ceiling = cl
	}

	return &blockGasRateChecker{monitor: c.monitor, ceiling: ceiling}, nil
}

// Check retreive current BlockGasRate and see that each block has gas rate below ceiling.
func (c *blockGasRateChecker) Check() error {
	series := c.monitor.GetBlockGasRate()
	last := series.GetLatest()
	if last == nil {
		return nil // no blocks
	}

	items := series.GetRange(0, last.Position+1)
	for _, point := range items {
		if point.Value > c.ceiling {
			return fmt.Errorf("Exceeded gas ceiling; Block %d has gas rate of %f > %f", point.Position, point.Value, c.ceiling)
		}
	}

	return nil
}
