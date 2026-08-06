// Copyright 2024 Fantom Foundation
// This file is part of Norma System Testing Infrastructure for Sonic.
//
// Norma is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// Norma is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with Norma. If not, see <http://www.gnu.org/licenses/>.

package docker

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"sync"
)

// logReplayLines is the number of most recent lines retained so that a
// subscriber attaching after the process has already produced output
// still sees recent context. It matches the buffer size documented on
// network.Host.StreamLog.
const logReplayLines = 150

// logSubscriberBuffer bounds how far a single subscriber may fall
// behind. Once exceeded, lines are dropped for that subscriber only, so
// one stalled consumer cannot block the exec output stream (and with it
// the client process writing to it).
const logSubscriberBuffer = 8192

// logBroadcaster fans a single line-oriented byte stream out to any
// number of independent io.ReadCloser subscribers.
//
// It exists because the client process runs as a `docker exec` rather
// than as the container's entrypoint, so its output is not part of the
// container's stdout and therefore not retrievable via ContainerLogs.
// The broadcaster is owned by the Container and outlives individual
// exec invocations, so subscribers keep working across a client
// restart.
type logBroadcaster struct {
	mu          sync.Mutex
	subscribers map[*logSubscription]struct{}
	replay      [][]byte
	closed      bool
}

func newLogBroadcaster() *logBroadcaster {
	return &logBroadcaster{
		subscribers: make(map[*logSubscription]struct{}),
		replay:      make([][]byte, 0, logReplayLines),
	}
}

// publish forwards one line (including its trailing newline, if any) to
// every current subscriber and records it in the replay buffer. The
// caller must not retain or mutate line afterwards.
func (b *logBroadcaster) publish(line []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}

	if len(b.replay) == logReplayLines {
		copy(b.replay, b.replay[1:])
		b.replay = b.replay[:logReplayLines-1]
	}
	b.replay = append(b.replay, line)

	for sub := range b.subscribers {
		sub.offer(line)
	}
}

// close releases all subscribers, causing pending and future Read calls
// to return io.EOF once buffered lines are drained.
func (b *logBroadcaster) close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	b.closed = true
	for sub := range b.subscribers {
		close(sub.lines)
		delete(b.subscribers, sub)
	}
}

// subscribe returns a reader that yields the replayed tail followed by
// every subsequently published line. The caller must Close it.
//
// The reader follows the stream and therefore only reports EOF once the
// broadcaster is closed, i.e. when the container stops. Callers that must
// not wait that long pass a bounding ctx: once it is done, the reader
// drains what is already buffered and then reports EOF.
func (b *logBroadcaster) subscribe(ctx context.Context) io.ReadCloser {
	b.mu.Lock()
	defer b.mu.Unlock()

	sub := &logSubscription{
		broadcaster: b,
		ctx:         ctx,
		lines:       make(chan []byte, logSubscriberBuffer),
		done:        make(chan struct{}),
	}
	for _, line := range b.replay {
		sub.offer(line)
	}
	if b.closed {
		close(sub.lines)
		return sub
	}
	b.subscribers[sub] = struct{}{}
	return sub
}

// logSubscription is a single consumer of a logBroadcaster, exposed as
// an io.ReadCloser so it is interchangeable with the ContainerLogs
// stream it replaces.
type logSubscription struct {
	broadcaster *logBroadcaster
	// ctx bounds how long the reader follows the stream; nil means follow
	// until the broadcaster is closed.
	ctx       context.Context
	lines     chan []byte
	pending   []byte
	dropped   int
	closeOnce sync.Once
	done      chan struct{}
}

// offer enqueues a line without blocking. The broadcaster lock is held
// by the caller.
func (s *logSubscription) offer(line []byte) {
	select {
	case s.lines <- line:
	default:
		s.dropped++
	}
}

// Read blocks until at least one line is available, mirroring the
// tailing semantics of the docker log stream. It reports EOF when the
// broadcaster is closed, when this subscription is closed, or when the
// bounding context is done and no buffered lines remain.
func (s *logSubscription) Read(p []byte) (int, error) {
	for len(s.pending) == 0 {
		// Always drain what is already buffered, so a caller bounding the
		// read with a deadline still receives the lines produced so far.
		select {
		case line, ok := <-s.lines:
			if !ok {
				return 0, io.EOF
			}
			s.pending = line
			continue
		default:
		}

		var ctxDone <-chan struct{}
		if s.ctx != nil {
			ctxDone = s.ctx.Done()
		}
		select {
		case line, ok := <-s.lines:
			if !ok {
				return 0, io.EOF
			}
			s.pending = line
		case <-s.done:
			return 0, io.EOF
		case <-ctxDone:
			return 0, io.EOF
		}
	}
	n := copy(p, s.pending)
	s.pending = s.pending[n:]
	return n, nil
}

// Close unsubscribes this reader. It is safe to call more than once.
func (s *logSubscription) Close() error {
	s.closeOnce.Do(func() {
		b := s.broadcaster
		b.mu.Lock()
		delete(b.subscribers, s)
		dropped := s.dropped
		b.mu.Unlock()

		if dropped > 0 {
			slog.Warn("log subscriber fell behind, lines were dropped",
				"dropped", dropped)
		}
		close(s.done)
	})
	return nil
}

// lineSplitter adapts an arbitrary byte stream into whole-line callbacks.
// The client writes log records line by line, but a stream carries no
// record boundaries, so partial writes are buffered until a newline
// arrives. Each emitted line is a fresh slice owned by the callback.
type lineSplitter struct {
	buf  []byte
	emit func(line []byte)
}

func (w *lineSplitter) Write(p []byte) (int, error) {
	w.buf = append(w.buf, p...)
	for {
		i := bytes.IndexByte(w.buf, '\n')
		if i < 0 {
			break
		}
		line := make([]byte, i+1)
		copy(line, w.buf[:i+1])
		w.emit(line)
		w.buf = w.buf[i+1:]
	}
	return len(p), nil
}

// flush emits any trailing bytes not terminated by a newline, so the
// final line of a process that exits without one is not lost.
func (w *lineSplitter) flush() {
	if len(w.buf) == 0 {
		return
	}
	line := make([]byte, len(w.buf))
	copy(line, w.buf)
	w.emit(line)
	w.buf = nil
}
