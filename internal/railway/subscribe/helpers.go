package subscribe

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"time"

	"github.com/brody192/locomotive/internal/logger"
	"github.com/coder/websocket"
)

const (
	pingInterval = 30 * time.Second
	pingTimeout  = 10 * time.Second

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

// ResubscribeWithRetry closes the old connection and repeatedly calls createFn
// until it succeeds, the context is cancelled, or maxRetryDuration elapses.
// Extra slog attributes can be passed for per-call logging context.
func ResubscribeWithRetry(ctx context.Context, conn *Conn, maxRetryDuration time.Duration, createFn func(ctx context.Context) (*Conn, error), logAttrs ...slog.Attr) (*Conn, error) {
	conn.CloseNow()

	retryStart := time.Now()

	for attempt := 1; ; attempt++ {
		elapsed := time.Since(retryStart)
		if elapsed > maxRetryDuration {
			return nil, fmt.Errorf("failed to resubscribe after %v (%d attempts): maximum retry duration exceeded", elapsed.Round(time.Second), attempt-1)
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
			newConn, err := createFn(ctx)
			if err != nil {
				attrs := append([]slog.Attr{
					logger.ErrAttr(err),
					slog.Int("attempt", attempt),
					slog.Duration("elapsed", elapsed.Round(time.Millisecond)),
				}, logAttrs...)
				logger.Stdout.LogAttrs(ctx, slog.LevelDebug, "error resubscribing, will retry in 1 second", attrs...)

				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(1 * time.Second):
					continue
				}
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
}
