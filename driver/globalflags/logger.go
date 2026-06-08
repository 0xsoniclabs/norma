package globalflags

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/ethereum/go-ethereum/log"
	"github.com/urfave/cli/v2"
)

// relativeTimeHandler wraps a slog.Handler and replaces each log record's
// timestamp with the elapsed time since this handler was created, so log
// output shows experiment-relative time rather than wall-clock time.
// The underlying terminal handler formats time as "MM-DD|HH:MM:SS.mmm", so
// the output will read "01-01|HH:MM:SS.mmm" for runs under 24 h, where the
// time part represents seconds elapsed since the experiment started.
type relativeTimeHandler struct {
	inner     slog.Handler
	startTime time.Time
}

func newRelativeTimeHandler(inner slog.Handler) *relativeTimeHandler {
	return &relativeTimeHandler{inner: inner, startTime: time.Now()}
}

func (h *relativeTimeHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h *relativeTimeHandler) Handle(ctx context.Context, r slog.Record) error {
	elapsed := max(r.Time.Sub(h.startTime), 0)
	// Shift the record time to Unix epoch + elapsed so the terminal handler's
	// "MM-DD|HH:MM:SS.mmm" format effectively shows "01-01|HH:MM:SS.mmm".
	r.Time = time.Unix(0, int64(elapsed)).UTC()
	return h.inner.Handle(ctx, r)
}

func (h *relativeTimeHandler) WithGroup(name string) slog.Handler {
	return &relativeTimeHandler{inner: h.inner.WithGroup(name), startTime: h.startTime}
}

func (h *relativeTimeHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &relativeTimeHandler{inner: h.inner.WithAttrs(attrs), startTime: h.startTime}
}

var (
	verbosityFlag = cli.IntFlag{
		Name:  "verbosity",
		Usage: "Changes logging verbosity: 0=silent, 1=error, 2=warn, 3=info, 4=debug, 5=detail",
		Value: 3,
	}

	vmoduleFlag = cli.StringFlag{
		Name: "vmodule",
		Usage: `Changes per-module verbosity:

                  The syntax of the argument is a comma-separated list of pattern=N, where the
                  pattern is a literal file name or "glob" pattern matching and N is a V level.

                  For instance:
                  - pattern="gopher.go=3"
                  sets the V level to 3 in all Go files named "gopher.go"
                  - pattern="foo=3"
                  sets V to 3 in all files of any packages whose import path ends in "foo"
                  - pattern="foo/*=3"
                  sets V to 3 in all files of any packages whose import path contains "foo"
`,
	}
)

var AllLoggerFlags = []cli.Flag{
	&verbosityFlag,
	&vmoduleFlag,
}

// SetupLogger sets up the logger for the application using the provided context.
func SetupLogger(ctx *cli.Context) error {

	output := io.Writer(os.Stdout)
	termHandler := log.NewTerminalHandler(output, true)
	handler := newRelativeTimeHandler(termHandler)
	glogger := log.NewGlogHandler(handler)

	verbosity := log.FromLegacyLevel(ctx.Int(verbosityFlag.Name))
	glogger.Verbosity(verbosity)
	vmodule := ctx.String(vmoduleFlag.Name)
	err := glogger.Vmodule(vmodule)
	if err != nil {
		return fmt.Errorf("failed to set --%s: %w", vmoduleFlag.Name, err)
	}

	log.SetDefault(log.NewLogger(glogger))

	return nil
}
