package subscriptions

import (
	_ "embed"
	"encoding/json"

	"github.com/flexstack/uuid"
)

//go:embed environment_logs.graphql
var EnvironmentLogsSubscription string

//go:embed canvas_invalidation.graphql
var CanvasInvalidationSubscription string

//go:embed http_logs.graphql
var HttpLogsSubscription string

// NewSubscribeMessage builds a graphql-transport-ws "subscribe" message envelope
// (a fresh random id, the subscribe type, and the given payload), ready to be written
// to a WebSocket connection — either on a freshly dialed connection or over an
// existing one to restart a completed subscription.
func NewSubscribeMessage(payload any) ([]byte, error) {
	return json.Marshal(map[string]any{
		"id":      uuid.Must(uuid.NewV4()),
		"type":    SubscriptionTypeSubscribe,
		"payload": payload,
	})
}
