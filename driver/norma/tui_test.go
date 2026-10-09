package main

import (
	"io"
	"log/slog"
	"math"
	"testing"
	"time"

	"github.com/0xsoniclabs/norma/driver/monitoring"

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

func TestPlotTab_LinesOfLateSubjectsStartWithGaps(t *testing.T) {
	p := newPlotTab("test", nil)
	p.add(map[string]float64{"a": 1})
	p.add(map[string]float64{"a": 2, "b": 3})
	p.add(map[string]float64{"b": 4})

	require.Equal(t, []string{"a", "b"}, p.subjects)
	require.Equal(t, []float64{1, 2}, p.lines[0][:2])
	require.True(t, math.IsNaN(p.lines[0][2]))
	require.True(t, math.IsNaN(p.lines[1][0]))
	require.Equal(t, []float64{3, 4}, p.lines[1][1:])
}

func TestRate_IsIncreasePerSecondOverWindow(t *testing.T) {
	series := &monitoring.SyncedSeries[monitoring.Time, float64]{}
	_, ok := rate(series)
	require.False(t, ok)

	start := time.Now().Add(-8 * time.Second)
	for i, value := range []float64{0, 100, 120, 140, 160, 180, 200} {
		require.NoError(t, series.Append(monitoring.NewTime(start.Add(time.Duration(i)*time.Second)), value))
	}
	got, ok := rate(series)
	require.True(t, ok)
	require.Equal(t, 20.0, got)

	stale := &monitoring.SyncedSeries[monitoring.Time, float64]{}
	require.NoError(t, stale.Append(monitoring.NewTime(start.Add(-time.Minute)), 0))
	require.NoError(t, stale.Append(monitoring.NewTime(start), 100))
	_, ok = rate(stale)
	require.False(t, ok, "no rate once the counter is no longer collected")

	require.NoError(t, series.Append(monitoring.NewTime(start.Add(7*time.Second)), 5))
	_, ok = rate(series)
	require.False(t, ok, "no rate across a counter reset")
}
