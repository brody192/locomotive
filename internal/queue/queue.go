package queue

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"sync"
	"time"

	"github.com/brody192/locomotive/internal/logger"
)

// DispatchFunc is called to process each queued item.
// Returning a non-nil error triggers a retry according to the dispatcher's backoff policy.
type DispatchFunc[T any] func(ctx context.Context, item T) error

type (
	nameParam              struct{ v string }
	maxQueueSizeParam      struct{ v int }
	maxRetriesParam        struct{ v int }
	initialBackoffParam    struct{ v time.Duration }
	maxBackoffParam        struct{ v time.Duration }
	backoffMultiplierParam struct{ v float64 }
	ttlParam               struct{ v time.Duration }
)

func Name(name string) nameParam                               { return nameParam{name} }
func MaxQueueSize(size int) maxQueueSizeParam                   { return maxQueueSizeParam{size} }
func MaxRetries(retries int) maxRetriesParam                    { return maxRetriesParam{retries} }
func InitialBackoff(d time.Duration) initialBackoffParam        { return initialBackoffParam{d} }
func MaxBackoff(d time.Duration) maxBackoffParam                { return maxBackoffParam{d} }
func BackoffMultiplier(multiplier float64) backoffMultiplierParam { return backoffMultiplierParam{multiplier} }
func TTL(d time.Duration) ttlParam                              { return ttlParam{d} }

type config struct {
	name              string
	maxQueueSize      int
	maxRetries        int
	initialBackoff    time.Duration
	maxBackoff        time.Duration
	backoffMultiplier float64
	ttl               time.Duration
}

func (c config) validate() error {
	if c.name == "" {
		return fmt.Errorf("Name must not be empty")
	}
	if c.maxQueueSize <= 0 {
		return fmt.Errorf("MaxQueueSize must be positive, got %d", c.maxQueueSize)
	}
	if c.maxRetries < 0 {
		return fmt.Errorf("MaxRetries must be non-negative, got %d", c.maxRetries)
	}
	if c.initialBackoff <= 0 {
		return fmt.Errorf("InitialBackoff must be positive, got %s", c.initialBackoff)
	}
	if c.maxBackoff <= 0 {
		return fmt.Errorf("MaxBackoff must be positive, got %s", c.maxBackoff)
	}
	if c.backoffMultiplier <= 0 {
		return fmt.Errorf("BackoffMultiplier must be positive, got %f", c.backoffMultiplier)
	}
	if c.ttl <= 0 {
		return fmt.Errorf("TTL must be positive, got %s", c.ttl)
	}
	return nil
}

type queueItem[T any] struct {
	payload    T
	enqueuedAt time.Time
}

// Dispatcher processes items through a bounded queue with retry, exponential backoff, and TTL semantics.
type Dispatcher[T any] struct {
	config   config
	dispatch DispatchFunc[T]
	items    chan queueItem[T]
	log      *slog.Logger
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup

	// OnSuccess is called after an item is dispatched successfully.
	// The second argument is the total number of attempts (1 means first-try success).
	OnSuccess func(item T, attempts int)

	// OnDrop is called when an item is dropped. The reason describes why.
	OnDrop func(item T, reason string)
}

// NewDispatcher creates a new Dispatcher with the given parameters.
// Call Start to begin processing.
func NewDispatcher[T any](
	name nameParam,
	maxQueueSize maxQueueSizeParam,
	maxRetries maxRetriesParam,
	initialBackoff initialBackoffParam,
	maxBackoff maxBackoffParam,
	backoffMultiplier backoffMultiplierParam,
	ttl ttlParam,
	fn DispatchFunc[T],
) (*Dispatcher[T], error) {
	cfg := config{
		name:              name.v,
		maxQueueSize:      maxQueueSize.v,
		maxRetries:        maxRetries.v,
		initialBackoff:    initialBackoff.v,
		maxBackoff:        maxBackoff.v,
		backoffMultiplier: backoffMultiplier.v,
		ttl:               ttl.v,
	}

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid dispatcher config: %w", err)
	}

	return &Dispatcher[T]{
		config:   cfg,
		dispatch: fn,
		items:    make(chan queueItem[T], cfg.maxQueueSize),
		log:      logger.Stdout.With(slog.String("dispatcher", cfg.name)),
	}, nil
}

// Start begins the processing worker. Must be called before Enqueue.
func (d *Dispatcher[T]) Start(ctx context.Context) {
	d.ctx, d.cancel = context.WithCancel(ctx)
	d.wg.Add(1)
	go d.worker()

	d.log.Debug("dispatcher started",
		slog.Int("max_queue_size", d.config.maxQueueSize),
		slog.Int("max_retries", d.config.maxRetries),
		slog.Duration("ttl", d.config.ttl),
		slog.Duration("initial_backoff", d.config.initialBackoff),
		slog.Duration("max_backoff", d.config.maxBackoff),
		slog.Float64("backoff_multiplier", d.config.backoffMultiplier),
	)
}

// Enqueue adds an item to the dispatch queue.
// Returns false if the queue is full and the item was dropped.
func (d *Dispatcher[T]) Enqueue(item T) bool {
	qi := queueItem[T]{
		payload:    item,
		enqueuedAt: time.Now(),
	}

	select {
	case d.items <- qi:
		d.log.Debug("item enqueued", slog.Int("queue_depth", len(d.items)))
		return true
	default:
		d.log.Warn("queue full, dropping new item",
			slog.Int("queue_size", d.config.maxQueueSize),
		)
		if d.OnDrop != nil {
			d.OnDrop(item, "queue full")
		}
		return false
	}
}

// QueueDepth returns the number of items currently buffered.
func (d *Dispatcher[T]) QueueDepth() int {
	return len(d.items)
}

// Stop cancels processing and waits for the worker to finish draining.
func (d *Dispatcher[T]) Stop() {
	d.cancel()
	d.wg.Wait()
	d.log.Debug("dispatcher stopped")
}

func (d *Dispatcher[T]) worker() {
	defer d.wg.Done()

	for {
		select {
		case <-d.ctx.Done():
			d.drain()
			return
		case item := <-d.items:
			d.processItem(item)
		}
	}
}

func (d *Dispatcher[T]) drain() {
	remaining := len(d.items)
	if remaining == 0 {
		return
	}

	d.log.Info("draining remaining items from queue", slog.Int("count", remaining))

	for range remaining {
		select {
		case item := <-d.items:
			if err := d.dispatch(context.Background(), item.payload); err != nil {
				d.log.Warn("failed to dispatch item during drain",
					slog.String("err", err.Error()),
				)
				if d.OnDrop != nil {
					d.OnDrop(item.payload, fmt.Sprintf("drain failed: %s", err.Error()))
				}
			} else if d.OnSuccess != nil {
				d.OnSuccess(item.payload, 1)
			}
		default:
			return
		}
	}
}

func (d *Dispatcher[T]) processItem(item queueItem[T]) {
	if age := time.Since(item.enqueuedAt); age > d.config.ttl {
		d.log.Warn("dropping expired item",
			slog.Duration("age", age.Round(time.Millisecond)),
			slog.Duration("ttl", d.config.ttl),
		)
		if d.OnDrop != nil {
			d.OnDrop(item.payload, fmt.Sprintf("ttl exceeded: age %s", age.Round(time.Millisecond)))
		}
		return
	}

	maxAttempts := 1 + d.config.maxRetries
	var lastErr error

	for attempt := range maxAttempts {
		if attempt > 0 {
			if age := time.Since(item.enqueuedAt); age > d.config.ttl {
				d.log.Warn("dropping item during retry, ttl exceeded",
					slog.Int("attempt", attempt+1),
					slog.Duration("age", age.Round(time.Millisecond)),
					slog.Duration("ttl", d.config.ttl),
				)
				if d.OnDrop != nil {
					d.OnDrop(item.payload, "ttl exceeded during retry")
				}
				return
			}
		}

		if err := d.dispatch(d.ctx, item.payload); err != nil {
			lastErr = err

			if attempt < d.config.maxRetries {
				backoff := d.backoff(attempt)
				d.log.Warn("dispatch failed, retrying",
					slog.Int("attempt", attempt+1),
					slog.Int("max_attempts", maxAttempts),
					slog.Duration("next_backoff", backoff),
					slog.String("err", err.Error()),
				)

				select {
				case <-d.ctx.Done():
					return
				case <-time.After(backoff):
				}
				continue
			}

			d.log.Error("dropping item after exhausting all retries",
				slog.Int("attempts", maxAttempts),
				slog.String("last_err", lastErr.Error()),
			)
			if d.OnDrop != nil {
				d.OnDrop(item.payload, fmt.Sprintf("max retries exhausted: %s", lastErr.Error()))
			}
			return
		}

		if attempt > 0 {
			d.log.Info("item dispatched successfully after retry",
				slog.Int("attempts", attempt+1),
			)
		} else {
			d.log.Debug("item dispatched successfully")
		}

		if d.OnSuccess != nil {
			d.OnSuccess(item.payload, attempt+1)
		}
		return
	}
}

func (d *Dispatcher[T]) backoff(attempt int) time.Duration {
	return calculateBackoff(attempt, d.config.initialBackoff, d.config.maxBackoff, d.config.backoffMultiplier)
}

func calculateBackoff(attempt int, initial, max time.Duration, multiplier float64) time.Duration {
	b := float64(initial) * math.Pow(multiplier, float64(attempt))
	if b > float64(max) {
		b = float64(max)
	}
	return time.Duration(b)
}
