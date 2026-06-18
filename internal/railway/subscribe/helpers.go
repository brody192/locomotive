package subscribe

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"time"

	"github.com/brody192/locomotive/internal/logger"
	"github.com/brody192/locomotive/internal/railway/gql/subscriptions"
	"github.com/coder/websocket"
)

const (
	pingInterval = 30 * time.Second
	pingTimeout  = 10 * time.Second

	// resubscribeBackoff is applied before every resubscribe attempt — including the
	// first — so a connection that is established but then immediately dropped or
	// completed by the server cannot spin into a tight reconnect loop.
	resubscribeBackoff = 1 * time.Second

	// maxConcurrentSubscriptionOpens bounds how many subscriptions may be initializing
	// at the same time, to stay comfortably within the backend's limit on concurrent
	// subscription initialization. Already-established streams are not limited, so a
	// slot is held only for the duration of the open (dial + handshake + subscribe, or
	// a reuse subscribe write), never for the lifetime of the stream.
	maxConcurrentSubscriptionOpens = 3

	MetadataKeyLogType              = "log_type"
	MetadataKeyProjectName          = "project_name"
	MetadataKeyProjectID            = "project_id"
	MetadataKeyEnvironmentName      = "environment_name"
	MetadataKeyEnvironmentID        = "environment_id"
	MetadataKeyServiceName          = "service_name"
	MetadataKeyServiceID            = "service_id"
	MetadataKeyDeploymentID         = "deployment_id"
	MetadataKeyDeploymentInstanceID = "deployment_instance_id"

	LogTypeEnvironment             = "environment"
	LogTypeHTTP                    = "http"
	LogTypeEnvironmentInvalidation = "environment_invalidation"
)

// subscriptionOpenLimiter is a global counting semaphore that bounds how many
// subscriptions may be initializing at once across every subscription type.
var subscriptionOpenLimiter = make(chan struct{}, maxConcurrentSubscriptionOpens)

// AcquireOpenSlot blocks until a subscription-open slot is available or ctx is
// cancelled. The returned release function must be called once the open has finished
// (whether it succeeded or failed) to free the slot for the next opener.
func AcquireOpenSlot(ctx context.Context) (release func(), err error) {
	select {
	case subscriptionOpenLimiter <- struct{}{}:
		return func() { <-subscriptionOpenLimiter }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Conn wraps a websocket.Conn with an automatic ping loop.
// The ping loop starts on creation and stops when CloseNow is called.
type Conn struct {
	*websocket.Conn
	stopPing context.CancelFunc
}

// NewConn wraps a websocket.Conn and starts a background ping loop.
func NewConn(ctx context.Context, conn *websocket.Conn) *Conn {
	pingCtx, stopPing := context.WithCancel(ctx)

	c := &Conn{
		Conn:     conn,
		stopPing: stopPing,
	}

	go c.pingLoop(pingCtx)

	return c
}

func randomPingInterval() time.Duration {
	// ±~17% jitter around pingInterval (25s–35s for a 30s interval)
	minInterval := pingInterval * 5 / 6
	jitterRange := pingInterval / 3
	return minInterval + time.Duration(rand.Int64N(int64(jitterRange)))
}

func (c *Conn) pingLoop(ctx context.Context) {
	for {
		interval := randomPingInterval()
		select {
		case <-ctx.Done():
			return
		case <-time.After(interval):
			pingCtx, cancel := context.WithTimeout(ctx, pingTimeout)
			err := c.Conn.Ping(pingCtx)
			cancel()

			if err != nil {
				logger.Stdout.Debug("ping failed, closing connection to trigger resubscribe",
					logger.ErrAttr(err),
				)
				c.Conn.CloseNow()
				return
			}

			logger.Stdout.Debug("ping succeeded", slog.Duration("interval", interval))
		}
	}
}

// CloseNow stops the ping loop and closes the underlying connection.
func (c *Conn) CloseNow() (err error) {
	if c == nil {
		return nil
	}

	c.stopPing()

	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("recovered from close now panic: %v", r)
		}
	}()

	return c.Conn.CloseNow()
}

// Read reads from the underlying connection with panic recovery.
func (c *Conn) Read(ctx context.Context) (mT websocket.MessageType, b []byte, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("recovered from read panic: %v", r)
		}
	}()

	return c.Conn.Read(ctx)
}

// Subscribe sends a new graphql-transport-ws subscribe message over the existing
// connection, restarting a completed subscription without redialing — no new TLS
// handshake or connection_init/ack round-trip. It is safe to call concurrently with
// the background ping loop: coder/websocket serializes data and control frames behind
// a single write mutex.
func (c *Conn) Subscribe(ctx context.Context, payload any) error {
	msg, err := subscriptions.NewSubscribeMessage(payload)
	if err != nil {
		return err
	}

	// Reusing the socket still initializes a new subscription on the backend, so it
	// counts against the concurrent-open limit.
	release, err := AcquireOpenSlot(ctx)
	if err != nil {
		return err
	}
	defer release()

	return c.Conn.Write(ctx, websocket.MessageText, msg)
}

// resubscribeWithRetry closes the old connection and repeatedly calls createFn
// until it succeeds, the context is cancelled, or maxRetryDuration elapses.
//
// A constant resubscribeBackoff is applied before every attempt — including the
// first. The pre-first-attempt delay is what prevents a connection that establishes
// successfully but is then immediately dropped or completed by the server from
// spinning into a tight reconnect loop.
func resubscribeWithRetry(ctx context.Context, conn *Conn, maxRetryDuration time.Duration, createFn func(ctx context.Context) (*Conn, error)) (*Conn, error) {
	conn.CloseNow()

	retryStart := time.Now()

	for attempt := 1; ; attempt++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(resubscribeBackoff):
		}

		elapsed := time.Since(retryStart)
		if elapsed > maxRetryDuration {
			return nil, fmt.Errorf("failed to resubscribe after %v (%d attempts): maximum retry duration exceeded", elapsed.Round(time.Second), attempt-1)
		}

		newConn, err := createFn(ctx)
		if err != nil {
			logger.Stdout.LogAttrs(ctx, slog.LevelDebug, "error resubscribing, will retry",
				logger.ErrAttr(err),
				slog.Int("attempt", attempt),
				slog.Duration("elapsed", elapsed.Round(time.Millisecond)),
			)

			continue
		}

		if attempt > 1 {
			logger.Stdout.LogAttrs(ctx, slog.LevelInfo, "successfully resubscribed",
				slog.Int("attempts", attempt),
				slog.Duration("elapsed", time.Since(retryStart).Round(time.Millisecond)),
			)
		}

		return newConn, nil
	}
}

// resubscribeReusing restarts a subscription over the EXISTING connection by sending a
// fresh subscribe message, avoiding a full redial (no new TLS handshake or
// connection_init/ack round-trip). The constant resubscribeBackoff is applied first so
// a server that immediately completes the subscription cannot cause a tight loop.
//
// If the existing connection can no longer be written to (e.g. the server closed it),
// it falls back to a full redial via resubscribeWithRetry. The returned connection is
// the same one on reuse, or a fresh one on fallback.
func resubscribeReusing(ctx context.Context, conn *Conn, maxRetryDuration time.Duration, payload any, redialFn func(ctx context.Context) (*Conn, error)) (*Conn, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(resubscribeBackoff):
	}

	if err := conn.Subscribe(ctx, payload); err != nil {
		logger.Stdout.LogAttrs(ctx, slog.LevelDebug, "could not reuse connection, falling back to redial", logger.ErrAttr(err))

		return resubscribeWithRetry(ctx, conn, maxRetryDuration, redialFn)
	}

	return conn, nil
}

// Subscription owns a Conn and knows how to re-establish it, either by reusing the
// existing socket (Reuse) or redialing (Redial). The resume state stays in the caller:
// payload is called fresh on every (re)subscribe, so a caller that tails forward simply
// closes over its last-seen position instead of threading it through every call.
type Subscription struct {
	conn             *Conn
	dial             func(ctx context.Context, payload any) (*Conn, error)
	payload          func() any
	maxRetryDuration time.Duration
}

// NewSubscription opens the initial connection and returns a Subscription that can
// re-establish it. dial opens a fresh socket (a bound CreateWebSocketSubscription), and
// payload returns the message to (re)subscribe with, evaluated at the moment of each
// (re)subscribe — including this initial one — so the caller never builds a payload at
// the call site.
func NewSubscription(ctx context.Context, dial func(ctx context.Context, payload any) (*Conn, error), payload func() any, maxRetryDuration time.Duration) (*Subscription, error) {
	conn, err := dial(ctx, payload())
	if err != nil {
		return nil, err
	}

	return &Subscription{
		conn:             conn,
		dial:             dial,
		payload:          payload,
		maxRetryDuration: maxRetryDuration,
	}, nil
}

// Read reads the next message from the current connection.
func (s *Subscription) Read(ctx context.Context) (websocket.MessageType, []byte, error) {
	return s.conn.Read(ctx)
}

// Close tears down the current connection.
func (s *Subscription) Close() error {
	return s.conn.CloseNow()
}

// Reuse restarts the subscription over the existing socket, falling back to a redial if
// the socket is no longer usable. Used when the stream completed cleanly.
func (s *Subscription) Reuse(ctx context.Context) error {
	conn, err := resubscribeReusing(ctx, s.conn, s.maxRetryDuration, s.payload(), s.redial)
	if err != nil {
		return err
	}

	s.conn = conn
	return nil
}

// Redial reconnects with a fresh socket. Used when the current connection is broken.
func (s *Subscription) Redial(ctx context.Context) error {
	conn, err := resubscribeWithRetry(ctx, s.conn, s.maxRetryDuration, s.redial)
	if err != nil {
		return err
	}

	s.conn = conn
	return nil
}

func (s *Subscription) redial(ctx context.Context) (*Conn, error) {
	return s.dial(ctx, s.payload())
}

// Run reads messages until ctx is cancelled, driving the (re)subscribe policy shared by
// every subscription:
//   - a connection error → redial (the socket is broken)
//   - a non-"next" message, e.g. "complete" → reuse the socket (resubscribe over it)
//   - a malformed message → log and skip (one bad frame shouldn't kill the stream)
//   - a "next" message → hand the raw payload to onNext
//
// onNext returning an error stops Run and returns that error; returning nil continues.
func (s *Subscription) Run(ctx context.Context, onNext func(payload []byte) error) error {
	for {
		_, payload, err := s.Read(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}

			logger.Stdout.Debug("connection error, resubscribing", logger.ErrAttr(err))

			if err := s.Redial(ctx); err != nil {
				return err
			}

			continue
		}

		var envelope struct {
			Type subscriptions.SubscriptionType `json:"type"`
		}
		if err := json.Unmarshal(payload, &envelope); err != nil {
			logger.Stdout.Debug("could not parse subscription message, skipping", logger.ErrAttr(err))
			continue
		}

		if envelope.Type != subscriptions.SubscriptionTypeNext {
			logger.Stdout.Debug("subscription ended, resubscribing over existing connection",
				slog.String("type", string(envelope.Type)))

			if err := s.Reuse(ctx); err != nil {
				return err
			}

			continue
		}

		if err := onNext(payload); err != nil {
			return err
		}
	}
}
