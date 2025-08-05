package railway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/brody192/locomotive/internal/railway/gql/subscriptions"
	"github.com/coder/websocket"
	"github.com/flexstack/uuid"
)

func (g *GraphQLClient) CreateWebSocketSubscription(ctx context.Context, payload any) (*websocket.Conn, error) {
	subPayload := map[string]any{
		"id":      uuid.Must(uuid.NewV4()),
		"type":    subscriptions.SubscriptionTypeSubscribe,
		"payload": payload,
	}

	payloadBytes, err := json.Marshal(&subPayload)
	if err != nil {
		return nil, err
	}

	opts := &websocket.DialOptions{
		HTTPHeader: http.Header{
			"Authorization": []string{"Bearer " + g.AuthToken.String()},
			"Content-Type":  []string{"application/json"},
		},
		Subprotocols: []string{"graphql-transport-ws"},
	}

	ctxTimeout, cancel := context.WithTimeout(ctx, (10 * time.Second))
	defer cancel()

	c, _, err := websocket.Dial(ctxTimeout, g.BaseSubscriptionURL, opts)
	if err != nil {
		return nil, err
	}

	c.SetReadLimit(-1)

	if err := c.Write(ctx, websocket.MessageText, connectionInit); err != nil {
		return nil, err
	}

	_, ackMessage, err := c.Read(ctx)
	if err != nil {
		return nil, err
	}

	if !bytes.Equal(ackMessage, connectionAck) {
		return nil, errors.New("did not receive connection ack from server")
	}

	if err := c.Write(ctx, websocket.MessageText, payloadBytes); err != nil {
		return nil, err
	}

	return c, nil
}
