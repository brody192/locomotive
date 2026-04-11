package logger

import (
	"log/slog"
	"os"
	"strconv"
)

var (
	lowerLevel = slog.LevelVar{}

	Stdout = slog.New(&fromHandler{
		handler: slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: &lowerLevel,
		}),
	})

	Stderr = slog.New(&fromHandler{
		handler: slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
			Level: &lowerLevel,
		}),
	})
)

func init() {
	lowerLevel.Set(slog.LevelInfo)

	if b, _ := strconv.ParseBool(os.Getenv("DEBUG")); b {
		lowerLevel.Set(slog.LevelDebug)
		Stdout.Debug("debug logging enabled")
	}
}
