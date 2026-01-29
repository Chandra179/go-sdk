# Kafka Examples

This directory contains production-ready examples for producing and consuming messages with Kafka using the Go SDK's `pkg/kafka/` implementation.

## Running the Examples

### 1. Producer Example

The producer publishes 10 order events to the configured topic:

```bash
# Navigate to producer directory
cd internal/examples/kafka/producer

# Run with default settings
go run main.go

# Or with custom settings
KAFKA_BROKERS=localhost:9092,localhost:9093 KAFKA_ORDERS_TOPIC=orders go run main.go
```

**Expected Output:**
```
Published order order-1 (amount: $10.50)
Published order order-2 (amount: $21.00)
Published order order-3 (amount: $31.50)
...
```

### 2. Consumer Example

The consumer listens for order events on the configured topic:

```bash
# Navigate to consumer directory  
cd internal/examples/kafka/consumer

# Run with default settings
go run main.go

# Or with custom settings
KAFKA_BROKERS=localhost:9092 KAFKA_ORDERS_TOPIC=orders go run main.go
```

**Expected Output:**
```
Starting consumer for topic: orders
Processing order: ID=order-1, UserID=user-1, Amount=$10.50, Status=created
Processing order: ID=order-2, UserID=user-2, Amount=$21.00, Status=created
Processing order: ID=order-3, UserID=user-3, Amount=$31.50, Status=created
...
```
