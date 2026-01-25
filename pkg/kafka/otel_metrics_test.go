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
		err := InitOtelMetrics()
		if err != nil {
			t.Errorf("InitOtelMetrics() returned error: %v", err)
		}

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
		err := InitOtelMetrics()
		if err != nil {
			t.Errorf("InitOtelMetrics() returned error: %v", err)
		}

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
		err := InitOtelMetrics()
		if err != nil {
			t.Errorf("InitOtelMetrics() returned error: %v", err)
		}

		ctx := context.Background()
		topic := "test-topic"
		durationSeconds := 0.1

		// Should not panic
		RecordProducerSendLatency(ctx, topic, durationSeconds)
	})
}

func TestRecordConsumerMessageProcessed(t *testing.T) {
	t.Run("records consumer message processed metric", func(t *testing.T) {
		otel.SetMeterProvider(noop.NewMeterProvider())
		err := InitOtelMetrics()
		if err != nil {
			t.Errorf("InitOtelMetrics() returned error: %v", err)
		}

		ctx := context.Background()
		topic := "test-topic"
		groupID := "test-group"

		// Should not panic
		RecordConsumerMessageProcessed(ctx, topic, groupID)
	})
}

func TestRecordConsumerProcessingError(t *testing.T) {
	t.Run("records consumer processing error metric", func(t *testing.T) {
		otel.SetMeterProvider(noop.NewMeterProvider())
		err := InitOtelMetrics()
		if err != nil {
			t.Errorf("InitOtelMetrics() returned error: %v", err)
		}

		ctx := context.Background()
		topic := "test-topic"
		errorType := "deserialization_error"

		// Should not panic
		RecordConsumerProcessingError(ctx, topic, errorType)
	})
}

func TestRecordConsumerLag(t *testing.T) {
	t.Run("records consumer lag metric", func(t *testing.T) {
		otel.SetMeterProvider(noop.NewMeterProvider())
		err := InitOtelMetrics()
		if err != nil {
			t.Errorf("InitOtelMetrics() returned error: %v", err)
		}

		ctx := context.Background()
		topic := "test-topic"
		partition := int32(0)
		groupID := "test-group"
		lag := int64(100)

		// Should not panic
		RecordConsumerLag(ctx, topic, partition, groupID, lag)
	})
}

func TestRecordConsumerRebalanceEvent(t *testing.T) {
	t.Run("records consumer rebalance event metric", func(t *testing.T) {
		otel.SetMeterProvider(noop.NewMeterProvider())
		err := InitOtelMetrics()
		if err != nil {
			t.Errorf("InitOtelMetrics() returned error: %v", err)
		}

		ctx := context.Background()
		groupID := "test-group"
		eventType := "assign"

		// Should not panic
		RecordConsumerRebalanceEvent(ctx, groupID, eventType)
	})
}

func TestRecordDLQMessageSent(t *testing.T) {
	t.Run("records DLQ message sent metric", func(t *testing.T) {
		otel.SetMeterProvider(noop.NewMeterProvider())
		err := InitOtelMetrics()
		if err != nil {
			t.Errorf("InitOtelMetrics() returned error: %v", err)
		}

		ctx := context.Background()
		originalTopic := "test-topic"
		dlqTopic := "test-topic-dlq"

		// Should not panic
		RecordDLQMessageSent(ctx, originalTopic, dlqTopic)
	})
}

func TestRecordRetryMessageSent(t *testing.T) {
	t.Run("records retry message sent metric", func(t *testing.T) {
		otel.SetMeterProvider(noop.NewMeterProvider())
		err := InitOtelMetrics()
		if err != nil {
			t.Errorf("InitOtelMetrics() returned error: %v", err)
		}

		ctx := context.Background()
		originalTopic := "test-topic"
		retryTopic := "test-topic-retry"

		// Should not panic
		RecordRetryMessageSent(ctx, originalTopic, retryTopic)
	})
}
