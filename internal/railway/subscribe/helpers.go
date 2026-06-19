package subscribe

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"time"

	"github.com/brody192/locomotive/internal/logger"
	"github.com/brody192/locomotive/internal/queue"
	"github.com/brody192/locomotive/internal/railway/gql/subscriptions"
	"github.com/coder/websocket"
)

const (
	pingInterval = 30 * time.Second
	pingTimeout  = 10 * time.Second

	// Re-establishing a subscription is driven by queue.RetryBackoff (see Run). A stream
	// that delivers data resets the backoff; consecutive streams that end without
	// delivering anything — e.g. a backend completing immediately while shedding load —
	// grow the delay from resubscribeInitialBackoff toward resubscribeMaxBackoff, so
	// locomotive stops re-issuing streams at a fixed cadence and lets the backend recover
	// instead of perpetuating the loop. The jitter de-synchronizes many subscriptions so
	// they don't resubscribe in lockstep.
	resubscribeInitialBackoff = 1 * time.Second
	resubscribeMaxBackoff     = 30 * time.Second
	resubscribeBackoffFactor  = 2.0
	resubscribeBackoffJitter  = 0.5

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
)

// LogType identifies a subscription's log stream — used both as the metadata log_type
// value and to label a Subscription in its logs.
type LogType string

const (
	LogTypeEnvironment             LogType = "environment"
	LogTypeHTTP                    LogType = "http"
	LogTypeEnvironmentInvalidation LogType = "environment_invalidation"
)

// logAttrSubscription is the structured-log attribute key carrying a subscription's
// LogType, so every line for a subscription — stream events and retries alike — is tagged
// the same way with the same value.
const logAttrSubscription = "subscription"

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

	// broken is set once a read fails or the connection is closed, marking the socket as
	// no longer usable for another subscribe. Only touched from the read/connect goroutine.
	broken bool
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

	c.broken = true
	c.stopPing()

	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("recovered from close now panic: %v", r)
		}
	}()

	return c.Conn.CloseNow()
}

// Read reads from the underlying connection with panic recovery. A failed read marks the
// connection broken so it won't be reused.
func (c *Conn) Read(ctx context.Context) (mT websocket.MessageType, b []byte, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("recovered from read panic: %v", r)
		}
		if err != nil {
			c.broken = true
		}
	}()

	return c.Conn.Read(ctx)
}

// reusable reports whether the connection can carry another subscribe without redialing —
// false once a read has failed or it has been closed. Nil-safe, so callers can probe a
// not-yet-dialed connection.
func (c *Conn) reusable() bool {
	return c != nil && !c.broken
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

// errStreamEnded marks a stream that ended without delivering any data — a retryable
// condition that drives RetryBackoff to wait and re-establish the subscription.
var errStreamEnded = errors.New("subscription stream ended without delivering data")

// Subscription owns a Conn and knows how to re-establish it, either by reusing the
// existing socket or redialing. The resume state stays in the caller: payload is called
// fresh on every (re)subscribe, so a caller that tails forward simply closes over its
// last-seen position instead of threading it through every call.
type Subscription struct {
	logType LogType
	conn    *Conn
	dial    func(ctx context.Context, payload any) (*Conn, error)
	payload func() any
}

// NewSubscription returns a Subscription. logType labels it in logs, dial opens a fresh
// socket (a bound CreateWebSocketSubscription), and payload returns the message to
// (re)subscribe with, evaluated at the moment of each (re)subscribe so the caller never
// builds a payload at the call site. The connection is opened lazily by Run.
func NewSubscription(logType LogType, dial func(ctx context.Context, payload any) (*Conn, error), payload func() any) *Subscription {
	return &Subscription{
		logType: logType,
		dial:    dial,
		payload: payload,
	}
}

// Close tears down the current connection.
func (s *Subscription) Close() error {
	if s.conn == nil {
		return nil
	}

	return s.conn.CloseNow()
}

// connect establishes the stream for the next consume: it sends a fresh subscribe over the
// existing socket when that socket is still usable, and redials otherwise (no socket yet,
// or the last one broke). A reuse whose write fails falls back to a redial.
func (s *Subscription) connect(ctx context.Context) error {
	if s.conn.reusable() {
		err := s.conn.Subscribe(ctx, s.payload())
		if err == nil {
			logger.Stdout.Debug("resubscribed over existing connection",
				slog.String(logAttrSubscription, string(s.logType)))
			return nil
		}

		logger.Stdout.LogAttrs(ctx, slog.LevelDebug, "could not reuse connection, falling back to redial",
			slog.String(logAttrSubscription, string(s.logType)),
			logger.ErrAttr(err))
	}

	if s.conn != nil {
		s.conn.CloseNow()
		s.conn = nil
	}

	conn, err := s.dial(ctx, s.payload())
	if err != nil {
		// A genuine failure to establish the connection (dial/handshake/auth), as opposed
		// to a stream that merely completed. Surface it at error level so a wedged
		// subscription is visible even though Run retries indefinitely. Context
		// cancellation is a normal shutdown, not an error.
		if ctx.Err() == nil {
			logger.Stdout.LogAttrs(ctx, slog.LevelError, "failed to establish subscription connection",
				slog.String(logAttrSubscription, string(s.logType)),
				logger.ErrAttr(err))
		}

		return err
	}

	s.conn = conn

	logger.Stdout.Debug("established new connection",
		slog.String(logAttrSubscription, string(s.logType)))

	return nil
}

// consume reads from the current connection until the stream ends, the context is
// cancelled, or onNext fails, handing each "next" payload to onNext. It returns whether at
// least one "next" message arrived (a productive stream) and a terminal error — context
// cancellation or an onNext failure — that should stop Run rather than be retried. A
// stream that merely ended (a non-"next" message, or a connection error) returns a nil
// error; whether the socket can be reused afterward is tracked by the Conn itself (a
// failed read marks it broken).
func (s *Subscription) consume(ctx context.Context, onNext func(payload []byte) error) (delivered bool, err error) {
	for {
		_, payload, readErr := s.conn.Read(ctx)
		if readErr != nil {
			if ctx.Err() != nil {
				return delivered, ctx.Err()
			}

			logger.Stdout.Debug("connection error, stream ended",
				slog.String(logAttrSubscription, string(s.logType)),
				logger.ErrAttr(readErr))

			return delivered, nil
		}

		var envelope struct {
			Type subscriptions.SubscriptionType `json:"type"`
		}
		if err := json.Unmarshal(payload, &envelope); err != nil {
			logger.Stdout.Debug("could not parse subscription message, skipping",
				slog.String(logAttrSubscription, string(s.logType)),
				logger.ErrAttr(err))
			continue
		}

		if envelope.Type != subscriptions.SubscriptionTypeNext {
			logger.Stdout.Debug("subscription ended",
				slog.String(logAttrSubscription, string(s.logType)),
				slog.String("type", string(envelope.Type)))

			return delivered, nil
		}

		delivered = true

		if err := onNext(payload); err != nil {
			return delivered, err
		}
	}
}

// Run establishes the stream and consumes it until ctx is cancelled (or onNext fails),
// re-establishing it whenever it ends. Re-establishment goes through queue.RetryBackoff:
// each attempt connects (reusing the socket after a clean end, redialing otherwise) and
// consumes the stream. An attempt that ends without delivering anything is retryable, so
// consecutive empty streams — e.g. a backend completing immediately while shedding load —
// grow the jittered backoff instead of spinning in a tight loop; an attempt that
// delivered at least one message succeeds, resetting the backoff before the next stream.
//
// onNext returning an error stops Run and returns that error; ctx cancellation returns
// its error. Otherwise Run retries indefinitely so a transient backend outage is ridden
// out rather than surfaced as a fatal error.
func (s *Subscription) Run(ctx context.Context, onNext func(payload []byte) error) error {
	for {
		err := queue.RetryBackoff(ctx,
			queue.Name(string(s.logType)),
			queue.MaxRetries(-1), // retry until ctx is cancelled or onNext fails
			queue.InitialBackoff(resubscribeInitialBackoff),
			queue.MaxBackoff(resubscribeMaxBackoff),
			queue.BackoffMultiplier(resubscribeBackoffFactor),
			queue.BackoffJitter(resubscribeBackoffJitter),
			func(ctx context.Context) error {
				if err := s.connect(ctx); err != nil {
					return queue.Retryable(fmt.Errorf("establishing %s subscription: %w", s.logType, err))
				}

				delivered, err := s.consume(ctx, onNext)
				if err != nil {
					return err
				}

				// Any delivered message — even one — counts as success and resets the
				// backoff. Only a stream that ended without delivering anything (e.g. the
				// backend completing immediately while shedding load) is retried, so the
				// backoff grows for genuinely empty streams, not for merely quiet ones.
				if !delivered {
					return queue.Retryable(errStreamEnded)
				}

				return nil
			},
		)
		if err != nil {
			return err
		}

		// A productive stream ended; loop to re-establish. RetryBackoff waits before every
		// attempt — including the first of this next call — so the productive path can't
		// spin into a same-second resubscribe loop.
	}
}
