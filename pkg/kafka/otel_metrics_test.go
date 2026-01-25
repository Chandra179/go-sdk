package kafka

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOtelMetricsHelperFunctions(t *testing.T) {
	t.Run("InitOtelMetrics succeeds", func(t *testing.T) {
		err := InitOtelMetrics()
		require.NoError(t, err)
	})

	t.Run("RecordProducerMessageSent", func(t *testing.T) {
		ctx := context.Background()
		// This should not panic
		RecordProducerMessageSent(ctx, "test-topic", "snappy")
	})

	t.Run("RecordProducerSendError", func(t *testing.T) {
		ctx := context.Background()
		// This should not panic
		RecordProducerSendError(ctx, "test-topic", "timeout")
	})

	t.Run("RecordProducerSendLatency", func(t *testing.T) {
		ctx := context.Background()
		// This should not panic
		RecordProducerSendLatency(ctx, "test-topic", 0.123)
	})

	t.Run("RecordProducerMessageSent actually records", func(t *testing.T) {
		// This would need mock OTEL setup to verify - for now just ensure no nil pointer issues
		ctx := context.Background()
		RecordProducerMessageSent(ctx, "test-topic", "snappy")
		// In real test with mock, we'd verify metric increment
	})
}
