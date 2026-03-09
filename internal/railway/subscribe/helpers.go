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
