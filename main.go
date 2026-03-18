package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"

	"github.com/brody192/locomotive/internal/config"
	"github.com/brody192/locomotive/internal/errgroup"
	"github.com/brody192/locomotive/internal/logger"
	"github.com/brody192/locomotive/internal/logline/serializer"
	"github.com/brody192/locomotive/internal/railway"
	"github.com/brody192/locomotive/internal/railway/subscribe/environment_logs"
	"github.com/brody192/locomotive/internal/railway/subscribe/http_logs"
)

func main() {
	os.Exit(run())
}

func run() int {
	logger.Stdout.Info("Preparing the locomotive for departure...")

	gqlClient, err := railway.NewClient(&railway.GraphQLClient{
		AuthToken:           config.Global.RailwayApiKey,
		BaseURL:             "https://backboard.railway.app/graphql/v2",
		BaseSubscriptionURL: "wss://backboard.railway.app/graphql/internal",
	})
	if err != nil {
		logger.Stderr.Error("error creating graphql client", logger.ErrAttr(err))
		return 1
	}

	allServicesExist, foundServices, missingServices, err := railway.VerifyAllServicesExistWithinEnvironment(gqlClient, config.Global.ServiceIds, config.Global.EnvironmentId)
	if err != nil {
		logger.Stderr.Error("error verifying if services exist within the environment", logger.ErrAttr(err))
		return 1
	}

	if !allServicesExist {
		logger.Stderr.Error("all services must exist within the environment set by the LOCOMOTIVE_ENVIRONMENT_ID variable",
			slog.Any("missing_service_ids", missingServices),
			slog.Any("configured_service_ids", config.Global.ServiceIds),
			slog.Any("found_service_ids", foundServices),
			slog.Any("environment_id", config.Global.EnvironmentId),
		)

		return 1
	}

	logger.Stdout.Info("The locomotive is ready to depart...",
		slog.String("webhook_url_host", config.Global.WebhookUrl.Host),
		slog.Any("service_ids", config.Global.ServiceIds),
		slog.Any("environment_id", config.Global.EnvironmentId),
		slog.Any("webhook_mode", config.Global.WebhookMode),
		slog.Bool("enable_http_logs", config.Global.EnableHttpLogs),
		slog.Bool("enable_deploy_logs", config.Global.EnableDeployLogs),
	)

	deployLogsProcessed := atomic.Int64{}
	httpLogsProcessed := atomic.Int64{}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	reportStatusAsync(ctx, &deployLogsProcessed, &httpLogsProcessed)

	errGroup, _ := errgroup.NewErrGroup(ctx)
	defer errGroup.Cancel()

	errGroup.Go(func(ctx context.Context) error {
		if !config.Global.EnableDeployLogs {
			logger.Stdout.Info("Deploy log transport is disabled. To enable it, set LOCOMOTIVE_ENABLE_DEPLOY_LOGS=true")
			return nil
		}

		return runLogPipeline(ctx, "deploy-logs", serializer.DeployLogs,
			func(ctx context.Context, track chan []environment_logs.EnvironmentLogWithMetadata) error {
				return environment_logs.SubscribeToServiceLogs(ctx, gqlClient, track, config.Global.EnvironmentId, config.Global.ServiceIds)
			}, &deployLogsProcessed)
	})

	errGroup.Go(func(ctx context.Context) error {
		if !config.Global.EnableHttpLogs {
			logger.Stdout.Info("HTTP log transport is disabled. To enable it, set LOCOMOTIVE_ENABLE_HTTP_LOGS=true")
			return nil
		}

		return runLogPipeline(ctx, "http-logs", serializer.HttpLogs,
			func(ctx context.Context, track chan []http_logs.DeploymentHttpLogWithMetadata) error {
				return http_logs.SubscribeToHttpLogs(ctx, gqlClient, track, config.Global.EnvironmentId, config.Global.ServiceIds)
			}, &httpLogsProcessed)
	})

	logger.Stdout.Info("The locomotive is waiting for cargo...")

	if err := errGroup.Wait(); err != nil {
		logger.Stderr.Error("error returned from subscription(s)", logger.ErrAttr(err))
		return 1
	}

	return 0
}
