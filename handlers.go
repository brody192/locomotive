package main

import (
	"context"
	"log/slog"
	"sync/atomic"

	"github.com/brody192/locomotive/internal/config"
	"github.com/brody192/locomotive/internal/logger"
	"github.com/brody192/locomotive/internal/otel"
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
				var err error
				var serializedLogs []byte

				if config.Otel.Enabled {
					err = otel.EmitEnvironmentLogs(ctx, logs)
				} else {
					serializedLogs, err = webhook.SendDeployLogsWebhook(logs)
				}

				if err != nil {
					attrs := []any{logger.ErrAttr(err)}

					if serializedLogs != nil {
						attrs = append(attrs, slog.String("serialized_logs", string(serializedLogs)))
					}

					logger.Stderr.Error("error sending deploy logs", attrs...)
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
				var err error
				var serializedLogs []byte

				if config.Otel.Enabled {
					err = otel.EmitHttpLogs(ctx, logs)
				} else {
					serializedLogs, err = webhook.SendHttpLogsWebhook(logs)
				}

				if err != nil {
					attrs := []any{logger.ErrAttr(err)}

					if serializedLogs != nil {
						attrs = append(attrs, slog.String("serialized_logs", string(serializedLogs)))
					}

					logger.Stderr.Error("error sending http logs", attrs...)
					continue
				}

				httpLogsProcessed.Add(int64(len(logs)))
			}
		}
	}()
}
