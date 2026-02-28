package auth

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"

	gen "gosdk/internal/db/gen"
	generated "gosdk/internal/db/gen"
	"gosdk/pkg/db"
)

// UserRepository handles database operations for users using type-safe queries
type UserRepository struct {
	queries *gen.Queries
}

// NewUserRepository creates a new UserRepository with generated type-safe queries
func NewUserRepository(database db.SQLExecutor) *UserRepository {
	return &UserRepository{
		queries: gen.New(database),
	}
}

// GetByProviderAndSubject retrieves a user by OAuth provider and subject ID
func (r *UserRepository) GetByProviderAndSubject(ctx context.Context, provider, subjectID string) (*gen.User, error) {
	user, err := r.queries.GetUserByProviderAndSubject(ctx, gen.GetUserByProviderAndSubjectParams{
		Provider:  provider,
		SubjectID: subjectID,
	})

	if err == sql.ErrNoRows {
		return nil, ErrUserNotFound
	}
	if err != nil {
		if db.IsTimeoutError(err) {
			return nil, fmt.Errorf("database timeout: %w", db.ErrDatabaseTimeout)
		}
		return nil, fmt.Errorf("failed to query user: %w", err)
	}

	return &user, nil
}

// Create inserts a new user into the database
func (r *UserRepository) Create(ctx context.Context, provider, subjectID, email, fullName string) (*generated.User, error) {
	userID := uuid.New()
	now := time.Now()

	user, err := r.queries.CreateUser(ctx, generated.CreateUserParams{
		ID:        userID,
		Provider:  provider,
		SubjectID: subjectID,
		Email:     email,
		FullName:  sql.NullString{String: fullName, Valid: fullName != ""},
		CreatedAt: sql.NullTime{Time: now, Valid: true},
		UpdatedAt: sql.NullTime{Time: now, Valid: true},
	})

	if err != nil {
		if db.IsDuplicateKeyError(err) {
			// User was created concurrently, try to fetch it
			return r.GetByProviderAndSubject(ctx, provider, subjectID)
		}
		if db.IsTimeoutError(err) {
			return nil, fmt.Errorf("database timeout: %w", db.ErrDatabaseTimeout)
		}
		return nil, fmt.Errorf("failed to insert user: %w", err)
	}

	return &user, nil
}

// GetOrCreate retrieves an existing user or creates a new one
func (r *UserRepository) GetOrCreate(ctx context.Context, provider, subjectID, email, fullName string) (*generated.User, error) {
	userID := uuid.New()
	now := time.Now()

	user, err := r.queries.GetOrCreateUser(ctx, generated.GetOrCreateUserParams{
		ID:        userID,
		Provider:  provider,
		SubjectID: subjectID,
		Email:     email,
		FullName:  sql.NullString{String: fullName, Valid: fullName != ""},
		CreatedAt: sql.NullTime{Time: now, Valid: true},
	})

	if err != nil {
		if db.IsTimeoutError(err) {
			return nil, fmt.Errorf("database timeout: %w", db.ErrDatabaseTimeout)
		}
		return nil, fmt.Errorf("failed to get or create user: %w", err)
	}

	return &user, nil
}

// GetByID retrieves a user by their unique ID
func (r *UserRepository) GetByID(ctx context.Context, id uuid.UUID) (*generated.User, error) {
	user, err := r.queries.GetUserByID(ctx, id)

	if err == sql.ErrNoRows {
		return nil, ErrUserNotFound
	}
	if err != nil {
		if db.IsTimeoutError(err) {
			return nil, fmt.Errorf("database timeout: %w", db.ErrDatabaseTimeout)
		}
		return nil, fmt.Errorf("failed to get user by ID: %w", err)
	}

	return &user, nil
}

// Update updates an existing user's information
func (r *UserRepository) Update(ctx context.Context, id uuid.UUID, email, fullName string) (*generated.User, error) {
	user, err := r.queries.UpdateUser(ctx, generated.UpdateUserParams{
		ID:        id,
		Email:     email,
		FullName:  sql.NullString{String: fullName, Valid: fullName != ""},
		UpdatedAt: sql.NullTime{Time: time.Now(), Valid: true},
	})

	if err == sql.ErrNoRows {
		return nil, ErrUserNotFound
	}
	if err != nil {
		if db.IsTimeoutError(err) {
			return nil, fmt.Errorf("database timeout: %w", db.ErrDatabaseTimeout)
		}
		return nil, fmt.Errorf("failed to update user: %w", err)
	}

	return &user, nil
}

// Delete removes a user from the database
func (r *UserRepository) Delete(ctx context.Context, id uuid.UUID) error {
	err := r.queries.DeleteUser(ctx, id)
	if err != nil {
		if db.IsTimeoutError(err) {
			return fmt.Errorf("database timeout: %w", db.ErrDatabaseTimeout)
		}
		return fmt.Errorf("failed to delete user: %w", err)
	}
	return nil
}
