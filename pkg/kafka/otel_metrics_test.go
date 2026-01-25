package kafka

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric/noop"
)

func TestOtelMetricsHelperFunctions(t *testing.T) {
	t.Run("InitOtelMetrics succeeds", func(t *testing.T) {
		err := InitOtelMetrics()
		require.NoError(t, err)
	})
	t.Run("RecordProducerMessageSent", func(t *testing.T) {
		ctx := context.Background()
		RecordProducerMessageSent(ctx, "test-topic", "snappy")
	})
	t.Run("RecordProducerSendError", func(t *testing.T) {
		ctx := context.Background()
		RecordProducerSendError(ctx, "test-topic", "timeout")
	})
	t.Run("RecordProducerSendLatency", func(t *testing.T) {
		ctx := context.Background()
		RecordProducerSendLatency(ctx, "test-topic", 0.123)
	})
}

func TestRecordProducerMessageSent(t *testing.T) {
	t.Run("records producer message sent metric", func(t *testing.T) {
		otel.SetMeterProvider(noop.NewMeterProvider())
		InitOtelMetrics()

		ctx := context.Background()
		topic := "test-topic"
		compression := "gzip"

		// Should not panic
		RecordProducerMessageSent(ctx, topic, compression)
	})
}

func TestRecordProducerSendError(t *testing.T) {
	t.Run("records producer send error metric", func(t *testing.T) {
		otel.SetMeterProvider(noop.NewMeterProvider())
		InitOtelMetrics()

		ctx := context.Background()
		topic := "test-topic"
		errorType := "connection_error"

		// Should not panic
		RecordProducerSendError(ctx, topic, errorType)
	})
}

func TestRecordProducerSendLatency(t *testing.T) {
	t.Run("records producer send latency metric", func(t *testing.T) {
		otel.SetMeterProvider(noop.NewMeterProvider())
		InitOtelMetrics()

		ctx := context.Background()
		topic := "test-topic"
		durationSeconds := 0.1

		// Should not panic
		RecordProducerSendLatency(ctx, topic, durationSeconds)
	})
}
