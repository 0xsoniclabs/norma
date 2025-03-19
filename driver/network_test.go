package driver

import (
	"github.com/0xsoniclabs/norma/driver/parser"
	"golang.org/x/exp/slices"
	"testing"
)

var one int = 1
var two int = 2
var three int = 3

func TestNewValidator(t *testing.T) {
	tests := []struct {
		name     string
		input    parser.Validator
		expected Validator
	}{
		{
			name: "Default values",
			input: parser.Validator{
				Name: "validator1",
			},
			expected: Validator{
				Name:      "validator1",
				Instances: 1,
				ImageName: DefaultClientDockerImageName,
			},
		},
		{
			name: "Custom image name",
			input: parser.Validator{
				Name:      "validator2",
				ImageName: "custom-image",
			},
			expected: Validator{
				Name:      "validator2",
				Instances: 1,
				ImageName: "custom-image",
			},
		},
		{
			name: "Custom instances",
			input: parser.Validator{
				Name:      "validator3",
				Instances: &three,
			},
			expected: Validator{
				Name:      "validator3",
				Instances: 3,
				ImageName: DefaultClientDockerImageName,
			},
		},
		{
			name: "Failing validator",
			input: parser.Validator{
				Name:    "validator1",
				Failing: true,
			},
			expected: Validator{
				Name:      "validator1",
				Failing:   true,
				Instances: 1,
				ImageName: DefaultClientDockerImageName,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NewValidator(tt.input)
			if got, want := result, tt.expected; got != want {
				t.Errorf("unexpected validator: got %v, want %v", got, want)
			}
		})
	}
}

func TestNewValidators(t *testing.T) {
	tests := []struct {
		name     string
		input    []parser.Validator
		expected Validators
	}{
		{
			name:  "Empty validator list with default values",
			input: []parser.Validator{},
			expected: []Validator{
				{Name: "validator", Instances: 1, ImageName: DefaultClientDockerImageName},
			},
		},
		{
			name: "Single validator with default values",
			input: []parser.Validator{
				{Name: "validator1"},
			},
			expected: []Validator{
				{Name: "validator1", Instances: 1, ImageName: DefaultClientDockerImageName},
			},
		},
		{
			name: "Multiple validators with custom values",
			input: []parser.Validator{
				{Name: "validator1", Instances: &two, ImageName: "custom-image1"},
				{Name: "validator2", ImageName: "custom-image2"},
			},
			expected: []Validator{
				{Name: "validator1", Instances: 2, ImageName: "custom-image1"},
				{Name: "validator2", Instances: 1, ImageName: "custom-image2"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NewValidators(tt.input)
			if got, want := result, tt.expected; !slices.Equal(got, want) {
				t.Errorf("unexpected validators: got %v, want %v", got, want)
			}
		})
	}
}

func TestGetNumValidators(t *testing.T) {
	tests := []struct {
		name     string
		input    Validators
		expected int
	}{
		{
			name:     "Empty validator list",
			input:    NewValidators([]parser.Validator{}),
			expected: 1, // creates one default validator
		},
		{
			name: "Single validator with default instances",
			input: NewValidators([]parser.Validator{
				{Name: "validator1"}}),
			expected: 1,
		},
		{
			name: "Multiple validators with custom instances",
			input: NewValidators([]parser.Validator{
				{Name: "validator1", Instances: &two},
				{Name: "validator2", Instances: &three},
			}),
			expected: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.input.GetNumValidators()
			if got, want := result, tt.expected; got != want {
				t.Errorf("unexpected number of validators: got %v, want %v", got, want)
			}
		})
	}
}
