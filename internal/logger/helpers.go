package logger

import (
	"context"
	"fmt"
	"log/slog"
	"runtime"
	"strings"
)

type fromHandler struct {
	handler slog.Handler
}

func (h *fromHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.handler.Enabled(ctx, level)
}

func (h *fromHandler) Handle(ctx context.Context, r slog.Record) error {
	if r.PC != 0 {
		frame, _ := runtime.CallersFrames([]uintptr{r.PC}).Next()
		name := frame.Function
		if i := strings.LastIndex(name, "/"); i >= 0 {
			name = name[i+1:]
		}
		r.AddAttrs(slog.String("from", fmt.Sprintf("%s:%d", name, frame.Line)))
	}
	return h.handler.Handle(ctx, r)
}

func (h *fromHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &fromHandler{handler: h.handler.WithAttrs(attrs)}
}

func (h *fromHandler) WithGroup(name string) slog.Handler {
	return &fromHandler{handler: h.handler.WithGroup(name)}
}
