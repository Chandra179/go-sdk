package logger

import (
	"context"
	"log/slog"
	"os"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel/log/global"
)

type AppLogger struct {
	handler slog.Logger
}

func NewLogger(env string) *AppLogger {
	otelHandler := otelslog.NewHandler("my-go-app",
		otelslog.WithLoggerProvider(global.GetLoggerProvider()),
	)
	opts := &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}
	if env == "production" {
		opts.Level = slog.LevelInfo
	}
	stdoutHandler := slog.NewJSONHandler(os.Stdout, opts)

	// This ensures every log goes to BOTH places
	multiHandler := FanOutHandler{
		handlers: []slog.Handler{stdoutHandler, otelHandler},
	}

	logger := slog.New(multiHandler)

	return &AppLogger{handler: *logger}
}

func (l *AppLogger) Info(ctx context.Context, msg string, fields ...Field) {
	l.log(ctx, slog.LevelInfo, msg, fields...)
}

func (l *AppLogger) Debug(ctx context.Context, msg string, fields ...Field) {
	l.log(ctx, slog.LevelDebug, msg, fields...)
}

func (l *AppLogger) Warn(ctx context.Context, msg string, fields ...Field) {
	l.log(ctx, slog.LevelWarn, msg, fields...)
}

func (l *AppLogger) Error(ctx context.Context, msg string, fields ...Field) {
	l.log(ctx, slog.LevelError, msg, fields...)
}

func (l *AppLogger) log(ctx context.Context, level slog.Level, msg string, fields ...Field) {
	attrs := make([]any, len(fields))
	for i, f := range fields {
		attrs[i] = slog.Any(f.Key, f.Value)
	}
	l.handler.Log(ctx, level, msg, attrs...)
}

type FanOutHandler struct {
	handlers []slog.Handler
}

func (h FanOutHandler) Enabled(ctx context.Context, l slog.Level) bool {
	for _, handler := range h.handlers {
		if handler.Enabled(ctx, l) {
			return true
		}
	}
	return false
}

func (h FanOutHandler) Handle(ctx context.Context, r slog.Record) error {
	for _, handler := range h.handlers {
		if handler.Enabled(ctx, r.Level) {
			_ = handler.Handle(ctx, r.Clone())
		}
	}
	return nil
}

func (h FanOutHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newHandlers := make([]slog.Handler, len(h.handlers))
	for i, handler := range h.handlers {
		newHandlers[i] = handler.WithAttrs(attrs)
	}
	return FanOutHandler{handlers: newHandlers}
}

func (h FanOutHandler) WithGroup(name string) slog.Handler {
	newHandlers := make([]slog.Handler, len(h.handlers))
	for i, handler := range h.handlers {
		newHandlers[i] = handler.WithGroup(name)
	}
	return FanOutHandler{handlers: newHandlers}
}
