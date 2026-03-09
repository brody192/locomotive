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
	var prevDeployLogs, prevHttpLogs atomic.Int64

	go func() {
		// Phase 1: poll at high frequency until the first logs arrive
		t := time.NewTicker(500 * time.Millisecond)

		for range t.C {
			dl := deployLogsProcessed.Load()
			hl := httpLogsProcessed.Load()

			if dl > 0 || hl > 0 {
				logger.Stdout.Info("The locomotive is chugging along...",
					slog.Int64("deploy_logs_processed", dl),
					slog.Int64("http_logs_processed", hl),
				)

				prevDeployLogs.Store(dl)
				prevHttpLogs.Store(hl)

				break
			}
		}

		t.Stop()

		// Phase 2: periodic status reporting
		t = time.NewTicker(config.Global.ReportStatusEvery)
		defer t.Stop()

		for range t.C {
			dl := deployLogsProcessed.Load()
			hl := httpLogsProcessed.Load()

			if dl == 0 && hl == 0 {
				continue
			}

			statusLog := logger.Stdout.With(
				slog.Int64("deploy_logs_processed", dl),
				slog.Int64("http_logs_processed", hl),
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

			if dl == prevDeployLogs.Load() && hl == prevHttpLogs.Load() {
				statusLog.Info("The locomotive is waiting for cargo...")
			} else {
				statusLog.Info("The locomotive is chugging along...")
			}

			prevDeployLogs.Store(dl)
			prevHttpLogs.Store(hl)
		}
	}()
}
