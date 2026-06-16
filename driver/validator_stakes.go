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

package driver

// GetValidatorStakes returns the list of validator stakes based on the provided
// configuration. If a validator has a stake of 0, it defaults to 5 million.
func GetValidatorStakes(validators Validators) []uint64 {
	if validators.GetNumValidators() == 0 {
		return []uint64{}
	}
	stakes := make([]uint64, 0, validators.GetNumValidators())
	for _, val := range validators {
		instances := max(val.Instances, 1)
		for range instances {
			if val.Stake == 0 {
				stakes = append(stakes, 5_000_000)
				continue
			}
			stakes = append(stakes, val.Stake)
		}
	}
	return stakes
}
