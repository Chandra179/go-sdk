package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"
)

// postgresClient implements the DB interface for PostgreSQL using lib/pq.
type postgresClient struct {
	db     *sql.DB
	config ConnectionConfig
}

// NewPostgresClient creates a new PostgreSQL database client with the given DSN
// and connection pool configuration. The connection is verified with a ping
// before returning, so a non-nil return value is ready to use.
//
// The caller is responsible for calling Close() when the client is no longer
// needed (typically at process shutdown).
func NewPostgresClient(dsn string, config ConnectionConfig) (DB, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrConnectionFailed, err)
	}

	// Configure connection pool — zero values preserve sql.DB defaults.
	if config.MaxOpenConns > 0 {
		db.SetMaxOpenConns(config.MaxOpenConns)
	}
	if config.MaxIdleConns > 0 {
		db.SetMaxIdleConns(config.MaxIdleConns)
	}
	if config.ConnMaxLifetime > 0 {
		db.SetConnMaxLifetime(config.ConnMaxLifetime)
	}
	if config.ConnMaxIdleTime > 0 {
		db.SetConnMaxIdleTime(config.ConnMaxIdleTime)
	}

	// Verify the connection before handing the client to the caller.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("%w: %v", ErrConnectionFailed, err)
	}

	return &postgresClient{db: db, config: config}, nil
}

// withQueryTimeout wraps ctx with QueryTimeout when the config has one set and
// the context does not already carry a shorter deadline.
func (c *postgresClient) withQueryTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if c.config.QueryTimeout <= 0 {
		return ctx, func() {}
	}
	if deadline, ok := ctx.Deadline(); ok {
		if time.Until(deadline) <= c.config.QueryTimeout {
			// Caller's deadline is already tighter — don't override it.
			return ctx, func() {}
		}
	}
	return context.WithTimeout(ctx, c.config.QueryTimeout)
}

// ExecContext executes a query without returning any rows.
func (c *postgresClient) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	ctx, cancel := c.withQueryTimeout(ctx)
	defer cancel()
	return c.db.ExecContext(ctx, query, args...)
}

// PrepareContext creates a prepared statement for later use.
func (c *postgresClient) PrepareContext(ctx context.Context, query string) (*sql.Stmt, error) {
	ctx, cancel := c.withQueryTimeout(ctx)
	defer cancel()
	return c.db.PrepareContext(ctx, query)
}

// QueryContext executes a query that returns multiple rows.
func (c *postgresClient) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	ctx, cancel := c.withQueryTimeout(ctx)
	defer cancel()
	return c.db.QueryContext(ctx, query, args...)
}

// QueryRowContext executes a query that returns at most one row.
func (c *postgresClient) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	ctx, cancel := c.withQueryTimeout(ctx)
	defer cancel()
	return c.db.QueryRowContext(ctx, query, args...)
}

// WithTransaction executes fn within a database transaction at the requested
// isolation level.
//
// The DBTX passed to fn is the underlying *sql.Tx, which is safe to pass to
// any sqlc-generated query. The transaction is committed when fn returns nil
// and rolled back otherwise. A panic inside fn triggers a rollback and is
// re-raised after the rollback completes.
func (c *postgresClient) WithTransaction(
	ctx context.Context,
	isolationLevel sql.IsolationLevel,
	fn func(ctx context.Context, tx DBTX) error,
) error {
	tx, err := c.db.BeginTx(ctx, &sql.TxOptions{Isolation: isolationLevel})
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p) // re-raise after rollback
		}
	}()

	if err := fn(ctx, tx); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return fmt.Errorf("transaction failed: %v, rollback also failed: %w", err, rbErr)
		}
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// Close releases all resources held by the connection pool.
func (c *postgresClient) Close() error {
	return c.db.Close()
}

// PingContext verifies the database is reachable.
func (c *postgresClient) PingContext(ctx context.Context) error {
	return c.db.PingContext(ctx)
}

// ---- Error classification helpers ------------------------------------------
//
// These helpers are intentionally separate from the sentinel errors so callers
// can decide how to handle driver-level errors without importing lib/pq
// themselves. Both functions fall back to string matching when the error is not
// a *pq.Error (e.g. wrapped errors from pgx or other drivers).

// IsTimeoutError reports whether err represents a query or connection timeout.
//
// It recognises:
//   - context.DeadlineExceeded
//   - PostgreSQL error codes 57014 (query_canceled), 57013 (statement_timeout),
//     57012 (canceling_statement_due_to_timeout), 40001 (serialization_failure)
//   - Common timeout-related substrings for driver-agnostic fallback
func IsTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	// lib/pq fast path — checked first to avoid string allocation.
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		switch pqErr.Code {
		case "57014", // query_canceled
			"57013", // statement_timeout
			"57012", // canceling_statement_due_to_timeout
			"40001": // serialization_failure (lock timeout)
			return true
		}
	}

	// Driver-agnostic fallback (covers pgx, wrapped errors, etc.).
	msg := strings.ToLower(err.Error())
	for _, kw := range []string{
		"timeout",
		"deadline exceeded",
		"context deadline",
		"connection timed out",
		"i/o timeout",
		"query_canceled",
	} {
		if strings.Contains(msg, kw) {
			return true
		}
	}
	return false
}

// IsDuplicateKeyError reports whether err is a unique-constraint violation.
//
// It recognises:
//   - PostgreSQL error code 23505 (unique_violation)
//   - Common duplicate-key substrings for driver-agnostic fallback
func IsDuplicateKeyError(err error) bool {
	if err == nil {
		return false
	}

	// lib/pq fast path.
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		return pqErr.Code == "23505" // unique_violation
	}

	// Driver-agnostic fallback.
	msg := strings.ToLower(err.Error())
	for _, kw := range []string{
		"duplicate key",
		"unique constraint",
		"unique violation",
		"already exists",
		"23505",
	} {
		if strings.Contains(msg, kw) {
			return true
		}
	}
	return false
}
