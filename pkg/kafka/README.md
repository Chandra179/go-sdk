# Kafka Package

This package provides a simple interface for working with Kafka in Go applications.

## Usage

### Initialization

```go
import "gosdk/pkg/kafka"

client := kafka.NewClient([]string{"localhost:9092"})
defer client.Close()
```

### Publishing Messages

```go
producer, err := client.Producer()
if err != nil {
    log.Fatal(err)
}

message := kafka.Message{
    Topic: "test-topic",
    Key:   []byte("key"),
    Value: []byte("value"),
}

err = producer.Publish(context.Background(), message)
```

### Consuming Messages

```go
consumer, err := client.Consumer("my-group-id")
if err != nil {
    log.Fatal(err)
}

handler := func(msg kafka.Message) error {
    fmt.Printf("Received: %s\n", msg.Value)
    return nil
}

err = consumer.Subscribe(context.Background(), []string{"test-topic"}, handler)
```

## Components

- **Client**: Main entry point for creating producers and consumers
- **Producer**: Publishes messages to Kafka topics
- **Consumer**: Subscribes to and consumes messages from Kafka topics

## Configuration

Kafka configuration is loaded from environment variables:

- `KAFKA_BROKERS`: Comma-separated list of Kafka broker addresses (default: `localhost:9092`)
