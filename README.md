# Go Template Project

This project is a Go template demonstrating reusable packages and runnable example services.

## API (Swagger)
```
http://localhost:8080/swagger/index.html
```

## Project Structure
```
├── cmd/                               # Runnable applications
│   └── myapp/                         # Main application
│       └── main.go                    # Entry point
│
├── internal/                          # Internal services (not importable externally)
│   ├── app/                           # Application initialization
│   │   ├── bootstrap/                 # Component initialization (DB, cache, OAuth2, Kafka, OTEL)
│   │   │   ├── cache.go               # Redis cache initialization
│   │   │   ├── database.go            # Database connection setup
│   │   │   ├── migrations.go          # Database migrations
│   │   │   ├── oauth2.go              # OAuth2 provider setup
│   │   │   └── otel.go                # OpenTelemetry setup
│   │   ├── health/                    # Health check endpoints
│   │   │   └── checker.go             # Health check implementation
│   │   ├── middleware/                # HTTP middleware (auth, logging, request ID, CORS)
│   │   │   ├── auth.go                # Authentication middleware
│   │   │   ├── cors.go                # CORS middleware
│   │   │   ├── logging.go             # Request logging middleware
│   │   │   └── request_id.go          # Request ID middleware
│   │   ├── routes/                    # HTTP route setup
│   │   │   ├── auth.go                # Auth routes
│   │   │   └── infra.go               # Infrastructure routes (health, metrics)
│   │   ├── server.go                  # Server setup, lifecycle management
│   │   ├── provider.go                # Dependency injection provider
│   │   └── README.md                  # App architecture documentation
│   ├── db/                           # Database layer
│   │   ├── gen/                      # sqlc generated code
│   │   │   ├── db.go                 # Database interface
│   │   │   ├── models.go             # Data models
│   │   │   ├── querier.go            # Query interface
│   │   │   └── users.sql.go          # Generated user queries
│   │   ├── migrations/               # SQL migration files
│   │   │   ├── 000001_users_and_oidc.up.sql
│   │   │   └── 000001_users_and_oidc.down.sql
│   │   ├── queries/                  # SQL query files for sqlc
│   │   │   └── users.sql
│   │   └── README.md                 # Database documentation
│   ├── service/                     # Domain services
│   │   ├── auth/                     # Authentication service
│   │   │   ├── handler.go            # HTTP handlers (Gin)
│   │   │   ├── repository.go         # Data access layer
│   │   │   ├── service.go            # Business logic
│   │   │   └── types.go              # DTOs and models
│   │   └── session/                  # Session service
│   │       ├── client.go             # Session client interface
│   │       ├── redis_session.go      # Redis session implementation
│   │       └── redis_session_test.go # Session tests
│   └── cfg/                          # Centralized config loading
│       ├── config.go                 # Main config loader
│       ├── http.go                   # HTTP server config
│       ├── kafka.go                  # Kafka config
│       ├── kafka.yaml                # Kafka config YAML
│       ├── loader.go                 # Config file loading
│       ├── oauth.go                  # OAuth2 config
│       ├── otel.go                   # OpenTelemetry config
│       ├── postgres.go               # PostgreSQL config
│       ├── rabbitmq.go               # RabbitMQ config
│       ├── rabbitmq.yaml             # RabbitMQ config YAML
│       └── redis.go                  # Redis config
│
├── api/                               # API specifications
│   ├── docs.go                        # Swagger documentation
│   ├── swagger.yaml                   # OpenAPI spec
│   └── swagger.json                   # OpenAPI spec (JSON)
│
├── otel/                              # Monitoring & tracing configs
│   ├── config.alloy                   # Alloy OTel config
│   ├── loki.yaml                      # Loki logging config
│   ├── prometheus.yml                 # Prometheus metrics config
│   └── grafana-datasources.yaml      # Grafana datasources
│
├── scripts/                           # Utility scripts
│   └── install.sh                     # Installation script
│
├── img/                               # Documentation images
├── Makefile                           # Build commands
├── sqlc.yaml                          # sqlc configuration for type-safe SQL
├── docker-compose.yml                 # Local services
├── Dockerfile                         # Container image
├── api.http                          # API testing file
├── .env.example                       # Environment variables template
└── .golangci.yml                      # GolangCI Linter config
```

## OpenTelemetry

This project implements a comprehensive observability stack using OpenTelemetry (OTEL) for distributed tracing, metrics, and logging.

![Otel architecture](img/otel_arch.png)

1. **Metrics**: Application → OTLP Receiver → Batch Processor → Prometheus Exporter → Prometheus (via remote_write)
2. **Docker Logs**: Application (Zerolog JSON) → Docker stdout → Alloy Docker Log Scraper → Loki Process → Loki Write → Loki




## Database & SQL Code Generation (sqlc)

This project uses **[sqlc](https://docs.sqlc.dev/)** for type-safe SQL code generation. sqlc generates Go code from SQL queries, providing compile-time safety and eliminating the need for ORMs.

### Generating Code

After modifying SQL queries or schema, regenerate Go code:

```bash
sqlc generate
```




## Kafka Architecture

The project provides a robust Kafka client implementation using [franz-go](https://github.com/twmb/franz-go) with OpenTelemetry integration for observability.

![Kafka architecture](img/kafka_arch.png)





## RabbitMQ Architecture

The project provides a robust RabbitMQ client AMQP implementation with automatic reconnection, channel pooling, and Dead Letter Exchange (DLX) support for reliable message processing.






### Data Flow

1. **Publishing**: Application → Producer → Publisher Channel Pool → RabbitMQ Exchange
2. **Consuming**: RabbitMQ Queue → Consumer Channel Pool → Message Handler → ACK/NACK
3. **Error Handling**: Failed messages → Retry Queue (with TTL) → Main Queue → Parking Lot

For detailed usage and configuration, see [pkg/rabbitmq/README.md](pkg/rabbitmq/README.md).