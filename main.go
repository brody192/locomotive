package main

import (
	"context"
	"log/slog"
	"os"
	"sync/atomic"
	"time"

	"github.com/brody192/locomotive/internal/config"
	"github.com/brody192/locomotive/internal/errgroup"
	"github.com/brody192/locomotive/internal/logger"
	"github.com/brody192/locomotive/internal/otel"
	"github.com/brody192/locomotive/internal/railway"
	"github.com/brody192/locomotive/internal/railway/subscribe/environment_logs"
	"github.com/brody192/locomotive/internal/railway/subscribe/http_logs"
)

func main() {
	logger.Stdout.Info("Preparing the locomotive for departure...")

	// Initialize OTEL if enabled
	if config.Otel.Enabled {
		ctx := context.Background()
		if err := otel.Setup(ctx, otel.Config{
			Enabled:         config.Otel.Enabled,
			Endpoint:        config.Otel.Endpoint,
			ServiceName:     config.Otel.ServiceName,
			EnvironmentName: config.Otel.EnvironmentName,
		}); err != nil {
			logger.Stderr.Error("failed to setup OTEL", logger.ErrAttr(err))
			os.Exit(1)
		}
		defer func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := otel.Shutdown(shutdownCtx); err != nil {
				logger.Stderr.Error("failed to shutdown OTEL", logger.ErrAttr(err))
			}
		}()
	}

	gqlClient, err := railway.NewClient(&railway.GraphQLClient{
		AuthToken:           config.Global.RailwayApiKey,
		BaseURL:             "https://backboard.railway.app/graphql/v2",
		BaseSubscriptionURL: "wss://backboard.railway.app/graphql/internal",
	})
	if err != nil {
		logger.Stderr.Error("error creating graphql client", logger.ErrAttr(err))
		os.Exit(1)
	}

	allServicesExist, foundServices, missingServices, err := railway.VerifyAllServicesExistWithinEnvironment(gqlClient, config.Global.ServiceIds, config.Global.EnvironmentId)
	if err != nil {
		logger.Stderr.Error("error verifying if services exist within the environment", logger.ErrAttr(err))
		os.Exit(1)
	}

	if !allServicesExist {
		logger.Stderr.Error("all services must exist within the environment set by the LOCOMOTIVE_ENVIRONMENT_ID variable",
			slog.Any("missing_service_ids", missingServices),
			slog.Any("configured_service_ids", config.Global.ServiceIds),
			slog.Any("found_service_ids", foundServices),
			slog.Any("environment_id", config.Global.EnvironmentId),
		)

		os.Exit(1)
	}

	logAttrs := []any{
		slog.Any("service_ids", config.Global.ServiceIds),
		slog.Any("environment_id", config.Global.EnvironmentId),
		slog.Bool("enable_http_logs", config.Global.EnableHttpLogs),
		slog.Bool("enable_deploy_logs", config.Global.EnableDeployLogs),
		slog.Bool("otel_enabled", config.Otel.Enabled),
	}

	if config.Otel.Enabled {
		logAttrs = append(logAttrs,
			slog.String("otel_endpoint", config.Otel.Endpoint),
			slog.String("otel_service_name", config.Otel.ServiceName),
		)
	} else {
		logAttrs = append(logAttrs,
			slog.String("webhook_url_host", config.Global.WebhookUrl.Host),
			slog.Any("webhook_mode", config.Global.WebhookMode),
		)
	}

	logger.Stdout.Info("The locomotive is ready to depart...", logAttrs...)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serviceLogTrack := make(chan []environment_logs.EnvironmentLogWithMetadata)
	httpLogTrack := make(chan []http_logs.DeploymentHttpLogWithMetadata)

	deployLogsProcessed := atomic.Int64{}
	httpLogsProcessed := atomic.Int64{}

	reportStatusAsync(&deployLogsProcessed, &httpLogsProcessed)

	handleDeployLogsAsync(ctx, &deployLogsProcessed, serviceLogTrack)
	handleHttpLogsAsync(ctx, &httpLogsProcessed, httpLogTrack)

	errGroup := errgroup.NewErrGroup()

	errGroup.Go(func() error {
		if !config.Global.EnableDeployLogs {
			logger.Stdout.Info("Deploy log transport is disabled. To enable it, set LOCOMOTIVE_ENABLE_DEPLOY_LOGS=true")
			return nil
		}

		return startStreamingDeployLogs(ctx, gqlClient, serviceLogTrack, config.Global.EnvironmentId, config.Global.ServiceIds)
	})

	errGroup.Go(func() error {
		if !config.Global.EnableHttpLogs {
			logger.Stdout.Info("HTTP log transport is disabled. To enable it, set LOCOMOTIVE_ENABLE_HTTP_LOGS=true")
			return nil
		}

		return startStreamingHttpLogs(ctx, gqlClient, httpLogTrack, config.Global.EnvironmentId, config.Global.ServiceIds)
	})

	logger.Stdout.Info("The locomotive is waiting for cargo...")

	if err := errGroup.Wait(); err != nil {
		logger.Stderr.Error("error returned from subscription(s)", logger.ErrAttr(err))
		os.Exit(1)
	}
}
