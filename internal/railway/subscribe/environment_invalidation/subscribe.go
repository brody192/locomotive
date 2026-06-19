package environment_invalidation

import (
	"context"
	"encoding/json"
	"time"

	"github.com/brody192/locomotive/internal/logger"
	"github.com/brody192/locomotive/internal/railway"
	"github.com/brody192/locomotive/internal/railway/gql/subscriptions"
	"github.com/brody192/locomotive/internal/railway/subscribe"
	"github.com/flexstack/uuid"
)

func invalidationRequestPayload(environmentId uuid.UUID) *subscriptions.CanvasInvalidationSubscriptionPayload {
	return &subscriptions.CanvasInvalidationSubscriptionPayload{
		Query: subscriptions.CanvasInvalidationSubscription,
		Variables: &subscriptions.CanvasInvalidationSubscriptionVariables{
			EnvironmentId: environmentId,
		},
	}
}

func SubscribeToInvalidationRequests(ctx context.Context, g *railway.GraphQLClient, environmentHashTrack chan<- string, environmentId uuid.UUID) error {
	sub, err := subscribe.NewSubscription(ctx, subscribe.LogTypeEnvironmentInvalidation, g.CreateWebSocketSubscription, func() any {
		return invalidationRequestPayload(environmentId)
	}, (3600 * time.Second))
	if err != nil {
		return err
	}

	defer func() { sub.Close() }()

	lastHash := ""

	return sub.Run(ctx, func(payload []byte) error {
		invalidationRequest := &subscriptions.CanvasInvalidationData{}
		if err := json.Unmarshal(payload, &invalidationRequest); err != nil {
			logger.Stdout.Error("failed to unmarshal invalidation request", logger.ErrAttr(err))
			return nil
		}

		id := invalidationRequest.Payload.Data.CanvasInvalidation.ID

		// First message just seeds the baseline; only forward subsequent changes.
		if lastHash == "" {
			lastHash = id
			return nil
		}

		if id == lastHash {
			return nil
		}

		lastHash = id

		select {
		case environmentHashTrack <- id:
		case <-ctx.Done():
			return ctx.Err()
		}

		return nil
	})
}
