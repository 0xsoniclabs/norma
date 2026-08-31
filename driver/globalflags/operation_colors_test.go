package globalflags

import (
	"bytes"
	"context"
	"log/slog"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/0xsoniclabs/norma/driver/parser"
	"github.com/ethereum/go-ethereum/log"
	"github.com/stretchr/testify/require"
)

func TestOperationColors_OperationColor_CoversEveryStepFunction(t *testing.T) {
	require := require.New(t)

	for _, op := range parser.AllStepFunctions() {
		_, ok := operationColor(op)
		require.True(ok, "operation %q has no colour assigned", op)
	}
}

func TestOperationColors_OperationColor_AssignsUniqueColorPerOperation(t *testing.T) {
	require := require.New(t)

	seen := map[string]parser.StepFunction{}
	for _, op := range parser.AllStepFunctions() {
		color, ok := operationColor(op)
		require.True(ok, "operation %q has no colour assigned", op)

		previous, clash := seen[color]
		require.False(clash, "operations %q and %q share colour %q", previous, op, color)
		seen[color] = op
	}
}

func TestLogHandler_NewLogHandler_WritesPlainOutputWhenColorIsOff(t *testing.T) {
	require := require.New(t)

	buf := &bytes.Buffer{}
	logger := slog.New(newLogHandler(buf, false))

	logger.Info(string(parser.FuncKillSonic), "step", 1)

	require.NotContains(buf.String(), "\x1b[")
}

func TestOperationColorHandler_Handle_ColorsTheMessage(t *testing.T) {
	require := require.New(t)
	logger, buf := newTestLogger(t)

	logger.Info(string(parser.FuncHealDb), "step", 9, "identifier", "frail")

	color, ok := operationColor(parser.FuncHealDb)
	require.True(ok)
	require.Equal([]string{"healDb"}, coloredSpans(buf.String(), color))
}

func TestOperationColorHandler_Handle_LeavesRestOfLineUntouched(t *testing.T) {
	require := require.New(t)
	logger, buf := newTestLogger(t)

	logger.Info(string(parser.FuncHealDb), "step", 9, "identifier", "frail")

	color, ok := operationColor(parser.FuncHealDb)
	require.True(ok)

	head, rest, found := strings.Cut(buf.String(), color)
	require.True(found)
	require.Contains(head, "INFO")
	require.NotContains(head, "healDb")

	_, tail, found := strings.Cut(rest, colorReset)
	require.True(found)
	require.Contains(tail, "step")
	require.Contains(tail, "identifier")
	require.Contains(tail, "=frail")
	require.NotContains(tail, color)
}

func TestOperationColorHandler_Handle_ColorsEveryOperation(t *testing.T) {
	for _, op := range parser.AllStepFunctions() {
		t.Run(string(op), func(t *testing.T) {
			require := require.New(t)
			logger, buf := newTestLogger(t)

			logger.Info(string(op), "step", 1, "duration", "1s")

			color, ok := operationColor(op)
			require.True(ok)
			require.Equal([]string{string(op)}, coloredSpans(buf.String(), color))
		})
	}
}

func TestOperationColors_PhaseColor_AssignsUniqueColorPerPhase(t *testing.T) {
	require := require.New(t)

	started, ok := phaseColor("started")
	require.True(ok)
	completed, ok := phaseColor("completed")
	require.True(ok)
	require.NotEqual(started, completed)
}

func TestOperationColorHandler_Handle_ColorsThePhaseValueInThePhaseColor(t *testing.T) {
	for _, phase := range []string{"started", "completed"} {
		t.Run(phase, func(t *testing.T) {
			require := require.New(t)
			logger, buf := newTestLogger(t)

			logger.Info(string(parser.FuncStartNode), "step", 1, "phase", phase, "identifier", "validator-1")

			color, ok := phaseColor(phase)
			require.True(ok)
			require.Equal([]string{phase}, coloredSpans(buf.String(), color))
		})
	}
}

// A phase attribute on a line that names no operation is not a step phase and
// keeps its plain colour.
func TestOperationColorHandler_Handle_LeavesPhaseAloneOutsideOperationLines(t *testing.T) {
	require := require.New(t)
	logger, buf := newTestLogger(t)

	logger.Info("some other message", "phase", "started")

	color, ok := phaseColor("started")
	require.True(ok)
	require.NotContains(buf.String(), color)
}

func TestOperationColorHandler_Handle_UsesDifferentColorPerOperation(t *testing.T) {
	require := require.New(t)
	logger, buf := newTestLogger(t)

	logger.Info(string(parser.FuncHealDb), "step", 1)
	logger.Info(string(parser.FuncStartNode), "step", 2)

	healColor, ok := operationColor(parser.FuncHealDb)
	require.True(ok)
	startColor, ok := operationColor(parser.FuncStartNode)
	require.True(ok)

	healLine, startLine, found := strings.Cut(buf.String(), "\n")
	require.True(found)

	require.Equal([]string{"healDb"}, coloredSpans(healLine, healColor))
	require.NotContains(healLine, startColor)
	require.Equal([]string{"startNode"}, coloredSpans(startLine, startColor))
	require.NotContains(startLine, healColor)
}

// A line logged while an operation runs carries its own message and keeps its
// own colour.
func TestOperationColorHandler_Handle_DoesNotColorLaterLines(t *testing.T) {
	require := require.New(t)
	logger, buf := newTestLogger(t)

	logger.Info(string(parser.FuncHealDb), "step", 1)
	buf.Reset()
	logger.Info("Stopping sonicd", "node", "frail")

	color, ok := operationColor(parser.FuncHealDb)
	require.True(ok)
	require.NotContains(buf.String(), color)
}

// Setup, monitoring, and node output must come out as the terminal handler wrote it.
func TestOperationColorHandler_Handle_PassesLinesWithoutOperationThrough(t *testing.T) {
	require := require.New(t)

	wrapped := &bytes.Buffer{}
	logger := slog.New(newLogHandler(wrapped, true))

	plain := &bytes.Buffer{}
	reference := slog.New(log.NewTerminalHandler(plain, true))

	for _, emit := range []func(l *slog.Logger){
		func(l *slog.Logger) { l.Info("reading scenario file", "path", "scenario.yml") },
		func(l *slog.Logger) { l.Info("Stopping sonicd", "node", "frail") },
		func(l *slog.Logger) { l.Warn("node not responding", "node", "frail") },
		func(l *slog.Logger) { l.Error("step failed", "step", 7) },
	} {
		emit(logger)
		emit(reference)
	}

	require.Equal(stripTimestamps(plain.String()), stripTimestamps(wrapped.String()))
}

// A failure has to stay recognisable as one rather than read as a step.
func TestOperationColorHandler_Handle_LeavesWarningsAndErrorsToTheirLevel(t *testing.T) {
	tests := map[string]struct {
		emit func(l *slog.Logger)
	}{
		"warning": {emit: func(l *slog.Logger) { l.Warn(string(parser.FuncChecks), "step", 6) }},
		"error":   {emit: func(l *slog.Logger) { l.Error(string(parser.FuncChecks), "step", 6) }},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			require := require.New(t)
			logger, buf := newTestLogger(t)

			test.emit(logger)

			color, ok := operationColor(parser.FuncChecks)
			require.True(ok)
			require.NotContains(buf.String(), color)
		})
	}
}

// Check functions are not steps and have no colour of their own.
func TestOperationColorHandler_Handle_IgnoresMessagesThatAreNotOperations(t *testing.T) {
	require := require.New(t)
	logger, buf := newTestLogger(t)

	logger.Info(string(parser.FuncCheckBlockHashes), "step", 6)

	require.NotContains(buf.String(), "\x1b[38;5;")
}

func TestOperationColorHandler_Handle_SerializesConcurrentWrites(t *testing.T) {
	require := require.New(t)
	logger, buf := newTestLogger(t)
	handler := logger.Handler()

	const goroutines = 8
	const perGoroutine = 50
	stamp := time.Date(2026, 8, 19, 10, 0, 0, 0, time.UTC)

	done := make(chan struct{})
	for i := range goroutines {
		go func(i int) {
			defer func() { done <- struct{}{} }()

			message := "concurrent"
			if i%2 == 0 {
				message = string(parser.FuncStartNode)
			}
			for range perGoroutine {
				r := slog.NewRecord(stamp, slog.LevelInfo, message, 0)
				require.NoError(handler.Handle(context.Background(), r))
			}
		}(i)
	}
	for range goroutines {
		<-done
	}

	require.Equal(goroutines*perGoroutine, strings.Count(buf.String(), "\n"))
}

func TestOperationColorHandler_WithAttrs_KeepsColoringOperationLines(t *testing.T) {
	require := require.New(t)
	logger, buf := newTestLogger(t)

	logger.With("node", "frail").Info(string(parser.FuncStartNode), "step", 1)

	color, ok := operationColor(parser.FuncStartNode)
	require.True(ok)
	require.Equal([]string{"startNode"}, coloredSpans(buf.String(), color))
}

func TestMessageColorWriter_Write_ColorsOnlyTheMessage(t *testing.T) {
	const color = "\x1b[38;5;42m"

	tests := map[string]struct {
		message string
		line    string
		want    []string
	}{
		"message present": {
			message: "startNode",
			line:    "INFO [TIME] startNode    step=1 identifier=frail\n",
			want:    []string{"startNode"},
		},
		"message absent": {
			message: "startNode",
			line:    "INFO [TIME] other message step=1\n",
			want:    nil,
		},
		"name also in an attribute is left alone": {
			message: "startNode",
			line:    "INFO [TIME] startNode    node=startNode\n",
			want:    []string{"startNode"},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			require := require.New(t)

			out := &bytes.Buffer{}
			w := newMessageColorWriter(out)
			w.color, w.message = color, test.message

			n, err := w.Write([]byte(test.line))
			require.NoError(err)
			require.Equal(len(test.line), n)
			require.Equal(test.want, coloredSpans(out.String(), color))
		})
	}
}

func TestMessageColorWriter_Write_ColorsThePhaseValue(t *testing.T) {
	const color = "\x1b[38;5;42m"
	// The terminal handler writes a coloured attribute as
	// "<levelcolor>key\x1b[0m=value"; the writer keys on that shape.
	const attr = "\x1b[32mphase\x1b[0m="

	startedColor, ok := phaseColor("started")
	if !ok {
		t.Fatal("phase \"started\" has no colour assigned")
	}

	tests := map[string]struct {
		phase string
		line  string
		want  []string
	}{
		"phase present": {
			phase: "started",
			line:  "INFO [TIME] startNode    \x1b[32mstep\x1b[0m=1 " + attr + "started\n",
			want:  []string{"started"},
		},
		"phase absent from the line": {
			phase: "started",
			line:  "INFO [TIME] startNode    \x1b[32mstep\x1b[0m=1\n",
			want:  nil,
		},
		"no phase expected": {
			phase: "",
			line:  "INFO [TIME] startNode    " + attr + "started\n",
			want:  nil,
		},
		"phase without a colour is left alone": {
			phase: "resumed",
			line:  "INFO [TIME] startNode    " + attr + "resumed\n",
			want:  nil,
		},
		"value in another attribute is left alone": {
			phase: "started",
			line:  "INFO [TIME] startNode    \x1b[32mid\x1b[0m=started " + attr + "started\n",
			want:  []string{"started"},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			require := require.New(t)

			out := &bytes.Buffer{}
			w := newMessageColorWriter(out)
			w.color, w.message, w.phase = color, "startNode", test.phase

			n, err := w.Write([]byte(test.line))
			require.NoError(err)
			require.Equal(len(test.line), n)
			require.Equal(test.want, coloredSpans(out.String(), startedColor))
		})
	}
}

func TestMessageColorWriter_Write_PassesLineThroughWithoutColor(t *testing.T) {
	require := require.New(t)

	const line = "INFO [TIME] startNode step=1\n"

	out := &bytes.Buffer{}
	w := newMessageColorWriter(out)

	n, err := w.Write([]byte(line))
	require.NoError(err)
	require.Equal(len(line), n)
	require.Equal(line, out.String())
}

// newTestLogger returns a logger writing coloured output into a buffer.
func newTestLogger(t *testing.T) (*slog.Logger, *bytes.Buffer) {
	t.Helper()
	buf := &bytes.Buffer{}
	return slog.New(newLogHandler(buf, true)), buf
}

// coloredSpans returns the regions of a line the given colour applies to.
func coloredSpans(line, color string) []string {
	var spans []string
	rest := line
	for {
		i := strings.Index(rest, color)
		if i < 0 {
			return spans
		}
		rest = rest[i+len(color):]
		end := strings.Index(rest, colorReset)
		if end < 0 {
			return spans
		}
		spans = append(spans, rest[:end])
		rest = rest[end+len(colorReset):]
	}
}

// timestampPattern matches the stamp the terminal handler writes, so output from
// two loggers can be compared.
var timestampPattern = regexp.MustCompile(`\[\d\d-\d\d\|\d\d:\d\d:\d\d\.\d\d\d\]`)

func stripTimestamps(s string) string {
	return timestampPattern.ReplaceAllString(s, "[TIME]")
}
