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

// Retry calls fn repeatedly with exponential backoff until it succeeds, returns a
// non-retryable error, the context is cancelled, or maxRetries is exhausted.
//
// fn must wrap retryable errors with [Retryable]. Any other non-nil error stops the loop immediately.
func Retry(
	ctx context.Context,
	name nameParam,
	maxRetries maxRetriesParam,
	initialBackoff initialBackoffParam,
	maxBackoff maxBackoffParam,
	backoffMultiplier backoffMultiplierParam,
	fn func(ctx context.Context) error,
) error {
	log := logger.Stdout.With(slog.String("retry", name.v))

	maxAttempts := 1 + maxRetries.v
	var lastErr error

	for attempt := range maxAttempts {
		err := fn(ctx)
		if err == nil {
			if attempt > 0 {
				log.Info("succeeded after retry", slog.Int("attempts", attempt+1))
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

		if attempt < maxRetries.v {
			backoff := calculateBackoff(attempt, initialBackoff.v, maxBackoff.v, backoffMultiplier.v)
			log.Warn("failed, retrying",
				slog.Int("attempt", attempt+1),
				slog.Int("max_attempts", maxAttempts),
				slog.Duration("next_backoff", backoff),
				slog.String("err", lastErr.Error()),
			)

			select {
			case <-ctx.Done():
				return fmt.Errorf("context cancelled during retry backoff: %w", context.Cause(ctx))
			case <-time.After(backoff):
			}
			continue
		}

		log.Error("giving up after exhausting all retries",
			slog.Int("attempts", maxAttempts),
			slog.String("last_err", lastErr.Error()),
		)
	}

	return fmt.Errorf("all %d attempts failed, last error: %w", maxAttempts, lastErr)
}
