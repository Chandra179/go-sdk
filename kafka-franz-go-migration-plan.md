# Kafka Migration: segmentio/kafka-go → twmb/franz-go Implementation Plan

**Goal:** Completely rewrite the Kafka implementation in `pkg/kafka/` to use `github.com/twmb/franz-go` instead of `github.com/segmentio/kafka-go`, maintaining the same public interface where possible while leveraging franz-go's superior performance and features.

**Architecture:** franz-go uses a unified client model (one client produces AND consumes), functional options pattern for configuration, and handles connection pooling internally. We'll adapt our wrapper types to work with franz-go's `kgo.Client` and `kgo.Record` types while keeping our public `Message`, `Producer`, and `Consumer` interfaces stable.

**Tech Stack:**
- franz-go (github.com/twmb/franz-go/pkg/kgo) - Core Kafka client
- kadm (github.com/twmb/franz-go/pkg/kadm) - Admin operations
- kmsg (github.com/twmb/franz-go/pkg/kmsg) - Low-level messages (indirect)
- OpenTelemetry - Metrics integration (unchanged)

---

## Pre-Migration Setup

### Task 1: Update go.mod Dependencies

**Files:**
- Modify: `go.mod`
- Modify: `go.sum` (auto-generated)

**Actions:**
1. Remove from require section: `github.com/segmentio/kafka-go v0.4.49`
2. Add to require section: `github.com/twmb/franz-go v1.18.0` and `github.com/twmb/franz-go/pkg/kadm v1.14.0`
3. Run: `cd /home/mando/Work/go-sdk && go mod tidy`

**Commit:** `deps: replace segmentio/kafka-go with twmb/franz-go`

---

## Core Types and Configuration

### Task 2: Update types.go - Message Conversion Helpers

**Files:**
- Modify: `pkg/kafka/types.go`

**Actions:**
1. Add franz-go imports
2. Add `toRecord()` method to convert our `Message` to `kgo.Record`
3. Add `fromRecord()` function to convert `kgo.Record` back to our `Message`

**Commit:** `feat(kafka): add Message to kgo.Record conversion helpers`

---

## Security and TLS Configuration

### Task 3: Rewrite security.go for franz-go

**Files:**
- Modify: `pkg/kafka/security.go`

**Actions:**
1. Replace segmentio `Dialer` with franz-go TLS configuration via `kgo.Dialer()`
2. Keep `SecurityConfig` struct (config compatibility)
3. Create `CreateTLSConfig()` and `CreateDialerOptions()` functions

**Commit:** `feat(kafka): rewrite security.go for franz-go TLS support`

---

## Core Client Implementation

### Task 4: Rewrite client.go - Unified franz-go Client

**Files:**
- Modify: `pkg/kafka/client.go`

**Key Changes:**
1. Replace separate `Writer`/`Reader` with single `*kgo.Client`
2. Remove manual connection pooling (franz-go handles internally)
3. Keep `KafkaClient` struct wrapping `kgo.Client`
4. Maintain `Producer()` and `Consumer()` methods returning our interfaces
5. Update `Ping()` and `PingDetailed()` for franz-go metadata API
6. Keep `Config` struct for backward compatibility
7. Add `UnderlyingClient()` for advanced use cases

**Commit:** `feat(kafka): rewrite client.go with unified franz-go client`

---

## Producer Implementation

### Task 5: Rewrite producer.go for franz-go

**Files:**
- Modify: `pkg/kafka/producer.go`

**Key Changes:**
1. Replace `kafka-go.Writer` with `*kgo.Client` (passed from KafkaClient)
2. Use `client.ProduceSync()` and `client.Produce()` for publishing
3. Map compression types: `kgo.GzipCompression`, `kgo.SnappyCompression`, etc.
4. Map acks: `kgo.AllISRAcks()`, `kgo.LeaderAck()`, `kgo.NoAck()`
5. Keep idempotency/deduplication logic (same behavior)
6. Maintain `Publish()` and `PublishWithSchema()` signatures

**Commit:** `feat(kafka): rewrite producer.go for franz-go Produce API`

---

## Consumer Implementation

### Task 6: Rewrite consumer.go for franz-go

**Files:**
- Modify: `pkg/kafka/consumer.go`

**Key Changes:**
1. Replace `kafka-go.Reader` with `*kgo.Client.PollFetches()` loop
2. Keep `Start()` method signature (consumer handler pattern)
3. Process `kgo.Fetch` records using `RecordIter()` or callbacks
4. Handle offset commits via `client.CommitUncommittedOffsets()`
5. Maintain retry/DLQ header propagation logic
6. Keep retry count extraction and backoff logic

**Commit:** `feat(kafka): rewrite consumer.go for franz-go PollFetches API`

---

## Admin Operations

### Task 7: Rewrite admin.go using franz-go's kadm package

**Files:**
- Modify: `pkg/kafka/admin.go`

**Key Changes:**
1. Use `kadm.NewClient()` instead of raw `kafka-go` connections
2. Replace `conn.CreateTopics()` with `admClient.CreateTopics()`
3. Replace `conn.ReadPartitions()` with `admClient.ListTopics()`
4. Keep `TopicExists()`, `ValidateTopicsExist()`, `CreateTopic()`, `DeleteTopic()` methods
5. Add retry logic using `retry-go` (same pattern)

**Commit:** `feat(kafka): rewrite admin.go using franz-go kadm package`

---

## Deprecated Components

### Task 8: Mark pool.go as deprecated (franz-go handles pooling)

**Files:**
- Modify: `pkg/kafka/pool.go`

**Actions:**
1. Add deprecation notice comment
2. Make all functions return no-ops (backward compatibility only)
3. Connection pooling now handled internally by franz-go

**Commit:** `feat(kafka): deprecate pool.go - franz-go handles connection pooling internally`

---

## Transaction Support

### Task 9: Mark transaction.go as deprecated/simplified

**Files:**
- Modify: `pkg/kafka/transaction.go`

**Actions:**
1. Add deprecation notice
2. Simplify wrapper (franz-go has native transaction support via `kgo.TransactionalID()`)
3. Keep basic API for backward compatibility
4. Add note: for full EOS, configure `kgo.Client` with transactional ID at creation

**Commit:** `feat(kafka): deprecate transaction.go - franz-go has native transaction support`

---

## Unchanged Files

### Task 10: Verify unchanged files compile correctly

**Files that remain unchanged (no Kafka imports):**
- `pkg/kafka/circuitbreaker.go` - Generic pattern, no Kafka imports
- `pkg/kafka/dlq.go` - Uses Message type only
- `pkg/kafka/retry.go` - Uses Message type only
- `pkg/kafka/schema.go` - Uses Schema Registry, not Kafka client
- `pkg/kafka/health.go` - Uses Client interface only
- `pkg/kafka/metrics.go` - Uses Client interface only

**Actions:**
1. Run: `cd /home/mando/Work/go-sdk && go build ./pkg/kafka/...`
2. Fix any compilation issues

---

## Testing

### Task 11: Update integration tests

**Files:**
- Modify: `pkg/kafka/integration_test.go`
- Modify: `pkg/kafka/producer_integration_test.go`
- Modify: `pkg/kafka/consumer_integration_test.go`
- Modify: `pkg/kafka/admin_integration_test.go`
- Modify: `pkg/kafka/retry_dlq_integration_test.go`

**Actions:**
1. Update test imports (remove segmentio, may add franz-go for assertions)
2. Update any direct Kafka client calls to use franz-go equivalents
3. Run: `cd /home/mando/Work/go-sdk && go test ./pkg/kafka/... -v -run Integration`

**Commit:** `test(kafka): update integration tests for franz-go`

---

## Examples Update

### Task 12: Update example files

**Files:**
- Modify: `examples/kafka/producer/producer.go`
- Modify: `examples/kafka/consumer/consumer.go`

**Actions:**
1. Verify examples compile unchanged (public interface maintained)
2. Run: `cd /home/mando/Work/go-sdk && go build ./examples/kafka/...`

**Commit:** `chore(examples): verify kafka examples work with franz-go`

---

## Bootstrap Update

### Task 13: Update bootstrap/kafka.go

**Files:**
- Modify: `internal/app/bootstrap/kafka.go`

**Actions:**
1. Verify no changes needed (same interface maintained)
2. Run: `cd /home/mando/Work/go-sdk && go build ./internal/app/bootstrap/...`

**Commit:** `chore(bootstrap): verify kafka bootstrap works with franz-go`

---

## Final Verification

### Task 14: Full build and test

**Actions:**
1. Run: `cd /home/mando/Work/go-sdk && go build ./...`
2. Run: `cd /home/mando/Work/go-sdk && go test ./pkg/kafka/... -v`
3. Verify no segmentio references: `grep -r "segmentio/kafka-go" --include="*.go" .`
4. Final commit: `feat(kafka): complete migration from segmentio/kafka-go to twmb/franz-go`

---

## Key Architectural Differences

| Aspect | segmentio/kafka-go | twmb/franz-go |
|--------|-------------------|---------------|
| Client Model | Separate Writer/Reader | Unified kgo.Client |
| Configuration | Struct-based | Functional options |
| Connection Pooling | Manual (ConnectionPool) | Automatic (built-in) |
| Admin API | Raw connections | kadm package |
| Transactions | Manual implementation | Native support |
| Compression | `kafka-go/compress` | Built-in constants |
| Acks | `RequireAll`, `RequireOne` | `AllISRAcks()`, `LeaderAck()` |

---

## Expected Benefits

1. **Performance**: 2-6x faster consuming, 2.4x faster producing (per franz-go benchmarks)
2. **Simplicity**: No manual connection management
3. **Features**: Support for all KIPs from Kafka 0.8.0 through 4.1+
4. **Maintenance**: Active development, comprehensive documentation
5. **Transactions**: Native EOS support without custom implementation

---

## Breaking Changes

**None for public interfaces.** All existing code using `pkg/kafka` should work with minimal or no changes:
- `kafka.Client` interface maintained
- `kafka.Message` struct maintained
- `kafka.Producer.Publish()` signature unchanged
- `kafka.Consumer.Start()` signature unchanged
- `kafka.Config` struct maintained

**Deprecated (but functional):**
- `kafka.ConnectionPool` - franz-go handles internally
- `kafka.ConnectionManager` - franz-go handles internally
- `kafka.KafkaTransaction` wrapper - use native franz-go transactions

---

## Migration Commands (for execution)

```bash
cd /home/mando/Work/go-sdk

# Update dependencies
go mod edit -droprequire=github.com/segmentio/kafka-go
go get github.com/twmb/franz-go@v1.18.0
go get github.com/twmb/franz-go/pkg/kadm@v1.14.0
go mod tidy

# Build and test
go build ./pkg/kafka/...
go test ./pkg/kafka/...
go build ./...
```

---

## Files Summary

| File | Status | Changes |
|------|--------|---------|
| `types.go` | Modify | Add conversion helpers |
| `security.go` | Rewrite | TLS via franz-go dialer |
| `client.go` | Rewrite | Unified kgo.Client |
| `producer.go` | Rewrite | Produce via kgo.Client |
| `consumer.go` | Rewrite | PollFetches loop |
| `admin.go` | Rewrite | Use kadm package |
| `pool.go` | Deprecate | No-ops for compatibility |
| `transaction.go` | Deprecate | Simplified wrapper |
| `circuitbreaker.go` | Unchanged | No Kafka imports |
| `dlq.go` | Unchanged | Uses Message type |
| `retry.go` | Unchanged | Uses Message type |
| `schema.go` | Unchanged | Schema Registry only |
| `health.go` | Unchanged | Uses Client interface |
| `metrics.go` | Unchanged | Uses Client interface |
| `go.mod` | Update dependencies | Replace segmentio with franz-go |
