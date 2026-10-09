package main

import (
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStepHandler_ReportsStartedSteps(t *testing.T) {
	var started []int
	logger := slog.New(stepHandler{slog.NewTextHandler(io.Discard, nil), func(step int) {
		started = append(started, step)
	}})

	logger.Info("startNode", "step", 1, "phase", "started", "identifier", "A")
	logger.Info("startNode", "step", 1, "phase", "completed")
	logger.Info("waitFor", "step", 2, "phase", "started")
	logger.Info("other", "step", "x", "phase", "started")
	logger.Info("progress update")

	require.Equal(t, []int{0, 1}, started)
}
