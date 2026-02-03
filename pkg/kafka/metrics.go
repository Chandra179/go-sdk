package kafka

import (
	"context"
	"time"

	"github.com/sony/gobreaker"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

var (
	// meter is the package-level meter for Kafka metrics
	meter = otel.Meter("gosdk/pkg/kafka")
)

// KafkaMetrics holds all Kafka-related metrics
type KafkaMetrics struct {
	// Producer metrics
	MessagesPublished metric.Int64Counter
	MessagesFailed    metric.Int64Counter
	PublishLatency    metric.Float64Histogram
	MessagesInFlight  metric.Int64UpDownCounter

	// Consumer metrics
	MessagesConsumed  metric.Int64Counter
	MessagesCommitted metric.Int64Counter
	ConsumerLag       metric.Int64Gauge
	CommitLatency     metric.Float64Histogram

	// Retry/DLQ metrics
	MessagesRetried   metric.Int64Counter
	MessagesSentToDLQ metric.Int64Counter
	RetryLatency      metric.Float64Histogram

	// Circuit breaker metrics
	CircuitBreakerState    metric.Int64Gauge
	CircuitBreakerFailures metric.Int64Counter
}

// NewKafkaMetrics creates and initializes all Kafka metrics
func NewKafkaMetrics() (*KafkaMetrics, error) {
	m := &KafkaMetrics{}
	var err error

	// Producer metrics
	m.MessagesPublished, err = meter.Int64Counter(
		"kafka.producer.messages.published",
		metric.WithDescription("Total number of messages successfully published"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, err
	}

	m.MessagesFailed, err = meter.Int64Counter(
		"kafka.producer.messages.failed",
		metric.WithDescription("Total number of messages that failed to publish"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, err
	}

	m.PublishLatency, err = meter.Float64Histogram(
		"kafka.producer.publish.latency",
		metric.WithDescription("Time taken to publish messages"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, err
	}

	m.MessagesInFlight, err = meter.Int64UpDownCounter(
		"kafka.producer.messages.inflight",
		metric.WithDescription("Number of messages currently in-flight"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, err
	}

	// Consumer metrics
	m.MessagesConsumed, err = meter.Int64Counter(
		"kafka.consumer.messages.consumed",
		metric.WithDescription("Total number of messages consumed"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, err
	}

	m.MessagesCommitted, err = meter.Int64Counter(
		"kafka.consumer.messages.committed",
		metric.WithDescription("Total number of messages successfully committed"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, err
	}

	m.ConsumerLag, err = meter.Int64Gauge(
		"kafka.consumer.lag",
		metric.WithDescription("Consumer lag per partition"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, err
	}

	m.CommitLatency, err = meter.Float64Histogram(
		"kafka.consumer.commit.latency",
		metric.WithDescription("Time taken to commit offsets"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, err
	}

	// Retry/DLQ metrics
	m.MessagesRetried, err = meter.Int64Counter(
		"kafka.retry.messages",
		metric.WithDescription("Total number of messages retried"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, err
	}

	m.MessagesSentToDLQ, err = meter.Int64Counter(
		"kafka.dlq.messages",
		metric.WithDescription("Total number of messages sent to DLQ"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, err
	}

	m.RetryLatency, err = meter.Float64Histogram(
		"kafka.retry.latency",
		metric.WithDescription("Time spent in retry mechanism"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, err
	}

	// Circuit breaker metrics
	m.CircuitBreakerState, err = meter.Int64Gauge(
		"kafka.circuitbreaker.state",
		metric.WithDescription("Current state of circuit breaker (0=closed, 1=open, 2=half-open)"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, err
	}

	m.CircuitBreakerFailures, err = meter.Int64Counter(
		"kafka.circuitbreaker.failures",
		metric.WithDescription("Total number of circuit breaker failures"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, err
	}

	return m, nil
}

// RecordPublish records producer publish metrics
func (km *KafkaMetrics) RecordPublish(ctx context.Context, topic string, duration time.Duration, err error) {
	attrs := []attribute.KeyValue{
		attribute.String("topic", topic),
	}

	if err != nil {
		km.MessagesFailed.Add(ctx, 1, metric.WithAttributes(attrs...))
	} else {
		km.MessagesPublished.Add(ctx, 1, metric.WithAttributes(attrs...))
		km.PublishLatency.Record(ctx, duration.Seconds(), metric.WithAttributes(attrs...))
	}
}

// RecordConsume records consumer message consumption
func (km *KafkaMetrics) RecordConsume(ctx context.Context, topic string, partition int, lag int64) {
	attrs := []attribute.KeyValue{
		attribute.String("topic", topic),
		attribute.Int("partition", partition),
	}

	km.MessagesConsumed.Add(ctx, 1, metric.WithAttributes(attrs...))
	km.ConsumerLag.Record(ctx, lag, metric.WithAttributes(attrs...))
}

// RecordCommit records offset commit metrics
func (km *KafkaMetrics) RecordCommit(ctx context.Context, topic string, partition int, duration time.Duration, err error) {
	attrs := []attribute.KeyValue{
		attribute.String("topic", topic),
		attribute.Int("partition", partition),
	}

	if err == nil {
		km.MessagesCommitted.Add(ctx, 1, metric.WithAttributes(attrs...))
	}
	km.CommitLatency.Record(ctx, duration.Seconds(), metric.WithAttributes(attrs...))
}

// RecordRetry records retry metrics
func (km *KafkaMetrics) RecordRetry(ctx context.Context, topic string, retryCount int) {
	attrs := []attribute.KeyValue{
		attribute.String("topic", topic),
		attribute.Int("retry_count", retryCount),
	}
	km.MessagesRetried.Add(ctx, 1, metric.WithAttributes(attrs...))
}

// RecordDLQ records DLQ routing metrics
func (km *KafkaMetrics) RecordDLQ(ctx context.Context, originalTopic string, dlqTopic string) {
	attrs := []attribute.KeyValue{
		attribute.String("original_topic", originalTopic),
		attribute.String("dlq_topic", dlqTopic),
	}
	km.MessagesSentToDLQ.Add(ctx, 1, metric.WithAttributes(attrs...))
}

// RecordCircuitBreakerState records circuit breaker state changes
func (km *KafkaMetrics) RecordCircuitBreakerState(ctx context.Context, name string, state gobreaker.State) {
	attrs := []attribute.KeyValue{
		attribute.String("name", name),
	}
	km.CircuitBreakerState.Record(ctx, int64(state), metric.WithAttributes(attrs...))
}

// RecordCircuitBreakerFailure records circuit breaker failures
func (km *KafkaMetrics) RecordCircuitBreakerFailure(ctx context.Context, name string) {
	attrs := []attribute.KeyValue{
		attribute.String("name", name),
	}
	km.CircuitBreakerFailures.Add(ctx, 1, metric.WithAttributes(attrs...))
}

// MetricsProducer wraps a producer with metrics collection
type MetricsProducer struct {
	producer Producer
	metrics  *KafkaMetrics
}

// NewMetricsProducer creates a new metrics-wrapped producer
func NewMetricsProducer(producer Producer, metrics *KafkaMetrics) *MetricsProducer {
	return &MetricsProducer{
		producer: producer,
		metrics:  metrics,
	}
}

// Publish implements the Producer interface with metrics
func (m *MetricsProducer) Publish(ctx context.Context, msg Message) error {
	start := time.Now()

	m.metrics.MessagesInFlight.Add(ctx, 1, metric.WithAttributes(
		attribute.String("topic", msg.Topic),
	))
	defer m.metrics.MessagesInFlight.Add(ctx, -1, metric.WithAttributes(
		attribute.String("topic", msg.Topic),
	))

	err := m.producer.Publish(ctx, msg)

	m.metrics.RecordPublish(ctx, msg.Topic, time.Since(start), err)

	return err
}

// Close implements the Producer interface
func (m *MetricsProducer) Close() error {
	return m.producer.Close()
}

// MetricsConsumer wraps a consumer with metrics collection
type MetricsConsumer struct {
	consumer Consumer
	metrics  *KafkaMetrics
}

// NewMetricsConsumer creates a new metrics-wrapped consumer
func NewMetricsConsumer(consumer Consumer, metrics *KafkaMetrics) *MetricsConsumer {
	return &MetricsConsumer{
		consumer: consumer,
		metrics:  metrics,
	}
}

// Start implements the Consumer interface with metrics
func (m *MetricsConsumer) Start(ctx context.Context, handler ConsumerHandler) error {
	// Wrap the handler to collect metrics
	metricsHandler := func(msg Message) error {
		err := handler(msg)

		// Record consumption metrics
		// Note: Lag is not available here, would need to be passed or fetched separately
		m.metrics.RecordConsume(ctx, msg.Topic, 0, 0)

		return err
	}

	return m.consumer.Start(ctx, metricsHandler)
}

// Close implements the Consumer interface
func (m *MetricsConsumer) Close() error {
	return m.consumer.Close()
}
