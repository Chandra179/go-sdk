# Go Template Project

This project is a Go template demonstrating reusable packages and runnable example services.

## API Documentation (Swagger)

Once the application is running, access the Swagger UI at:
```
http://localhost:8080/swagger/index.html
```

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
├── sqlc.yaml                          # sqlc configuration for type-safe SQL
├── db/                                # Database migrations and queries
│   ├── migrations/                    # SQL migration files
│   └── queries/                       # SQL query files for sqlc
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

## OpenTelemetry

This project implements a comprehensive observability stack using OpenTelemetry (OTEL) for distributed tracing, metrics, and logging.

![Otel architecture](img/otel_arch.png)

1. **Metrics**: Application → OTLP Receiver → Batch Processor → Prometheus Exporter → Prometheus (via remote_write)
2. **Docker Logs**: Application (Zerolog JSON) → Docker stdout → Alloy Docker Log Scraper → Loki Process → Loki Write → Loki

## Database & SQL Code Generation (sqlc)

This project uses **[sqlc](https://docs.sqlc.dev/)** for type-safe SQL code generation. sqlc generates Go code from SQL queries, providing compile-time safety and eliminating the need for ORMs.
