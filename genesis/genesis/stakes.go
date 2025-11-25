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
