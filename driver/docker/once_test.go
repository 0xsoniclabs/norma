package docker

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestOnceByKey_RunsTheWorkOnce_ForConcurrentCallers(t *testing.T) {
	var once onceByKey[int]
	release := make(chan struct{})
	runs := 0
	work := func() (int, error) {
		runs++
		<-release
		return 7, nil
	}

	results := make(chan int)
	for range 3 {
		go func() {
			value, _ := once.do(t.Context(), "key", work)
			results <- value
		}()
	}
	time.Sleep(10 * time.Millisecond)
	close(release)
	for range 3 {
		if got := <-results; got != 7 {
			t.Errorf("unexpected value %d", got)
		}
	}
	if value, _ := once.do(t.Context(), "key", work); value != 7 || runs != 1 {
		t.Errorf("work ran %d times, last value %d", runs, value)
	}
}

// A caller whose context ends stops waiting for another caller's run, which
// may build an image for minutes.
func TestOnceByKey_StopsWaiting_WhenTheCallersContextEnds(t *testing.T) {
	var once onceByKey[int]
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	go func() {
		_, _ = once.do(context.Background(), "key", func() (int, error) {
			<-release
			return 1, nil
		})
	}()
	time.Sleep(10 * time.Millisecond)

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer cancel()
	if _, err := once.do(ctx, "key", func() (int, error) { return 2, nil }); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected the caller's deadline, got %v", err)
	}
}

func TestOnceByKey_RunsTheWorkAgain_WhenItFailed(t *testing.T) {
	var once onceByKey[int]
	if _, err := once.do(t.Context(), "key", func() (int, error) { return 0, errors.New("failed") }); err == nil {
		t.Fatal("expected the failure")
	}
	if value, err := once.do(t.Context(), "key", func() (int, error) { return 3, nil }); err != nil || value != 3 {
		t.Errorf("expected the work to run again, got %d, %v", value, err)
	}
}
