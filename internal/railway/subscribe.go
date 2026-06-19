package railway

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/brody192/locomotive/internal/railway/gql/subscriptions"
	"github.com/brody192/locomotive/internal/railway/subscribe"
	"github.com/coder/websocket"
)

func (g *GraphQLClient) CreateWebSocketSubscription(ctx context.Context, payload any) (*subscribe.Conn, error) {
	payloadBytes, err := subscriptions.NewSubscribeMessage(payload)
	if err != nil {
		return nil, err
	}

	opts := &websocket.DialOptions{
		HTTPHeader: http.Header{
			"Authorization": []string{fmt.Sprintf("Bearer %s", g.AuthToken.String())},
			"Content-Type":  []string{"application/json"},
		},
		Subprotocols: []string{"graphql-transport-ws"},
	}

	// Limit how many subscriptions initialize concurrently. The slot is released as
	// soon as this function returns — the established stream does not hold it.
	release, err := subscribe.AcquireOpenSlot(ctx)
	if err != nil {
		return nil, err
	}
	defer release()

	// Bound the whole handshake (dial + init + ack + subscribe) so a stuck open cannot
	// hold a concurrency slot indefinitely.
	ctxTimeout, cancel := context.WithTimeout(ctx, (10 * time.Second))
	defer cancel()

	c, _, err := websocket.Dial(ctxTimeout, g.BaseSubscriptionURL, opts)
	if err != nil {
		return nil, err
	}

	c.SetReadLimit(-1)

	if err := c.Write(ctxTimeout, websocket.MessageText, connectionInit); err != nil {
		c.CloseNow()
		return nil, err
	}

	_, ackMessage, err := c.Read(ctxTimeout)
	if err != nil {
		c.CloseNow()
		return nil, err
	}

	if !bytes.Equal(ackMessage, connectionAck) {
		c.CloseNow()
		return nil, errors.New("did not receive connection ack from server")
	}

	if err := c.Write(ctxTimeout, websocket.MessageText, payloadBytes); err != nil {
		c.CloseNow()
		return nil, err
	}

	return subscribe.NewConn(ctx, c), nil
}
