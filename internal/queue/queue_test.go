package queue

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

var errReceiverDown = errors.New("receiver down")

// receiver stands in for the webhook: it fails while down and counts every attempt.
type receiver struct {
	up       atomic.Bool
	attempts atomic.Int64

	mu    sync.Mutex
	times []time.Time
}

func (r *receiver) dispatch(_ context.Context, _ int) error {
	r.attempts.Add(1)
	r.mu.Lock()
	r.times = append(r.times, time.Now())
	r.mu.Unlock()
	if !r.up.Load() {
		return errReceiverDown
	}
	return nil
}

func (r *receiver) lastAttempt() time.Time {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.times[len(r.times)-1]
}

// outcome counts what the dispatcher reported for each item.
type outcome struct {
	delivered atomic.Int64
	dropped   atomic.Int64
}

func (o *outcome) total() int64 { return o.delivered.Load() + o.dropped.Load() }

func newTestDispatcher(t *testing.T, maxRetries int, initial, max, ttl time.Duration, workers int, r *receiver) (*Dispatcher[int], *outcome) {
	t.Helper()

	d, err := NewDispatcher(
		Name("test"),
		MaxQueueSize(100),
		MaxRetries(maxRetries),
		InitialBackoff(initial),
		MaxBackoff(max),
		BackoffMultiplier(2),
		TTL(ttl),
		Workers(workers),
		r.dispatch,
	)
	if err != nil {
		t.Fatalf("NewDispatcher: %v", err)
	}

	o := &outcome{}
	d.OnSuccess = func(int, int) { o.delivered.Add(1) }
	d.OnDrop = func(int, string) { o.dropped.Add(1) }

	return d, o
}

func waitFor(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not met before timeout")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// A receiver outage shorter than the TTL, with room in the queue, should lose nothing.
// With a fixed retry budget the same outage drops the batches whose budget runs out first.
func TestDispatcherOutageShorterThanTTL(t *testing.T) {
	run := func(t *testing.T, maxRetries int) *outcome {
		r := &receiver{}
		d, o := newTestDispatcher(t, maxRetries, 5*time.Millisecond, 40*time.Millisecond, 5*time.Second, 4, r)
		d.Start(context.Background())
		defer d.Stop()

		for i := range 30 {
			d.Enqueue(i)
			time.Sleep(10 * time.Millisecond)
		}
		time.Sleep(300 * time.Millisecond) // receiver down for about 600 ms in total
		r.up.Store(true)

		waitFor(t, 3*time.Second, func() bool { return o.total() == 30 })
		return o
	}

	t.Run("until ttl", func(t *testing.T) {
		o := run(t, -1)
		if got := o.dropped.Load(); got != 0 {
			t.Fatalf("dropped %d of 30, want 0", got)
		}
		if got := o.delivered.Load(); got != 30 {
			t.Fatalf("delivered %d of 30, want 30", got)
		}
	})

	t.Run("fixed retry budget", func(t *testing.T) {
		o := run(t, 2)
		if o.dropped.Load() == 0 {
			t.Fatal("expected a fixed retry budget to drop batches during the outage")
		}
	})
}

// Retrying until the TTL still gives up: the item is dropped and reported once the next
// retry would start after the TTL, and no attempt is made after it.
func TestDispatcherUntilTTLDropsAtTTL(t *testing.T) {
	r := &receiver{}
	ttl := 150 * time.Millisecond
	d, o := newTestDispatcher(t, -1, 5*time.Millisecond, 20*time.Millisecond, ttl, 1, r)
	d.Start(context.Background())
	defer d.Stop()

	enqueued := time.Now()
	d.Enqueue(1)

	waitFor(t, time.Second, func() bool { return o.dropped.Load() == 1 })

	if got := o.delivered.Load(); got != 0 {
		t.Fatalf("delivered %d, want 0", got)
	}
	if got := r.attempts.Load(); got < 3 {
		t.Fatalf("made %d attempts before the TTL, want several", got)
	}
	if last := r.lastAttempt(); last.After(enqueued.Add(ttl)) {
		t.Fatalf("attempt made %s after the TTL", last.Sub(enqueued.Add(ttl)))
	}
}

// A non-negative MaxRetries keeps its meaning: one attempt plus that many retries.
func TestDispatcherFixedRetries(t *testing.T) {
	r := &receiver{}
	d, o := newTestDispatcher(t, 2, 5*time.Millisecond, 20*time.Millisecond, 5*time.Second, 1, r)
	d.Start(context.Background())
	defer d.Stop()

	d.Enqueue(1)

	waitFor(t, time.Second, func() bool { return o.dropped.Load() == 1 })

	if got := r.attempts.Load(); got != 3 {
		t.Fatalf("made %d attempts, want 3", got)
	}
}

// Items a worker is holding in retry backoff when the dispatcher stops must not vanish:
// drain() gets them like any queued item, and every item is reported as delivered or dropped.
func TestDispatcherStopDuringBackoff(t *testing.T) {
	t.Run("receiver back by shutdown", func(t *testing.T) {
		r := &receiver{}
		d, o := newTestDispatcher(t, -1, 200*time.Millisecond, time.Second, time.Minute, 4, r)
		d.Start(context.Background())

		for i := range 10 {
			d.Enqueue(i)
		}
		waitFor(t, time.Second, func() bool { return r.attempts.Load() >= 4 }) // every worker holds an item in backoff

		r.up.Store(true)
		d.Stop()

		if got := o.delivered.Load(); got != 10 {
			t.Fatalf("delivered %d of 10, dropped %d", got, o.dropped.Load())
		}
	})

	t.Run("receiver still down", func(t *testing.T) {
		r := &receiver{}
		d, o := newTestDispatcher(t, -1, 200*time.Millisecond, time.Second, time.Minute, 4, r)
		d.Start(context.Background())

		for i := range 10 {
			d.Enqueue(i)
		}
		waitFor(t, time.Second, func() bool { return r.attempts.Load() >= 4 })

		d.Stop()

		if got := o.total(); got != 10 {
			t.Fatalf("reported %d of 10 items (delivered %d, dropped %d)", got, o.delivered.Load(), o.dropped.Load())
		}
	})
}

// Stopping while a send is in flight must not abort it: the receiver may already have
// the batch, and aborting it would make drain() send it a second time.
func TestDispatcherStopDuringSend(t *testing.T) {
	var (
		mu       sync.Mutex
		received = map[int]int{}
		inFlight atomic.Int64
	)
	// A slow receiver that, like the HTTP client, gives up when its context is
	// cancelled, after the request has already reached it.
	slow := func(ctx context.Context, item int) error {
		mu.Lock()
		received[item]++
		mu.Unlock()
		inFlight.Add(1)
		select {
		case <-time.After(300 * time.Millisecond):
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	d, err := NewDispatcher(
		Name("test"), MaxQueueSize(100), MaxRetries(-1),
		InitialBackoff(10*time.Millisecond), MaxBackoff(time.Second), BackoffMultiplier(2),
		TTL(time.Minute), Workers(4), slow,
	)
	if err != nil {
		t.Fatalf("NewDispatcher: %v", err)
	}
	var delivered, dropped atomic.Int64
	d.OnSuccess = func(int, int) { delivered.Add(1) }
	d.OnDrop = func(int, string) { dropped.Add(1) }

	d.Start(context.Background())
	for i := range 10 {
		d.Enqueue(i)
	}
	waitFor(t, time.Second, func() bool { return inFlight.Load() >= 4 }) // every worker mid-send

	d.Stop()

	if got := delivered.Load(); got != 10 {
		t.Fatalf("delivered %d of 10, dropped %d", got, dropped.Load())
	}
	for item, n := range received {
		if n != 1 {
			t.Errorf("item %d reached the receiver %d times", item, n)
		}
	}
}
