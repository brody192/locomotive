package environment_invalidation

import (
	"context"
	"encoding/json"
	"fmt"
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
	sub, err := subscribe.NewSubscription(ctx, g.CreateWebSocketSubscription, func() any {
		return invalidationRequestPayload(environmentId)
	}, (3600 * time.Second))
	if err != nil {
		return err
	}

	defer func() { sub.Close() }()

	lastHash := ""

	for {
		_, payload, err := sub.Read(ctx)
		if err != nil {
			logger.Stdout.Debug("resubscribing",
				logger.ErrAttr(err),
			)

			// Connection broken: redial.
			if err := sub.Redial(ctx); err != nil {
				return err
			}

			continue
		}

		invalidationRequest := &subscriptions.CanvasInvalidationData{}

		if err := json.Unmarshal(payload, &invalidationRequest); err != nil {
			return fmt.Errorf("error unmarshalling invalidation request: %w", err)
		}

		if invalidationRequest.Type != subscriptions.SubscriptionTypeNext {
			logger.Stdout.Debug("subscription ended, resubscribing over existing connection",
				logger.ErrAttr(fmt.Errorf("log type not next: %s", invalidationRequest.Type)),
			)

			// Subscription completed but the socket is still alive: reuse it.
			if err := sub.Reuse(ctx); err != nil {
				return err
			}

			continue
		}

		if lastHash == "" {
			// logger.Stdout.Debug("skipping because last hash is empty", slog.String("id", invalidationRequest.Payload.Data.CanvasInvalidation.ID))
			lastHash = invalidationRequest.Payload.Data.CanvasInvalidation.ID
			continue
		}

		if invalidationRequest.Payload.Data.CanvasInvalidation.ID == lastHash {
			// logger.Stdout.Debug("skipping because last hash is the same", slog.String("id", invalidationRequest.Payload.Data.CanvasInvalidation.ID))
			continue
		}

		lastHash = invalidationRequest.Payload.Data.CanvasInvalidation.ID

		select {
		case environmentHashTrack <- invalidationRequest.Payload.Data.CanvasInvalidation.ID:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
