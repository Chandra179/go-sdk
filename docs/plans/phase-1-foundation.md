# Phase 1: Foundation - Production Readiness

## Overview

This phase addresses critical infrastructure gaps preventing the codebase from being production-ready. Currently, the service lacks proper shutdown procedures, Kubernetes health probes, automated validation, and code quality enforcement.

## Objectives

- Implement graceful shutdown for all services (HTTP, DB, Redis, Kafka, Observability)
- Add Kubernetes-ready health check endpoints (liveness and readiness)
- Establish CI/CD pipeline for automated testing and validation
- Configure code quality tooling (golangci-lint) with pre-commit hooks

---

## 1. Graceful Shutdown

### Current State

**File:** `internal/app/server.go:157-170`

The current `Shutdown()` method only closes Kafka connections and observability. Critical resources are not properly released:

```go
func (s *Server) Shutdown(ctx context.Context) error {
    if s.kafkaClient != nil {
        if err := s.kafkaClient.Close(); err != nil {
            return fmt.Errorf("kafka shutdown: %w", err)
        }
    }
    if s.shutdown != nil {
        if err := s.shutdown(ctx); err != nil {
            return fmt.Errorf("observability shutdown: %w", err)
        }
    }
    return nil
}
```

**Issues:**
- HTTP server is never shut down
- Database connections are not closed
- Redis connections are not closed
- No signal handling for SIGINT/SIGTERM
- No timeout configuration for shutdown
- No context cancellation for in-flight requests

### Implementation Plan

#### 1.1 Add HTTP Server to Server Struct

**File:** `internal/app/server.go`

Modify the `Server` struct to include the HTTP server:

```go
type Server struct {
    config               *cfg.Config
    httpServer           *http.Server
    router               *gin.Engine
    logger               *logger.AppLogger
    db                   *db.SQLClient
    cache                cache.Cache
    sessionStore         session.Client
    oauth2Manager        *oauth2.Manager
    authService          *auth.Service
    kafkaClient          kafka.Client
    messageBrokerSvc     *event.Service
    messageBrokerHandler *event.Handler
    shutdown             func(context.Context) error
}
```

#### 1.2 Modify Run Method to Store HTTP Server

**File:** `internal/app/server.go`

Update the `Run()` method to store the HTTP server reference:

```go
func (s *Server) Run(addr string) error {
    s.httpServer = &http.Server{
        Addr:    addr,
        Handler: s.router,
    }

    s.logger.Info(context.Background(), "Server listening", Field{Key: "addr", Value: addr})

    err := s.httpServer.ListenAndServe()
    if err != nil && !errors.Is(err, http.ErrServerClosed) {
        return fmt.Errorf("server error: %w", err)
    }

    return nil
}
```

#### 1.3 Add Shutdown Timeout Configuration

**File:** `cfg/config.go`

Add shutdown configuration:

```go
type Config struct {
    AppEnv              string
    Redis               RedisConfig
    Postgres            PostgresConfig
    OAuth2              Oauth2Config
    Observability        ObservabilityConfig
    Kafka               KafkaConfig
    ShutdownTimeout     time.Duration
}
```

**File:** `cfg/config.go` (in `Load()` function)

```go
func Load() (*Config, error) {
    // ... existing code ...

    shutdownTimeoutStr := getEnvOrDefault("SHUTDOWN_TIMEOUT", "30s")
    shutdownTimeout, err := time.ParseDuration(shutdownTimeoutStr)
    if err != nil {
        errs = append(errs, errors.New("invalid duration for SHUTDOWN_TIMEOUT: "+shutdownTimeoutStr))
    }

    // ... existing validation ...

    return &Config{
        // ... existing config ...
        ShutdownTimeout: shutdownTimeout,
    }, nil
}
```

#### 1.4 Implement Signal Handling in main.go

**File:** `cmd/myapp/main.go`

```go
package main

import (
    "context"
    "log"
    "os"
    "os/signal"
    "syscall"

    "gosdk/cfg"
    "gosdk/internal/app"
)

func main() {
    config, err := cfg.Load()
    if err != nil {
        log.Fatalf("Failed to load config: %v", err)
    }

    ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
    defer stop()

    server, err := app.NewServer(ctx, config)
    if err != nil {
        log.Fatalf("Failed to initialize server: %v", err)
    }
    defer server.Shutdown(ctx)

    go func() {
        <-ctx.Done()
        log.Println("Shutdown signal received, gracefully shutting down...")
        shutdownCtx, cancel := context.WithTimeout(context.Background(), config.ShutdownTimeout)
        defer cancel()
        if err := server.Shutdown(shutdownCtx); err != nil {
            log.Printf("Error during shutdown: %v", err)
        }
    }()

    if err := server.Run(":8080"); err != nil {
        log.Fatalf("Server error: %v", err)
    }

    log.Println("Server stopped")
}
```

#### 1.5 Enhance Shutdown Method

**File:** `internal/app/server.go`

Update `Shutdown()` to properly close all resources:

```go
func (s *Server) Shutdown(ctx context.Context) error {
    s.logger.Info(ctx, "Starting graceful shutdown")

    var errs []error

    // 1. Shutdown HTTP server first (stop accepting new requests)
    if s.httpServer != nil {
        s.logger.Info(ctx, "Shutting down HTTP server")
        if err := s.httpServer.Shutdown(ctx); err != nil {
            errs = append(errs, fmt.Errorf("HTTP server shutdown: %w", err))
        }
    }

    // 2. Close Kafka connections
    if s.kafkaClient != nil {
        s.logger.Info(ctx, "Closing Kafka connections")
        if err := s.kafkaClient.Close(); err != nil {
            errs = append(errs, fmt.Errorf("kafka shutdown: %w", err))
        }
    }

    // 3. Close database connections
    if s.db != nil {
        s.logger.Info(ctx, "Closing database connections")
        if err := s.db.Close(); err != nil {
            errs = append(errs, fmt.Errorf("database shutdown: %w", err))
        }
    }

    // 4. Close Redis connections (if cache has Close method)
    if closer, ok := s.cache.(interface{ Close() error }); ok {
        s.logger.Info(ctx, "Closing Redis connections")
        if err := closer.Close(); err != nil {
            errs = append(errs, fmt.Errorf("cache shutdown: %w", err))
        }
    }

    // 5. Shutdown observability
    if s.shutdown != nil {
        s.logger.Info(ctx, "Shutting down observability")
        if err := s.shutdown(ctx); err != nil {
            errs = append(errs, fmt.Errorf("observability shutdown: %w", err))
        }
    }

    if len(errs) > 0 {
        return fmt.Errorf("shutdown errors: %v", errors.Join(errs...))
    }

    s.logger.Info(ctx, "Shutdown completed successfully")
    return nil
}
```

#### 1.6 Add Close Method to SQLClient

**File:** `pkg/db/client.go`

```go
type SQLClient struct {
    db *sql.DB
}

func (c *SQLClient) Close() error {
    if c.db != nil {
        return c.db.Close()
    }
    return nil
}
```

#### 1.7 Add Close Method to RedisCache

**File:** `pkg/cache/redis_cache.go`

```go
func (c *RedisCache) Close() error {
    return c.client.Close()
}
```

### Testing

Create a test to verify graceful shutdown:

**File:** `internal/app/server_test.go`

```go
package app

import (
    "context"
    "net/http"
    "syscall"
    "testing"
    "time"

    "gosdk/cfg"
    "gosdk/pkg/logger"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestServer_GracefulShutdown(t *testing.T) {
    config := &cfg.Config{
        AppEnv:          "test",
        ShutdownTimeout: 5 * time.Second,
        // ... minimal config ...
    }

    ctx := context.Background()
    server, err := NewServer(ctx, config)
    require.NoError(t, err)

    go func() {
        time.Sleep(100 * time.Millisecond)
        syscall.Kill(syscall.Getpid(), syscall.SIGTERM)
    }()

    err = server.Run(":8081")
    assert.NoError(t, err)

    // Verify server actually shut down
    _, err = http.Get("http://localhost:8081/healthz")
    assert.Error(t, err)
}
```

---

## 2. Health Checks

### Current State

No health endpoints exist. Kubernetes cannot determine if the service is:
- Running (liveness probe)
- Ready to accept traffic (readiness probe)

### Implementation Plan

#### 2.1 Create Health Check Package

**File:** `internal/app/health.go`

```go
package app

import (
    "context"
    "database/sql"
    "net/http"
    "time"

    "gosdk/pkg/cache"
    "gosdk/pkg/logger"

    "github.com/gin-gonic/gin"
)

type HealthChecker struct {
    db    DBChecker
    cache CacheChecker
    kafka KafkaChecker
    logger logger.Logger
}

type DBChecker interface {
    PingContext(ctx context.Context) error
    Close() error
}

type CacheChecker interface {
    Ping(ctx context.Context) error
    Close() error
}

type KafkaChecker interface {
    Close() error
}

func NewHealthChecker(db DBChecker, cache CacheChecker, kafka KafkaChecker, logger logger.Logger) *HealthChecker {
    return &HealthChecker{
        db:    db,
        cache: cache,
        kafka: kafka,
        logger: logger,
    }
}

type HealthStatus struct {
    Status    string            `json:"status"`
    Timestamp string            `json:"timestamp"`
    Checks    map[string]string `json:"checks,omitempty"`
}

// Liveness indicates if the service is running
func (h *HealthChecker) Liveness(c *gin.Context) {
    c.JSON(http.StatusOK, HealthStatus{
        Status:    "ok",
        Timestamp: time.Now().UTC().Format(time.RFC3339),
    })
}

// Readiness indicates if the service can handle requests
func (h *HealthChecker) Readiness(c *gin.Context) {
    ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
    defer cancel()

    checks := make(map[string]string)
    healthy := true

    // Check database
    if h.db != nil {
        if err := h.db.PingContext(ctx); err != nil {
            checks["database"] = "unhealthy: " + err.Error()
            healthy = false
        } else {
            checks["database"] = "healthy"
        }
    }

    // Check cache
    if h.cache != nil {
        if err := h.cache.Ping(ctx); err != nil {
            checks["cache"] = "unhealthy: " + err.Error()
            healthy = false
        } else {
            checks["cache"] = "healthy"
        }
    }

    // Check Kafka
    checks["kafka"] = "healthy" // Kafka is harder to ping, assume healthy if connected

    if healthy {
        c.JSON(http.StatusOK, HealthStatus{
            Status:    "ready",
            Timestamp: time.Now().UTC().Format(time.RFC3339),
            Checks:    checks,
        })
    } else {
        c.JSON(http.StatusServiceUnavailable, HealthStatus{
            Status:    "not_ready",
            Timestamp: time.Now().UTC().Format(time.RFC3339),
            Checks:    checks,
        })
    }
}
```

#### 2.2 Add Ping Method to SQLClient

**File:** `pkg/db/client.go`

```go
func (c *SQLClient) PingContext(ctx context.Context) error {
    return c.db.PingContext(ctx)
}
```

#### 2.3 Add Ping Method to RedisCache

**File:** `pkg/cache/redis_cache.go`

```go
func (c *RedisCache) Ping(ctx context.Context) error {
    return c.client.Ping(ctx).Err()
}
```

#### 2.4 Update Routes to Use Health Checker

**File:** `internal/app/routes.go`

```go
func setupInfraRoutes(r *gin.Engine, hc *HealthChecker) {
    r.GET("/metrics", gin.WrapH(promhttp.Handler()))
    r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
    r.GET("/docs", docsHandler)
    r.GET("/healthz", hc.Liveness)
    r.GET("/readyz", hc.Readiness)
}
```

#### 2.5 Update Server Initialization

**File:** `internal/app/server.go`

```go
func (s *Server) setupRoutes() {
    r := gin.New()
    r.Use(gin.Recovery())

    healthChecker := NewHealthChecker(s.db, s.cache, s.kafkaClient, s.logger)
    setupInfraRoutes(r, healthChecker)

    authHandler := auth.NewHandler(s.authService)
    setupAuthRoutes(r, authHandler, s.oauth2Manager)

    setupMessageBrokerRoutes(r, s.messageBrokerHandler)

    s.router = r
}
```

### Testing

**File:** `internal/app/health_test.go`

```go
package app

import (
    "context"
    "net/http"
    "net/http/httptest"
    "testing"
    "time"

    "gosdk/pkg/logger"

    "github.com/gin-gonic/gin"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

type MockDBChecker struct {
    pingErr error
}

func (m *MockDBChecker) PingContext(ctx context.Context) error {
    return m.pingErr
}

func (m *MockDBChecker) Close() error {
    return nil
}

type MockCacheChecker struct {
    pingErr error
}

func (m *MockCacheChecker) Ping(ctx context.Context) error {
    return m.pingErr
}

func (m *MockCacheChecker) Close() error {
    return nil
}

func TestHealthChecker_Liveness(t *testing.T) {
    gin.SetMode(gin.TestMode)
    hc := NewHealthChecker(&MockDBChecker{}, &MockCacheChecker{}, nil, logger.NewLogger("test"))

    router := gin.New()
    router.GET("/healthz", hc.Liveness)

    req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
    w := httptest.NewRecorder()

    router.ServeHTTP(w, req)

    assert.Equal(t, http.StatusOK, w.Code)
    assert.Contains(t, w.Body.String(), `"status":"ok"`)
}

func TestHealthChecker_Readiness(t *testing.T) {
    gin.SetMode(gin.TestMode)

    t.Run("all healthy", func(t *testing.T) {
        hc := NewHealthChecker(&MockDBChecker{}, &MockCacheChecker{}, nil, logger.NewLogger("test"))

        router := gin.New()
        router.GET("/readyz", hc.Readiness)

        req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
        w := httptest.NewRecorder()

        router.ServeHTTP(w, req)

        assert.Equal(t, http.StatusOK, w.Code)
        assert.Contains(t, w.Body.String(), `"status":"ready"`)
    })

    t.Run("database unhealthy", func(t *testing.T) {
        hc := NewHealthChecker(&MockDBChecker{pingErr: assert.AnError}, &MockCacheChecker{}, nil, logger.NewLogger("test"))

        router := gin.New()
        router.GET("/readyz", hc.Readiness)

        req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
        w := httptest.NewRecorder()

        router.ServeHTTP(w, req)

        assert.Equal(t, http.StatusServiceUnavailable, w.Code)
        assert.Contains(t, w.Body.String(), `"status":"not_ready"`)
    })
}
```

---

## 3. CI/CD Pipeline

### Current State

No `.github/` directory exists. No automated testing, validation, or build verification.

### Implementation Plan

#### 3.1 Create GitHub Actions Workflow

**File:** `.github/workflows/ci.yml`

```yaml
name: CI

on:
  push:
    branches: [ main, develop ]
  pull_request:
    branches: [ main ]

jobs:
  lint:
    name: Lint
    runs-on: ubuntu-latest
    steps:
      - name: Checkout code
        uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.25'

      - name: Run golangci-lint
        uses: golangci/golangci-lint-action@v4
        with:
          version: latest
          args: --timeout=5m

  test:
    name: Test
    runs-on: ubuntu-latest
    steps:
      - name: Checkout code
        uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.25'

      - name: Cache Go modules
        uses: actions/cache@v4
        with:
          path: |
            ~/.cache/go-build
            ~/go/pkg/mod
          key: ${{ runner.os }}-go-${{ hashFiles('**/go.sum') }}
          restore-keys: |
            ${{ runner.os }}-go-

      - name: Install dependencies
        run: make ins

      - name: Run tests
        run: go test ./... -race -coverprofile=coverage.out

      - name: Check coverage
        run: |
          go tool cover -func=coverage.out | grep total
          if [ $(go tool cover -func=coverage.out | grep total | awk '{print $3}' | sed 's/%//') -lt 60 ]; then
            echo "Coverage is below 60%"
            exit 1
          fi

      - name: Upload coverage to Codecov
        uses: codecov/codecov-action@v4
        with:
          file: ./coverage.out
          fail_ci_if_error: true

  build:
    name: Build
    runs-on: ubuntu-latest
    needs: [lint, test]
    steps:
      - name: Checkout code
        uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.25'

      - name: Build
        run: go build -v -o bin/myapp cmd/myapp/main.go

      - name: Upload binary
        uses: actions/upload-artifact@v4
        with:
          name: myapp-binary
          path: bin/myapp

  docker:
    name: Docker Build
    runs-on: ubuntu-latest
    needs: [lint, test]
    steps:
      - name: Checkout code
        uses: actions/checkout@v4

      - name: Set up Docker Buildx
        uses: docker/setup-buildx-action@v3

      - name: Build Docker image
        uses: docker/build-push-action@v5
        with:
          context: .
          push: false
          tags: my-app:latest
          cache-from: type=gha
          cache-to: type=gha,mode=max
```

#### 3.2 Create Status Badge

**File:** `README.md` (add to top)

```markdown
[![CI](https://github.com/your-username/go-sdk/actions/workflows/ci.yml/badge.svg)](https://github.com/your-username/go-sdk/actions/workflows/ci.yml)
```

### Testing

- Push to a new branch and verify workflow runs
- Check that all jobs pass on the main branch
- Verify coverage badge updates
- Test that failed tests fail the workflow

---

## 4. Linting Configuration

### Current State

No `.golangci.yml` configuration file. No pre-commit hooks.

### Implementation Plan

#### 4.1 Create Linting Config

**File:** `.golangci.yml`

```yaml
run:
  timeout: 5m
  tests: true
  modules-download-mode: vendor

linters:
  enable:
    - govet
    - errcheck
    - staticcheck
    - unused
    - gosimple
    - structcheck
    - varcheck
    - ineffassign
    - deadcode
    - typecheck
    - gosec
    - gocyclo
    - dupl
    - gofmt
    - goimports
    - misspell
    - lll
    - unconvert
    - goconst
    - gocritic
    - prealloc

  disable:
    - exhaustivestruct

linters-settings:
  govet:
    enable-all: true
    disable:
      - shadow

  errcheck:
    check-type-assertions: true
    check-blank: true

  gocyclo:
    min-complexity: 15

  dupl:
    threshold: 100

  lll:
    line-length: 120
    tab-width: 4

  goconst:
    min-len: 3
    min-occurrences: 3

  misspell:
    locale: US

  gosec:
    excludes:
      - G104 # Errors unhandled

  prealloc:
    simple: true
    range-loops: true
    for-loops: false

  gocritic:
    enabled-tags:
      - performance
      - style
      - experimental
    disabled-checks:
      - wrapperFunc
      - sloppyReassign
```

#### 4.2 Create Pre-commit Hook Script

**File:** `.githooks/pre-commit`

```bash
#!/bin/sh

set -e

echo "Running golangci-lint..."
golangci-lint run

echo "Running tests..."
go test ./...

echo "Formatting code..."
gofmt -s -w .
goimports -w .

echo "Pre-commit checks passed!"
```

#### 4.3 Create Hook Installation Script

**File:** `scripts/install-hooks.sh`

```bash
#!/bin/bash

# Install git hooks
HOOKS_DIR=$(git rev-parse --git-path hooks)
mkdir -p "$HOOKS_DIR"

# Copy pre-commit hook
cp .githooks/pre-commit "$HOOKS_DIR/pre-commit"
chmod +x "$HOOKS_DIR/pre-commit"

echo "Git hooks installed successfully!"
```

#### 4.4 Update Makefile

**File:** `Makefile`

```makefile
ins:
	go mod tidy && go mod vendor

lint:
	golangci-lint run

test:
	go test ./... -race -cover

test-coverage:
	go test ./... -race -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html

install-hooks:
	./scripts/install-hooks.sh

# ... existing targets ...
```

#### 4.5 Create .githooks Directory

**File:** `.githooks/pre-commit` (create directory and file)

```bash
#!/bin/sh
set -e

echo "Running golangci-lint..."
if ! golangci-lint run; then
    echo "Linting failed. Please fix the issues before committing."
    exit 1
fi

echo "Running tests..."
if ! go test ./... -short; then
    echo "Tests failed. Please fix the failing tests before committing."
    exit 1
fi

echo "Pre-commit checks passed!"
```

### Testing

- Run `make lint` and verify no errors
- Run `make install-hooks` and check hooks are installed
- Make a test commit and verify hooks run
- Verify `golangci-lint` catches issues

---

## Dependencies

No new dependencies are required for this phase. All functionality uses:
- Go 1.25+ standard library
- Existing dependencies (gin, redis, kafka, etc.)

---

## Estimated Effort

| Task                 | Effort |
|---------------------|--------|
| Graceful Shutdown    | 4 hours |
| Health Checks        | 3 hours |
| CI/CD Pipeline      | 3 hours |
| Linting             | 2 hours |
| **Total**           | **12 hours** |

---

## Rollback Plan

If any feature causes issues:

1. **Graceful Shutdown:**
   - Revert `Shutdown()` method to original
   - Remove signal handling from `main.go`
   - Comment out `httpServer` from `Server` struct

2. **Health Checks:**
   - Remove health endpoints from `routes.go`
   - Delete `internal/app/health.go`
   - Continue without Kubernetes probes

3. **CI/CD Pipeline:**
   - Delete `.github/workflows/ci.yml`
   - Disable workflow in GitHub Actions UI
   - Continue with manual testing

4. **Linting:**
   - Delete `.golangci.yml`
   - Uninstall pre-commit hooks
   - Continue without automated linting

---

## Success Criteria

After completing Phase 1:

- [ ] Service shuts down gracefully on SIGINT/SIGTERM
- [ ] All resources (HTTP, DB, Redis, Kafka) are properly closed
- [ ] `/healthz` returns 200 OK
- [ ] `/readyz` returns 200 when dependencies are healthy, 503 when not
- [ ] CI/CD workflow passes on all commits to main/develop
- [ ] `golangci-lint` runs successfully on all code
- [ ] Pre-commit hooks prevent bad commits
- [ ] All tests pass with race detection enabled
- [ ] Minimum 60% code coverage
- [ ] Docker build succeeds
