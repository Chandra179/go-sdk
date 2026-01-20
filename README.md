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
│   │   └── server.go                  # Server setup, dependency injection
│   └── service/                       # Domain services
│       ├── auth/                      # Authentication service
│       │   ├── handler.go             # HTTP handlers (Gin)
│       │   ├── service.go             # Business logic
│       │   └── types.go               # DTOs and models
│       ├── event/                     # Event service
│       └── session/                   # Session service
│
├── pkg/                               # Reusable library packages
│   ├── cache/                         # Cache interfaces, Redis helpers
│   ├── db/                            # Database connectors, helpers
│   ├── idgen/                         # ID generation utilities
│   ├── kafka/                         # Kafka client and helpers
│   ├── logger/                        # Zerolog wrapper & helpers
│   ├── oauth2/                        # OAuth2 manager & token helpers
│   ├── passkey/                       # Passkey/WebAuthn utilities
│   ├── testutil/                      # Testing utilities
│   └── validator/                     # Input validation utilities
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
├── observability/                     # Monitoring & tracing configs
│   ├── config.alloy                   # Alloy OTel config
│   ├── loki.yaml                      # Loki logging config
│   ├── prometheus.yml                 # Prometheus metrics config
│   └── grafana-datasources.yaml       # Grafana datasources
│
├── k8s/                               # Kubernetes manifests
│   ├── deployment.yaml
│   ├── service.yaml
│   └── configmap.yaml
│
├── AGENTS.md                          # Agent coding guidelines
├── IMPROVEMENTS.md                    # Technical debt & improvements
├── Makefile                           # Build commands
├── docker-compose.yml                 # Local services
├── Dockerfile                         # Container image
├── .golangci.yml                      # Linting configuration
├── add-secrets.sh                     # Secrets management helper
├── test.http                          # API testing file
└── .env.example                       # Environment variables template
```

## Key Points
1. **`cmd/` folder**  
   - Each subdirectory represents a **separate runnable service or example**.  

2. **`pkg/` folder**  
   - Contains **reusable packages** for core functionality.