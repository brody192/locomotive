package http_logs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/brody192/locomotive/internal/logger"
	"github.com/brody192/locomotive/internal/railway"
	"github.com/brody192/locomotive/internal/railway/gql/subscriptions"
	"github.com/brody192/locomotive/internal/railway/subscribe"
	"github.com/brody192/locomotive/internal/railway/subscribe/deployment_changes"
	"github.com/brody192/locomotive/internal/slice"
	"github.com/flexstack/uuid"
)

// httpLogsInitialBacklog is the lower time bound used for the very first subscription
// of a deployment, to pick up logs emitted shortly before locomotive connected.
const httpLogsInitialBacklog = 24 * time.Hour

// httpLogsPayload builds the subscription payload. beforeDate is the exclusive lower
// time bound: the backend streams logs with timestamp > beforeDate. On resubscribe we
// pass the last-seen log timestamp so the backend only returns what's new, instead of
// re-scanning (and re-sending) the whole backlog window every time.
func httpLogsPayload(deploymentId uuid.UUID, beforeDate time.Time) *subscriptions.HttpLogsSubscriptionPayload {
	return &subscriptions.HttpLogsSubscriptionPayload{
		Query: subscriptions.HttpLogsSubscription,
		Variables: &subscriptions.HttpLogsSubscriptionVariables{
			BeforeDate:   beforeDate.UTC().Format(time.RFC3339Nano),
			BeforeLimit:  500,
			DeploymentId: deploymentId,
			Filter:       "",
		},
	}
}

func SubscribeToHttpLogs(ctx context.Context, g *railway.GraphQLClient, logTrack chan<- []DeploymentHttpLogWithMetadata, environmentId uuid.UUID, serviceIds []uuid.UUID) error {
	// initTime is the floor for forwarded logs: we only ship logs emitted after startup,
	// shared by every per-deployment goroutine.
	initTime := time.Now()

	// Cancel everything this function starts (per-deployment goroutines, the deployment
	// changes subscription, the flush loop) when it returns.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	deploymentIdSlice := slice.NewSync[uuid.UUID]()
	changeDetected := make(chan struct{})
	errorChan := make(chan error, 1)

	go func() {
		logger.Stdout.Debug("starting deployment ID changes subscription", slog.String("environment_id", environmentId.String()), slog.Any("service_ids", serviceIds))

		if err := deployment_changes.SubscribeToDeploymentIdChanges(ctx, g, deploymentIdSlice, changeDetected, environmentId, serviceIds); err != nil {
			if errors.Is(err, context.Canceled) {
				errorChan <- ctx.Err()
				return
			}

			errorChan <- fmt.Errorf("error subscribing to deployment id changes: %w", err)

			return
		}
	}()

	bufferedLogTrack := make(chan []DeploymentHttpLogWithMetadata)
	var httpLogBuffer []DeploymentHttpLogWithMetadata

	go func() {
		ticker := time.NewTicker(flushInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if len(httpLogBuffer) == 0 {
					continue
				}

				toSend := httpLogBuffer
				httpLogBuffer = nil

				select {
				case logTrack <- toSend:
				case <-ctx.Done():
					return
				}
			case logs := <-bufferedLogTrack:
				httpLogBuffer = append(httpLogBuffer, logs...)
			}
		}
	}()

	// running maps each deployment with a live goroutine to its cancel func. It is only
	// touched by the loop below (single writer), so it needs no synchronization. A
	// goroutine reports its exit on done so its entry can be reclaimed.
	running := map[uuid.UUID]context.CancelFunc{}
	done := make(chan uuid.UUID, 16)

	startLogGoroutine := func(deploymentID uuid.UUID) {
		logger.Stdout.Debug("starting HTTP log goroutine for deployment", slog.String("deployment_id", deploymentID.String()))

		depCtx, depCancel := context.WithCancel(ctx)
		running[deploymentID] = depCancel

		go func() {
			err := getHttpLogs(depCtx, g, deploymentID, initTime, bufferedLogTrack)
			metadataDeploymentCache.Delete(deploymentID)

			// A cancelled deployment (no longer wanted, or shutdown) is a clean exit;
			// anything else is fatal for the whole HTTP log pipeline.
			if err != nil && !errors.Is(err, context.Canceled) {
				select {
				case errorChan <- err:
				default:
				}
			}

			select {
			case done <- deploymentID:
			case <-ctx.Done():
			}
		}()
	}

	// syncDeployments starts goroutines for newly-wanted deployments and cancels ones no
	// longer wanted (e.g. a deployment that's been torn down).
	syncDeployments := func() {
		wanted := deploymentIdSlice.Get()

		wantedIDs := make(map[uuid.UUID]struct{}, len(wanted))
		for _, id := range wanted {
			wantedIDs[id] = struct{}{}
			if _, ok := running[id]; !ok {
				startLogGoroutine(id)
			}
		}

		for id, depCancel := range running {
			if _, ok := wantedIDs[id]; !ok {
				logger.Stdout.Debug("deployment no longer wanted, stopping goroutine", slog.String("deployment_id", id.String()))
				depCancel()
			}
		}
	}

	// Wait for initial deployment IDs
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errorChan:
		return err
	case <-changeDetected:
		logger.Stdout.Debug("initial deployment IDs received", slog.Any("deployment_ids", deploymentIdSlice.Get()))
		syncDeployments()
	}

	// Main loop to handle deployment ID changes
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-errorChan:
			return err
		case id := <-done:
			delete(running, id)
		case <-changeDetected:
			syncDeployments()
		}
	}
}

func getHttpLogs(ctx context.Context, g *railway.GraphQLClient, deploymentID uuid.UUID, initTime time.Time, logTrack chan<- []DeploymentHttpLogWithMetadata) error {
	// logTimes is our cursor into the log stream: it starts at the backlog horizon and
	// advances to the last log we forward, so the payload provider always asks for logs
	// after what we've already seen — on the first connect and every resubscribe alike.
	logTimes := time.Now().Add(-httpLogsInitialBacklog)

	sub := subscribe.NewSubscription(subscribe.LogTypeHTTP, g.CreateWebSocketSubscription, func() any {
		return httpLogsPayload(deploymentID, logTimes)
	})

	defer func() { sub.Close() }()

	logger.Stdout.Debug("successfully created HTTP log subscription", slog.String("deployment_id", deploymentID.String()))

	metadata, err := getMetadataForDeployment(ctx, g, deploymentID)
	if err != nil {
		return fmt.Errorf("error getting metadata for deployment %s: %w", deploymentID, err)
	}

	metadata[subscribe.MetadataKeyLogType] = string(subscribe.LogTypeHTTP)

	return sub.Run(ctx, func(payload []byte) error {
		logs := &subscriptions.HttpLogsData{}
		if err := json.Unmarshal(payload, &logs); err != nil {
			logger.Stdout.Error("failed to unmarshal log payload",
				slog.String("deployment_id", deploymentID.String()),
				logger.ErrAttr(err),
			)

			return nil
		}

		if len(logs.Payload.Data.HTTPLogs) == 0 {
			return nil
		}

		filteredHttpLogs := make([]DeploymentHttpLogWithMetadata, 0, len(logs.Payload.Data.HTTPLogs))

		for i := range logs.Payload.Data.HTTPLogs {
			logTimestamp, err := getTimeStampAttributeFromHttpLog(logs.Payload.Data.HTTPLogs[i])
			if err != nil {
				logger.Stdout.Error("failed to get timestamp from http log",
					slog.String("deployment_id", deploymentID.String()),
					logger.ErrAttr(err),
				)

				// we return an error here because this isn't something we can recover from
				return fmt.Errorf("failed to get timestamp from http log: %w", err)
			}

			if !logTimestamp.After(logTimes) || logTimestamp.Before(initTime) {
				continue
			}

			path, err := getStringAttributeFromHttpLog(logs.Payload.Data.HTTPLogs[i], "path")
			if err != nil {
				logger.Stdout.Error("failed to get path from http log",
					slog.String("deployment_id", deploymentID.String()),
					logger.ErrAttr(err),
				)
			}

			statusCode, err := getInt64AttributeFromHttpLog(logs.Payload.Data.HTTPLogs[i], "httpStatus")
			if err != nil {
				logger.Stdout.Error("failed to get status code from http log",
					slog.String("deployment_id", deploymentID.String()),
					logger.ErrAttr(err),
				)
			}

			filteredHttpLogs = append(filteredHttpLogs, DeploymentHttpLogWithMetadata{
				Timestamp: logTimestamp,

				Log:        logs.Payload.Data.HTTPLogs[i],
				Path:       path,
				StatusCode: statusCode,

				Metadata: metadata,
			})

			logTimes = logTimestamp
		}

		if len(filteredHttpLogs) == 0 {
			return nil
		}

		select {
		case logTrack <- filteredHttpLogs:
		case <-ctx.Done():
			return ctx.Err()
		}

		return nil
	})
}
