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
	"strconv"
)

// init registers all currently supported checkers
func init() {
	registerCheckerType("block_height_checker", blockHeightCheckerFactory)
	registerCheckerType("blocks_hashes_checker", blocksHashesCheckerFactory)
	registerCheckerType("blocks_rolling_checker", blocksRollingCheckerFactory)

	registerDefaultNetworkCheck("default_block_height", "block_height_checker",
		map[string]string{"slack": strconv.Itoa(defaultSlack)},
	)
	registerDefaultNetworkCheck("default_blocks_hashes", "blocks_hashes_checker",
		map[string]string{},
	)
	registerDefaultNetworkCheck("default_blocks_rolling", "blocks_rolling_checker",
		map[string]string{"tolerance": strconv.Itoa(defaultTolerance)},
	)
}

// these support adding custom configurations into each Checker Factory
type configuredFactory func(config map[string]string) (Factory, error)
type configuredFactoryRegistry map[string]configuredFactory

// supportedChecker is a map of currently supported checkers
var supportedChecker = make(configuredFactoryRegistry)

// IsSupportedChecker returns true if the given key is a supported checker
func IsSupportedChecker(key string) bool {
	_, ok := supportedChecker[key]
	return ok
}

// registerCheckerType registers a new supported checker
func registerCheckerType(typ string, factory configuredFactory) {
	supportedChecker[typ] = factory
}

var blockHeightCheckerFactory = func(config map[string]string) (Factory, error) {
	var slack uint8 = defaultSlack

	// if not configured, simply use default value - else overwrite it
	val, exist := config["slack"]
	if exist {
		s, err := strconv.ParseUint(val, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("error parsing slack; %v", err)
		}
		slack = uint8(s)
	}

	return NewBlockHeightChecker(slack), nil
}

var blocksHashesCheckerFactory = func(config map[string]string) (Factory, error) {
	return NewBlockHashesChecker(), nil
}

var blocksRollingCheckerFactory = func(config map[string]string) (Factory, error) {
	var tolerance uint8 = defaultTolerance

	// if not configured, simply use default value - else overwrite it
	val, exist := config["tolerance"]
	if exist {
		t, err := strconv.ParseUint(val, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("error parsing slack; %v", err)
		}
		tolerance = uint8(t)
	}

	if tolerance < 5 {
		return nil, fmt.Errorf("minimum tolerance sample size is 5")
	}

	return NewBlockRollingChecker(tolerance), nil
}
