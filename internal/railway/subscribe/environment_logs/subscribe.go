package environment_logs

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/brody192/locomotive/internal/logger"
	"github.com/brody192/locomotive/internal/railway"
	"github.com/brody192/locomotive/internal/railway/gql/subscriptions"
	"github.com/brody192/locomotive/internal/railway/subscribe"
	"github.com/flexstack/uuid"
)

// environmentLogsPayload builds the subscription payload. beforeDate is the exclusive
// lower time bound: the backend streams logs with timestamp > beforeDate. We pass our
// cursor (connect time, then the last-seen log timestamp) so the backend only returns
// what's new, instead of re-scanning (and re-sending) a backlog window every time.
func environmentLogsPayload(environmentId uuid.UUID, serviceIds []uuid.UUID, beforeDate time.Time) *subscriptions.EnvironmentLogsSubscriptionPayload {
	return &subscriptions.EnvironmentLogsSubscriptionPayload{
		Query: subscriptions.EnvironmentLogsSubscription,
		Variables: &subscriptions.EnvironmentLogsSubscriptionVariables{
			EnvironmentId: environmentId,
			Filter:        buildServiceFilter(serviceIds),

			BeforeDate:  beforeDate.UTC().Format(time.RFC3339Nano),
			BeforeLimit: 500,
		},
	}
}

func SubscribeToServiceLogs(ctx context.Context, g *railway.GraphQLClient, logTrack chan<- []EnvironmentLogWithMetadata, environmentId uuid.UUID, serviceIds []uuid.UUID) error {
	metadataMap, err := getMetadataMapForEnvironment(ctx, g.Client, environmentId)
	if err != nil {
		return fmt.Errorf("error getting metadata map: %w", err)
	}

	// LogTime is our cursor into the log stream: it starts at connect time (we only
	// forward logs from startup onward) and advances to the last log we forward, so the
	// payload provider always asks for logs after what we've already seen — on the first
	// connect and every resubscribe alike.
	LogTime := time.Now().UTC()

	sub, err := subscribe.NewSubscription(ctx, g.CreateWebSocketSubscription, func() any {
		return environmentLogsPayload(environmentId, serviceIds, LogTime)
	}, (3600 * time.Second))
	if err != nil {
		return err
	}

	defer func() { sub.Close() }()

	for {
		_, logPayload, err := sub.Read(ctx)
		if err != nil {
			logger.Stdout.Debug("resubscribing",
				logger.ErrAttr(err),
			)

			// Connection broken: redial, resuming from the last log we saw.
			if err := sub.Redial(ctx); err != nil {
				return err
			}

			continue
		}

		logs := &subscriptions.EnvironmentLogsData{}

		if err := json.Unmarshal(logPayload, &logs); err != nil {
			return fmt.Errorf("error unmarshalling service logs: %w", err)
		}

		if logs.Type != subscriptions.SubscriptionTypeNext {
			logger.Stdout.Debug("subscription ended, resubscribing over existing connection",
				slog.String("reason", fmt.Sprintf("log type not next: %s", logs.Type)),
			)

			// Subscription completed but the socket is still alive: reuse it,
			// resuming from the last log we saw.
			if err := sub.Reuse(ctx); err != nil {
				return err
			}

			continue
		}

		filteredLogs := make([]EnvironmentLogWithMetadata, 0, len(logs.Payload.Data.EnvironmentLogs))

		for i := range logs.Payload.Data.EnvironmentLogs {
			// skip logs with empty messages and no attributes
			// we check for 1 attribute because empty logs will always have at least one attribute, the level
			if logs.Payload.Data.EnvironmentLogs[i].Message == "" && len(logs.Payload.Data.EnvironmentLogs[i].Attributes) == 1 {
				continue
			}

			// skip container logs, container logs have trailing zeros in the timestamp
			if strings.HasSuffix(logs.Payload.Data.EnvironmentLogs[i].Timestamp.Format(time.StampNano), "000000000") {
				logger.Stdout.Debug("skipping container log message")
				continue
			}

			// on first subscription skip logs if they were logged before the first subscription, on resubscription skip logs if they were already processed
			if !logs.Payload.Data.EnvironmentLogs[i].Timestamp.After(LogTime) {
				// logger.Stdout.Debug("skipping stale log message")
				continue
			}

			LogTime = logs.Payload.Data.EnvironmentLogs[i].Timestamp

			serviceName := metadataName(metadataMap, logs.Payload.Data.EnvironmentLogs[i].Tags.ServiceID, "service")
			environmentName := metadataName(metadataMap, logs.Payload.Data.EnvironmentLogs[i].Tags.EnvironmentID, "environment")
			projectName := metadataName(metadataMap, logs.Payload.Data.EnvironmentLogs[i].Tags.ProjectID, "project")

			filteredLogs = append(filteredLogs, EnvironmentLogWithMetadata{
				Log: logs.Payload.Data.EnvironmentLogs[i],
				Metadata: map[string]string{
					subscribe.MetadataKeyProjectName: projectName,
					subscribe.MetadataKeyProjectID:   logs.Payload.Data.EnvironmentLogs[i].Tags.ProjectID.String(),

					subscribe.MetadataKeyEnvironmentName: environmentName,
					subscribe.MetadataKeyEnvironmentID:   logs.Payload.Data.EnvironmentLogs[i].Tags.EnvironmentID.String(),

					subscribe.MetadataKeyServiceName: serviceName,
					subscribe.MetadataKeyServiceID:   logs.Payload.Data.EnvironmentLogs[i].Tags.ServiceID.String(),

					subscribe.MetadataKeyDeploymentID:         logs.Payload.Data.EnvironmentLogs[i].Tags.DeploymentID.String(),
					subscribe.MetadataKeyDeploymentInstanceID: logs.Payload.Data.EnvironmentLogs[i].Tags.DeploymentInstanceID.String(),

					subscribe.MetadataKeyLogType: subscribe.LogTypeEnvironment,
				},
			})
		}

		if len(filteredLogs) == 0 {
			continue
		}

		select {
		case logTrack <- filteredLogs:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
