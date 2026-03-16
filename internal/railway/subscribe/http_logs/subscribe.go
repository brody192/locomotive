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

func createHttpLogSubscription(ctx context.Context, g *railway.GraphQLClient, deploymentId uuid.UUID) (*subscribe.Conn, error) {
	payload := &subscriptions.HttpLogsSubscriptionPayload{
		Query: subscriptions.HttpLogsSubscription,
		Variables: &subscriptions.HttpLogsSubscriptionVariables{
			BeforeDate:   time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339Nano),
			BeforeLimit:  500,
			DeploymentId: deploymentId,
			Filter:       "",
		},
	}

	return g.CreateWebSocketSubscription(ctx, payload)
}

func resubscribeHttpLogsWithRetry(ctx context.Context, g *railway.GraphQLClient, deploymentId uuid.UUID, conn *subscribe.Conn) (*subscribe.Conn, error) {
	return subscribe.ResubscribeWithRetry(ctx, conn, (3600 * time.Second), func(ctx context.Context) (*subscribe.Conn, error) {
		return createHttpLogSubscription(ctx, g, deploymentId)
	}, slog.String("deployment_id", deploymentId.String()))
}

func SubscribeToHttpLogs(ctx context.Context, g *railway.GraphQLClient, logTrack chan<- []DeploymentHttpLogWithMetadata, environmentId uuid.UUID, serviceIds []uuid.UUID) error {
	deploymentIdSlice := slice.NewSync[deployment_changes.DeploymentIdWithInfo]()
	changeDetected := make(chan struct{})
	errorChan := make(chan error, 1)

	ctx = context.WithValue(ctx, funcInitTimeKey, time.Now())

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

				toSend := append([]DeploymentHttpLogWithMetadata(nil), httpLogBuffer...)
				httpLogBuffer = httpLogBuffer[:0]

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

	// Track which deployment IDs have active goroutines
	activeDeploymentIds := slice.NewSync[uuid.UUID]()

	startLogGoroutine := func(deployment deployment_changes.DeploymentIdWithInfo) {
		activeDeploymentIds.Append(deployment.ID)

		go func() {
			defer activeDeploymentIds.Delete(deployment.ID)

			if err := getHttpLogs(ctx, g, deployment, bufferedLogTrack, deploymentIdSlice); err != nil {
				select {
				case errorChan <- err:
				default:
				}
			}
		}()
	}

	// Wait for initial deployment IDs
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errorChan:
		return err
	case <-changeDetected:
		logger.Stdout.Debug("initial deployment IDs received", slog.Any("deployment_ids", deploymentIdSlice.Get()))

		for _, deployment := range deploymentIdSlice.Get() {
			logger.Stdout.Debug("starting initial HTTP log goroutine for deployment", slog.String("deployment_id", deployment.ID.String()))
			startLogGoroutine(deployment)
		}
	}

	// Main loop to handle deployment ID changes
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-errorChan:
			return err
		case <-changeDetected:
			for _, deployment := range deploymentIdSlice.Get() {
				if !activeDeploymentIds.Contains(deployment.ID) {
					logger.Stdout.Debug("starting new goroutine for new deployment", slog.String("deployment_id", deployment.ID.String()))
					startLogGoroutine(deployment)
				}
			}
		}
	}
}

func getHttpLogs(ctx context.Context, g *railway.GraphQLClient, initialDeployment deployment_changes.DeploymentIdWithInfo, logTrack chan<- []DeploymentHttpLogWithMetadata, activeDeployments *slice.Sync[deployment_changes.DeploymentIdWithInfo]) error {
	conn, err := createHttpLogSubscription(ctx, g, initialDeployment.ID)
	if err != nil {
		return fmt.Errorf("failed to create subscription for deployment %s: %w", initialDeployment.ID, err)
	}

	defer func() { conn.CloseNow() }()

	initTime, ok := ctx.Value(funcInitTimeKey).(time.Time)
	if !ok {
		return fmt.Errorf("missing or invalid init time in context for deployment %s", initialDeployment.ID)
	}

	logTimes := initialDeployment.CreatedAt

	logger.Stdout.Debug("successfully created HTTP log subscription", slog.String("deployment_id", initialDeployment.ID.String()))

	metadata, err := getMetadataForDeployment(ctx, g, initialDeployment.ID)
	if err != nil {
		return fmt.Errorf("error getting metadata for deployment %s: %w", initialDeployment.ID, err)
	}

	metadata[subscribe.MetadataKeyLogType] = subscribe.LogTypeHTTP

	// Main loop for reading from this specific connection
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			// Check if this deployment ID is still wanted
			if !activeDeployments.Contains(initialDeployment) {
				logger.Stdout.Debug("deployment id no longer wanted, exiting goroutine",
					slog.String("deployment_id", initialDeployment.ID.String()),
				)

				return nil
			}

			_, logPayload, err := conn.Read(ctx)
			if err != nil {
				if errors.Is(err, context.DeadlineExceeded) {
					// No data available, continue
					continue
				}

				if !activeDeployments.Contains(initialDeployment) {
					logger.Stdout.Debug("deployment id no longer wanted, exiting goroutine",
						slog.String("deployment_id", initialDeployment.ID.String()),
					)

					return nil
				}

				logger.Stdout.Debug("resubscribing",
					slog.String("deployment_id", initialDeployment.ID.String()),
					logger.ErrAttr(err),
				)

				// Close old connection and create new one
				newConn, err := resubscribeHttpLogsWithRetry(ctx, g, initialDeployment.ID, conn)
				if err != nil {
					return fmt.Errorf("failed to resubscribe for deployment %s: %w", initialDeployment.ID, err)
				}

				conn = newConn

				continue
			}

			logs := &subscriptions.HttpLogsData{}

			if err := json.Unmarshal(logPayload, &logs); err != nil {
				logger.Stdout.Error("failed to unmarshal log payload",
					slog.String("deployment_id", initialDeployment.ID.String()),
					logger.ErrAttr(err),
				)

				continue
			}

			if logs.Type != subscriptions.SubscriptionTypeNext {
				logger.Stdout.Debug("unexpected log type, resubscribing",
					slog.String("deployment_id", initialDeployment.ID.String()),
					slog.String("type", string(logs.Type)),
				)

				// Close old connection and create new one
				newConn, err := resubscribeHttpLogsWithRetry(ctx, g, initialDeployment.ID, conn)
				if err != nil {
					logger.Stdout.Error("failed to resubscribe",
						slog.String("deployment_id", initialDeployment.ID.String()),
						logger.ErrAttr(err),
					)

					return err
				}

				conn = newConn

				continue
			}

			if len(logs.Payload.Data.HTTPLogs) == 0 {
				continue
			}

			filteredHttpLogs := []DeploymentHttpLogWithMetadata{}

			for i := range logs.Payload.Data.HTTPLogs {
				logTimestamp, err := getTimeStampAttributeFromHttpLog(logs.Payload.Data.HTTPLogs[i])
				if err != nil {
					logger.Stdout.Error("failed to get timestamp from http log",
						slog.String("deployment_id", initialDeployment.ID.String()),
						logger.ErrAttr(err),
					)

					// we return an error here because this isn't something we can recover from
					// returning here will cause the goroutine to exit and the parent SubscribeToHttpLogs function to return the error
					return fmt.Errorf("failed to get timestamp from http log: %w", err)
				}

				if !logTimestamp.After(logTimes) || logTimestamp.Before(initTime) {
					continue
				}

				path, err := getStringAttributeFromHttpLog(logs.Payload.Data.HTTPLogs[i], "path")
				if err != nil {
					logger.Stdout.Error("failed to get path from http log",
						slog.String("deployment_id", initialDeployment.ID.String()),
						logger.ErrAttr(err),
					)
				}

				statusCode, err := getInt64AttributeFromHttpLog(logs.Payload.Data.HTTPLogs[i], "httpStatus")
				if err != nil {
					logger.Stdout.Error("failed to get status code from http log",
						slog.String("deployment_id", initialDeployment.ID.String()),
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
				continue
			}

			select {
			case logTrack <- filteredHttpLogs:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
}
