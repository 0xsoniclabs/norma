package genesis

import (
	"reflect"
	"strings"
	"testing"
)

func TestValidatorStakes_CanBeParsed(t *testing.T) {
	tests := map[string]struct {
		input    string
		expected []uint64
	}{
		"one validator": {
			input: "500",
			expected: []uint64{
				500,
			},
		},
		"multiple validators": {
			input: "500,600,700",
			expected: []uint64{
				500,
				600,
				700,
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			result, err := ParseStakeString(tc.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if !reflect.DeepEqual(result, tc.expected) {
				t.Fatalf("expected %v, got %v", tc.expected, result)
			}
		})
	}
}

func TestValidatorStakes_ReturnsErrors(t *testing.T) {
	tests := map[string]string{
		"empty string":        "",
		"invalid format":      "1-5000000",
		"invalid stake value": "fiveMillion",
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := ParseStakeString(test)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !strings.Contains(err.Error(), "invalid syntax") {
				t.Fatalf("expected error contains %q, got %v", "invalid syntax", err)
			}
		})
	}
}
