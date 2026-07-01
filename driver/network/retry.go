package network

import (
	"context"
	"errors"
	"log/slog"
	"time"
)

const DefaultRetryAttempts = 180

// ErrPermanent is a sentinel error to be wrapped with fmt.Errorf when a
// function passed to Retry or RetryReturn should not be retried.
// Example: return fmt.Errorf("container exited: %w", network.ErrPermanent)
var ErrPermanent = errors.New("permanent")

// RetryReturn executes the input function until it produces no error.
// It however executes only the configured number of times with the configured
// delay between attempts. If the execution is not successful since,
// the execution returns the last error.
// When execution is successful, the execution result is returned from this method.
// The context can be used to abort the retry loop early.
// If the function returns a PermanentError, the retry loop stops immediately.
func RetryReturn[Out any](
	ctx context.Context,
	numAttempts int,
	delay time.Duration,
	do func(context.Context) (Out, error),
) (Out, error) {
	var out Out
	var err error
	for i := 0; i < numAttempts; i++ {
		if ctx.Err() != nil {
			return out, ctx.Err()
		}
		out, err = do(ctx)
		if err == nil {
			break
		}
		if errors.Is(err, ErrPermanent) {
			return out, err // don't retry when the error is permanent
		}

		slog.Debug("query failed, retrying", "error", err, "attempt", i+1, "max_attempts", numAttempts)
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(delay):
		}
	}
	return out, err
}

// Retry executes the input function until it produces no error.
// It however executes only the configured number of times with the configured
// delay between attempts. If the execution is not successful since,
// the execution returns the last error.
// The context can be used to abort the retry loop early.
func Retry(
	ctx context.Context,
	numAttempts int,
	delay time.Duration,
	do func(context.Context) error,
) error {
	_, err := RetryReturn(ctx, numAttempts, delay, func(ctx context.Context) (*int, error) {
		err := do(ctx)
		if err != nil {
			slog.Error("retry failed", "error", err)
		}
		return nil, err
	})
	return err
}
