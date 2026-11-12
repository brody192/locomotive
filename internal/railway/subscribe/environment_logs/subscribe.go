package environment_logs

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/brody192/locomotive/internal/logger"
	"github.com/brody192/locomotive/internal/railway"
	"github.com/brody192/locomotive/internal/railway/gql/subscriptions"
	"github.com/brody192/locomotive/internal/railway/subscribe"
	"github.com/flexstack/uuid"
)

// environmentLogsPayload builds the subscription payload. beforeDate is the exclusive
// lower time bound. Reconnect overlap is bounded by the retained replay cache.
func environmentLogsPayload(environmentId uuid.UUID, serviceIds []uuid.UUID, beforeDate time.Time) *subscriptions.EnvironmentLogsSubscriptionPayload {
	return &subscriptions.EnvironmentLogsSubscriptionPayload{
		Query: subscriptions.EnvironmentLogsSubscription,
		Variables: &subscriptions.EnvironmentLogsSubscriptionVariables{
			EnvironmentId: environmentId,
			Filter:        buildServiceFilter(serviceIds),
			BeforeDate:    beforeDate.UTC().Format(time.RFC3339Nano),
			// Request a large batch so we keep up with high-throughput environments.
			BeforeLimit: 5000,
		},
	}
}

type serviceLogForwarder struct {
	startedAt       time.Time
	newestForwarded time.Time
	metadataMap     map[uuid.UUID]string
	logTrack        chan<- []EnvironmentLogWithMetadata
	replay          logReplayCache
}

func (f *serviceLogForwarder) subscriptionPayload(environmentId uuid.UUID, serviceIds []uuid.UUID) *subscriptions.EnvironmentLogsSubscriptionPayload {
	before := f.replay.resumeBefore(f.startedAt, f.newestForwarded)
	return environmentLogsPayload(environmentId, serviceIds, before)
}

func SubscribeToServiceLogs(ctx context.Context, g *railway.GraphQLClient, logTrack chan<- []EnvironmentLogWithMetadata, environmentId uuid.UUID, serviceIds []uuid.UUID) error {
	metadataMap, err := getMetadataMapForEnvironment(ctx, g.Client, environmentId)
	if err != nil {
		return fmt.Errorf("error getting metadata map: %w", err)
	}

	startedAt := time.Now().UTC()
	forwarder := &serviceLogForwarder{
		startedAt:       startedAt,
		newestForwarded: startedAt,
		metadataMap:     metadataMap,
		logTrack:        logTrack,
	}

	sub := subscribe.NewSubscription(subscribe.LogTypeEnvironment, g.CreateWebSocketSubscription, func() any {
		return forwarder.subscriptionPayload(environmentId, serviceIds)
	})
	defer func() { sub.Close() }()

	return sub.Run(ctx, func(payload []byte) error {
		return forwarder.forward(ctx, payload)
	})
}

func (f *serviceLogForwarder) forward(ctx context.Context, payload []byte) error {
	logs := &subscriptions.EnvironmentLogsData{}
	if err := json.Unmarshal(payload, &logs); err != nil {
		logger.Stdout.Error("failed to unmarshal service logs", logger.ErrAttr(err))
		return nil
	}

	filteredLogs := make([]EnvironmentLogWithMetadata, 0, len(logs.Payload.Data.EnvironmentLogs))
	replayed := make(map[logFingerprint]int)
	added := make(map[logFingerprint]forwardedLog)
	newest := f.newestForwarded
	for i := range logs.Payload.Data.EnvironmentLogs {
		log := logs.Payload.Data.EnvironmentLogs[i]
		// Empty logs have only the level attribute.
		if log.Message == "" && len(log.Attributes) == 1 {
			continue
		}

		// Container logs have trailing zeros in the timestamp.
		if strings.HasSuffix(log.Timestamp.Format(time.StampNano), "000000000") {
			logger.Stdout.Debug("skipping container log message")
			continue
		}

		// Keep the startup boundary fixed. Older and equal-timestamp arrivals
		// after startup are valid records. The reconnect position must not
		// be used to discard these independently received lines.
		if !log.Timestamp.After(f.startedAt) {
			continue
		}
		key, err := fingerprintLog(log)
		if err != nil {
			return fmt.Errorf("fingerprint service log: %w", err)
		}
		if replayed[key] < f.replay.remaining[key] {
			replayed[key]++
			continue
		}
		addition := added[key]
		addition.timestamp = log.Timestamp
		addition.count++
		added[key] = addition
		if log.Timestamp.After(newest) {
			newest = log.Timestamp
		}

		serviceName := metadataName(f.metadataMap, log.Tags.ServiceID, "service")
		environmentName := metadataName(f.metadataMap, log.Tags.EnvironmentID, "environment")
		projectName := metadataName(f.metadataMap, log.Tags.ProjectID, "project")

		filteredLogs = append(filteredLogs, EnvironmentLogWithMetadata{
			Log: log,
			Metadata: map[string]string{
				subscribe.MetadataKeyProjectName: projectName,
				subscribe.MetadataKeyProjectID:   log.Tags.ProjectID.String(),

				subscribe.MetadataKeyEnvironmentName: environmentName,
				subscribe.MetadataKeyEnvironmentID:   log.Tags.EnvironmentID.String(),

				subscribe.MetadataKeyServiceName: serviceName,
				subscribe.MetadataKeyServiceID:   log.Tags.ServiceID.String(),

				subscribe.MetadataKeyDeploymentID:         log.Tags.DeploymentID.String(),
				subscribe.MetadataKeyDeploymentInstanceID: log.Tags.DeploymentInstanceID.String(),

				subscribe.MetadataKeyLogType: string(subscribe.LogTypeEnvironment),
			},
		})
	}

	if len(filteredLogs) == 0 {
		f.replay.commit(replayed, added, f.startedAt, f.newestForwarded)
		return nil
	}

	select {
	case f.logTrack <- filteredLogs:
		f.newestForwarded = newest
		f.replay.commit(replayed, added, f.startedAt, newest)
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
