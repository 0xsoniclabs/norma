package driver

import (
	"testing"

	"golang.org/x/exp/slices"
)

func TestGetValidatorStakes(t *testing.T) {
	tests := []struct {
		name       string
		validators Validators
		expected   []uint64
	}{
		{
			name:       "Empty validators",
			validators: Validators{},
			expected:   []uint64{},
		},
		{
			name: "Single validator uses explicit stake",
			validators: Validators{
				{Name: "validator1", Instances: 1, Stake: 42},
			},
			expected: []uint64{42},
		},
		{
			name: "Zero stake defaults to five million",
			validators: Validators{
				{Name: "validator1", Instances: 1, Stake: 0},
			},
			expected: []uint64{5_000_000},
		},
		{
			name: "Instances expand stake entries",
			validators: Validators{
				{Name: "validator1", Instances: 2, Stake: 10},
				{Name: "validator2", Instances: 3, Stake: 20},
			},
			expected: []uint64{10, 10, 20, 20, 20},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetValidatorStakes(tt.validators)
			if got, want := result, tt.expected; !slices.Equal(got, want) {
				t.Errorf("unexpected stakes: got %v, want %v", got, want)
			}
		})
	}
}
