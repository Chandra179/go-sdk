# Kafka Package

A Kafka client library for Go providing producer, consumer, and DLQ/retry support with TLS encryption and Prometheus metrics.

## Configuration

The Kafka client is configured via environment variables. Required variables are marked below.

### Required Variables
- `KAFKA_BROKERS` - Comma-separated list of Kafka broker addresses

### Producer Configuration
- `KAFKA_PRODUCER_ACKS` - Acknowledgment level: "all" (high durability), "none", "leader"
- `KAFKA_PRODUCER_BATCH_SIZE` - Batch size in bytes (default: 1048576)
- `KAFKA_PRODUCER_LINGER_MS` - Time to wait before sending batch in milliseconds (default: 10)
- `KAFKA_PRODUCER_COMPRESSION` - Compression type: "none", "gzip", "snappy", "lz4", "zstd" (Required)
- `KAFKA_PRODUCER_MAX_ATTEMPTS` - Number of retry attempts (default: 10)
- `KAFKA_PRODUCER_ASYNC` - Async writes (default: false)
- `KAFKA_PRODUCER_COMMIT_INTERVAL_MS` - Commit interval in milliseconds (default: 1000)

### Consumer Configuration
- `KAFKA_CONSUMER_MIN_BYTES` - Minimum fetch bytes (default: 1024)
- `KAFKA_CONSUMER_MAX_BYTES` - Maximum fetch bytes (default: 10485760)
- `KAFKA_CONSUMER_COMMIT_INTERVAL_MS` - Offset commit interval in milliseconds (default: 1000)
- `KAFKA_CONSUMER_MAX_POLL_RECORDS` - Max records per poll (default: 500)
- `KAFKA_CONSUMER_HEARTBEAT_INTERVAL_MS` - Heartbeat interval in milliseconds (default: 3000)
- `KAFKA_CONSUMER_SESSION_TIMEOUT_MS` - Session timeout in milliseconds (default: 10000)
- `KAFKA_CONSUMER_WATCH_PARTITION_CHANGES` - Watch for partition changes (default: true)

### Retry/DLQ Configuration
- `KAFKA_RETRY_SHORT_ATTEMPTS` - Immediate retry attempts (default: 3)
- `KAFKA_RETRY_INITIAL_BACKOFF_MS` - Initial backoff in milliseconds (default: 100)
- `KAFKA_RETRY_MAX_BACKOFF_MS` - Maximum backoff in milliseconds (default: 1000)
- `KAFKA_RETRY_MAX_LONG_ATTEMPTS` - Maximum total retry attempts (default: 3)
- `KAFKA_RETRY_DLQ_ENABLED` - Enable dead letter queue (default: true)
- `KAFKA_RETRY_DLQ_TOPIC_PREFIX` - DLQ topic suffix (default: ".dlq")
- `KAFKA_RETRY_TOPIC_SUFFIX` - Retry topic suffix (default: ".retry")

### Security (TLS) Configuration
- `KAFKA_SECURITY_ENABLED` - Enable TLS (default: false)
- `KAFKA_TLS_CERT_FILE` - Path to TLS certificate file
- `KAFKA_TLS_KEY_FILE` - Path to TLS key file
- `KAFKA_TLS_CA_FILE` - Path to TLS CA certificate file

## Usage

### Basic Producer with TLS

```go
package main

import (
	"context"
	"log"

	"gosdk/pkg/kafka"
)

func main() {
	cfg, err := kafka.Load()
	if err != nil {
		log.Fatal(err)
	}

	kafka.RegisterMetrics()

	client, err := kafka.NewClient(cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	producer, err := client.Producer()
	if err != nil {
		log.Fatal(err)
	}

	err = producer.Publish(context.Background(), kafka.Message{
		Topic: "orders",
		Value: []byte(`{"id": "123"}`),
		Key:   []byte("order-123"),
	})
	if err != nil {
		log.Fatal(err)
	}
}
```

### Consumer with DLQ and Retry

```go
package main

import (
	"context"
	"log"

	"gosdk/pkg/kafka"
)

func main() {
	cfg, err := kafka.Load()
	if err != nil {
		log.Fatal(err)
	}

	kafka.RegisterMetrics()

	client, err := kafka.NewClient(cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	consumer, err := client.Consumer("order-processor")
	if err != nil {
		log.Fatal(err)
	}

	handler := func(msg kafka.Message) error {
		log.Printf("Received message: %s", string(msg.Value))
		return processOrder(msg.Value)
	}

	err = consumer.Subscribe(context.Background(), []string{"orders"}, handler)
	if err != nil {
		log.Fatal(err)
	}

	select {}
}

func processOrder(data []byte) error {
	return nil
}
```

### Using StartConsumer with Retry Configuration

```go
package main

import (
	"context"
	"log"
	"time"

	"gosdk/pkg/kafka"
)

func main() {
	cfg, err := kafka.Load()
	if err != nil {
		log.Fatal(err)
	}

	kafka.RegisterMetrics()

	client, err := kafka.NewClient(cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	retryConfig := kafka.RetryConfig{
		ShortRetryAttempts:   3,
		InitialBackoff:     100,
		MaxBackoff:         1000,
		MaxLongRetryAttempts: 5,
		DLQEnabled:         true,
	}

	handler := func(msg kafka.Message) error {
		log.Printf("Processing message: %s", string(msg.Value))
		if shouldFail(msg) {
			return fmt.Errorf("simulated failure")
		}
		return nil
	}

	err := kafka.StartConsumer(
		context.Background(),
		client,
		"order-processor",
		[]string{"orders"},
		handler,
		retryConfig,
	)
	if err != nil {
		log.Fatal(err)
	}

	select {}
}

func shouldFail(msg kafka.Message) bool {
	return false
}
```

## Prometheus Metrics

After registering metrics with `kafka.RegisterMetrics()`, the following metrics are available on `/metrics` endpoint:

### Producer Metrics
- `kafka_producer_messages_sent_total` - Total messages sent, labels: topic, compression
- `kafka_producer_send_errors_total` - Total send errors, labels: topic, error_type
- `kafka_producer_send_latency_seconds` - Send latency histogram, labels: topic

### Consumer Metrics
- `kafka_consumer_messages_processed_total` - Total messages processed, labels: topic, group_id
- `kafka_consumer_processing_errors_total` - Total processing errors, labels: topic, error_type
- `kafka_consumer_lag` - Consumer lag gauge, labels: topic, partition, group_id
- `kafka_consumer_rebalance_events_total` - Rebalance events, labels: group_id, event_type

### DLQ/Retry Metrics
- `kafka_dlq_messages_sent_total` - Messages sent to DLQ, labels: original_topic, dlq_topic
- `kafka_retry_messages_sent_total` - Messages sent to retry topic, labels: original_topic, retry_topic

## Best Practices

1. **Always call `kafka.RegisterMetrics()` explicitly** before using the client to enable Prometheus metrics
2. **Use context.Context** for all operations to support cancellation
3. **Close clients properly** using `defer client.Close()` to ensure graceful shutdown
4. **Handle errors appropriately** - The library provides structured logging for troubleshooting
5. **Configure TLS for production** - Enable `KAFKA_SECURITY_ENABLED` and provide cert files
6. **Choose appropriate compression** - Use "snappy" for balance of CPU and compression ratio
7. **Monitor consumer lag** - Use Prometheus metrics to track `kafka_consumer_lag`

## TLS Setup

To use TLS with Kafka:

1. Generate or obtain certificates for your Kafka brokers
2. Set `KAFKA_SECURITY_ENABLED=true`
3. Set paths to cert files:
   - `KAFKA_TLS_CERT_FILE=/path/to/client-cert.pem`
   - `KAFKA_TLS_KEY_FILE=/path/to/client-key.pem`
   - `KAFKA_TLS_CA_FILE=/path/to/ca-cert.pem`

## Breaking Changes from Previous Version

1. **Constructor signatures changed**:
   - `NewClient()` now takes `*Config` instead of `[]string`
   - `NewKafkaProducer()` now takes `*ProducerConfig, []string, *Dialer`
   - `NewKafkaConsumer()` now takes `*ConsumerConfig, []string, string, *Dialer`

2. **Required environment variables**:
   - `KAFKA_BROKERS` is now required
   - `KAFKA_PRODUCER_COMPRESSION` is now required

3. **Explicit metrics registration**:
   - `kafka.RegisterMetrics()` must be called explicitly

4. **Consumer configuration**:
   - Consumer now uses config-based initialization
   - Structured logging is built-in
