package globalflags

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"sync"

	"github.com/0xsoniclabs/norma/driver/parser"
	"github.com/ethereum/go-ethereum/log"
)

// newLogHandler builds the handler the application logs through. When the output
// can carry colour, a line whose message is an operation has that message
// printed in the operation's colour; the rest of the line, and every line that
// names no operation, is left to go-ethereum's terminal handler exactly as
// before.
func newLogHandler(output io.Writer, useColor bool) slog.Handler {
	if !useColor {
		return log.NewTerminalHandler(output, false)
	}

	writer := newMessageColorWriter(output)
	return newOperationColorHandler(log.NewTerminalHandler(writer, true), writer)
}

// operationColorHandler prints the message of a log line that names a node
// operation in that operation's colour, giving each operation its own.
//
// The executor logs a step by using the operation as the message, so the
// terminal handler's message column becomes a colour-coded column of step names.
// Lines logged in between by the node, network, load, and check packages carry
// messages of their own and are passed through untouched.
type operationColorHandler struct {
	inner  slog.Handler
	writer *messageColorWriter
	// mu makes selecting a colour and writing the line it applies to atomic
	// against the many goroutines that log concurrently during a run. It is
	// shared with every handler derived through WithAttrs or WithGroup, since
	// they all write through the same writer.
	mu *sync.Mutex
}

// newOperationColorHandler wraps inner so that operation lines are coloured.
// inner must write to writer, and must emit one record per Write, as
// go-ethereum's TerminalHandler does by formatting each record into a buffer and
// writing it in a single call.
func newOperationColorHandler(inner slog.Handler, writer *messageColorWriter) slog.Handler {
	return &operationColorHandler{
		inner:  inner,
		writer: writer,
		mu:     &sync.Mutex{},
	}
}

func (h *operationColorHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h *operationColorHandler) Handle(ctx context.Context, r slog.Record) error {
	// Warnings and errors keep the level colouring the terminal handler gives
	// them. Tinting a failure in the colour of the step it happened in would mute
	// the one line the reader most needs to notice.
	color := ""
	if r.Level < slog.LevelWarn {
		color, _ = operationColor(parser.StepFunction(r.Message))
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	if color != "" {
		h.writer.color = color
		h.writer.message = r.Message
		defer func() {
			h.writer.color, h.writer.message = "", ""
		}()
	}

	return h.inner.Handle(ctx, r)
}

func (h *operationColorHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &operationColorHandler{
		inner:  h.inner.WithAttrs(attrs),
		writer: h.writer,
		mu:     h.mu,
	}
}

func (h *operationColorHandler) WithGroup(name string) slog.Handler {
	return &operationColorHandler{
		inner:  h.inner.WithGroup(name),
		writer: h.writer,
		mu:     h.mu,
	}
}

// messageColorWriter colours the message inside an already formatted log line,
// leaving every other part of it untouched.
//
// color and message are set and cleared by operationColorHandler.Handle while it
// holds the shared lock, which is also what serialises the Write in between. An
// empty colour passes the line through byte for byte.
type messageColorWriter struct {
	out     io.Writer
	color   string
	message string
}

func newMessageColorWriter(out io.Writer) *messageColorWriter {
	return &messageColorWriter{out: out}
}

func (w *messageColorWriter) Write(p []byte) (int, error) {
	if w.color == "" || w.message == "" {
		return w.out.Write(p)
	}

	// The terminal handler writes "<level>[<time>] <message>", and the time
	// format contains no ']', so the message follows the first "] ". Operation
	// names are plain identifiers, so the handler never quotes or escapes them
	// and the message appears verbatim. Requiring it to follow keeps a line
	// formatted differently from being rewritten by accident.
	const prefix = "] "
	i := bytes.Index(p, []byte(prefix+w.message))
	if i < 0 {
		return w.out.Write(p)
	}

	start := i + len(prefix)
	end := start + len(w.message)

	buf := make([]byte, 0, len(p)+len(w.color)+len(colorReset))
	buf = append(buf, p[:start]...)
	buf = append(buf, w.color...)
	buf = append(buf, p[start:end]...)
	buf = append(buf, colorReset...)
	buf = append(buf, p[end:]...)

	if _, err := w.out.Write(buf); err != nil {
		return 0, err
	}
	// Report the caller's length, not the recoloured one, so it sees a full write.
	return len(p), nil
}
