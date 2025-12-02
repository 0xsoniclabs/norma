package genesis

import (
	"fmt"
	"strconv"
	"strings"
)

// ParseStakeString parses a stake string in the format "stake(,stake)*"
func ParseStakeString(stakeStr string) ([]uint64, error) {
	res := make([]uint64, 0)
	stakes := strings.Split(stakeStr, ",")
	for _, s := range stakes {

		stake, err := strconv.Atoi(s)
		if err != nil {
			return nil, fmt.Errorf("invalid stake format: %w", err)
		}
		res = append(res, uint64(stake))
	}
	return res, nil
}

// GetStakesString returns the stake of all validators as a string, which
// can be used in environment variables to initialize the genesis.
//
// This string has format "stake(,stake)*"
func GetStakesString(stakes []uint64) string {
	elems := make([]string, 0, len(stakes))
	for _, val := range stakes {
		elems = append(elems, fmt.Sprintf("%d", val))
	}
	return strings.Join(elems, ",")
}
