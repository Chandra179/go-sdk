# App

Application initialization, dependency injection, and HTTP server setup.

## Architecture

This package follows the **Composition Root** pattern with clear separation of concerns:

![App architecture](/img/app_arch.png)


### Key Components

1. **Provider** (`provider.go`) - The composition root that wires all dependencies
   - `Infrastructure`: Holds all infrastructure (DB, cache, Kafka, etc.)
   - `Services`: Holds all domain services (auth, session)
   - Manages resource lifecycle (initialization and shutdown)

2. **Server** (`server.go`) - HTTP transport layer
   - Only concern: HTTP routing and handling
   - Receives all dependencies from Provider
   - Lightweight and focused

3. **Bootstrap** (`bootstrap/`) - Low-level initialization helpers
   - Used by Provider to create concrete implementations
   - Focused initialization functions

## Directory Structure

```
internal/app/
├── bootstrap/           # Component initialization helpers
│   ├── database.go      # Database and migrations
│   ├── cache.go         # Cache/Redis initialization
│   ├── oauth2.go        # OAuth2 manager
│   ├── kafka.go         # Kafka/Event system
│   ├── migrations.go    # Database migrations
│   └── otel.go          # OTEL tracing, metrics, logging
├── health/              # Health check endpoints
│   └── checker.go       # Health checker implementation
├── middleware/          # HTTP middleware
│   ├── auth.go          # Session authentication
│   ├── logging.go       # Request logging
│   ├── request_id.go    # Request ID injection
│   └── cors.go          # CORS handling
├── routes/              # HTTP route setup
│   ├── infra.go         # Infrastructure routes (metrics, swagger, health)
│   └── auth.go          # Authentication routes
├── provider.go          # Composition root, dependency injection
├── http_server.go       # HTTP transport layer
└── README.md            # This file
```

## Design Principles

1. **Composition Root** - Single place (`Provider`) where the entire dependency graph is constructed
2. **Clear Layer Separation**
   - Infrastructure: Stateful external resources (DB, cache, etc.)
   - Services: Stateless business logic
   - Server: HTTP transport only
3. **No Circular Dependencies** - Callback pattern breaks OAuth2 ↔ AuthService cycle
4. **Graceful Shutdown** - Resources closed in reverse order of initialization
5. **Fail-Fast Cleanup** - Resources cleaned up if initialization fails partway
6. **Interface-based Dependencies** - All layers depend on interfaces for testability

## Usage

```go
// In main.go or cmd/myapp/main.go

// 1. Create the Provider (initializes all infrastructure and services)
provider, err := app.NewProvider(ctx, config)
if err != nil {
    log.Fatalf("Failed to initialize: %v", err)
}

// 2. Create the Server
server, err := app.NewServer(provider)
if err != nil {
    provider.Infra.Close(ctx)
    log.Fatalf("Failed to create server: %v", err)
}

// 3. Run
if err := server.Run(); err != nil {
    log.Fatal(err)
}

// 4. Shutdown (on signal)
server.Shutdown(ctx)
provider.Infra.Close(ctx)
```