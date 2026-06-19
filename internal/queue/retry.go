package queue

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/brody192/locomotive/internal/logger"
)

type retryableError struct {
	err error
}

func (e *retryableError) Error() string { return e.err.Error() }
func (e *retryableError) Unwrap() error { return e.err }

// Retryable wraps an error to indicate it should be retried.
// Non-retryable errors returned from the function passed to Retry cause an immediate return.
func Retryable(err error) error {
	if err == nil {
		return nil
	}
	return &retryableError{err: err}
}

func isRetryable(err error) bool {
	var re *retryableError
	return errors.As(err, &re)
}

func unwrapRetryable(err error) error {
	var re *retryableError
	if errors.As(err, &re) {
		return re.err
	}
	return err
}

// RetryBackoff calls fn repeatedly with exponential, jittered backoff until it succeeds,
// returns a non-retryable error, the context is cancelled, or maxRetries is exhausted. A
// maxRetries below zero retries indefinitely (until success, a non-retryable error, or
// context cancellation) — for long-lived reconnect loops that should keep trying.
//
// The first attempt runs immediately. The delay before the Nth retry is
// initialBackoff * multiplier^(N-1), capped at maxBackoff, then varied symmetrically by up
// to jitter (a fraction in [0,1]) so concurrent retriers don't resynchronize — never
// exceeding maxBackoff, so the jitter only reaches downward once a delay is at the cap.
//
// fn must wrap retryable errors with [Retryable]; any other non-nil error stops the loop
// immediately. Per-attempt retries are logged at debug level, since this primitive is
// meant for high-frequency reconnects where higher levels would be noise.
func RetryBackoff(
	ctx context.Context,
	name nameParam,
	maxRetries maxRetriesParam,
	initialBackoff initialBackoffParam,
	maxBackoff maxBackoffParam,
	backoffMultiplier backoffMultiplierParam,
	backoffJitter backoffJitterParam,
	fn func(ctx context.Context) error,
) error {
	log := logger.Stdout.With(slog.String("retry", name.v))

	unlimited := maxRetries.v < 0
	var lastErr error

	for attempt := 0; ; attempt++ {
		err := fn(ctx)
		if err == nil {
			if attempt > 0 {
				log.Debug("succeeded after retry", slog.Int("attempts", attempt+1))
			}
			return nil
		}

		if !isRetryable(err) {
			log.Debug("non-retryable error, stopping",
				slog.Int("attempt", attempt+1),
				slog.String("err", err.Error()),
			)
			return err
		}

		lastErr = unwrapRetryable(err)

		if !unlimited && attempt >= maxRetries.v {
			log.Error("giving up after exhausting all retries",
				slog.Int("attempts", attempt+1),
				slog.String("last_err", lastErr.Error()),
			)
			return fmt.Errorf("all %d attempts failed, last error: %w", attempt+1, lastErr)
		}

		backoff := applyJitter(
			calculateBackoff(attempt, initialBackoff.v, maxBackoff.v, backoffMultiplier.v),
			backoffJitter.v,
			maxBackoff.v,
		)

		log.Debug("failed, retrying",
			slog.Int("attempt", attempt+1),
			slog.Duration("next_retry", backoff),
			slog.String("err", lastErr.Error()),
		)

		select {
		case <-ctx.Done():
			return fmt.Errorf("context cancelled during retry backoff: %w", context.Cause(ctx))
		case <-time.After(backoff):
		}
	}
}
