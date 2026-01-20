# Kafka Architecture Approach & Rationale

## Executive Summary

This document explains the architectural approach and design decisions for building a production-grade Kafka client library in Go. The architecture prioritizes **reliability, exactly-once delivery semantics, and Kubernetes-native deployment** while maintaining simplicity and avoiding external dependencies on complex Kafka frameworks.

---

## 1. Core Architectural Principles

### 1.1 Layered Architecture

The architecture follows a strict layered approach to ensure separation of concerns:

```
┌─────────────────────────────────────────────┐
│         Application Layer                   │  Business logic
│    (Services, Handlers, Controllers)        │
└────────────────┬────────────────────────────┘
                 │
┌────────────────▼────────────────────────────┐
│       Kafka Client Library Layer            │  Abstraction & Orchestration
│  (Producer/Consumer pools, Health, Config) │
└────────────────┬────────────────────────────┘
                 │
┌────────────────▼────────────────────────────┐
│      Infrastructure Layer                    │  Low-level Kafka ops
│       (segmentio/kafka-go)                  │
└─────────────────────────────────────────────┘
```

**Benefits**:
- **Testability**: Each layer can be mocked and tested independently
- **Flexibility**: Can swap infrastructure layer (e.g., switch from kafka-go to franz-go) without changing business logic
- **Maintainability**: Clear boundaries make the code easier to understand and modify

### 1.2 Interface-First Design

All major components are defined as interfaces first:

```go
type Producer interface {
    Publish(ctx context.Context, msg Message) error
    Close() error
}

type Consumer interface {
    Subscribe(ctx context.Context, topics []string, handler ConsumerHandler) error
    Close() error
}

type OffsetStore interface {
    Store(ctx context.Context, topic string, partition int, offset int64) error
    Fetch(ctx context.Context, topic string, partition int) (int64, error)
}
```

**Benefits**:
- **Mockability**: Easy to create test doubles
- **Extensibility**: Multiple implementations (Kafka, Redis, PostgreSQL) can coexist
- **Dependency Injection**: Follows SOLID principles, especially Dependency Inversion

---

## 2. Exactly-Once Delivery Semantics

### 2.1 The Challenge

Exactly-once delivery is challenging because:
1. **Producer duplicates**: Network issues can cause duplicate messages
2. **Consumer duplicates**: Retries after processing failures can duplicate work
3. **Offset management**: Commits can be lost or applied multiple times

### 2.2 Producer-Side Exactly-Once

**Strategy**: Idempotent Producer with Sequence Numbers

```
┌─────────────────────────────────────────────────────────────┐
│                    Idempotent Producer                       │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐     │
│  │  Producer ID │  │  Sequence #  │  │ Dedup Cache  │     │
│  │  (unique)    │  │ (monotonic)  │  │  (LRU)       │     │
│  └──────────────┘  └──────────────┘  └──────────────┘     │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
                   ┌────────────────────┐
                   │  Kafka Broker      │
                   │  (dedup by ID)     │
                   └────────────────────┘
```

**How it works**:
1. Each producer gets a unique `ProducerID`
2. Messages are tagged with monotonically increasing sequence numbers
3. Broker deduplicates messages with same ProducerID + Sequence
4. Local LRU cache provides quick dedup check before sending to Kafka

**Implementation Details**:
- ProducerID: Generated from hostname + PID + timestamp
- Sequence numbers: `atomic.Int64` for thread safety
- LRU cache: 10,000 entries, 5-minute TTL
- Kafka configuration: `EnableIdempotence: true`

### 2.3 Consumer-Side Exactly-Once

**Strategy**: Two-Phase Commit with External Offset Storage

```
┌──────────────┐
│ Poll Message │
└──────┬───────┘
       │
       ▼
┌─────────────────────────────────────────────────────────────┐
│              Transaction Manager                           │
│  ┌──────────────────────────────────────────────────────┐  │
│  │  BEGIN TRANSACTION                                   │  │
│  │                                                       │  │
│  │  1. Process message (business logic)                  │  │
│  │  2. Update application state (DB)                      │  │
│  │  3. Store offset in offset table                       │  │
│  │  4. Commit transaction                               │  │
│  └──────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────┘
       │
       ▼
┌──────────────┐
│ Commit Kafka │  (only if transaction succeeds)
│    Offset     │
└──────────────┘
```

**Key Components**:

**1. Offset Storage Interface**:
```go
type OffsetStore interface {
    // Store offset within a transaction
    Store(ctx context.Context, tx interface{}, topic string,
          partition int, offset int64) error

    // Fetch offset for resuming after crash
    Fetch(ctx context.Context, topic string, partition int) (int64, error)
}
```

**2. Business-Level Idempotency**:
```go
// Even with offset commits, business logic should be idempotent
func ProcessEvent(event Event) error {
    // Check if event already processed
    if exists, _ := repo.EventProcessed(event.ID); exists {
        return nil // Already processed, safe to skip
    }

    // Process event
    if err := repo.ProcessEvent(event); err != nil {
        return err
    }

    // Mark as processed
    return repo.MarkEventProcessed(event.ID)
}
```

**Why PostgreSQL for offset storage?**
- Transactional guarantees (ACID)
- Already in the tech stack
- Reliable and durable
- Supports complex queries for lag monitoring
- Consistent with application state

### 2.4 Exactly-Once Trade-offs

| Approach | Complexity | Performance | Reliability |
|----------|------------|-------------|-------------|
| At-least-once (current) | Low | High | Good |
| Exactly-once (planned) | High | Medium | Excellent |
| At-most-once | Low | High | Poor |

**Why exactly-once?**
- Business requirement for no duplicates (financial/audit trails)
- Acceptable complexity cost for reliability
- Medium volume (10K-100K msgs/sec) allows transaction overhead

---

## 3. Producer Architecture

### 3.1 Batching Strategy

**Problem**: Sending one message at a time is inefficient (network overhead)

**Solution**: Hybrid Time + Size-based Batching

```
Message Flow:
1. Messages added to buffer
2. Timer starts (100ms timeout)
3. Flush triggers on:
   - Buffer reaches 1000 messages OR
   - Buffer reaches 1MB OR
   - Timer expires (100ms)
4. Batch compressed (Snappy)
5. Batch sent to Kafka
```

**Adaptive Batching**:
```go
// Under high load: prioritize throughput (larger batches)
// Under low load: prioritize latency (smaller batches)
func (b *Batcher) AdjustBatchSize(currentThroughput float64) {
    if currentThroughput > 50000 {
        b.config.MaxMessages = 2000  // Larger batches
    } else {
        b.config.MaxMessages = 1000  // Default
    }
}
```

**Why 100ms timeout?**
- Balance between latency and throughput
- < 100ms perceived as real-time
- 100ms allows enough messages to accumulate for efficiency

### 3.2 Retry Strategy

**Problem**: Transient failures (network, broker unavailable) cause message loss

**Solution**: Exponential Backoff with Error Classification

```
Error Classification:
┌─────────────────────────────────────────────────────────────┐
│                    Error Classifier                         │
│  ┌──────────────────────┐  ┌──────────────────────────┐   │
│  │    Retriable Errors  │  │  Non-Retriable Errors    │   │
│  │  - Network timeout   │  │  - Invalid topic         │   │
│  │  - Broker down       │  │  - Auth failed           │   │
│  │  - Leader election   │  │  - Invalid message       │   │
│  │                       │  │                          │   │
│  │  Retry with backoff   │  │  Fail immediately        │   │
│  └──────────────────────┘  └──────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

**Exponential Backoff Algorithm**:
```
Attempt 1: 0ms   (immediate retry)
Attempt 2: 100ms
Attempt 3: 200ms
Attempt 4: 400ms
Max backoff: 5s
```

**Why exponential backoff?**
- Prevents overwhelming broker during outages
- Gives broker time to recover
- Standard pattern for distributed systems

### 3.3 Connection Pooling

**Problem**: Creating connections is expensive

**Solution**: Producer Pool with Health Checks

```go
type ProducerPool struct {
    pool         chan *kafka.Writer
    maxSize      int           // 5 producers
    idleTimeout  time.Duration // 5min
    healthCheck  time.Duration // 30s
}
```

**Pool Management**:
1. Initialize pool with 5 producers
2. Producers are reused (leased and returned)
3. Idle producers closed after 5 minutes
4. Health checks every 30 seconds
5. Unhealthy producers replaced automatically

---

## 4. Consumer Architecture

### 4.1 Consumer Group Management

**Problem**: Distributing partitions across consumers

**Solution**: Kafka Consumer Groups with Sticky Rebalancing

```
Consumer Group Flow:
┌─────────────────────────────────────────────────────────────┐
│                    Consumer Group                           │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐        │
│  │ Consumer 1  │  │ Consumer 2  │  │ Consumer 3  │        │
│  │  (pod-0)    │  │  (pod-1)    │  │  (pod-2)    │        │
│  └──────┬──────┘  └──────┬──────┘  └──────┬──────┘        │
│         │                │                │                │
│         └────────────────┼────────────────┘                │
│                          │                                 │
│                   ┌──────▼──────┐                           │
│                   │  Rebalance  │  Assign partitions       │
│                   │  Coordinator │  (sticky strategy)       │
│                   └─────────────┘                           │
└─────────────────────────────────────────────────────────────┘
```

**Sticky Rebalancing**:
- Minimizes partition movement during rebalancing
- Partitions stick to consumers when possible
- Better for Kubernetes (stable pod names)
- Reduces processing disruption

**Static Membership** (Kubernetes):
```go
// Use pod name as instance ID
instanceID := os.Getenv("POD_NAME")
consumer := kafka.NewConsumer(kafka.ConsumerConfig{
    GroupID:    "event-consumer",
    InstanceID: instanceID,  // Static membership
})
```

**Benefits**:
- No unnecessary rebalancing during rolling updates
- Faster recovery from failures
- Less network overhead

### 4.2 Message Processing Pipeline

**Problem**: Processing messages reliably while maintaining throughput

**Solution**: Parallel Worker Pool with Error Handling

```
┌─────────────────────────────────────────────────────────────┐
│                 Message Processing Pipeline                 │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐        │
│  │   Poll      │  │  Validate   │  │  Process    │        │
│  │  (batch)    │  │  (schema)   │  │  (parallel) │        │
│  └──────┬──────┘  └──────┬──────┘  └──────┬──────┘        │
│         │                │                │                │
│         │                │                ├────────────────┐
│         │                │                │                │
│         ▼                ▼                ▼                ▼
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌────────┐
│  │  Batch      │  │  Validation │  │   Success   │  │  DLQ   │
│  │  Metrics    │  │    Error    │  │  + Commit   │  │  + Log │
│  └─────────────┘  └─────────────┘  └─────────────┘  └────────┘
└─────────────────────────────────────────────────────────────┘
```

**Worker Pool Implementation**:
```go
type WorkerPool struct {
    workers    chan chan Message
    workerQuit chan bool
    quit       chan bool
    queue      chan Message
    results    chan Result
}

// Fan-out: Distribute messages to workers
// Fan-in: Collect results from workers
func (wp *WorkerPool) Process(messages []Message) []Result {
    // Fan-out
    for _, msg := range messages {
        wp.queue <- msg
    }

    // Fan-in
    results := make([]Result, len(messages))
    for i := 0; i < len(messages); i++ {
        results[i] = <-wp.results
    }

    return results
}
```

**Worker Count Formula**:
```
Workers = CPU cores * 2
```

**Rationale**:
- Optimal for I/O-bound operations (network, disk)
- Prevents thread starvation
- Allows parallel processing without overwhelming system

### 4.3 Dead-Letter Queue (DLQ)

**Problem**: Some messages can't be processed (invalid data, schema errors)

**Solution**: Separate DLQ Topic with Retry Counting

```
Message Flow with DLQ:
┌─────────────┐
│  Consume    │
│   Message   │
└──────┬──────┘
       │
       ▼
┌─────────────────────────────────────────────────────────────┐
│                  Retry Decision Logic                       │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐     │
│  │  Retry < 3  │  │  Retry = 3   │  │  Non-Retriable │     │
│  │              │  │              │  │               │     │
│  │  Queue for   │  │  Send to DLQ │  │  Send to DLQ  │     │
│  │  retry       │  │              │  │  immediately  │     │
│  └──────────────┘  └──────────────┘  └──────────────┘     │
└─────────────────────────────────────────────────────────────┘
```

**DLQ Message Format**:
```json
{
  "original_topic": "user-events",
  "original_partition": 2,
  "original_offset": 123456,
  "original_key": "user-123",
  "error_type": "validation_error",
  "error_message": "Invalid email format",
  "timestamp": "2025-01-18T10:00:00Z",
  "retry_count": 3,
  "processing_duration_ms": 150,
  "original_message": {
    "key": "user-123",
    "value": "{\"email\":\"invalid\"}",
    "headers": {"content-type": "application/json"}
  }
}
```

**DLQ Benefits**:
- Never lose failed messages
- Separate processing for failures
- Rich metadata for debugging
- 7-day retention for investigation

### 4.4 Backpressure Handling

**Problem**: Consumer can be overwhelmed (producer > consumer rate)

**Solution**: Backpressure Control with Metrics

```
Backpressure Detection:
┌─────────────────────────────────────────────────────────────┐
│                    Backpressure Manager                      │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐     │
│  │ In-Flight   │  │  Processing  │  │  Queue Depth │     │
│  │   Metrics    │  │   Latency    │  │    Metrics   │     │
│  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘     │
│         │                 │                 │                │
│         └─────────────────┼─────────────────┘                │
│                           │                                  │
│                   ┌───────▼────────┐                           │
│                   │  Threshold    │  > 80%: Log warning      │
│                   │  Check        │  > 90%: Reduce fetch     │
│                   └────────────────┘                           │
└─────────────────────────────────────────────────────────────┘
```

**Backpressure Actions**:
1. Log warning at 80% capacity
2. Reduce fetch size at 90% capacity
3. Trigger HPA alert at 95% capacity
4. Reject new connections at 99% capacity

---

## 5. Kubernetes-Native Design

### 5.1 Graceful Shutdown

**Problem**: Kubernetes kills pods abruptly, losing in-flight messages

**Solution**: Graceful Shutdown Lifecycle

```
Graceful Shutdown Flow:
┌─────────────────────────────────────────────────────────────┐
│                  Kubernetes Lifecycle                       │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐     │
│  │  SIGTERM     │  │  Pre-Stop    │  │  SIGKILL     │     │
│  │  (received)  │  │  (hook)      │  │  (forced)    │     │
│  │              │  │              │  │              │     │
│  │  Stop new    │  │  Wait 25s    │  │  Force kill │     │
│  │  messages    │  │  for drain   │  │  (last       │     │
│  │              │  │              │  │  resort)     │     │
│  └──────┬───────┘  └──────┬───────┘  └──────────────┘     │
│         │                 │                                │
│         └─────────────────┼────────────────┐               │
│                           │                │               │
│                   ┌───────▼────────┐       │               │
│                   │  App Layer    │       │               │
│                   │  Shutdown     │       │               │
│                   └───────────────┘       │               │
│                           │                │               │
│                   ┌───────▼────────────────▼───────────┐   │
│                   │  1. Stop accepting new messages    │   │
│                   │  2. Process in-flight (20s)        │   │
│                   │  3. Commit final offsets           │   │
│                   │  4. Close connections              │   │
│                   └────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

**Pre-Stop Hook**:
```yaml
lifecycle:
  preStop:
    exec:
      command: ["/bin/sh", "-c", "curl -X POST http://localhost:8080/health/shutdown && sleep 25"]
```

**Why 25 second sleep?**
- Kubernetes default terminationGracePeriodSeconds: 30s
- App has 25s for graceful shutdown
- 5s buffer for system overhead

### 5.2 Health Checks

**Liveness Probe**: Can the pod communicate with Kafka?
```go
func (c *Client) LivenessProbe() error {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    // Check if Kafka is reachable
    if err := c.Ping(ctx); err != nil {
        return fmt.Errorf("kafka not reachable: %w", err)
    }

    // Check if producer/consumer are healthy
    if err := c.producer.Health(); err != nil {
        return fmt.Errorf("producer unhealthy: %w", err)
    }

    return nil
}
```

**Readiness Probe**: Is the consumer ready to process messages?
```go
func (c *Consumer) ReadinessProbe() error {
    // Check if consumer group joined
    if !c.IsJoinedGroup() {
        return fmt.Errorf("consumer not joined group")
    }

    // Check if consumer has partitions assigned
    if !c.HasPartitions() {
        return fmt.Errorf("no partitions assigned")
    }

    // Lag is OK, but warn
    lag := c.GetLag()
    if lag > 10000 {
        log.Warn("Consumer has backlog, but ready")
    }

    return nil
}
```

**Health Check Intervals**:
- Liveness: Every 10s, 30s initial delay
- Readiness: Every 5s, 10s initial delay
- Timeout: 5s for liveness, 3s for readiness

### 5.3 Horizontal Pod Autoscaling (HPA)

**Problem**: Fixed number of consumers can't handle variable load

**Solution**: Metrics-based Autoscaling

```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: kafka-consumer-hpa
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: kafka-consumer
  minReplicas: 3
  maxReplicas: 10
  metrics:
  - type: Pods
    pods:
      metric:
        name: kafka_consumer_lag
      target:
        type: AverageValue
        averageValue: "5000"  # Scale up if lag > 5000 messages
  - type: Pods
    pods:
      metric:
        name: kafka_processing_time_seconds
      target:
        type: AverageValue
        averageValue: "0.5"  # Scale up if processing > 500ms
```

**Scaling Logic**:
- Scale UP: Consumer lag > 5,000 OR processing time > 500ms
- Scale DOWN: Consumer lag < 1,000 AND processing time < 100ms (for 5 minutes)
- Stabilization window: 5 minutes (prevent flapping)

---

## 6. Configuration Management

### 6.1 Configuration Philosophy

**Principles**:
1. **12-Factor App**: Configuration in environment, not code
2. **Validation**: Fail fast on invalid configuration
3. **Sensible Defaults**: Work out of the box, tuneable for production
4. **Documentation**: Every config option has explanation

### 6.2 Configuration Hierarchy

```
Configuration Sources (in priority order):
1. Environment Variables (highest priority)
2. Config File (YAML/JSON)
3. Default Values (lowest priority)

Example:
KAFKA_BROKERS env var > config file > default ["localhost:9092"]
```

### 6.3 Validation Strategy

```go
func (c *Config) Validate() error {
    var errs []error

    // Critical validations (fail fast)
    if len(c.Brokers) == 0 {
        errs = append(errs, errors.New("KAFKA_BROKERS required"))
    }

    // Exactly-once prerequisites
    if c.Producer.EnableIdempotent {
        if c.Producer.ProducerID == "" {
            errs = append(errs, errors.New("PRODUCER_ID required for idempotent producer"))
        }
        if !c.Consumer.AutoCommit {
            if c.Consumer.GroupID == "" {
                errs = append(errs, errors.New("GROUP_ID required for manual commits"))
            }
        }
    }

    // Performance validations (warn, don't fail)
    if c.Producer.MaxInFlight > 10 {
        log.Warn("MaxInFlight > 10 may cause ordering issues")
    }

    if len(errs) > 0 {
        return fmt.Errorf("config validation failed: %w", errors.Join(errs...))
    }

    return nil
}
```

---

## 7. Observability Strategy

### 7.1 Metrics-Driven Development

**Three-Layer Metrics Model**:

```
┌─────────────────────────────────────────────────────────────┐
│                    Metrics Layer                            │
│  ┌──────────────────────────────────────────────────────┐  │
│  │                    RED Method                          │  │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  │  │
│  │  │    Rate     │  │    Errors   │  │  Duration   │  │  │
│  │  │ (requests/s)│  │  (errors/s) │  │    (ms)     │  │  │
│  │  └─────────────┘  └─────────────┘  └─────────────┘  │  │
│  └──────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────┘
```

**Producer Metrics**:
- Rate: `kafka_producer_messages_total` (counter)
- Errors: `kafka_producer_errors_total` (counter)
- Duration: `kafka_produce_latency_seconds` (histogram)

**Consumer Metrics**:
- Rate: `kafka_consumer_messages_total` (counter)
- Errors: `kafka_consumer_errors_total` (counter)
- Duration: `kafka_consume_latency_seconds` (histogram)
- Lag: `kafka_consumer_lag_bytes` (gauge)

### 7.2 Structured Logging

**Why Structured Logging?**
- Machine-parseable (JSON)
- Queryable in log aggregators (ELK, Loki)
- Consistent schema
- Supports log levels

**Log Schema**:
```json
{
  "timestamp": "2025-01-18T10:00:00Z",
  "level": "info",
  "message": "Message published",
  "context": {
    "trace_id": "abc123",
    "span_id": "def456",
    "operation": "kafka.publish",
    "topic": "user-events",
    "partition": 0,
    "offset": 12345,
    "key": "user-123",
    "message_size": 1024,
    "duration_ms": 15.2
  }
}
```

### 7.3 Distributed Tracing

**Trace Propagation Flow**:
```
Producer Side:
┌─────────────────┐
│  HTTP Request   │  trace-id: abc123
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│  Process Event  │  span-id: def456 (parent)
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│  Publish to     │  span-id: ghi789 (child)
│  Kafka          │  Headers: {traceparent: "abc123-def456..."}
└─────────────────┘

Consumer Side:
┌─────────────────┐
│  Consume from   │  Extract traceparent from headers
│  Kafka          │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│  Process Event  │  span-id: jkl012 (child of ghi789)
└─────────────────┘
```

**Benefits**:
- End-to-end request tracing
- Performance bottleneck identification
- Distributed debugging

---

## 8. Error Handling Strategy

### 8.1 Error Taxonomy

```go
type ErrorCategory int

const (
    ErrorRetriable    ErrorCategory = iota // Temporary, retry
    ErrorNonRetriable                      // Permanent, fail
    ErrorFatal                             // Fatal, restart
)

type KafkaError struct {
    Category   ErrorCategory
    Message    string
    Code       int
    Timestamp  time.Time
    Context    map[string]interface{}
}
```

**Error Classification**:

| Error Type | Category | Action |
|------------|----------|--------|
| Network timeout | Retriable | Retry with backoff |
| Broker not available | Retriable | Retry with backoff |
| Invalid topic | Non-Retriable | Fail immediately |
| Invalid message | Non-Retriable | Send to DLQ |
| Configuration error | Fatal | Log and restart |
| Out of memory | Fatal | Log and restart |

### 8.2 Circuit Breaker Pattern

**Problem**: Continuous retries to failing service cause cascading failures

**Solution**: Circuit Breaker

```
Circuit States:
┌─────────────────────────────────────────────────────────────┐
│                    Circuit Breaker                           │
│  ┌──────────────────────────────────────────────────────┐  │
│  │              CLOSED (Normal)                        │  │
│  │  - Allow requests                                    │  │
│  │  - Count failures                                     │  │
│  │  - Trip to OPEN after N failures                      │  │
│  └──────────────────────────────────────────────────────┘  │
│                           │                                  │
│                           ▼ (N failures)                     │
│  ┌──────────────────────────────────────────────────────┐  │
│  │               OPEN (Failed)                           │  │
│  │  - Reject all requests                                 │  │
│  │  - After timeout, go to HALF_OPEN                     │  │
│  │  - Alert triggered                                     │  │
│  └──────────────────────────────────────────────────────┘  │
│                           │                                  │
│                           ▼ (timeout)                       │
│  ┌──────────────────────────────────────────────────────┐  │
│  │             HALF_OPEN (Testing)                        │  │
│  │  - Allow one request                                   │  │
│  │  - Success → CLOSED                                   │  │
│  │  - Failure → OPEN                                     │  │
│  └──────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────┘
```

**Configuration**:
```go
type CircuitBreakerConfig struct {
    MaxFailures    int           // 5 failures before trip
    Timeout        time.Duration // 1 minute in OPEN state
    HalfOpenMax    int           // 1 request in HALF_OPEN
}
```

---

## 9. Security Considerations

### 9.1 TLS/SSL Encryption

**Why TLS?**
- Encrypt data in transit
- Prevent man-in-the-middle attacks
- Regulatory compliance (PCI-DSS, HIPAA, GDPR)

**TLS Configuration**:
```go
type TLSConfig struct {
    Enabled            bool
    CAFile            string   // Certificate authority
    CertFile          string   // Client certificate
    KeyFile           string   // Client private key
    InsecureSkipVerify bool    // ⚠️ Only for development
}
```

### 9.2 SASL Authentication

**SASL Mechanisms**:
- **PLAIN**: Username/password (simple, but not encrypted)
- **SCRAM-SHA-256**: Hashed credentials (recommended)
- **SCRAM-SHA-512**: Stronger hash (most secure)

**Why SCRAM over PLAIN?**
- PLAIN sends credentials in plaintext
- SCRAM uses challenge-response (never sends password)
- Both require TLS for security

---

## 10. Performance Optimization

### 10.1 Batching Optimization

**Trade-off Curve**:
```
Throughput
    │
100K│        ●
    │      ●
50K │    ●
    │  ●
    │●
    └────────────────────────────────
      10ms   100ms   1s
           Latency
```

**Analysis**:
- 10ms batch: High latency, low throughput
- 100ms batch: Balance (chosen)
- 1s batch: Low latency penalty, diminishing returns

### 10.2 Compression Choice

| Compression | Ratio | CPU | Speed | Use Case |
|------------|-------|-----|-------|----------|
| None | 1:1 | Low | Fastest | Development |
| Snappy | 2:1 | Low | Fast | **Default** |
| Gzip | 4:1 | Medium | Medium | Archive |
| LZ4 | 2.5:1 | Low | Very Fast | High throughput |
| Zstd | 3:1 | Medium-High | Medium | Best compression |

**Why Snappy?**
- Good compression ratio (2:1)
- Low CPU overhead
- Fast compression/decompression
- Go native implementation available

### 10.3 Connection Pool Sizing

**Producer Pool Size Formula**:
```
Pool Size = (Throughput * Latency) / Batch Size

Example:
Throughput = 50,000 msg/s
Latency = 0.1s
Batch Size = 1,000 messages

Pool Size = (50,000 * 0.1) / 1,000 = 5 producers
```

---

## 11. Testing Strategy

### 11.1 Testing Pyramid

```
        ┌─────────┐
        │ E2E     │  5% - Integration tests with real Kafka
        │ (10)    │
        ├─────────┤
        │         │
        │ Tests   │  25% - Integration tests with testcontainers
        │ (50)    │
        ├─────────┤
        │         │
        │ Tests   │  70% - Unit tests with mocks
        │ (140)   │
        └─────────┘
```

### 11.2 Testcontainers

**Why Testcontainers?**
- Real Kafka (not a mock)
- Portable (runs in Docker)
- Fast compared to external cluster
- Tests with actual Kafka behavior

**Example**:
```go
func TestKafkaIntegration(t *testing.T) {
    ctx := context.Background()

    // Start Kafka container
    kafkaContainer, err := testcontainers.GenericContainer(ctx, ...)
    defer kafkaContainer.Terminate(ctx)

    // Test with real Kafka
    client := NewClient([]string{kafkaAddress})
    // ... assertions
}
```

### 11.3 Chaos Engineering

**Chaos Scenarios**:
1. **Network partitions**: Consumer can't reach broker
2. **Broker failure**: One broker goes down
3. **Slow broker**: Simulate high latency
4. **Message duplication**: Produce same message twice
5. **Consumer crash**: Kill consumer mid-processing

**Why Chaos Engineering?**
- Proactive failure testing
- Verify graceful degradation
- Test recovery procedures

---

## 12. Design Trade-offs Summary

### 12.1 Library Choice

**Decision**: Stick with `segmentio/kafka-go`

**Pros**:
- Pure Go (no CGo)
- Easy cross-compilation
- Actively maintained
- Sufficient for medium volume

**Cons**:
- Slower than confluent-kafka-go
- Fewer features than Sarama

**Alternatives Considered**:
- `twmb/franz-go`: Better performance, but newer ecosystem
- `confluent-kafka-go`: Best performance, but CGo dependency

### 12.2 Offset Storage

**Decision**: PostgreSQL for offset storage

**Pros**:
- Transactional guarantees
- ACID compliance
- Already in tech stack
- Queryable (lag monitoring)

**Cons**:
- Slower than Redis
- Adds database load

**Alternatives Considered**:
- Kafka offsets: Fast, but no transactional support
- Redis: Faster, but not transactional

### 12.3 Exactly-Once Complexity

**Decision**: Full exactly-once implementation

**Pros**:
- No duplicate messages
- Business requirement met
- Audit trail compliance

**Cons**:
- Higher complexity
- Performance overhead
- Requires external offset storage

**Alternatives Considered**:
- At-least-once: Simpler, but duplicates possible
- At-most-once: Simplest, but data loss possible

---

## 13. Success Metrics

### 13.1 Technical Metrics

| Metric | Target | Why |
|--------|--------|-----|
| P99 Latency | < 100ms | User perception |
| Throughput | 20,000 msg/s | Business requirement |
| Uptime | 99.9% | SLA |
| CPU Usage | < 70% | Headroom for spikes |
| Memory Usage | < 512MB | Resource efficiency |
| Consumer Lag | < 10,000 | Real-time processing |

### 13.2 Business Metrics

| Metric | Target | Why |
|--------|--------|-----|
| Message Loss | 0 messages | Data integrity |
| Duplicate Processing | < 0.1% | Exactly-once |
| Recovery Time | < 5min | MTTR |
| Onboarding Time | < 1 day | Developer productivity |

---

## 14. Conclusion

This architecture balances **reliability, performance, and maintainability** while avoiding external framework complexity. Key strengths:

1. **Production-Ready**: Exactly-once delivery, DLQ, graceful shutdown
2. **Kubernetes-Native**: Health checks, autoscaling, lifecycle management
3. **Observable**: Metrics, logs, traces at every layer
4. **Testable**: Interface-first design, comprehensive testing strategy
5. **Secure**: TLS, SASL, error classification

The architecture is **ready for implementation** with a clear 9-week roadmap and well-defined success criteria.

---

## References

1. **Kafka Documentation**: https://kafka.apache.org/documentation/
2. **Exactly-Once Semantics**: https://www.confluent.io/blog/exactly-once-semantics-are-possible-heres-how-apache-kafka-does-it/
3. **Kafka Best Practices**: https://www.confluent.io/blog/kafka-client-best-practices/
4. **Kubernetes Patterns**: https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle/
5. **Circuit Breaker Pattern**: https://martinfowler.com/bliki/CircuitBreaker.html
6. **Testcontainers**: https://golang.testcontainers.org/
