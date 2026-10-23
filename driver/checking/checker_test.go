package checking

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/0xsoniclabs/norma/genesis"
	"go.uber.org/mock/gomock"
)

func TestCheckerConfig_Success(t *testing.T) {
	configs := []CheckerConfig{
		{"failing": true},
		{"tolerance": 1},
		{"duration": int64(1)},
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
		{"duration": -1},
		{"duration": 1},
		{"duration": int64(-1)},
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

func TestFailingChecker_PassesOnlyWhenTheAssertionFails(t *testing.T) {
	tests := map[string]struct {
		checkErr error
		wantErr  string
	}{
		"assertion fails":    {checkErr: assertionf("network is down")},
		"wrapped assertion":  {checkErr: fmt.Errorf("gave up: %w", assertionf("too old block"))},
		"check passes":       {wantErr: "failure expected"},
		"node not reachable": {checkErr: errors.New("connection refused"), wantErr: "could not decide"},
		"context cancelled":  {checkErr: context.Canceled, wantErr: "could not decide"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			checker := NewMockChecker(ctrl)
			checker.EXPECT().Check(gomock.Any()).Return(test.checkErr)

			err := NewFailingChecker(checker).Check(t.Context())
			if test.wantErr == "" {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Errorf("expected an error containing %q, got %v", test.wantErr, err)
			}
		})
	}
}
