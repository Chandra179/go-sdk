# Go Template Project

This project is a Go template demonstrating reusable packages and runnable example services.

## Project Structure
```
├── cmd/                               # Runnable applications
│   └── myapp/                         # Example application
│       └── main.go                    # Entry point
│
├── internal/                          # Internal services (not importable externally)
│   ├── app/                           # Application initialization
│   │   ├── bootstrap/                # Component initialization (DB, cache, OAuth2, Kafka, OTEL)
│   │   ├── health/                   # Health check endpoints
│   │   ├── middleware/               # HTTP middleware (auth, logging, request ID, CORS)
│   │   ├── routes/                   # HTTP route setup
│   │   ├── server.go                 # Server setup, lifecycle management
│   │   └── README.md                 # App architecture documentation
│   └── service/                       # Domain services
│       ├── auth/                      # Authentication service
│       │   ├── handler.go             # HTTP handlers (Gin)
│       │   ├── service.go             # Business logic
│       │   └── types.go               # DTOs and models
│       └── session/                   # Session service
│
├── pkg/                               # Reusable library packages
│   ├── cache/                         # Cache interfaces, Redis helpers
│   ├── db/                            # Database connectors, helpers
│   ├── idgen/                         # ID generation utilities
│   ├── kafka/                         # Kafka client and helpers with OpenTelemetry metrics
│   ├── logger/                        # Zerolog wrapper & helpers
│   ├── oauth2/                        # OAuth2 manager & token helpers
│   ├── passkey/                       # Passkey/WebAuthn utilities
│
├── db/                                # Database-related files
│   └── migrations/                    # SQL migration files
│
├── api/                               # API specifications
│   ├── docs.go                        # Swagger documentation
│   ├── swagger.yaml                   # OpenAPI spec
│   └── swagger.json                   # OpenAPI spec (JSON)
│
├── cfg/                               # Centralized config loading
│   └── config.go
│
├── otel/                     # Monitoring & tracing configs
│   ├── config.alloy                   # Alloy OTel config
│   ├── loki.yaml                      # Loki logging config
│   ├── prometheus.yml                 # Prometheus metrics config
│   └── grafana-datasources.yaml       # Grafana datasources
│
├── docs/                              # Documentation
├── AGENTS.md                          # Agent coding guidelines
├── Makefile                           # Build commands
├── docker-compose.yml                 # Local services
├── Dockerfile                         # Container image
├── add-secrets.sh                     # Secrets management helper
├── test.http                          # API testing file
└── .env.example                       # Environment variables template
```

## Key Points
1. **`cmd/` folder**  
   - Each subdirectory represents a **separate runnable service or example**.  

2. **`pkg/` folder**  
   - Contains **reusable packages** for core functionality.

## OpenTelemetry Metrics

This project includes OpenTelemetry metrics integration for the Kafka package. The metrics are automatically exposed at the `/metrics` endpoint and include:

### Available Kafka Metrics

#### Producer Metrics
- `kafka.producer.messages_sent`: Total number of messages sent to Kafka (counter)
  - Attributes: `topic`, `compression`
- `kafka.producer.send_errors`: Total number of send errors (counter)
  - Attributes: `topic`, `error_type`
- `kafka.producer.send_latency`: Time to send messages to Kafka (histogram)
  - Attributes: `topic`
  - Unit: seconds

#### Consumer Metrics
- `kafka.consumer.messages_processed`: Total number of messages processed (counter)
  - Attributes: `topic`, `group_id`
- `kafka.consumer.processing_errors`: Total number of processing errors (counter)
  - Attributes: `topic`, `error_type`
- `kafka.consumer.lag`: Consumer lag by topic and partition (gauge)
  - Attributes: `topic`, `partition`, `group_id`
- `kafka.consumer.rebalance_events`: Total number of consumer group rebalance events (counter)
  - Attributes: `group_id`, `event_type`

#### DLQ/Retry Metrics
- `kafka.dlq.messages_sent`: Total number of messages sent to DLQ (counter)
  - Attributes: `original_topic`, `dlq_topic`
- `kafka.retry.messages_sent`: Total number of messages sent to retry topic (counter)
  - Attributes: `original_topic`, `retry_topic`

### Metrics Access
Metrics are available in Prometheus exposition format at:
```
GET /metrics
```

### Configuration

#### Automatic Initialization
Metrics are automatically initialized when the Kafka package is used. The OpenTelemetry setup in `internal/app/bootstrap/otel.go` configures:
- Prometheus exporter for metrics exposition
- OTLP exporter for telemetry pipelines
- Resource attributes for service identification

#### Environment Variables
Key environment variables for metrics configuration:
```bash
# OpenTelemetry configuration
OTEL_SERVICE_NAME=myapp
OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317
OTEL_SAMPLER_RATIO=1.0

# Kafka configuration (for metric attributes)
KAFKA_BROKERS=localhost:9092
KAFKA_CLIENT_ID=myapp-producer
```

#### Migration Notes
This project has migrated from direct Prometheus client usage to OpenTelemetry metrics:
- Old metrics are deprecated and available for backward compatibility only
- New OTEL metrics provide richer context with attributes
- Metrics are now automatically registered during OTEL initialization
- Use `kafka.InitOtelMetrics()` to manually initialize if needed

### Usage Examples

#### Basic Producer with Metrics
```go
import (
    "gosdk/pkg/kafka"
    "go.opentelemetry.io/otel"
)

// Metrics are automatically recorded
client, _ := kafka.NewClient(config, logger)
producer, _ := client.Producer()

err := producer.Publish(ctx, kafka.Message{
    Topic: "events",
    Key:   []byte("user-123"),
    Value: []byte("event-data"),
})
// Metrics: kafka.producer.messages_sent incremented
// On error: kafka.producer.send_errors incremented
// Latency: kafka.producer.send_latency recorded
```

#### Consumer with Metrics
```go
handler := func(msg kafka.Message) error {
    // Process message
    err := processEvent(msg)
    if err != nil {
        // Metrics: kafka.consumer.processing_errors incremented
        return err
    }
    // Metrics: kafka.consumer.messages_processed incremented
    return nil
}

kafka.StartConsumer(ctx, client, logger, "my-group", 
    []string{"events"}, handler, retryConfig)
// Consumer lag and rebalance events tracked automatically
```

#### Manual Metrics Recording
```go
import "gosdk/pkg/kafka"

// Record custom metrics
kafka.RecordProducerMessageSent(ctx, "events", "gzip")
kafka.RecordConsumerLag(ctx, "events", 0, "my-group", 1000)
```

### Integration with Monitoring Systems

#### Prometheus Configuration
Add to your `prometheus.yml`:
```yaml
scrape_configs:
  - job_name: 'go-app'
    static_configs:
      - targets: ['localhost:8080']
    metrics_path: '/metrics'
    scrape_interval: 15s
```

#### Grafana Dashboard
Recommended Grafana panels for Kafka monitoring:
1. **Producer Throughput**: Rate of `kafka.producer.messages_sent`
2. **Producer Error Rate**: Rate of `kafka.producer.send_errors`
3. **Consumer Lag**: Current value of `kafka.consumer.lag`
4. **Consumer Processing Rate**: Rate of `kafka.consumer.messages_processed`
5. **DLQ Message Rate**: Rate of `kafka.dlq.messages_sent`

For detailed usage instructions and architecture information, see `pkg/kafka/README.md`.
