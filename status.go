package main

import (
	"context"
	"log/slog"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/brody192/locomotive/internal/config"
	"github.com/brody192/locomotive/internal/logger"
	"github.com/brody192/locomotive/internal/util"
)

// logCounts tracks a pipeline's outcomes: entries successfully shipped, and entries
// received from Railway but never shipped (serialize failures or dispatcher drops).
// Always pass by pointer — atomic.Int64 must not be copied.
type logCounts struct {
	processed atomic.Int64
	failed    atomic.Int64
}

func reportStatusAsync(ctx context.Context, deployLogs *logCounts, httpLogs *logCounts) {
	var prevDeployLogs, prevHttpLogs atomic.Int64

	go func() {
		// Phase 1: poll at high frequency until the first logs arrive
		t := time.NewTicker(500 * time.Millisecond)

		for {
			select {
			case <-ctx.Done():
				t.Stop()
				return
			case <-t.C:
			}

			dl := deployLogs.processed.Load()
			hl := httpLogs.processed.Load()
			df := deployLogs.failed.Load()
			hf := httpLogs.failed.Load()

			if dl > 0 || hl > 0 || df > 0 || hf > 0 {
				logger.Stdout.Info("The locomotive is chugging along...",
					slog.Int64("deploy_logs_processed", dl),
					slog.Int64("deploy_logs_failed", df),
					slog.Int64("http_logs_processed", hl),
					slog.Int64("http_logs_failed", hf),
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

		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}

			dl := deployLogs.processed.Load()
			hl := httpLogs.processed.Load()
			df := deployLogs.failed.Load()
			hf := httpLogs.failed.Load()

			if dl == 0 && hl == 0 && df == 0 && hf == 0 {
				continue
			}

			statusLog := logger.Stdout.With(
				slog.Int64("deploy_logs_processed", dl),
				slog.Int64("deploy_logs_failed", df),
				slog.Int64("http_logs_processed", hl),
				slog.Int64("http_logs_failed", hf),
			)

			if logger.Stdout.Enabled(context.Background(), slog.LevelDebug) {
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

			// "chugging" vs "waiting" is keyed on shipped logs; failures are still
			// reported above but don't count as forward progress.
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
