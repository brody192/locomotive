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

func createEnvironmentLogSubscription(ctx context.Context, client *railway.GraphQLClient, environmentId uuid.UUID, serviceIds []uuid.UUID) (*subscribe.Conn, error) {
	payload := &subscriptions.EnvironmentLogsSubscriptionPayload{
		Query: subscriptions.EnvironmentLogsSubscription,
		Variables: &subscriptions.EnvironmentLogsSubscriptionVariables{
			EnvironmentId: environmentId,
			Filter:        buildServiceFilter(serviceIds),

			// needed for seamless subscription resuming
			BeforeDate:  time.Now().UTC().Add(-5 * time.Minute).Format(time.RFC3339Nano),
			BeforeLimit: 500,
		},
	}

	return client.CreateWebSocketSubscription(ctx, payload)
}

func resubscribeServiceLogsWithRetry(ctx context.Context, client *railway.GraphQLClient, environmentId uuid.UUID, serviceIds []uuid.UUID, conn *subscribe.Conn) (*subscribe.Conn, error) {
	return subscribe.ResubscribeWithRetry(ctx, conn, (3600 * time.Second), func(ctx context.Context) (*subscribe.Conn, error) {
		return createEnvironmentLogSubscription(ctx, client, environmentId, serviceIds)
	})
}

func SubscribeToServiceLogs(ctx context.Context, g *railway.GraphQLClient, logTrack chan<- []EnvironmentLogWithMetadata, environmentId uuid.UUID, serviceIds []uuid.UUID) error {
	metadataMap, err := getMetadataMapForEnvironment(ctx, g.Client, environmentId)
	if err != nil {
		return fmt.Errorf("error getting metadata map: %w", err)
	}

	conn, err := createEnvironmentLogSubscription(ctx, g, environmentId, serviceIds)
	if err != nil {
		return err
	}

	defer func() { conn.CloseNow() }()

	LogTime := time.Now().UTC()

	for {
		_, logPayload, err := conn.Read(ctx)
		if err != nil {
			logger.Stdout.Debug("resubscribing",
				logger.ErrAttr(err),
			)

			conn, err = resubscribeServiceLogsWithRetry(ctx, g, environmentId, serviceIds, conn)
			if err != nil {
				return err
			}

			continue
		}

		logs := &subscriptions.EnvironmentLogsData{}

		if err := json.Unmarshal(logPayload, &logs); err != nil {
			return fmt.Errorf("error unmarshalling service logs: %w", err)
		}

		if logs.Type != subscriptions.SubscriptionTypeNext {
			logger.Stdout.Debug("resubscribing",
				slog.String("reason", fmt.Sprintf("log type not next: %s", logs.Type)),
			)

			conn, err = resubscribeServiceLogsWithRetry(ctx, g, environmentId, serviceIds, conn)
			if err != nil {
				return err
			}

			continue
		}

		filteredLogs := []EnvironmentLogWithMetadata{}

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
