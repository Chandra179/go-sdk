package kafka

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric/noop"
)

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
