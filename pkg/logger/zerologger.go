package logger

import (
	"io"
	"os"

	"github.com/rs/zerolog"
)

type ZeroLogger struct {
	logger zerolog.Logger
}

func NewZeroLog(env string) *ZeroLogger {
	return NewWithWriter(env, os.Stdout)
}

func NewWithWriter(env string, w io.Writer) *ZeroLogger {
	logger := zerolog.New(w).With().Timestamp().Logger()

	switch env {
	case "production":
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	default:
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}

	return &ZeroLogger{logger: logger}
}

// helper to convert our abstraction []Field -> zerolog fields
func convert(fields []Field) []any {
	items := make([]any, 0, len(fields)*2)
	for _, f := range fields {
		items = append(items, f.Key, f.Value)
	}
	return items
}

func (l *ZeroLogger) Debug(msg string, fields ...Field) {
	l.logger.Debug().Fields(convert(fields)).Msg(msg)
}

func (l *ZeroLogger) Info(msg string, fields ...Field) {
	l.logger.Info().Fields(convert(fields)).Msg(msg)
}

func (l *ZeroLogger) Warn(msg string, fields ...Field) {
	l.logger.Warn().Fields(convert(fields)).Msg(msg)
}

func (l *ZeroLogger) Error(msg string, fields ...Field) {
	l.logger.Error().Fields(convert(fields)).Msg(msg)
}
