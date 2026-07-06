package checking

import (
	"fmt"
	"testing"

	"github.com/0xsoniclabs/norma/genesis"
)

func TestCheckerConfig_Success(t *testing.T) {
	configs := []CheckerConfig{
		{"failing": true},
		{"tolerance": 1},
		{"start": int64(1)},
		{"ceiling": 1},
		{"slack": 1},
		{"rules": map[string]any{"Blocks": map[string]any{"MaxBlockGas": 1}}},
		{"rules": genesis.NetworkRulesPatch{Blocks: &genesis.BlocksPatch{MaxBlockGas: new(uint64(1))}}},
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
		{"start": 1},
		{"start": int64(-1)},
		{"ceiling": nil},
		{"rules": []any{"invalid"}},
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
