package testutil

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"gosdk/pkg/db"
)

func SetupPostgresDB(t *testing.T) *sql.DB {
	t.Helper()

	sqlDB, err := sql.Open("postgres", "postgres://testuser:testpass@localhost:5432/testdb?sslmode=disable")
	if err != nil {
		t.Skip("postgres not available:", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := sqlDB.PingContext(ctx); err != nil {
		t.Skip("postgres not available:", err)
	}

	return sqlDB
}

func CreateTestUser(ctx context.Context, db db.SQLExecutor) (string, error) {
	userID := "test-user-" + time.Now().Format("20060102-150405")
	now := time.Now()

	insertQuery := `
		INSERT INTO users (id, provider, subject_id, email, full_name, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`

	_, err := db.ExecContext(ctx, insertQuery, userID, "test", "123", "test@example.com", "Test User", now, now)
	if err != nil {
		return "", err
	}

	return userID, nil
}
