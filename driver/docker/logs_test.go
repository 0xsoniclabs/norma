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
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
	"testing"
)

// publishLines feeds newline-terminated lines into the broadcaster.
func publishLines(b *logBroadcaster, lines ...string) {
	for _, line := range lines {
		b.publish([]byte(line + "\n"))
	}
}

// readLines collects lines from a subscription until it reports EOF.
func readLines(t *testing.T, r io.Reader) []string {
	t.Helper()
	var got []string
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		got = append(got, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("failed to read subscription: %v", err)
	}
	return got
}

func TestLogBroadcaster_DeliversPublishedLinesToSubscriber(t *testing.T) {
	b := newLogBroadcaster()
	sub := b.subscribe(t.Context())
	defer func() { _ = sub.Close() }()

	publishLines(b, "first", "second")
	b.close()

	got := readLines(t, sub)
	want := []string{"first", "second"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("unexpected lines, got %v, want %v", got, want)
	}
}

// The Host contract requires every reader to observe the same output.
func TestLogBroadcaster_DeliversToAllSubscribers(t *testing.T) {
	b := newLogBroadcaster()
	subs := []io.ReadCloser{b.subscribe(t.Context()), b.subscribe(t.Context()), b.subscribe(t.Context())}

	publishLines(b, "shared")
	b.close()

	for i, sub := range subs {
		got := readLines(t, sub)
		if len(got) != 1 || got[0] != "shared" {
			t.Errorf("subscriber %d got %v, want [shared]", i, got)
		}
		_ = sub.Close()
	}
}

// A consumer attaching after the client has already produced output must
// still see recent context; monitoring attaches only once a node is up.
func TestLogBroadcaster_ReplaysRecentLinesToLateSubscriber(t *testing.T) {
	b := newLogBroadcaster()

	publishLines(b, "before")
	sub := b.subscribe(t.Context())
	defer func() { _ = sub.Close() }()
	publishLines(b, "after")
	b.close()

	got := readLines(t, sub)
	want := []string{"before", "after"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("unexpected lines, got %v, want %v", got, want)
	}
}

func TestLogBroadcaster_BoundsTheReplayBuffer(t *testing.T) {
	b := newLogBroadcaster()

	total := logReplayLines + 50
	for i := range total {
		publishLines(b, fmt.Sprintf("line-%d", i))
	}
	sub := b.subscribe(t.Context())
	defer func() { _ = sub.Close() }()
	b.close()

	got := readLines(t, sub)
	if len(got) != logReplayLines {
		t.Fatalf("unexpected replay length, got %d, want %d",
			len(got), logReplayLines)
	}
	// The retained window must be the most recent one.
	if want := fmt.Sprintf("line-%d", total-1); got[len(got)-1] != want {
		t.Errorf("unexpected last replayed line, got %q, want %q",
			got[len(got)-1], want)
	}
}

func TestLogBroadcaster_SubscribeAfterCloseYieldsEOF(t *testing.T) {
	b := newLogBroadcaster()
	publishLines(b, "only")
	b.close()

	sub := b.subscribe(t.Context())
	defer func() { _ = sub.Close() }()

	// The replayed tail is still served, then the reader must end.
	got := readLines(t, sub)
	if len(got) != 1 || got[0] != "only" {
		t.Errorf("unexpected lines, got %v, want [only]", got)
	}
}

func TestLogBroadcaster_PublishAfterCloseIsIgnored(t *testing.T) {
	b := newLogBroadcaster()
	b.close()
	publishLines(b, "late")

	sub := b.subscribe(t.Context())
	defer func() { _ = sub.Close() }()
	if got := readLines(t, sub); len(got) != 0 {
		t.Errorf("expected no lines after close, got %v", got)
	}
}

// A stalled consumer must not block the exec output stream, since that
// would in turn block the client process writing to it.
func TestLogBroadcaster_DroppingSubscriberDoesNotBlockPublisher(t *testing.T) {
	b := newLogBroadcaster()
	sub := b.subscribe(t.Context()) // never read from
	defer func() { _ = sub.Close() }()

	for i := range logSubscriberBuffer * 2 {
		publishLines(b, fmt.Sprintf("line-%d", i))
	}

	subscription, ok := sub.(*logSubscription)
	if !ok {
		t.Fatalf("unexpected subscription type %T", sub)
	}
	b.mu.Lock()
	dropped := subscription.dropped
	b.mu.Unlock()
	if dropped == 0 {
		t.Errorf("expected lines to be dropped for a stalled subscriber")
	}
}

// The subscription follows a running container and never reaches EOF on
// its own, so callers that cannot wait bound the read with a context. It
// must still hand over the lines produced so far before ending.
func TestLogSubscription_ContextBoundedReadDrainsThenEndsAtEOF(t *testing.T) {
	b := newLogBroadcaster()
	ctx, cancel := context.WithCancel(t.Context())

	sub := b.subscribe(ctx)
	defer func() { _ = sub.Close() }()
	publishLines(b, "produced")

	// The broadcaster stays open, standing in for a running container.
	cancel()

	got := readLines(t, sub)
	if len(got) != 1 || got[0] != "produced" {
		t.Errorf("unexpected lines, got %v, want [produced]", got)
	}
}

func TestLogSubscription_ContextBoundedReadDoesNotBlockWithoutOutput(t *testing.T) {
	b := newLogBroadcaster()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	sub := b.subscribe(ctx)
	defer func() { _ = sub.Close() }()

	if got := readLines(t, sub); len(got) != 0 {
		t.Errorf("expected no lines, got %v", got)
	}
}

func TestLogSubscription_CloseUnsubscribesAndIsIdempotent(t *testing.T) {
	b := newLogBroadcaster()
	sub := b.subscribe(t.Context())

	if err := sub.Close(); err != nil {
		t.Fatalf("unexpected error on close: %v", err)
	}
	if err := sub.Close(); err != nil {
		t.Fatalf("unexpected error on second close: %v", err)
	}

	b.mu.Lock()
	remaining := len(b.subscribers)
	b.mu.Unlock()
	if remaining != 0 {
		t.Errorf("closed subscriber was not removed, %d remain", remaining)
	}

	// A closed subscription reports EOF rather than blocking.
	if n, err := sub.Read(make([]byte, 8)); err != io.EOF || n != 0 {
		t.Errorf("unexpected read after close, n=%d err=%v", n, err)
	}
}

func TestLineSplitter_EmitsCompleteLinesAcrossWrites(t *testing.T) {
	var got []string
	w := &lineSplitter{emit: func(line []byte) {
		got = append(got, string(line))
	}}

	// A stream carries no record boundaries, so a line may arrive split
	// across writes and several lines may arrive in one.
	writes := []string{"par", "tial line\ntwo\nthree", "\n"}
	for _, chunk := range writes {
		n, err := w.Write([]byte(chunk))
		if err != nil {
			t.Fatalf("unexpected write error: %v", err)
		}
		if n != len(chunk) {
			t.Fatalf("short write, got %d, want %d", n, len(chunk))
		}
	}

	want := []string{"partial line\n", "two\n", "three\n"}
	if strings.Join(got, "") != strings.Join(want, "") {
		t.Errorf("unexpected lines, got %q, want %q", got, want)
	}
}

// The last line of a process that exits without a trailing newline must
// not be swallowed.
func TestLineSplitter_FlushEmitsUnterminatedTail(t *testing.T) {
	var got []string
	w := &lineSplitter{emit: func(line []byte) {
		got = append(got, string(line))
	}}

	if _, err := w.Write([]byte("no trailing newline")); err != nil {
		t.Fatalf("unexpected write error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("incomplete line must not be emitted before flush, got %q", got)
	}

	w.flush()
	if len(got) != 1 || got[0] != "no trailing newline" {
		t.Errorf("unexpected lines after flush, got %q", got)
	}

	// A second flush has nothing left to emit.
	w.flush()
	if len(got) != 1 {
		t.Errorf("flush must not re-emit, got %q", got)
	}
}

// Emitted lines must be owned by the callback: the splitter reuses its
// buffer, so handing out a view of it would corrupt earlier lines.
func TestLineSplitter_EmitsIndependentBuffers(t *testing.T) {
	var got [][]byte
	w := &lineSplitter{emit: func(line []byte) {
		got = append(got, line)
	}}

	if _, err := w.Write([]byte("first\nsecond\n")); err != nil {
		t.Fatalf("unexpected write error: %v", err)
	}
	if _, err := w.Write([]byte("0000000000000\n")); err != nil {
		t.Fatalf("unexpected write error: %v", err)
	}

	if string(got[0]) != "first\n" {
		t.Errorf("earlier line was overwritten, got %q", got[0])
	}
	if string(got[1]) != "second\n" {
		t.Errorf("earlier line was overwritten, got %q", got[1])
	}
}
