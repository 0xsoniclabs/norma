package docker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
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
		require.Equal(t, 7, <-results)
	}
	value, err := once.do(t.Context(), "key", work)
	require.NoError(t, err)
	require.Equal(t, 7, value)
	require.Equal(t, 1, runs)
}

// A caller whose context ends stops waiting for another caller's run, which
// may build an image for minutes.
func TestOnceByKey_StopsWaiting_WhenTheCallersContextEnds(t *testing.T) {
	var once onceByKey[int]
	started, release := make(chan struct{}), make(chan struct{})
	t.Cleanup(func() { close(release) })
	go func() {
		_, _ = once.do(context.Background(), "key", func() (int, error) {
			close(started)
			<-release
			return 1, nil
		})
	}()
	<-started

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer cancel()
	_, err := once.do(ctx, "key", func() (int, error) { return 2, nil })
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestOnceByKey_RunsTheWorkAgain_WhenItFailed(t *testing.T) {
	var once onceByKey[int]
	_, err := once.do(t.Context(), "key", func() (int, error) { return 0, errors.New("failed") })
	require.Error(t, err)

	value, err := once.do(t.Context(), "key", func() (int, error) { return 3, nil })
	require.NoError(t, err)
	require.Equal(t, 3, value)
}
