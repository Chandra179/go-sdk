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
│   ├── kafka/                         # Kafka client and helpers with OTEL metrics
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
