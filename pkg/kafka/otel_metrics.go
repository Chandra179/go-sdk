package kafka

import (
	"context"
	"errors"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// Package variables for all metric instruments
var (
	OtelProducerMessagesSent      metric.Int64Counter
	OtelProducerSendErrors        metric.Int64Counter
	OtelProducerSendLatency       metric.Float64Histogram
	OtelConsumerMessagesProcessed metric.Int64Counter
	OtelConsumerProcessingErrors  metric.Int64Counter
	OtelConsumerLag               metric.Int64Gauge
	OtelConsumerRebalanceEvents   metric.Int64Counter
	OtelDLQMessagesSent           metric.Int64Counter
	OtelRetryMessagesSent         metric.Int64Counter
)

// InitOtelMetrics initializes all OpenTelemetry metric instruments
func InitOtelMetrics() error {
	meter := otel.Meter("gosdk/kafka")
	var errs []error

	// Producer metrics
	var err error
	OtelProducerMessagesSent, err = meter.Int64Counter(
		"kafka.producer.messages_sent",
		metric.WithDescription("Total number of messages sent to Kafka"),
		metric.WithUnit("1"),
	)
	if err != nil {
		errs = append(errs, err)
	}

	OtelProducerSendErrors, err = meter.Int64Counter(
		"kafka.producer.send_errors",
		metric.WithDescription("Total number of send errors"),
		metric.WithUnit("1"),
	)
	if err != nil {
		errs = append(errs, err)
	}

	OtelProducerSendLatency, err = meter.Float64Histogram(
		"kafka.producer.send_latency",
		metric.WithDescription("Time to send messages to Kafka"),
		metric.WithUnit("s"),
	)
	if err != nil {
		errs = append(errs, err)
	}

	// Consumer metrics
	OtelConsumerMessagesProcessed, err = meter.Int64Counter(
		"kafka.consumer.messages_processed",
		metric.WithDescription("Total number of messages processed"),
		metric.WithUnit("1"),
	)
	if err != nil {
		errs = append(errs, err)
	}

	OtelConsumerProcessingErrors, err = meter.Int64Counter(
		"kafka.consumer.processing_errors",
		metric.WithDescription("Total number of processing errors"),
		metric.WithUnit("1"),
	)
	if err != nil {
		errs = append(errs, err)
	}

	OtelConsumerLag, err = meter.Int64Gauge(
		"kafka.consumer.lag",
		metric.WithDescription("Consumer lag by topic and partition"),
		metric.WithUnit("1"),
	)
	if err != nil {
		errs = append(errs, err)
	}

	OtelConsumerRebalanceEvents, err = meter.Int64Counter(
		"kafka.consumer.rebalance_events",
		metric.WithDescription("Total number of consumer group rebalance events"),
		metric.WithUnit("1"),
	)
	if err != nil {
		errs = append(errs, err)
	}

	// DLQ/Retry metrics
	OtelDLQMessagesSent, err = meter.Int64Counter(
		"kafka.dlq.messages_sent",
		metric.WithDescription("Total number of messages sent to DLQ"),
		metric.WithUnit("1"),
	)
	if err != nil {
		errs = append(errs, err)
	}

	OtelRetryMessagesSent, err = meter.Int64Counter(
		"kafka.retry.messages_sent",
		metric.WithDescription("Total number of messages sent to retry topic"),
		metric.WithUnit("1"),
	)
	if err != nil {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// Helper functions for recording metrics with context and attributes

// RecordProducerMessageSent records a producer message sent metric
func RecordProducerMessageSent(ctx context.Context, topic string, compression string) {
	if OtelProducerMessagesSent != nil {
		OtelProducerMessagesSent.Add(ctx, 1, metric.WithAttributes(
			attribute.String("topic", topic),
			attribute.String("compression", compression),
		))
	}
}

// RecordProducerSendError records a producer send error metric
func RecordProducerSendError(ctx context.Context, topic string, errorType string) {
	if OtelProducerSendErrors != nil {
		OtelProducerSendErrors.Add(ctx, 1, metric.WithAttributes(
			attribute.String("topic", topic),
			attribute.String("error_type", errorType),
		))
	}
}

// RecordProducerSendLatency records a producer send latency metric
func RecordProducerSendLatency(ctx context.Context, topic string, durationSeconds float64) {
	if OtelProducerSendLatency != nil {
		OtelProducerSendLatency.Record(ctx, durationSeconds, metric.WithAttributes(
			attribute.String("topic", topic),
		))
	}
}

// RecordConsumerMessageProcessed records a consumer message processed metric
func RecordConsumerMessageProcessed(ctx context.Context, topic string, groupID string) {
	if OtelConsumerMessagesProcessed != nil {
		OtelConsumerMessagesProcessed.Add(ctx, 1, metric.WithAttributes(
			attribute.String("topic", topic),
			attribute.String("group_id", groupID),
		))
	}
}

// RecordConsumerProcessingError records a consumer processing error metric
func RecordConsumerProcessingError(ctx context.Context, topic string, errorType string) {
	if OtelConsumerProcessingErrors != nil {
		OtelConsumerProcessingErrors.Add(ctx, 1, metric.WithAttributes(
			attribute.String("topic", topic),
			attribute.String("error_type", errorType),
		))
	}
}

// RecordConsumerLag records a consumer lag metric
func RecordConsumerLag(ctx context.Context, topic string, partition int32, groupID string, lag int64) {
	if OtelConsumerLag != nil {
		OtelConsumerLag.Record(ctx, lag, metric.WithAttributes(
			attribute.String("topic", topic),
			attribute.Int("partition", int(partition)),
			attribute.String("group_id", groupID),
		))
	}
}

// RecordConsumerRebalanceEvent records a consumer rebalance event metric
func RecordConsumerRebalanceEvent(ctx context.Context, groupID string, eventType string) {
	if OtelConsumerRebalanceEvents != nil {
		OtelConsumerRebalanceEvents.Add(ctx, 1, metric.WithAttributes(
			attribute.String("group_id", groupID),
			attribute.String("event_type", eventType),
		))
	}
}

// RecordDLQMessageSent records a DLQ message sent metric
func RecordDLQMessageSent(ctx context.Context, originalTopic string, dlqTopic string) {
	if OtelDLQMessagesSent != nil {
		OtelDLQMessagesSent.Add(ctx, 1, metric.WithAttributes(
			attribute.String("original_topic", originalTopic),
			attribute.String("dlq_topic", dlqTopic),
		))
	}
}

// RecordRetryMessageSent records a retry message sent metric
func RecordRetryMessageSent(ctx context.Context, originalTopic string, retryTopic string) {
	if OtelRetryMessagesSent != nil {
		OtelRetryMessagesSent.Add(ctx, 1, metric.WithAttributes(
			attribute.String("original_topic", originalTopic),
			attribute.String("retry_topic", retryTopic),
		))
	}
}
