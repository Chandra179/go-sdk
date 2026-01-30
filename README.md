# Go Template Project

This project is a Go template demonstrating reusable packages and runnable example services.

## Project Structure
```
├── cmd/                               # Runnable applications
│   ├── myapp/                         # Example application
│   │   └── main.go                    # Entry point
│   └── examples/                      # Example services
│       ├── consumer/                  # Kafka consumer example
│       └── producer/                  # Kafka producer example
│
├── internal/                          # Internal services (not importable externally)
│   ├── app/                           # Application initialization
│   │   ├── bootstrap/                 # Component initialization (DB, cache, OAuth2, Kafka, OTEL)
│   │   ├── health/                    # Health check endpoints
│   │   ├── middleware/                # HTTP middleware (auth, logging, request ID, CORS)
│   │   ├── routes/                    # HTTP route setup
│   │   ├── server.go                  # Server setup, lifecycle management
│   │   └── README.md                  # App architecture documentation
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
├── examples/                          # Standalone example implementations
│   └── kafka/                         # Kafka usage examples
│       ├── consumer/                  # Consumer example code
│       └── producer/                  # Producer example code
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
├── otel/                              # Monitoring & tracing configs
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
├── Dockerfile.examples                # Example services container
├── add-secrets.sh                     # Secrets management helper
├── test.http                          # API testing file
└── .env.example                       # Environment variables template
```

## Key Points
1. **`cmd/` folder**  
   - Each subdirectory represents a **separate runnable service or example**.  

2. **`pkg/` folder**  
   - Contains **reusable packages** for core functionality.

3. **`examples/` folder**
   - Contains **standalone example implementations** demonstrating library usage.

## OpenTelemetry Architecture

This project implements a comprehensive observability stack using OpenTelemetry (OTEL) for distributed tracing, metrics, and logging.

![Otel architecture](img/otel_arch.png)

### Data Flow

1. **Metrics**: Application → OTLP Receiver → Batch Processor → Prometheus Exporter → Prometheus (via remote_write)
2. **Docker Logs**: Application (Zerolog JSON) → Docker stdout → Alloy Docker Log Scraper → Loki Process → Loki Write → Loki

### Components

#### 1. Application Instrumentation (`internal/app/bootstrap/otel.go`)
- **Meter Provider**: Exports metrics via OTLP/gRPC to Alloy; also exposes Prometheus endpoint for scraping
- **Tracer Provider**: Sends traces via OTLP/gRPC (Note: Traces are currently dropped by Alloy - no exporter configured)
- **Propagator**: W3C Trace Context and Baggage propagation
- **Sampling**: Configurable trace sampling ratio (default: 1.0)
- **Logs**: Handled via Docker log scraping to Loki, not OTLP (no OTLP log exporter in code)

#### 2. Grafana Alloy (`otel/config.alloy`)
- **OTLP Receiver**: Accepts metrics on ports 4317 (gRPC) and 4318 (HTTP)
- **Batch Processor**: Batches metrics for efficient export
- **Prometheus Exporter**: Forwards metrics to Prometheus remote write endpoint
- **Docker Log Scraper**: Scrapes container logs from Docker socket
- **Loki Process**: Parses and adds labels to Docker logs
- **Loki Write**: Sends Docker logs to Loki

#### 3. Metrics Collection

**Custom Application Metrics** (via OTEL SDK):
- HTTP request latency, error rates
- Kafka producer/consumer metrics
- Business-specific counters and gauges

**Kafka Package Metrics** (`pkg/kafka/otel_metrics.go`):
- `kafka.producer.messages_sent` - Total messages sent
- `kafka.producer.send_errors` - Send error count
- `kafka.producer.send_latency` - Send latency histogram
- `kafka.consumer.messages_processed` - Messages processed
- `kafka.consumer.processing_errors` - Processing errors
- `kafka.consumer.lag` - Consumer lag by topic/partition
- `kafka.consumer.rebalance_events` - Rebalance events
- `kafka.dlq.messages_sent` - DLQ routing count
- `kafka.retry.messages_sent` - Retry routing count