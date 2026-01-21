# App

Application initialization, dependency injection, and HTTP server setup.

## Directory Structure

```
internal/app/
├── bootstrap/          # Component initialization
│   ├── database.go      # Database and migrations
│   ├── cache.go        # Cache/Redis initialization
│   ├── oauth2.go       # OAuth2 manager
│   ├── event.go        # Kafka/Event system
│   ├── migrations.go   # Database migrations
│   └── observability.go # OTEL tracing, metrics, logging
├── health/             # Health check endpoints
│   ├── checker.go      # Health checker implementation
│   └── checker_test.go # Health checker tests
├── middleware/         # HTTP middleware
│   ├── auth.go         # Session authentication
│   ├── logging.go      # Request logging
│   ├── request_id.go   # Request ID injection
│   └── cors.go         # CORS handling
├── routes/             # HTTP route setup
│   ├── infra.go        # Infrastructure routes (metrics, swagger, health)
│   ├── auth.go         # Authentication routes
│   └── event.go       # Event/message broker routes
├── server.go           # Main server struct and lifecycle
└── README.md           # This file
```

## Design Principles

1. **Interface-based dependency injection** - Functions depend on interfaces, not concrete implementations
2. **Clear separation of concerns** - Each subdirectory has a single responsibility
3. **Configurable behavior** - Migration paths and sampler ratios are environment-configurable
4. **Graceful shutdown** - All resources are properly closed on shutdown
5. **Middleware-first** - Request ID, logging, CORS applied before route handlers

## Configuration

New environment variables added:
- `MIGRATION_PATH` - Database migration file path (default: `db/migrations`)
- `OTEL_SAMPLER_RATIO` - OTEL trace sampling ratio (0.0-1.0, default: 0.1)

## Testing

Run tests:
```bash
go test ./internal/app/... -v
go test ./internal/app/... -cover
```

## Key Changes from Original

1. **Subdirectory organization** - Split flat files into logical packages
2. **Middleware layer** - Added auth, logging, request ID, CORS middleware
3. **Configurable values** - Migration path and sampler ratio no longer hardcoded
4. **Improved test structure** - Health tests moved to health package
5. **Better separation** - Bootstrap functions isolated in own package
