package main

import (
	"context"
	"log/slog"
	"os"
	"sync/atomic"

	"github.com/brody192/locomotive/internal/config"
	"github.com/brody192/locomotive/internal/errgroup"
	"github.com/brody192/locomotive/internal/logger"
	"github.com/brody192/locomotive/internal/railway"
	"github.com/brody192/locomotive/internal/railway/subscribe/environment_logs"
	"github.com/brody192/locomotive/internal/railway/subscribe/http_logs"
)

func main() {
	logger.Stdout.Info("Preparing the locomotive for departure...")

	gqlClient, err := railway.NewClient(&railway.GraphQLClient{
		AuthToken:           config.Global.RailwayApiKey,
		BaseURL:             "https://backboard.railway.app/graphql/v2",
		BaseSubscriptionURL: "wss://backboard.railway.app/graphql/internal",
	})
	if err != nil {
		logger.Stderr.Error("error creating graphql client", logger.ErrAttr(err))
		os.Exit(1)
	}

	environmentIds := config.Global.EnvironmentIds
	if len(environmentIds) == 0 {
		fetchedEnvironmentIds, err := railway.FetchAllEnvironmentIDs(context.Background(), gqlClient)
		if err != nil {
			logger.Stderr.Error("error fetching environment ids", logger.ErrAttr(err))
			os.Exit(1)
		}

		environmentIds = fetchedEnvironmentIds
	}

	logger.Stdout.Info("The locomotive is ready to depart...",
		slog.String("webhook_url_host", config.Global.WebhookUrl.Host),
		slog.Any("environment_ids", environmentIds),
		slog.Any("webhook_mode", config.Global.WebhookMode),
		slog.Bool("enable_http_logs", config.Global.EnableHttpLogs),
		slog.Bool("enable_deploy_logs", config.Global.EnableDeployLogs),
	)

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

		deployLogGroup := errgroup.NewErrGroup()

		for _, environmentId := range environmentIds {
			environmentId := environmentId

			deployLogGroup.Go(func() error {
				return startStreamingDeployLogs(ctx, gqlClient, serviceLogTrack, environmentId, nil)
			})
		}

		return deployLogGroup.Wait()
	})

	errGroup.Go(func() error {
		if !config.Global.EnableHttpLogs {
			logger.Stdout.Info("HTTP log transport is disabled. To enable it, set LOCOMOTIVE_ENABLE_HTTP_LOGS=true")
			return nil
		}

		httpLogGroup := errgroup.NewErrGroup()

		for _, environmentId := range environmentIds {
			environmentId := environmentId

			httpLogGroup.Go(func() error {
				return startStreamingHttpLogs(ctx, gqlClient, httpLogTrack, environmentId, nil)
			})
		}

		return httpLogGroup.Wait()
	})

	logger.Stdout.Info("The locomotive is waiting for cargo...")

	if err := errGroup.Wait(); err != nil {
		logger.Stderr.Error("error returned from subscription(s)", logger.ErrAttr(err))
		os.Exit(1)
	}
}
