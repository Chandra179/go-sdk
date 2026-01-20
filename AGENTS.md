# AGENTS.md

Build commands and code style guidelines for agentic coding agents in this Go SDK repository.

## Commands

### Build & Run
```bash
make ins              # tidy & vendor dependencies
make run              # run main app
make up               # start services (docker)
make build            # build & start services (docker)
make swag             # generate swagger docs
```

### Verification (MANDATORY)
Run these before submitting any changes:
```bash
golangci-lint run     # run golangci-lint (install if needed)
make test             # run all tests (race + cover)
```

### Testing (Specific)
```bash
# Run all tests in a package
go test -v ./internal/service/auth/...

# Run a specific test function
go test -v -run TestUserRepository_CreateUser ./pkg/db

# Run tests with race detection (recommended)
go test -race ./pkg/db/...

# Generate coverage report
make test-coverage
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
- **Constructors**: `NewXxx` (e.g., `NewServer`, `NewHandler`)
- **Interfaces**: Define in consuming package (consumer-driven)
- **Errors**: `ErrXxx` (e.g., `ErrUserNotFound`)
- **JSON Tags**: snake_case (e.g., `json:"user_id"`)
- **URLs**: Kebab-case in routes (e.g., `/auth/login-callback`)
- **Line Length**: Max 120 characters

### Interface-First Design
Define interfaces for dependencies to enable mocking.
```go
type UserRepository interface {
    GetByID(ctx context.Context, id string) (*User, error)
}
type PostgresUserRepo struct { ... }
```

### Error Handling
- Use `fmt.Errorf("%w", err)` to wrap errors
- Return early (guard clauses) to reduce nesting
- Define sentinel errors for `errors.Is` checks
- Collect errors into slices instead of failing immediately

### Service Layer Pattern
- **Handlers** (`handler.go`): Parse HTTP, validate, call service, map errors to HTTP codes
- **Services** (`service.go`): Business logic, orchestration, transaction management
- **Repositories** (`repository.go` or `pkg/db`): Data access

### Context Usage
- Always pass `context.Context` as first argument to I/O functions
- Respect context cancellation in long-running loops
- Use `*Context` variants for all DB operations (`ExecContext`, `QueryContext`, `QueryRowContext`)

### Configuration
Use `gosdk/cfg` to load configuration from environment variables.
```go
func Load() (*Config, error) {
    var errs []error
    host := mustEnv("REDIS_HOST", &errs)
    if len(errs) > 0 {
        return nil, errors.Join(errs...)
    }
    return &Config{...}, nil
}
```

### Database Access
Use `gosdk/pkg/db` SQLExecutor interface for all database operations.
```go
// Transaction
err := s.db.WithTransaction(ctx, sql.LevelReadCommitted, func(ctx context.Context, tx *sql.Tx) error {
    _, err := tx.ExecContext(ctx, query, args...)
    return err
})

// Query
err := s.db.QueryRowContext(ctx, query, args...).Scan(&user.ID, &user.Name)
```

### Testing Patterns
- Use `github.com/stretchr/testify` for assertions and mocking
- Table-driven tests with `t.Run()`: Arrange, Act, Assert
- Mock interfaces using `mock.Mock` for unit tests
- Test naming: `TestStructName_MethodName`

```go
func TestUserRepository_CreateUser(t *testing.T) {
    t.Run("success", func(t *testing.T) {
        // Arrange
        mockDB := new(MockSQLExecutor)
        repo := NewUserRepository(mockDB)
        // Act
        err := repo.CreateUser(ctx, name, email)
        // Assert
        assert.NoError(t, err)
        mockDB.AssertExpectations(t)
    })
}
```

### Swagger Documentation
Add Swagger annotations to handler functions.
```go
// @Summary Login with OAuth2 provider
// @Tags auth
// @Router /auth/login [post]
```

### Logging
Use `gosdk/pkg/logger` and pass `ctx` for tracing.
```go
logger.Info(ctx, "processing payment", logger.Field{Key: "amount", Value: 100})
```

## Project Architecture

- **cmd/**: Entry points (`main.go`)
- **internal/app/**: App bootstrapping (`server.go`)
- **internal/service/**: Domain logic (auth, event, session)
- **pkg/**: Reusable libraries (db, cache, kafka, logger)
- **cfg/**: Configuration loading from env vars
- **api/**: Swagger/OpenAPI definitions

## Work Priorities
Check `IMPROVEMENTS.md` for the backlog. Ensure new code has tests and documentation.
