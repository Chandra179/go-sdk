# AGENTS.md

Build commands and code style guidelines for agentic coding agents in this Go SDK repository.

## Commands

```bash
# Dependencies & Running
make ins              # tidy & vendor dependencies
make run              # run main app
go test ./...         # run all tests
go test ./pkg/db/...  # test specific package
go test -run TestUserRepository_CreateUser ./pkg/db  # single test
go test -v ./...      # verbose output
go test -race ./...   # race detection
go test -cover ./...  # coverage

# Docker & Build
make up               # start services
make build            # build & start services
make swag             # generate Swagger docs
```

## Code Style Guidelines

### Imports
Order: stdlib → third-party → internal (gosdk/*), blank lines between groups:
```go
import (
    "context"
    "fmt"

    "github.com/gin-gonic/gin"
    "github.com/redis/go-redis/v9"

    "gosdk/cfg"
    "gosdk/internal/app"
    "gosdk/pkg/db"
)
```

### Constructors
Use `NewXxx()` naming:
```go
func NewServer(ctx context.Context, config *cfg.Config) (*Server, error)
func NewHandler(service *Service) *Handler
func NewRedisCache(addr string) Cache
```

### Interface-First Design
Define interfaces in package root, implementations as private structs:
```go
type Cache interface {
    Set(ctx context.Context, key string, value string, ttl time.Duration) error
    Get(ctx context.Context, key string) (string, error)
}

type RedisCache struct {
    client *redis.Client
}

func NewRedisCache(addr string) Cache {
    return &RedisCache{client: redis.NewClient(&redis.Options{Addr: addr})}
}
```

### Service Layer Pattern
Organize as `handler.go` (HTTP), `service.go` (business logic), `types.go` (structs, constants, errors):
```go
type Handler struct { service *Service }
func NewHandler(service *Service) *Handler { return &Handler{service: service} }

type Service struct {
    oauth2Manager *oauth2.Manager
    sessionStore  session.Client
    db            db.SQLExecutor
}

type LoginRequest struct { Provider string `json:"provider" binding:"required"` }
var ErrUserNotFound = errors.New("user not found")
```

### Error Handling
Wrap with `%w`, define as package variables, check with `errors.Is()`:
```go
return nil, fmt.Errorf("failed to resolve user: %w", err)

var (
    ErrUserNotFound = errors.New("user not found")
    ErrInvalidState = errors.New("invalid state")
)

if errors.Is(err, ErrUserNotFound) { /* handle */ }
```

### Testing
Table-driven with `t.Run()`, setup helpers with `t.Helper()`, use `testify`:
```go
func TestRedisCache_Set(t *testing.T) {
    mr, cache := setupTestRedis(t)
    defer mr.Close()

    t.Run("successful set", func(t *testing.T) {
        err := cache.Set(ctx, "key", "value", time.Minute)
        assert.NoError(t, err)
    })
}

func setupTestRedis(t *testing.T) (*miniredis.Miniredis, *RedisCache) {
    t.Helper()
    mr, err := miniredis.Run()
    require.NoError(t, err)
    return mr, NewRedisCache(mr.Addr()).(*RedisCache)
}
```

### Context Usage
Pass `context.Context` as first parameter when needed:
```go
func (s *Service) GetOrCreateUser(ctx context.Context, ...) (string, error)
func (c *Cache) Set(ctx context.Context, key string, value string, ttl time.Duration) error
```

### Naming Conventions
- Exported: PascalCase (`NewServer`, `LoginHandler`)
- Unexported: camelCase (`initDatabase`, `setupRoutes`)
- JSON tags: snake_case (`user_id`, `session_id`)
- Constants: PascalCase (`SessionCookieName`, `CookieMaxAge`)
- Errors: `ErrXxx` prefix (`ErrUserNotFound`)

### Project Architecture
- **cmd/** - Runnable applications (entry points)
- **internal/** - Private application code (app, services)
- **pkg/** - Reusable library packages (db, cache, logger, oauth2, kafka)
- **cfg/** - Centralized configuration
- **api/** - API documentation (swagger, proto)
- **k8s/** - Kubernetes manifests

### Dependency Management
Uses `vendor/`. Run `make ins` after: adding imports, updating go.mod, changing dependencies

### Logging
Use centralized logger from `pkg/logger`:
```go
s.logger.Info(ctx, "Initializing server...")
s.logger.Error(ctx, "Failed to connect", "error", err)
```
