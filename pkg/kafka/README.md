# Kafka Package

A comprehensive Kafka client library for Go applications built on top of `segmentio/kafka-go`. This package provides producer, consumer, retry mechanisms, dead-letter queue (DLQ) support, TLS security, and OpenTelemetry metrics integration.

## Architecture Overview

### Core Components

#### 1. Client Layer (`client.go`)
- **KafkaClient**: Main entry point with lazy initialization
- Manages singleton producer and per-groupID consumers
- Thread-safe with mutex protection for concurrent access
- Health check functionality via `Ping()` method

#### 2. Producer (`producer.go`)
- **KafkaProducer**: High-performance message publishing
- Configurable compression (none, gzip, snappy, lz4, zstd)
- Batch processing and async/sync publishing modes
- Automatic retry with exponential backoff
- OpenTelemetry metrics for latency, error rates, and message counts

#### 3. Consumer (`consumer.go`)
- **KafkaConsumer**: Group-based message consumption
- Automatic offset management and commitment
- Configurable poll sizes and heartbeat intervals
- Graceful shutdown with context cancellation
- Partition change monitoring

#### 4. Error Handling & Resilience

##### Retry System (`retry.go`, `dlq.go`)
- **Short-term retries**: Immediate retries with exponential backoff for transient failures
- **Long-term retries**: Delayed retries via dedicated retry topics
- **Dead-Letter Queue (DLQ)**: Final destination for permanently failed messages
- Configurable retry attempts, backoff periods, and topic naming

##### Retry Flow:
```
Message Processing Failure
         ↓
Short-term Retries (3 attempts with backoff)
         ↓
Send to Retry Topic (.retry suffix)
         ↓
Long-term Retries (configured max attempts)
         ↓
Send to DLQ (.dlq suffix) if still failing
```

#### 5. Security (`security.go`)
- TLS encryption support with mutual authentication
- X.509 certificate-based security
- Configurable TLS version (minimum TLS 1.2)
- Certificate and CA validation

#### 6. Type System (`types.go`)
- Clean interface definitions for Client, Producer, Consumer
- Standardized Message structure with headers support
- Comprehensive error definitions with context
- Configuration structures for all components

## Key Features

### Lazy Initialization
- Producer and consumers created only on first access
- Optimized startup performance and resource usage
- Thread-safe singleton pattern

### Retry Strategy
- **Immediate retries** for transient errors (network glitches, temporary unavailability)
- **Delayed retries** via retry topics for persistent issues
- **DLQ routing** for messages that exhaust all retry attempts
- Configurable retry counts and backoff periods

### Message Headers
- Retry metadata tracking (retry count, failure timestamps)
- Original topic preservation for DLQ/retry processing
- Error context propagation through retry chain

### Configuration
- Environment-based configuration via central `cfg` package
- Separate configs for producer, consumer, security, and retry policies
- Default values with override capabilities

## Usage Patterns

### Basic Producer
```go
client, _ := kafka.NewClient(config, logger)
producer, _ := client.Producer()

err := producer.Publish(ctx, kafka.Message{
    Topic: "events",
    Key:   []byte("user-123"),
    Value: []byte("event-data"),
    Headers: map[string]string{
        "event-type": "user-action",
    },
})
```

### Consumer with Retry/DLQ
```go
handler := func(msg kafka.Message) error {
    // Process message
    return processEvent(msg)
}

kafka.StartConsumer(ctx, client, logger, "my-group", 
    []string{"events"}, handler, retryConfig)
```

## Configuration

### Producer Settings
- `RequiredAcks`: none, leader, or all
- `BatchSize`: Messages per batch for efficiency
- `CompressionType`: none, gzip, snappy, lz4, zstd
- `Async`: Synchronous vs asynchronous publishing
- `MaxAttempts`: Built-in producer retry attempts

### Consumer Settings
- `MinBytes/MaxBytes`: Fetch size bounds
- `CommitInterval`: Auto-commit frequency
- `MaxPollRecords`: Maximum messages per poll
- `HeartbeatInterval`: Consumer group heartbeat timing
- `WatchPartitionChanges`: Dynamic partition monitoring

### Retry Configuration
- `ShortRetryAttempts`: Immediate retry count
- `MaxLongRetryAttempts`: Total retry limit
- `InitialBackoff/MaxBackoff`: Exponential backoff bounds
- `DLQEnabled`: Enable dead-letter queue routing
- `DLQTopicPrefix/RetryTopicSuffix`: Topic naming conventions

## Error Handling

The package defines specific error types for different failure scenarios:
- Connection errors (`ErrKafkaConnection`)
- Publishing failures (`ErrKafkaPublish`)
- Configuration issues (`ErrTLSConfiguration`, `ErrInvalidCompression`)
- Initialization errors (`ErrProducerNotInitialized`)

All errors include context and are designed for `errors.Is` checks.

## Integration Points

- **Logging**: Uses `gosdk/pkg/logger` with context propagation
- **Configuration**: Integrates with `gosdk/cfg` for environment-based settings
- **Security**: TLS certificates managed through filesystem paths

This architecture provides a production-ready, resilient Kafka client with comprehensive observability and error handling capabilities suitable for high-throughput, mission-critical applications.