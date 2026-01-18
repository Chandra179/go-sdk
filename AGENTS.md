# AGENTS.md

Build commands and code style guidelines for agentic coding agents in this Go SDK repository.

## Commands

### Build & Run
```bash
make ins              # tidy & vendor dependencies
make run              # run main app
make up               # start services (docker)
make build            # build & start services (docker)
```

### Verification (MANDATORY)
Run these before submitting any changes:
```bash
make lint             # run golangci-lint
make test             # run all tests (race + cover)
```

### Testing (Specific)
```bash
# Run all tests in a package
go test -v ./internal/service/auth/...

# Run a specific test function
go test -v -run TestAuth_Login ./internal/service/auth

# Run tests with race detection (recommended)
go test -race ./pkg/db/...
```

## Code Style Guidelines

### Imports
Group imports: stdlib, third-party, then internal (`gosdk/*`). Separate groups with blank lines.
```go
import (
    "context"
    "fmt"
    "net/http"

    "github.com/gin-gonic/gin"
    "github.com/redis/go-redis/v9"

    "gosdk/cfg"
    "gosdk/internal/service/auth"
    "gosdk/pkg/logger"
)
```

### Naming Conventions
- **Constructors**: `NewXxx` (e.g., `NewServer`, `NewHandler`).
- **Interfaces**: Define in the consuming package (consumer-driven).
- **Errors**: `ErrXxx` (e.g., `ErrUserNotFound`).
- **JSON Tags**: snake_case (e.g., `json:"user_id"`).
- **URLs**: Kebab-case in routes (e.g., `/auth/login-callback`).

### Interface-First Design
Define interfaces for dependencies to enable easy mocking and testing.
```go
// In domain package
type UserRepository interface {
    GetByID(ctx context.Context, id string) (*User, error)
}

// Implementation in infrastructure layer
type PostgresUserRepo struct { ... }
```

### Error Handling
- Use `fmt.Errorf("%w", err)` to wrap errors.
- Return early (guard clauses) to keep nesting low.
- Define sentinel errors in the package for specific checks (`errors.Is`).

### Service Layer Pattern
- **Handlers** (`handler.go`): Parse HTTP, validate input, call service, map errors to HTTP codes.
- **Services** (`service.go`): Business logic, orchestration, transaction management.
- **Repositories** (`repository.go` or `pkg/db`): Data access.

### Context Usage
- Always pass `context.Context` as the first argument to functions performing I/O.
- Respect context cancellation in long-running loops.

### Configuration
Use `gosdk/cfg` to load configuration from environment variables.
```go
func Load() (*Config, error) {
    var errs []error
    // Use helper to collect errors instead of failing immediately
    host := mustEnv("REDIS_HOST", &errs)
    if len(errs) > 0 {
        return nil, errors.Join(errs...)
    }
    return &Config{...}, nil
}
```

### Database Access
Use `gosdk/pkg/db` for SQL operations. Always use contexts and handle transactions properly.
```go
// Transaction example
err := s.db.WithTransaction(ctx, sql.LevelReadCommitted, func(ctx context.Context, tx *sql.Tx) error {
    // Perform operations using tx
    return nil
})
```

### Logging
- Use `gosdk/pkg/logger`.
- Pass `ctx` to logger methods for tracing.
```go
logger.Info(ctx, "processing payment", logger.Field{Key: "amount", Value: 100})
```

## Project Architecture

- **cmd/**: Entry points (`main.go`).
- **internal/app/**: App bootstrapping (`server.go`).
- **internal/service/**: Domain logic (auth, event, session).
- **pkg/**: Reusable libraries (db, cache, kafka, logger).
- **cfg/**: Configuration loading.
- **api/**: Proto and Swagger definitions.

## Work Priorities
Check `IMPROVEMENTS.md` for the current backlog of tasks and technical debt.
Always ensure new code has accompanying tests and documentation.
