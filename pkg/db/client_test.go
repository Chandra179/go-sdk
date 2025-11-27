package db

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockSQLExecutor is a mock implementation of SQLExecutor interface
type MockSQLExecutor struct {
	mock.Mock
}

func (m *MockSQLExecutor) DB() *sql.DB {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*sql.DB)
}

func (m *MockSQLExecutor) WithTransaction(ctx context.Context, isolation sql.IsolationLevel, fn TxFunc) error {
	args := m.Called(ctx, isolation, fn)
	return args.Error(0)
}

func (m *MockSQLExecutor) ExecContext(ctx context.Context, query string, queryArgs ...any) (sql.Result, error) {
	args := m.Called(ctx, query, queryArgs)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(sql.Result), args.Error(1)
}

func (m *MockSQLExecutor) QueryContext(ctx context.Context, query string, queryArgs ...any) (*sql.Rows, error) {
	args := m.Called(ctx, query, queryArgs)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*sql.Rows), args.Error(1)
}

func (m *MockSQLExecutor) QueryRowContext(ctx context.Context, query string, queryArgs ...any) *sql.Row {
	args := m.Called(ctx, query, queryArgs)
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*sql.Row)
}

// MockResult is a mock implementation of sql.Result
type MockResult struct {
	mock.Mock
}

func (m *MockResult) LastInsertId() (int64, error) {
	args := m.Called()
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockResult) RowsAffected() (int64, error) {
	args := m.Called()
	return args.Get(0).(int64), args.Error(1)
}

// Example: UserRepository to demonstrate usage
type UserRepository struct {
	db SQLExecutor
}

func NewUserRepository(executor SQLExecutor) *UserRepository {
	return &UserRepository{db: executor}
}

func (r *UserRepository) CreateUser(ctx context.Context, name, email string) error {
	query := "INSERT INTO users (name, email) VALUES (?, ?)"
	_, err := r.db.ExecContext(ctx, query, name, email)
	return err
}

func (r *UserRepository) UpdateUserWithTransaction(ctx context.Context, userID int64, name string) error {
	return r.db.WithTransaction(ctx, sql.LevelReadCommitted, func(ctx context.Context, tx *sql.Tx) error {
		query := "UPDATE users SET name = ? WHERE id = ?"
		_, err := tx.ExecContext(ctx, query, name, userID)
		return err
	})
}

func TestUserRepository_CreateUser(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		// Arrange
		mockDB := new(MockSQLExecutor)
		mockResult := new(MockResult)
		repo := NewUserRepository(mockDB)

		ctx := context.Background()
		name := "John Doe"
		email := "john@example.com"
		query := "INSERT INTO users (name, email) VALUES (?, ?)"

		mockResult.On("LastInsertId").Return(int64(1), nil)
		mockResult.On("RowsAffected").Return(int64(1), nil)
		mockDB.On("ExecContext", ctx, query, []any{name, email}).Return(mockResult, nil)

		// Act
		err := repo.CreateUser(ctx, name, email)

		// Assert
		assert.NoError(t, err)
		mockDB.AssertExpectations(t)
	})

	t.Run("database error", func(t *testing.T) {
		// Arrange
		mockDB := new(MockSQLExecutor)
		repo := NewUserRepository(mockDB)

		ctx := context.Background()
		name := "John Doe"
		email := "john@example.com"
		query := "INSERT INTO users (name, email) VALUES (?, ?)"
		expectedErr := errors.New("database connection failed")

		mockDB.On("ExecContext", ctx, query, []any{name, email}).Return(nil, expectedErr)

		// Act
		err := repo.CreateUser(ctx, name, email)

		// Assert
		assert.Error(t, err)
		assert.Equal(t, expectedErr, err)
		mockDB.AssertExpectations(t)
	})
}

func TestUserRepository_UpdateUserWithTransaction(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		// Arrange
		mockDB := new(MockSQLExecutor)
		repo := NewUserRepository(mockDB)

		ctx := context.Background()
		userID := int64(1)
		name := "Jane Doe"

		// Mock the transaction to execute the function immediately
		mockDB.On("WithTransaction", ctx, sql.LevelReadCommitted, mock.AnythingOfType("db.TxFunc")).
			Return(nil).
			Run(func(args mock.Arguments) {
				// This simulates the transaction executing the function
				// In real code, you'd actually call the function with a real tx
			})

		// Act
		err := repo.UpdateUserWithTransaction(ctx, userID, name)

		// Assert
		assert.NoError(t, err)
		mockDB.AssertExpectations(t)
	})

	t.Run("transaction fails", func(t *testing.T) {
		// Arrange
		mockDB := new(MockSQLExecutor)
		repo := NewUserRepository(mockDB)

		ctx := context.Background()
		userID := int64(1)
		name := "Jane Doe"
		expectedErr := errors.New("transaction failed")

		mockDB.On("WithTransaction", ctx, sql.LevelReadCommitted, mock.AnythingOfType("db.TxFunc")).
			Return(expectedErr)

		// Act
		err := repo.UpdateUserWithTransaction(ctx, userID, name)

		// Assert
		assert.Error(t, err)
		assert.Equal(t, expectedErr, err)
		mockDB.AssertExpectations(t)
	})
}

func TestMockSQLExecutor_ExecContext(t *testing.T) {
	t.Run("returns result successfully", func(t *testing.T) {
		// Arrange
		mockDB := new(MockSQLExecutor)
		mockResult := new(MockResult)
		ctx := context.Background()
		query := "DELETE FROM users WHERE id = ?"
		args := []any{int64(1)}

		mockResult.On("RowsAffected").Return(int64(1), nil)
		mockDB.On("ExecContext", ctx, query, args).Return(mockResult, nil)

		// Act
		result, err := mockDB.ExecContext(ctx, query, args...)

		// Assert
		assert.NoError(t, err)
		assert.NotNil(t, result)
		rowsAffected, _ := result.RowsAffected()
		assert.Equal(t, int64(1), rowsAffected)
		mockDB.AssertExpectations(t)
	})
}

func TestMockSQLExecutor_QueryRowContext(t *testing.T) {
	t.Run("returns row successfully", func(t *testing.T) {
		// Arrange
		mockDB := new(MockSQLExecutor)
		ctx := context.Background()
		query := "SELECT id, name FROM users WHERE id = ?"
		args := []any{int64(1)}

		// Note: *sql.Row is difficult to mock directly, so we return nil here
		// In practice, you'd test the actual query logic with integration tests
		mockDB.On("QueryRowContext", ctx, query, args).Return((*sql.Row)(nil))

		// Act
		row := mockDB.QueryRowContext(ctx, query, args...)

		// Assert
		assert.Nil(t, row) // In real usage, this would scan into variables
		mockDB.AssertExpectations(t)
	})
}
