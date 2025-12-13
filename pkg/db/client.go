package db

import (
	"context"
	"database/sql"
)

type TxFunc func(ctx context.Context, tx *sql.Tx) error

// SQLExecutor defines the interface for golang native database/sql driver operations
type SQLExecutor interface {
	WithTransaction(ctx context.Context, isolation sql.IsolationLevel, fn TxFunc) error
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}
