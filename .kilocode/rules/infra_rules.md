# Infrastructure Modules Guide

This guide explains how to create and organize infrastructure service dependencies (Redis, Kafka, RabbitMQ, etc.) in the project.

## Overview

Infrastructure modules are reusable service dependencies that should be placed in the `pkg/` directory. Examples include:
- Redis (caching)
- Kafka (message queue)
- RabbitMQ (message queue)
- Database clients
- Logger
- OAuth2
- Passkey

## Directory Structure

Each infrastructure module should follow this structure:

```
pkg/
├── cache/          # Redis
├── kafka/          # Kafka
├── rabbitmq/       # RabbitMQ
├── logger/         # Logging
├── oauth2/         # OAuth2
├── passkey/        # Passkey
└── <module>/
    ├── config.go   # Configuration struct
    ├── health.go   # Health check
    ├── client.go   # Client initialization
    └── *.go        # Implementation files (optional)
```

## Required Files

### 1. config.go

Contains the configuration struct for the module:

```go
package redis

type Config struct {
    Host     string
    Port     string
    Password string
    DB       int
}
```

### 2. health.go

Contains health check functionality:

```go
package redis

import "context"

type HealthChecker interface {
    Ping(ctx context.Context) error
}

// health implements HealthChecker
type health struct {
    client *Client
}

func (h *health) Ping(ctx context.Context) error {
    return h.client.Ping(ctx).Err()
}
```

### 3. client.go

Contains the client initialization and module implementation. If the implementation is small (under 200 lines), it can all be in `client.go`. Otherwise, create separate `.go` files:

```go
package redis

import (
    "context"
    "time"

    "github.com/redis/go-redis/v9"
)

// Client wraps the Redis client
type Client struct {
    *redis.Client
}

// NewClient creates a new Redis client
func NewClient(cfg *Config) (*Client, error) {
    client := redis.NewClient(&redis.Options{
        Addr:     cfg.Host + ":" + cfg.Port,
        Password: cfg.Password,
        DB:       cfg.DB,
    })

    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    if err := client.Ping(ctx).Err(); err != nil {
        return nil, err
    }

    return &Client{Client: client}, nil
}
```

## Module Initialization Pattern

Infrastructure modules are initialized through a two-layer approach:

### Layer 1: Bootstrap (`internal/app/bootstrap/`)

Create initialization functions in `internal/app/bootstrap/` that wrap the module's client creation:

```go
// internal/app/bootstrap/cache.go
package bootstrap

import (
    "gosdk/pkg/cache"
)

func InitCache(host, port string) cache.Cache {
    addr := host + ":" + port
    return cache.NewRedisCache(addr)
}
```

### Layer 2: Provider (`internal/app/provider.go`)

The provider calls bootstrap functions to wire dependencies:

```go
// internal/app/provider.go
func NewProvider(ctx context.Context, config *cfg.Config) (*Provider, error) {
    // ... setup logger and OTel first
    
    infra, err := initBaseInfrastructure(config, appLogger, shutdownOTel, metricsHandler)
    // ...
}

func initBaseInfrastructure(config *cfg.Config, ...) (*Infrastructure, error) {
    infra := &Infrastructure{
        Logger:         appLogger,
        MetricsHandler: metricsHandler,
        shutdownOTel:   shutdownOTel,
    }

    // Initialize database first
    dbClient, err := bootstrap.InitDatabase(dsn, config.Postgres.MigrationPath, connConfig)
    if err != nil {
        return nil, fmt.Errorf("database initialization: %w", err)
    }
    infra.DB = dbClient

    // Initialize cache
    infra.Cache = bootstrap.InitCache(config.Redis.Host, config.Redis.Port)

    return infra, nil
}
```

## Configuration Loading

Configuration for infrastructure modules should be loaded in `internal/cfg/` following the [Configuration Guide](./cfg_rules.md).

### Example: Adding Redis Config

1. Create `internal/cfg/redis.go`:

```go
package cfg

import "time"

type RedisConfig struct {
    Host     string
    Port     string
    Password string
    DB       int
}

func (l *Loader) loadRedis() RedisConfig {
    return RedisConfig{
        Host:     l.requireEnv("REDIS_HOST"),
        Port:     l.requireEnv("REDIS_PORT"),
        Password: l.requireEnv("REDIS_PASSWORD"),
        DB:       l.getEnvIntOrDefault("REDIS_DB", 0),
    }
}
```

2. Update `internal/cfg/config.go`:

```go
type Config struct {
    // ... existing fields
    Redis RedisConfig
}
```

3. Update `Load()` function:

```go
cfg := &Config{
    // ... existing fields
    Redis: l.loadRedis(),
}
```

## Complete Flow

```
1. cfg.Load()                    # Load configuration from YAML + env
   │
   ├── internal/cfg/redis.go     # Redis config definition
   └── ...

2. app.NewProvider()            # Provider initialization
   │
   ├── bootstrap.InitOtel()      # Observability
   ├── initBaseInfrastructure()
   │   ├── bootstrap.InitDatabase()   # Database
   │   ├── bootstrap.InitCache()      # Cache
   │   └── ...
   └── bootstrap.InitOAuth2()    # OAuth2 (if needed)
```

## Best Practices

1. **Reusability**: Modules in `pkg/` should be reusable across different projects
2. **Configuration**: Always load configuration from `internal/cfg` using the hybrid approach (YAML + env)
3. **Health Checks**: Implement health checks for all infrastructure modules
4. **Bootstrap Layer**: Create initialization functions in `internal/app/bootstrap/`
5. **Provider Layer**: Call bootstrap functions from `internal/app/provider.go`
6. **Client Pattern**: Use a consistent client initialization pattern (`NewClient`)
7. **Interfaces**: Define interfaces for better testability
8. **Implementation Location**:
   - If implementation is under 200 lines → put in `client.go`
   - If implementation is complex → create separate `.go` files in the module directory
9. **Shutdown**: Implement proper cleanup in `Infrastructure.Close()`

## Example: Module Structure with Separate Implementation

For more complex modules:

```
pkg/kafka/
├── config.go       # Config struct
├── health.go       # Health check
├── client.go       # Client initialization
├── producer.go     # Producer implementation
├── consumer.go     # Consumer implementation
└── README.md       # Documentation (optional)
```

Corresponding bootstrap:

```
internal/app/bootstrap/
├── cache.go        # InitCache()
├── database.go     # InitDatabase()
├── kafka.go        # InitKafka() - if needed
└── ...
```
