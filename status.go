package main

import (
	"log/slog"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/brody192/locomotive/internal/config"
	"github.com/brody192/locomotive/internal/logger"
	"github.com/brody192/locomotive/internal/util"
)

func reportStatusAsync(deployLogsProcessed *atomic.Int64, httpLogsProcessed *atomic.Int64) {
	initReport := make(chan struct{}, 1)

	var prevDeployLogs, prevHttpLogs int64

	go func() {
		t := time.NewTicker(50 * time.Millisecond)
		defer t.Stop()

		for range t.C {
			deployLogsProcessed := deployLogsProcessed.Load()
			httpLogsProcessed := httpLogsProcessed.Load()

			if deployLogsProcessed > 0 || httpLogsProcessed > 0 {
				logger.Stdout.Info("The locomotive is chugging along...",
					slog.Int64("deploy_logs_processed", deployLogsProcessed),
					slog.Int64("http_logs_processed", httpLogsProcessed),
				)

				prevDeployLogs = deployLogsProcessed
				prevHttpLogs = httpLogsProcessed

				close(initReport)
				return
			}
		}
	}()

	go func() {
		<-initReport

		t := time.NewTicker(config.Global.ReportStatusEvery)
		defer t.Stop()

		for range t.C {
			deployLogsProcessed := deployLogsProcessed.Load()
			httpLogsProcessed := httpLogsProcessed.Load()

			if deployLogsProcessed == 0 && httpLogsProcessed == 0 {
				continue
			}

			statusLog := logger.Stdout.With(
				slog.Int64("deploy_logs_processed", deployLogsProcessed),
				slog.Int64("http_logs_processed", httpLogsProcessed),
			)

			if logger.StdoutLvl.Level() == slog.LevelDebug {
				memStats := &runtime.MemStats{}
				runtime.ReadMemStats(memStats)

				statusLog = statusLog.With(
					slog.String("total_alloc", util.ByteCountIEC(memStats.TotalAlloc)),
					slog.String("heap_alloc", util.ByteCountIEC(memStats.HeapAlloc)),
					slog.String("heap_in_use", util.ByteCountIEC(memStats.HeapInuse)),
					slog.String("stack_in_use", util.ByteCountIEC(memStats.StackInuse)),
					slog.String("other_sys", util.ByteCountIEC(memStats.OtherSys)),
					slog.String("sys", util.ByteCountIEC(memStats.Sys)),
				)
			}

			if deployLogsProcessed == prevDeployLogs && httpLogsProcessed == prevHttpLogs {
				statusLog.Info("The locomotive is waiting for cargo...")
			} else {
				statusLog.Info("The locomotive is chugging along...")
			}

			prevDeployLogs = deployLogsProcessed
			prevHttpLogs = httpLogsProcessed
		}
	}()
}
