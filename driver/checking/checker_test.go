package checking

import (
	"fmt"
	"testing"
)

func TestCheckerConfig_Success(t *testing.T) {
	configs := []CheckerConfig{
		{"failing": true},
		{"tolerance": 1},
		{"start": 1},
		{"ceiling": 1},
		{"slack": 1},
	}

	for i, config := range configs {
		t.Run(fmt.Sprintf("TestCheckerConfig_Success_%d", i), func(t *testing.T) {
			t.Parallel()
			if err := config.Check(); err != nil {
				t.Errorf("unexpected error: %s", err)
			}
		})
	}
}

func TestCheckerConfig_Failure(t *testing.T) {
	configs := []CheckerConfig{
		{"failing": "false"},
		{"tolerance": 1.0},
		{"start": -1},
		{"ceiling": nil},
	}

	for i, config := range configs {
		t.Run(fmt.Sprintf("TestCheckerConfig_Failure_%d", i), func(t *testing.T) {
			t.Parallel()
			if err := config.Check(); err == nil {
				t.Errorf("expected failure when checking %+v", config)
			}
		})
	}
}
