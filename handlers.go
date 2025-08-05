package main

import (
	"context"
	"sync/atomic"

	"github.com/brody192/locomotive/internal/logger"
	"github.com/brody192/locomotive/internal/railway/subscribe/environment_logs"
	"github.com/brody192/locomotive/internal/railway/subscribe/http_logs"
	"github.com/brody192/locomotive/internal/webhook"
)

func handleDeployLogsAsync(ctx context.Context, deployLogsProcessed *atomic.Int64, serviceLogTrack chan []environment_logs.EnvironmentLogWithMetadata) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case logs := <-serviceLogTrack:
				if err := webhook.SendDeployLogsWebhook(logs); err != nil {
					logger.Stderr.Error("error sending deploy logs webhook(s)", logger.ErrAttr(err))

					continue
				}

				deployLogsProcessed.Add(int64(len(logs)))
			}
		}
	}()
}

func handleHttpLogsAsync(ctx context.Context, httpLogsProcessed *atomic.Int64, httpLogTrack chan []http_logs.DeploymentHttpLogWithMetadata) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case logs := <-httpLogTrack:
				if err := webhook.SendHttpLogsWebhook(logs); err != nil {
					logger.Stderr.Error("error sending http logs webhook(s)", logger.ErrAttr(err))

					continue
				}

				httpLogsProcessed.Add(int64(len(logs)))
			}
		}
	}()
}
