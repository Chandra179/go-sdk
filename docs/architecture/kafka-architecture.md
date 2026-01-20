# Kafka Architecture Plan for Go SDK

## Executive Summary

**Target**: Build a production-grade Kafka client library supporting exactly-once delivery semantics for medium throughput (10K-100K msgs/sec) in Kubernetes environments.

**Core Philosophy**: Industry-standard patterns without external Kafka libraries, building on top of `segmentio/kafka-go` with production enhancements.

**Status**: Architecture design complete, ready for implementation

---

## 1. Architecture Overview

### 1.1 Component Diagram

```
┌─────────────────────────────────────────────────────────────┐
│                    Application Layer                        │
│  ┌──────────────┐          ┌──────────────┐                │
│  │  Producer    │          │  Consumer    │                │
│  │  Service     │          │  Service     │                │
│  └──────┬───────┘          └──────┬───────┘                │
└─────────┼────────────────────────┼────────────────────────┘
          │                        │
┌─────────▼────────────────────────▼────────────────────────┐
│                  Kafka Client Layer                        │
│  ┌──────────────────────────────────────────────────────┐ │
│  │           Kafka Client (singleton)                   │ │
│  │  - Connection Pool Management                         │ │
│  │  - Health Check                                      │ │
│  │  - Configuration Loader                               │ │
│  └────────┬──────────────────────────────────────┬──────┘ │
│           │                                      │        │
│  ┌────────▼────────┐                  ┌─────────▼───────┐ │
│  │  Producer Pool │                  │  Consumer Pool  │ │
│  │  - Idempotent  │                  │  - Group Mgmt   │ │
│  │  - Batching    │                  │  - Offset Mgmt  │ │
│  │  - Compression │                  │  - DLQ Routing  │ │
│  └────────┬────────┘                  └─────────┬───────┘ │
│           │                                      │        │
└───────────┼──────────────────────────────────────┼─────────┘
            │                                      │
┌───────────▼──────────────────────────────────────▼─────────┐
│            Infrastructure Layer (segmentio/kafka-go)       │
│  ┌─────────────────────────────────────────────────────┐  │
│  │              Kafka Protocol Implementation           │  │
│  └─────────────────────────────────────────────────────┘  │
└────────────────────────────────────────────────────────────┘
```

### 1.2 Package Structure

```
pkg/kafka/
├── README.md
├── client.go              # Main client (connection pool, health)
├── producer/
│   ├── producer.go        # Producer interface and implementation
│   ├── batcher.go         # Message batching
│   ├── idempotency.go     # Exactly-once producer support
│   └── config.go          # Producer configuration
├── consumer/
│   ├── consumer.go        # Consumer interface and implementation
│   ├── group.go           # Consumer group management
│   ├── offset_manager.go  # Manual offset commits
│   ├── dlq.go             # Dead-letter queue
│   └── config.go          # Consumer configuration
├── metrics/
│   ├── producer.go        # Producer metrics
│   ├── consumer.go        # Consumer metrics
│   └── registry.go        # Metrics registry
├── health/
│   ├── checker.go         # Health check implementation
│   └── probe.go           # Readiness/liveness probes
├── types.go               # Common types
├── errors.go              # Custom errors
└── config.go              # Configuration loader

internal/service/event/
├── service.go             # Business logic layer
├── handler.go             # HTTP handlers (unchanged)
├── types.go               # Request/response types
├── producer.go            # Producer service wrapper
├── consumer.go            # Consumer service wrapper
└── middleware.go          # Middleware for tracing/error handling
```

---

## 2. Exactly-Once Delivery Semantics

### 2.1 Exactly-Once Producer (Idempotent)

**Implementation Strategy**:
```go
// Required Producer Configurations
ProducerID string // Unique producer ID
EnableID   bool   // Enable idempotent producer

// Message Sequence Numbering
type ProducerMessage struct {
    SequenceID int64
    MessageID  string // Unique message identifier
    Message    kafka.Message
}

// IdempotentProducer wraps kafka-go writer
type IdempotentProducer struct {
    producerID string
    sequence   atomic.Int64
    dedupCache *lru.Cache // LRU cache for deduplication
}
```

**Key Features**:
- **Producer IDs**: Unique per producer instance
- **Sequence Numbers**: Monotonically increasing per producer
- **Deduplication Cache**: LRU cache to track sent messages
- **Max Request Timeout**: 30s (configurable)
- **Max In-Flight Requests**: 5 (to maintain ordering)

### 2.2 Exactly-Once Consumer

**Implementation Strategy**:
```go
// Two-Phase Commit Pattern
type ExactlyOnceConsumer struct {
    offsetStore  OffsetStore       // External offset storage
    transaction  Transaction       // Outbox pattern
    idempotency  IdempotencyChecker
}

// Processing Flow:
// 1. Read message with offset
// 2. Start transaction
// 3. Process message (business logic)
// 4. Store offset in transaction
// 5. Commit transaction
// 6. Commit Kafka offset (optional, as backup)
```

**Key Features**:
- **External Offset Storage**: Redis/PostgreSQL for transactional offset commits
- **Outbox Pattern**: Message stored in DB before publishing
- **Idempotency Checkers**: Business-level idempotency (e.g., check if event already processed)
- **Retry with Dedup**: Retries use message ID for deduplication

### 2.3 Offset Management

**Configuration**:
```go
type OffsetManager struct {
    Mode          OffsetCommitMode // Manual, Transactional
    Store         OffsetStoreType   // Kafka, Redis, PostgreSQL
    CommitInterval time.Duration    // 5s for batch commits
    SyncCommit    bool              // True for exactly-once
}

// OffsetStore interface for external storage
type OffsetStore interface {
    Store(ctx context.Context, topic string, partition int, offset int64) error
    Fetch(ctx context.Context, topic string, partition int) (int64, error)
}
```

---

## 3. Producer Architecture

### 3.1 Batching Strategy

**Configuration**:
```go
type BatcherConfig struct {
    MaxMessages    int           // 1000 messages per batch
    MaxSize        int           // 1MB per batch
    MaxWaitTime    time.Duration // 100ms max wait for full batch
    Compression    CompressionType // Snappy, Gzip, None
}

// Batcher implementation
type MessageBatcher struct {
    buffer      []kafka.Message
    mutex       sync.Mutex
    timer       *time.Timer
    flushChan   chan struct{}
}
```

**Medium Volume Optimization**:
- Batch size: 1000 messages or 1MB (whichever comes first)
- Flush interval: 100ms (balance latency vs throughput)
- Compression: Snappy (default for medium volume)
- Adaptive batching based on current throughput

### 3.2 Retry Strategy

**Configuration**:
```go
type RetryConfig struct {
    MaxRetries      int           // 3 retries
    BackoffBase     time.Duration // 100ms
    BackoffMax      time.Duration // 5s
    BackoffMultiplier float64     // 2.0
    RetriableErrors []error       // List of retriable errors
}

// Exponential backoff implementation
func (r *RetryConfig) GetBackoff(attempt int) time.Duration {
    backoff := r.BackoffBase * float64(math.Pow(r.BackoffMultiplier, float64(attempt)))
    if backoff > float64(r.BackoffMax) {
        return r.BackoffMax
    }
    return time.Duration(backoff)
}
```

**Retriable Errors**:
- Network timeouts
- Broker not available
- Leader not available
- Connection errors

**Non-Retriable Errors**:
- Invalid topic
- Invalid message
- Authentication errors

### 3.3 Connection Pooling

**Configuration**:
```go
type ProducerPool struct {
    pool         chan *kafka.Writer
    maxSize      int           // 5 producers in pool
    idleTimeout  time.Duration // 5min idle timeout
    healthCheck  time.Duration // 30s health check interval
}
```

---

## 4. Consumer Architecture

### 4.1 Consumer Group Management

**Configuration**:
```go
type ConsumerGroup struct {
    GroupID      string
    InstanceID   string // Optional static member ID
    Rebalance    RebalanceConfig
    Heartbeat    HeartbeatConfig
}

type RebalanceConfig struct {
    Strategy     string // Range, RoundRobin, Sticky
    Timeout      time.Duration
}

type HeartbeatConfig struct {
    Interval     time.Duration // 3s
    Timeout      time.Duration // 10s
}
```

**Kubernetes Integration**:
- Use pod name as instance ID for static membership
- Sticky rebalancing for minimal partition reassignment
- Graceful shutdown with 30s timeout

### 4.2 Message Processing Pipeline

```
┌─────────────┐    ┌──────────────┐    ┌─────────────┐    ┌─────────────┐
│  Poll Kafka │ -> │  Validation  │ -> │  Processing │ -> │  Commit     │
│  (batch)    │    │  (schema)    │    │  (async)    │    │  (manual)   │
└─────────────┘    └──────────────┘    └─────────────┘    └─────────────┘
                      │                      │                   │
                      v                      v                   v
                 ┌─────────┐          ┌──────────┐        ┌──────────┐
                 │  DLQ    │          │ Metrics  │        │ Tracing  │
                 └─────────┘          └──────────┘        └──────────┘
```

**Implementation**:
```go
// Processing pipeline
func (c *Consumer) processMessages(ctx context.Context) {
    for {
        messages, err := c.reader.FetchMessage(ctx, c.batchSize)
        if err != nil {
            handlePollError(err)
            continue
        }

        // Process in parallel with worker pool
        results := c.workerPool.Process(messages)

        // Handle failures
        for _, result := range results {
            if result.Error != nil {
                if !isRetriable(result.Error) {
                    c.dlq.Send(ctx, result.Message)
                }
            } else {
                c.offsetManager.Commit(result.Message)
            }
        }
    }
}
```

### 4.3 Dead-Letter Queue (DLQ)

**Configuration**:
```go
type DLQConfig struct {
    TopicPrefix      string        // "dlq-" prefix
    MaxRetries      int           // 3 retries before DLQ
    RetentionPeriod  time.Duration // 7 days
    IncludeHeaders   bool          // Include original headers
}

type DLQ struct {
    producer       *kafka.Writer
    retryCount     map[string]int // Message ID -> retry count
}
```

**DLQ Message Format**:
```json
{
  "original_topic": "original-topic",
  "original_partition": 0,
  "original_offset": 12345,
  "error": "validation failed",
  "timestamp": "2025-01-18T10:00:00Z",
  "retry_count": 3,
  "original_message": {
    "key": "...",
    "value": "...",
    "headers": {...}
  }
}
```

### 4.4 Backpressure Handling

**Implementation**:
```go
type BackpressureConfig struct {
    MaxInFlight     int           // Max unprocessed messages
    AlertThreshold  float64       // 80% of max triggers alert
    ScaleThreshold  float64       // 90% triggers scale recommendation
}

func (c *Consumer) handleBackpressure(ctx context.Context) {
    inFlight := c.metrics.CurrentInFlight()
    if inFlight > c.config.MaxInFlight {
        c.logger.Warn(ctx, "Backpressure detected, reducing fetch rate")
        c.reader.SetReadBackoffMax(1 * time.Second)
    }
}
```

---

## 5. Kubernetes Integration

### 5.1 Graceful Shutdown

**Implementation**:
```go
type GracefulShutdown struct {
    Timeout      time.Duration // 30s
    DrainTimeout time.Duration // 20s for in-flight messages
    StopChan     chan struct{}
}

// Lifecycle hooks
func (c *Client) Shutdown(ctx context.Context) error {
    // 1. Stop accepting new messages
    close(c.stopChan)

    // 2. Wait for in-flight processing
    select {
    case <-time.After(c.config.DrainTimeout):
        log.Warn("Shutdown timeout, forcing close")
    case <-c.processingDone:
        log.Info("All messages processed")
    }

    // 3. Commit final offsets
    c.offsetManager.CommitAll(ctx)

    // 4. Close connections
    return c.Close()
}
```

**PreStop Hook**:
```yaml
lifecycle:
  preStop:
    exec:
      command: ["/bin/sh", "-c", "curl -X POST http://localhost:8080/health/shutdown && sleep 25"]
```

### 5.2 Health Checks

**Liveness Probe** (Kafka connection):
```go
func (c *Client) LivenessProbe() error {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    if err := c.Ping(ctx); err != nil {
        return fmt.Errorf("kafka not reachable: %w", err)
    }
    return nil
}
```

**Readiness Probe** (Consumer group ready):
```go
func (c *Consumer) ReadinessProbe() error {
    if !c.IsJoinedGroup() {
        return fmt.Errorf("consumer not joined group")
    }
    if c.HasLag() {
        log.Warn("Consumer has backlog, but ready")
    }
    return nil
}
```

**Kubernetes Configuration**:
```yaml
livenessProbe:
  httpGet:
    path: /health/live
    port: 8080
  initialDelaySeconds: 30
  periodSeconds: 10
  timeoutSeconds: 5
  failureThreshold: 3

readinessProbe:
  httpGet:
    path: /health/ready
    port: 8080
  initialDelaySeconds: 10
  periodSeconds: 5
  timeoutSeconds: 3
  failureThreshold: 3
```

### 5.3 Horizontal Pod Autoscaling

**Metrics for HPA**:
```go
type HPAMetrics struct {
    MessagesPerSecond  float64
    ConsumerLag        int64
    ProcessingLatency  time.Duration
}

func (c *Consumer) GetHPAMetrics(ctx context.Context) HPAMetrics {
    return HPAMetrics{
        MessagesPerSecond: c.metrics.MessagesPerSecond(),
        ConsumerLag:       c.offsetManager.GetLag(),
        ProcessingLatency: c.metrics.AvgProcessingTime(),
    }
}
```

**HPA Configuration**:
```yaml
metrics:
  - type: Pods
    pods:
      metric:
        name: kafka_consumer_lag
      target:
        type: AverageValue
        averageValue: "1000"
  - type: Pods
    pods:
      metric:
        name: kafka_processing_time_seconds
      target:
        type: AverageValue
        averageValue: "0.5"
```

---

## 6. Configuration Management

### 6.1 Configuration Schema

```go
type Config struct {
    // Kafka Cluster
    Brokers      []string
    ClientID     string
    Security     SecurityConfig

    // Producer
    Producer     ProducerConfig

    // Consumer
    Consumer     ConsumerConfig

    // Monitoring
    Metrics      MetricsConfig

    // Health
    HealthCheck  HealthConfig
}

type ProducerConfig struct {
    // Batching
    BatchSize        int
    BatchSizeBytes   int
    BatchTimeout     time.Duration

    // Compression
    CompressionType  string

    // Retry
    MaxRetries       int
    RetryBackoff     time.Duration

    // Idempotency
    EnableIdempotent bool
    ProducerID       string

    // Performance
    MaxInFlight      int
}

type ConsumerConfig struct {
    // Group
    GroupID          string
    InstanceID       string

    // Offset
    AutoCommit       bool
    CommitInterval   time.Duration
    OffsetReset      string // earliest, latest, none

    // Fetching
    FetchMinBytes    int
    FetchMaxBytes    int
    FetchMaxWait     time.Duration

    // Processing
    MaxPollRecords   int
    SessionTimeout   time.Duration
    HeartbeatInterval time.Duration

    // DLQ
    EnableDLQ        bool
    DLQTopicPrefix   string
    DLQMaxRetries    int
}
```

### 6.2 Environment Variables

```bash
# Kafka Cluster
KAFKA_BROKERS=broker1:9092,broker2:9092,broker3:9092
KAFKA_CLIENT_ID=${HOSTNAME}
KAFKA_SECURITY_ENABLED=false

# Producer
KAFKA_PRODUCER_BATCH_SIZE=1000
KAFKA_PRODUCER_BATCH_SIZE_BYTES=1048576
KAFKA_PRODUCER_BATCH_TIMEOUT=100ms
KAFKA_PRODUCER_COMPRESSION=snappy
KAFKA_PRODUCER_MAX_RETRIES=3
KAFKA_PRODUCER_ENABLE_IDEMPOTENT=true

# Consumer
KAFKA_CONSUMER_GROUP_ID=event-consumer
KAFKA_CONSUMER_INSTANCE_ID=${POD_NAME}
KAFKA_CONSUMER_AUTO_COMMIT=false
KAFKA_CONSUMER_OFFSET_RESET=latest
KAFKA_CONSUMER_MAX_POLL_RECORDS=500
KAFKA_CONSUMER_ENABLE_DLQ=true
KAFKA_CONSUMER_DLQ_TOPIC_PREFIX=dlq-

# Health
KAFKA_HEALTH_CHECK_INTERVAL=30s
KAFKA_SHUTDOWN_TIMEOUT=30s
```

### 6.3 Configuration Validation

```go
func (c *Config) Validate() error {
    var errs []error

    // Validate brokers
    if len(c.Brokers) == 0 {
        errs = append(errs, errors.New("KAFKA_BROKERS required"))
    }

    // Validate exactly-once prerequisites
    if c.Producer.EnableIdempotent && c.Producer.ProducerID == "" {
        errs = append(errs, errors.New("PRODUCER_ID required for idempotent producer"))
    }

    if !c.Consumer.AutoCommit && c.Consumer.GroupID == "" {
        errs = append(errs, errors.New("GROUP_ID required for manual commits"))
    }

    return errors.Join(errs...)
}
```

---

## 7. Observability

### 7.1 Metrics

**Producer Metrics**:
```go
type ProducerMetrics struct {
    MessagesProduced    Counter
    BytesProduced      Counter
    ProduceLatency     Histogram
    ProduceErrors      Counter
    Retries            Counter
    BatchSize          Histogram
    CompressionRatio   Gauge
}
```

**Consumer Metrics**:
```go
type ConsumerMetrics struct {
    MessagesConsumed    Counter
    BytesConsumed      Counter
    ConsumerLag        Gauge
    ProcessingLatency  Histogram
    PollLatency        Histogram
    CommitLatency      Histogram
    DLQMessages        Counter
    Retries            Counter
    CurrentInFlight    Gauge
}
```

**Prometheus Metrics Format**:
```
kafka_producer_messages_total{topic="events"} 12345
kafka_producer_latency_seconds_bucket{topic="events",le="0.001"} 1000
kafka_producer_latency_seconds_bucket{topic="events",le="0.01"} 5000

kafka_consumer_messages_total{topic="events",group="consumer-1"} 10000
kafka_consumer_lag_bytes{topic="events",partition="0",group="consumer-1"} 1024000
kafka_consumer_processing_time_seconds{topic="events"} 0.05
```

### 7.2 Structured Logging

```go
type LogFields struct {
    Operation     string
    Topic         string
    Partition     int
    Offset        int64
    Key           string
    MessageSize   int
    Duration      time.Duration
    Error         error
    TraceID       string
    SpanID        string
}

// Example logs
logger.Info(ctx, "Message published",
    logger.Field{Key: "topic", Value: topic},
    logger.Field{Key: "partition", Value: 0},
    logger.Field{Key: "offset", Value: 12345},
    logger.Field{Key: "duration_ms", Value: 15.2},
)
```

### 7.3 Distributed Tracing

**OpenTelemetry Integration**:
```go
func (p *Producer) PublishWithTracing(ctx context.Context, msg kafka.Message) error {
    ctx, span := tracer.Start(ctx, "kafka.produce",
        trace.WithAttributes(
            attribute.String("kafka.topic", msg.Topic),
            attribute.String("kafka.key", string(msg.Key)),
        ),
    )
    defer span.End()

    // Add trace context to message headers
    msg.Headers["traceparent"] = span.SpanContext().String()

    return p.Publish(ctx, msg)
}
```

**Trace Propagation**:
```go
func (c *Consumer) ProcessWithTracing(ctx context.Context, msg kafka.Message) error {
    // Extract trace context from headers
    spanContext := extractSpanContext(msg.Headers)
    ctx = trace.ContextWithSpanContext(ctx, spanContext)

    ctx, span := tracer.Start(ctx, "kafka.consume")
    defer span.End()

    // Process message
    return c.handler(ctx, msg)
}
```

### 7.4 Health Dashboard

**Metrics Endpoint** (`/metrics`):
```go
func (c *Client) MetricsHandler() http.HandlerFunc {
    return promhttp.Handler()
}
```

**Health Endpoint** (`/health`):
```go
type HealthResponse struct {
    Status    string            `json:"status"`
    Kafka     HealthComponent   `json:"kafka"`
    Producer  HealthComponent   `json:"producer,omitempty"`
    Consumer  HealthComponent   `json:"consumer,omitempty"`
    Timestamp time.Time         `json:"timestamp"}
}
```

---

## 8. Error Handling

### 8.1 Error Classification

```go
type ErrorCategory int

const (
    ErrorRetriable    ErrorCategory = iota
    ErrorNonRetriable
    ErrorFatal
)

type KafkaError struct {
    Category   ErrorCategory
    Message    string
    Code       int
    Timestamp  time.Time
    Context    map[string]interface{}
}

func (e *KafkaError) IsRetriable() bool {
    return e.Category == ErrorRetriable
}
```

**Error Mappings**:
- **Retriable**: Network timeouts, broker unavailable, leader not available
- **Non-Retriable**: Invalid topic, authentication failed, invalid message
- **Fatal**: Configuration errors, unsupported operations

### 8.2 Error Handling Flow

```go
func (p *Producer) PublishWithRetry(ctx context.Context, msg kafka.Message) error {
    var lastErr error

    for attempt := 0; attempt <= p.config.MaxRetries; attempt++ {
        if attempt > 0 {
            backoff := p.retryConfig.GetBackoff(attempt - 1)
            select {
            case <-time.After(backoff):
            case <-ctx.Done():
                return ctx.Err()
            }
        }

        err := p.writer.WriteMessages(ctx, msg)
        if err == nil {
            return nil
        }

        lastErr = err
        if !isRetriableError(err) {
            return p.wrapError(err, ErrorNonRetriable)
        }
    }

    return p.wrapError(lastErr, ErrorRetriable)
}
```

### 8.3 Circuit Breaker

```go
type CircuitBreaker struct {
    maxFailures    int
    timeout        time.Duration
    failures       int
    lastFailure    time.Time
    state          CircuitState
    mu             sync.RWMutex
}

type CircuitState int

const (
    StateClosed CircuitState = iota
    StateOpen
    StateHalfOpen
)

func (cb *CircuitBreaker) Execute(fn func() error) error {
    if cb.isOpen() {
        return ErrCircuitOpen
    }

    err := fn()
    if err != nil {
        cb.recordFailure()
    } else {
        cb.recordSuccess()
    }

    return err
}
```

---

## 9. Security Considerations

### 9.1 TLS/SSL Configuration

```go
type TLSConfig struct {
    Enabled            bool
    CAFile            string
    CertFile          string
    KeyFile           string
    InsecureSkipVerify bool
}

func (c *TLSConfig) ToDialer() *kafkago.Dialer {
    if !c.Enabled {
        return &kafkago.Dialer{}
    }

    tlsConfig := &tls.Config{}
    if c.CAFile != "" {
        cert, err := tls.LoadX509KeyPair(c.CertFile, c.KeyFile)
        if err != nil {
            // handle error
        }
        tlsConfig.Certificates = []tls.Certificate{cert}
    }

    return &kafkago.Dialer{
        TLS: tlsConfig,
    }
}
```

### 9.2 SASL Authentication

```go
type SASLConfig struct {
    Mechanism string // PLAIN, SCRAM-SHA-256, SCRAM-SHA-512
    Username   string
    Password   string
}

func (c *SASLConfig) ToSASL() kafkago.SASL {
    switch c.Mechanism {
    case "PLAIN":
        return kafkago.SASL{
            Mechanism: kafkago.SASLTypePlainText,
        }
    case "SCRAM-SHA-256":
        return kafkago.SASL{
            Mechanism: kafkago.SASLTypeSCRAMSHA256,
        }
    case "SCRAM-SHA-512":
        return kafkago.SASL{
            Mechanism: kafkago.SASLTypeSCRAMSHA512,
        }
    default:
        return kafkago.SASL{}
    }
}
```

---

## 10. Testing Strategy

### 10.1 Unit Tests

```go
// Test Producer Batching
func TestProducerBatching(t *testing.T) {
    batcher := NewMessageBatcher(BatcherConfig{
        MaxMessages: 1000,
        MaxSize:     1024 * 1024,
        MaxWaitTime: 100 * time.Millisecond,
    })

    for i := 0; i < 1500; i++ {
        batcher.Add(kafka.Message{Value: []byte("test")})
    }

    // Should have 2 batches
    assert.Equal(t, 2, len(batcher.Flush()))
}

// Test DLQ Routing
func TestDLQRouting(t *testing.T) {
    dlq := NewDLQ(DLQConfig{
        TopicPrefix: "dlq-",
        MaxRetries: 3,
    })

    msg := kafka.Message{Topic: "original", Value: []byte("test")}

    // After 3 retries, should route to DLQ
    for i := 0; i < 3; i++ {
        dlq.Retry(context.Background(), msg)
    }

    dlqTopic, _ := dlq.GetDLQTopic(msg)
    assert.Equal(t, "dlq-original", dlqTopic)
}
```

### 10.2 Integration Tests (Testcontainers)

```go
func TestKafkaIntegration(t *testing.T) {
    ctx := context.Background()

    // Start Kafka container
    kafkaContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
        ContainerRequest: testcontainers.ContainerRequest{
            Image: "confluentinc/cp-kafka:latest",
            Env: map[string]string{
                "KAFKA_ZOOKEEPER_CONNECT": "zookeeper:2181",
                "KAFKA_ADVERTISED_LISTENERS": "PLAINTEXT://localhost:9092",
            },
            ExposedPorts: []string{"9092"},
        },
        Started: true,
    })
    defer kafkaContainer.Terminate(ctx)

    // Get Kafka address
    host, err := kafkaContainer.Host(ctx)
    port, err := kafkaContainer.MappedPort(ctx, "9092")

    // Test producer/consumer
    client := NewClient([]string{fmt.Sprintf("%s:%s", host, port.Port())})

    producer, _ := client.Producer()
    producer.Publish(ctx, kafka.Message{Topic: "test", Value: []byte("message")})

    consumer, _ := client.Consumer("test-group")
    consumer.Subscribe(ctx, []string{"test"}, func(msg kafka.Message) error {
        assert.Equal(t, "message", string(msg.Value))
        return nil
    })
}
```

### 10.3 Chaos Engineering Tests

```go
func TestKafkaResilience(t *testing.T) {
    // Test connection drops
    // Test broker failure
    // Test network partitions
    // Test message ordering
}
```

---

## 11. Implementation Roadmap

### Phase 1: Core Infrastructure (Week 1-2)
- [ ] Configuration loader with validation
- [ ] Connection pool management
- [ ] Health check implementation
- [ ] Basic producer/consumer interfaces

### Phase 2: Exactly-Once Producer (Week 2-3)
- [ ] Idempotent producer implementation
- [ ] Message batching with adaptive sizing
- [ ] Retry with exponential backoff
- [ ] Producer metrics

### Phase 3: Exactly-Once Consumer (Week 3-4)
- [ ] Consumer group management
- [ ] Manual offset commit implementation
- [ ] Offset storage interface (Redis/PostgreSQL)
- [ ] Worker pool for parallel processing

### Phase 4: Dead-Letter Queue (Week 4)
- [ ] DLQ producer
- [ ] Retry counter management
- [ ] DLQ message formatting
- [ ] DLQ metrics

### Phase 5: Kubernetes Integration (Week 4-5)
- [ ] Graceful shutdown implementation
- [ ] Liveness/readiness probes
- [ ] Pre-stop hook documentation
- [ ] HPA metrics

### Phase 6: Observability (Week 5-6)
- [ ] Prometheus metrics
- [ ] Structured logging
- [ ] OpenTelemetry tracing
- [ ] Health dashboard endpoints

### Phase 7: Security (Week 6)
- [ ] TLS/SSL support
- [ ] SASL authentication
- [ ] Security documentation

### Phase 8: Testing (Week 6-7)
- [ ] Unit tests for all components
- [ ] Integration tests with testcontainers
- [ ] Chaos engineering tests
- [ ] Performance benchmarks

### Phase 9: Documentation & Deployment (Week 7-8)
- [ ] API documentation
- [ ] Architecture diagrams
- [ ] Deployment guides
- [ ] Monitoring setup guides

---

## 12. Performance Targets

**For Medium Volume (10K-100K msgs/sec)**:

| Metric | Target | Notes |
|--------|--------|-------|
| Producer Latency (P50) | < 10ms | Including batching |
| Producer Latency (P99) | < 100ms | With retries |
| Consumer Latency (P50) | < 50ms | End-to-end |
| Consumer Lag | < 10,000 messages | Under normal load |
| Throughput (per pod) | 20,000 msgs/sec | 1000 messages * 20ms processing |
| CPU Usage | < 70% | During peak load |
| Memory Usage | < 512MB | Per pod |
| Network I/O | 100 MB/s | Per pod |

---

## 13. Key Design Decisions & Trade-offs

### 13.1 Segmentio/kafka-go vs. Alternative Libraries

**Decision**: Stick with `segmentio/kafka-go`

**Reasoning**:
- Pure Go (no CGo dependencies)
- Already integrated in codebase
- Sufficient for medium volume requirements
- Active maintenance (v0.4.49, 2025 updates)
- Better cross-platform support than confluent-kafka-go

**Trade-offs**:
- Slower than confluent-kafka-go (librdkafka)
- Newer than Sarama, but actively maintained
- Missing some advanced features (schema registry, transactions)

### 13.2 External Offset Storage

**Decision**: Use PostgreSQL for offset storage (exactly-once)

**Reasoning**:
- Transactional guarantees for exactly-once
- Already in tech stack
- Reliable and durable
- Supports complex queries for lag monitoring

**Alternatives**:
- Redis (faster, but less transactional)
- Kafka (limited transactional support)

### 13.3 Message Batching Strategy

**Decision**: Time-based + Size-based hybrid batching

**Reasoning**:
- Balance between latency and throughput
- 100ms timeout for low latency
- 1MB or 1000 messages for high throughput
- Adaptive batching based on current load

### 13.4 Consumer Group Rebalancing

**Decision**: Sticky rebalancing with static membership

**Reasoning**:
- Minimal partition reassignment
- Better for Kubernetes deployments (stable pod names)
- Reduced rebalancing overhead

---

## 14. Monitoring & Alerting

### 14.1 Critical Alerts

```yaml
# Prometheus Alert Rules
groups:
  - name: kafka_critical
    rules:
      - alert: KafkaConnectionLost
        expr: kafka_client_up == 0
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "Kafka connection lost"

      - alert: KafkaProducerErrorRateHigh
        expr: rate(kafka_producer_errors_total[5m]) > 0.1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Producer error rate > 10%"

      - alert: KafkaConsumerLagHigh
        expr: kafka_consumer_lag_bytes > 10000000
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "Consumer lag > 10MB"

      - alert: KafkaDLQMessagesHigh
        expr: rate(kafka_dlq_messages_total[5m]) > 10
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High DLQ message rate"
```

### 14.2 Grafana Dashboard Queries

```promql
# Producer Throughput
sum(rate(kafka_producer_messages_total[5m])) by (topic)

# Consumer Lag
kafka_consumer_lag_bytes

# End-to-end Latency
histogram_quantile(0.99, sum(rate(kafka_consumer_processing_time_seconds_bucket[5m])) by (le, topic))

# DLQ Rate
sum(rate(kafka_dlq_messages_total[5m])) by (topic)
```

---

## 15. Risk Mitigation

### 15.1 Identified Risks

| Risk | Impact | Likelihood | Mitigation |
|------|--------|------------|------------|
| Message loss during failover | High | Medium | External offset storage, graceful shutdown |
| Consumer group rebalancing disruption | Medium | High | Sticky rebalancing, static membership |
| Idempotency cache exhaustion | High | Low | LRU cache with TTL, message dedup by ID |
| Connection pool exhaustion | Medium | Medium | Circuit breaker, health checks |
| Kafka cluster degradation | High | Low | Multiple brokers, circuit breaker, retries |
| Performance regression | Medium | Medium | Continuous benchmarking, load testing |

### 15.2 Disaster Recovery

**Backup Strategy**:
- Offset storage backups (PostgreSQL PITR)
- DLQ topic retention (7 days)
- Configuration versioning

**Failover Strategy**:
- Multi-region Kafka deployment
- Circuit breakers prevent cascade failures
- Retry with exponential backoff
- Dead-letter queues for failed messages

---

## 16. Success Criteria

**Functional Requirements**:
- ✅ Exactly-once delivery semantics
- ✅ Dead-letter queue for failed messages
- ✅ Graceful shutdown in Kubernetes
- ✅ Health check endpoints
- ✅ Manual offset commits
- ✅ Message batching and compression
- ✅ Retry with exponential backoff

**Non-Functional Requirements**:
- ✅ < 100ms P99 latency
- ✅ 20,000 msgs/sec throughput per pod
- ✅ 99.9% uptime
- ✅ < 70% CPU at peak load
- ✅ < 512MB memory per pod
- ✅ Full observability (metrics, logs, traces)

---

## References

- **Kafka Documentation**: https://kafka.apache.org/documentation/
- **Segmentio/kafka-go**: https://github.com/segmentio/kafka-go
- **Exactly-Once Semantics**: https://www.confluent.io/blog/exactly-once-semantics-are-possible-heres-how-apache-kafka-does-it/
- **Kafka Best Practices**: https://www.confluent.io/blog/kafka-client-best-practices/
