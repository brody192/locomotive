package environment_invalidation

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/brody192/locomotive/internal/logger"
	"github.com/brody192/locomotive/internal/railway"
	"github.com/brody192/locomotive/internal/railway/gql/subscriptions"
	"github.com/brody192/locomotive/internal/railway/subscribe"
	"github.com/flexstack/uuid"
)

func createInvalidationRequestSubscription(ctx context.Context, g *railway.GraphQLClient, environmentId uuid.UUID) (*subscribe.Conn, error) {
	payload := &subscriptions.CanvasInvalidationSubscriptionPayload{
		Query: subscriptions.CanvasInvalidationSubscription,
		Variables: &subscriptions.CanvasInvalidationSubscriptionVariables{
			EnvironmentId: environmentId,
		},
	}

	return g.CreateWebSocketSubscription(ctx, payload)
}

func resubscribeWithRetry(ctx context.Context, g *railway.GraphQLClient, environmentId uuid.UUID, conn *subscribe.Conn) (*subscribe.Conn, error) {
	return subscribe.ResubscribeWithRetry(ctx, conn, (3600 * time.Second), func(ctx context.Context) (*subscribe.Conn, error) {
		return createInvalidationRequestSubscription(ctx, g, environmentId)
	})
}

func SubscribeToInvalidationRequests(ctx context.Context, g *railway.GraphQLClient, environmentHashTrack chan<- string, environmentId uuid.UUID) error {
	conn, err := createInvalidationRequestSubscription(ctx, g, environmentId)
	if err != nil {
		return err
	}

	defer func() { conn.CloseNow() }()

	lastHash := ""

	for {
		_, payload, err := conn.Read(ctx)
		if err != nil {
			logger.Stdout.Debug("resubscribing",
				slog.String("from", "SubscribeToInvalidationRequests_SafeConnRead"),
				logger.ErrAttr(err),
			)

			conn, err = resubscribeWithRetry(ctx, g, environmentId, conn)
			if err != nil {
				return err
			}

			continue
		}

		invalidationRequest := &subscriptions.CanvasInvalidationData{}

		if err := json.Unmarshal(payload, &invalidationRequest); err != nil {
			return fmt.Errorf("error unmarshalling invalidation request: %w", err)
		}

		if invalidationRequest.Type != subscriptions.SubscriptionTypeNext {
			logger.Stdout.Debug("resubscribing",
				slog.String("from", "SubscribeToInvalidationRequests_TypeNotNext"),
				logger.ErrAttr(fmt.Errorf("log type not next: %s", invalidationRequest.Type)),
			)

			conn, err = resubscribeWithRetry(ctx, g, environmentId, conn)
			if err != nil {
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
