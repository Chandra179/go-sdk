# Kafka Package

A comprehensive Kafka client library for Go providing producers, consumers, and administrative operations with built-in resilience patterns, observability, and schema registry integration.

## Features

- **Producer**: High-performance message publishing with idempotency, compression, and partition strategies
- **Consumer**: Group-based consumption with automatic retry, DLQ routing, and schema decoding
- **Admin**: Topic management, auto-creation of DLQ/retry topics
- **Resilience**: Circuit breaker, connection pooling, retry with exponential backoff
- **Transactions**: Exactly-once semantics (EOS) for atomic produce/consume cycles
- **Schema Registry**: Avro, JSON Schema, and Protobuf support via Confluent Schema Registry
- **Observability**: OpenTelemetry metrics for production monitoring
- **Security**: TLS/mTLS authentication support
- **Health Checks**: Liveness, readiness, and connectivity probes

## Architecture

```mermaid
flowchart TB
    subgraph Client["KafkaClient - Entry Point"]
        direction TB
        C["KafkaClient"]
        C -->|"Producer()"| P["KafkaProducer\n(singleton, lazy)"]
        C -->|"Consumer()"| Con["KafkaConsumer\n(per group+topics)"]
        C -->|"Admin"| A["KafkaAdmin"]
        C -->|"ConnectionPool"| CP["ConnectionPool"]
        C -->|"SchemaRegistry"| SR["SchemaRegistry"]
    end

    subgraph Producer["KafkaProducer"]
        direction TB
        P1["Idempotency\n(Deduplication)"]
        P2["Compression\n(gzip/snappy/lz4/zstd)"]
        P3["Partitioning\n(hash/roundrobin/leastbytes)"]
        P4["Batching"]
        P5["Metrics\n(OTEL)"]
    end

    subgraph Consumer["KafkaConsumer"]
        direction TB
        Con1["Auto-retry\n(exp backoff)"]
        Con2["DLQ Routing\n(Dead Letter Queue)"]
        Con3["Schema Decode\n(Avro/JSON/Protobuf)"]
        Con4["Consumer Lag\nMonitoring"]
        Con5["Metrics\n(OTEL)"]
    end

    subgraph Resilience["Resilience Layer"]
        direction LR
        R1["CircuitBreaker\n(Fail fast)"]
        R2["ConnectionPool\n(Pool reuse)"]
        R3["RetryDLQ\n(Short/Long retries)"]
        R4["Transactions\n(EOS)"]
        R5["Metrics\n(OTEL)"]
    end

    P --> Resilience
    Con --> Resilience

    style Client fill:#e1f5fe
    style Producer fill:#e8f5e8
    style Consumer fill:#fff3e0
    style Resilience fill:#fce4ec
```

## Quick Start

```go
import "gosdk/pkg/kafka"

// Create client with configuration
client, err := kafka.NewClient(&kafka.Config{
    Brokers: []string{"localhost:9092"},
    Producer: kafka.ProducerConfig{
        RequiredAcks: "all",
        BatchSize:    100,
        LingerMs:     10,
        CompressionType: "snappy",
    },
    Consumer: kafka.ConsumerConfig{
        StartOffset:   kafka.StartOffsetLatest,
        CommitInterval: time.Second,
    },
}, logger)

// Get producer and publish messages
producer, err := client.Producer()
err = producer.Publish(ctx, kafka.Message{
    Topic: "my-topic",
    Key:   []byte("key"),
    Value: []byte("hello world"),
})

// Get consumer and start consuming
consumer, err := client.Consumer("my-group", []string{"my-topic"})
consumer.Start(ctx, func(msg kafka.Message) error {
    fmt.Printf("Received: %s\n", msg.Value)
    return nil
})
```

## Core Components

### KafkaClient

The main entry point providing lazy initialization of producers and consumers:

```go
client, _ := kafka.NewClient(config, logger)

// Producer and consumers are lazily initialized
producer, _ := client.Producer()
consumer, _ := client.Consumer("group-id", []string{"topic-1", "topic-2"})

client.Close() // Closes all resources
```

**Features:**
- Singleton producer per client instance
- Per-group-topic consumer caching
- Connection pooling and management
- Schema registry integration

### KafkaProducer

High-performance message producer with:

```go
producer, _ := client.Producer()

// Simple publish
err := producer.Publish(ctx, kafka.Message{
    Topic: "orders",
    Key:   []byte(orderID),
    Value: orderBytes,
})

// Publish with schema
msg, err := producer.PublishWithSchema(ctx, "orders", key, orderData, schemaID)

// Stats and monitoring
stats := producer.Stats() // Idempotency cache stats
```

**Configuration Options:**

| Option | Description | Default |
|--------|-------------|---------|
| `RequiredAcks` | Acknowledgment level (all, none, leader) | "all" |
| `BatchSize` | Messages per batch | 100 |
| `LingerMs` | Max time to wait before sending | 10ms |
| `CompressionType` | none, gzip, snappy, lz4, zstd | "none" |
| `PartitionStrategy` | hash, roundrobin, leastbytes | "hash" |
| `Idempotency` | Enable exactly-once deduplication | enabled |

### KafkaConsumer

Group-based consumer with automatic retry and DLQ:

```go
consumer, _ := client.Consumer("order-processor", []string{"orders"})

// Start consuming with handler
consumer.Start(ctx, func(msg kafka.Message) error {
    // Process message
    return processOrder(msg)
})

// Schema-aware consumption
consumer.StartWithSchema(ctx, func(msg kafka.SchemaMessage) error {
    // msg.Data is already decoded
    return handleOrder(msg.Data)
})

consumer.Close()
```

**Retry Flow:**
```
1. Fetch message from Kafka
2. Process with short-term retries (3 attempts, exponential backoff)
3. If still failing → send to retry topic
4. After max retries → route to DLQ topic
5. Commit offset only on success or DLQ routing
```

### KafkaAdmin

Topic management and initialization:

```go
admin, _ := kafka.NewKafkaAdmin(brokers, dialer)

// Ensure topic exists
admin.EnsureTopicExists(ctx, "orders", 3, 2)

// Auto-create DLQ and retry topics for consumers
admin.InitializeTopicsForConsumer(ctx, []string{"orders"}, retryConfig)

// Check if topic exists
exists, _ := admin.TopicExists(ctx, "orders")
```

## Resilience Patterns

### Circuit Breaker

Prevents cascading failures by failing fast when Kafka is unavailable:

```go
cbConfig := kafka.CircuitBreakerConfig{
    FailureThreshold: 5,        // Open after 5 failures
    SuccessThreshold: 2,        // Close after 2 successes (half-open)
    Timeout:           30 * time.Second,
    HalfOpenMaxCalls:  3,
}

middleware := kafka.NewCircuitBreakerMiddleware(producer, cbConfig)
middleware.Publish(ctx, msg) // Protected call

state := middleware.State() // closed, open, half-open
stats := middleware.Stats()
```

### Connection Pooling

Manages Kafka connections efficiently:

```go
pool := client.ConnectionPool()
conn, _ := pool.Acquire(ctx, "tcp", "localhost:9092")
defer conn.Release()

// Connection manager for multi-broker setups
manager := client.ConnectionManager()
pool := manager.GetPool("broker-1:9092")
```

### Retry with Dead Letter Queue

Automatic retry with DLQ routing for failed messages:

```go
retryConfig := kafka.RetryConfig{
    MaxRetries:           3,
    InitialBackoff:       100,  // ms
    MaxBackoff:           5000, // ms
    DLQEnabled:           true,
    DLQTopicPrefix:       "dlq-",
    ShortRetryAttempts:   3,
    MaxLongRetryAttempts: 3,
    RetryTopicSuffix:     ".retry",
}
```

## Advanced Features

### Transactions (Exactly-Once Semantics)

Atomic produce/consume cycles:

```go
txConfig := kafka.TransactionConfig{
    Enabled:       true,
    TransactionID: "order-processor-1",
    Timeout:       60 * time.Second,
}

tx, _ := kafka.NewKafkaTransaction(txConfig, brokers, dialer, logger)

// Execute operations atomically
tx.WithTransaction(ctx, func(ctx context.Context) error {
    // Produce messages
    tx.Produce(ctx, msg1)
    tx.Produce(ctx, msg2)

    // Optionally consume and commit offsets
    // tx.Consume(ctx, topic)

    return nil // Commit on nil, abort on error
})
```

### Schema Registry Integration

Avro, JSON Schema, and Protobuf support:

```go
srConfig := kafka.SchemaRegistryConfig{
    Enabled:  true,
    URL:      "http://localhost:8081",
    Username: "user",
    Password: "pass",
    Format:   kafka.SchemaFormatAvro,
}

sr, _ := kafka.NewSchemaRegistry(srConfig, logger)

// Encode with schema
encoded, _ := sr.EncodeMessage(ctx, schemaID, orderData)

// Decode from wire format
decoded, schemaID, _ := sr.DecodeMessage(ctx, message)

// Producer/Consumer with schema
producer.SetSchemaRegistry(sr)
consumer.SetSchemaRegistry(sr)
```

## Observability

### OpenTelemetry Metrics

Full metric collection for production monitoring:

```go
metrics, _ := kafka.NewKafkaMetrics()

producer.SetMetrics(metrics)
consumer.SetMetrics(metrics)

// Metrics recorded:
// - kafka.producer.messages.published/failed
// - kafka.producer.publish.latency
// - kafka.consumer.messages.consumed/committed
// - kafka.consumer.lag
// - kafka.retry.messages / kafka.dlq.messages
// - kafka.circuitbreaker.state/failures
```

### Health Checks

Liveness and readiness probes:

```go
checker := kafka.NewHealthChecker(client, 5*time.Second)

// Full health check
status := checker.Check(ctx)
fmt.Println(status.Healthy())     // true if all checks pass
fmt.Println(status.Degraded())    // true if partially degraded

// Liveness (can we connect?)
liveness := checker.CheckLiveness(ctx)

// Readiness (is operational?)
readiness := checker.CheckReadiness(ctx)
```

## Security

TLS/mTLS configuration:

```go
securityConfig := kafka.SecurityConfig{
    Enabled:     true,
    TLSCertFile: "client.crt",
    TLSKeyFile:  "client.key",
    TLSCAFile:   "ca.crt",
}
```

## Configuration Reference

### Config Structure

```go
type Config struct {
    Brokers              []string
    Producer             ProducerConfig
    Consumer             ConsumerConfig
    Security             SecurityConfig
    Retry                RetryConfig
    Pool                 ConnectionPoolConfig
    Idempotency          IdempotencyConfig
    SchemaRegistryConfig SchemaRegistryConfig
    Transaction          TransactionConfig
}
```

### Default Values

| Component | Setting | Default |
|-----------|---------|---------|
| Producer | BatchSize | 100 |
| Producer | LingerMs | 10 |
| Producer | Compression | none |
| Producer | RequiredAcks | all |
| Consumer | StartOffset | latest |
| Consumer | CommitInterval | 1s |
| Idempotency | WindowSize | 5m |
| Idempotency | MaxCacheSize | 10000 |
| ConnectionPool | MaxConnections | 10 |
| ConnectionPool | IdleTimeout | 5m |
| CircuitBreaker | FailureThreshold | 5 |
| CircuitBreaker | SuccessThreshold | 2 |
| CircuitBreaker | Timeout | 30s |
| Retry | InitialBackoff | 100ms |
| Retry | MaxBackoff | 5000ms |
| Retry | ShortRetryAttempts | 3 |
| Retry | MaxLongRetryAttempts | 3 |

## Error Handling

```go
var (
    ErrProducerNotInitialized   = errors.New("producer not initialized")
    ErrConsumerNotInitialized   = errors.New("consumer not initialized")
    ErrInvalidMessage           = errors.New("invalid message")
    ErrTopicNotFound            = errors.New("topic not found")
    ErrKafkaConnection          = errors.New("kafka connection error")
    ErrCircuitBreakerOpen       = errors.New("circuit breaker is open")
    ErrSchemaRegistry           = errors.New("schema registry error")
    ErrTransaction              = errors.New("transaction error")
)
```

## Dependencies

- [kafka-go](https://github.com/segmentio/kafka-go) - Kafka client
- [srclient](https://github.com/riferrei/srclient) - Schema Registry client
- [retry-go](https://github.com/avast/retry-go) - Retry utilities
- OpenTelemetry SDK - Metrics and tracing

## Best Practices

1. **Single Client**: Create one KafkaClient per application, not per request
2. **Consumer Groups**: Use unique group IDs per deployment for competing consumers
3. **Idempotency**: Enable for exactly-once semantics in production
4. **DLQ**: Always enable for message durability and debugging
5. **Monitoring**: Wire up OpenTelemetry metrics in production
6. **Health Checks**: Integrate with Kubernetes probes
7. **Schema Evolution**: Use backward/forward compatibility in Schema Registry
8. **Transactions**: Use for financial/payment processing where exactly-once is required

## License

Part of the go-sdk project. See LICENSE file for details.
