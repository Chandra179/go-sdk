package user

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"gosdk/pkg/db"

	"github.com/google/uuid"
)

var (
	ErrUserNotFound              = errors.New("user not found")
	ErrFederatedIdentityNotFound = errors.New("federated identity not found")
)

// User represents a user in the system
type User struct {
	ID        uuid.UUID `json:"id"`
	Email     string    `json:"email"`
	FullName  string    `json:"full_name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// FederatedIdentity represents a linked OAuth provider identity
type FederatedIdentity struct {
	Provider    string    `json:"provider"`
	SubjectID   string    `json:"subject_id"`
	UserID      uuid.UUID `json:"user_id"`
	LastLoginAt time.Time `json:"last_login_at"`
}

// Service handles user business logic
type Service struct {
	db db.SQLExecutor
}

// NewService creates a new user service
func NewService(db db.SQLExecutor) *Service {
	return &Service{db: db}
}

// ResolveUser is the callback function for OAuth2 manager
// It returns the internal user ID (as string) for the given federated identity
func (s *Service) ResolveUser(ctx context.Context, provider, subjectID, email, fullName string) (string, error) {
	user, err := s.GetOrCreateUser(ctx, provider, subjectID, email, fullName)
	if err != nil {
		return "", err
	}
	return user.ID.String(), nil
}

// GetOrCreateUser finds existing user by federated identity or creates new user with federated identity
func (s *Service) GetOrCreateUser(ctx context.Context, provider, subjectID, email, fullName string) (*User, error) {
	// First, try to find existing federated identity
	fedIdentity, err := s.getFederatedIdentity(ctx, provider, subjectID)
	if err == nil {
		// Found existing identity, get the user
		user, err := s.getUserByID(ctx, fedIdentity.UserID)
		if err != nil {
			return nil, fmt.Errorf("failed to get user: %w", err)
		}

		// Update last login
		if err := s.updateLastLogin(ctx, provider, subjectID); err != nil {
			// Log but don't fail - this is non-critical
			fmt.Printf("warning: failed to update last login: %v\n", err)
		}

		return user, nil
	}

	if !errors.Is(err, ErrFederatedIdentityNotFound) {
		return nil, fmt.Errorf("failed to check federated identity: %w", err)
	}

	// No existing identity found, create new user and federated identity in transaction
	var user *User
	err = s.db.WithTransaction(ctx, sql.LevelReadCommitted, func(ctx context.Context, tx *sql.Tx) error {
		// Create new user
		userID := uuid.New()
		now := time.Now()

		insertUserQuery := `
			INSERT INTO users (id, email, full_name, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5)
		`
		_, err := tx.ExecContext(ctx, insertUserQuery, userID, email, fullName, now, now)
		if err != nil {
			return fmt.Errorf("failed to insert user: %w", err)
		}

		// Create federated identity
		insertFedIdentityQuery := `
			INSERT INTO federated_identities (provider, subject_id, user_id, last_login_at)
			VALUES ($1, $2, $3, $4)
		`
		_, err = tx.ExecContext(ctx, insertFedIdentityQuery, provider, subjectID, userID, now)
		if err != nil {
			return fmt.Errorf("failed to insert federated identity: %w", err)
		}

		user = &User{
			ID:        userID,
			Email:     email,
			FullName:  fullName,
			CreatedAt: now,
			UpdatedAt: now,
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return user, nil
}

// getUserByID retrieves a user by their internal ID
func (s *Service) getUserByID(ctx context.Context, userID uuid.UUID) (*User, error) {
	query := `
		SELECT id, email, full_name, created_at, updated_at
		FROM users
		WHERE id = $1
	`

	var user User
	err := s.db.QueryRowContext(ctx, query, userID).Scan(
		&user.ID,
		&user.Email,
		&user.FullName,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query user: %w", err)
	}

	return &user, nil
}

// getFederatedIdentity retrieves federated identity by provider and subject
func (s *Service) getFederatedIdentity(ctx context.Context, provider, subjectID string) (*FederatedIdentity, error) {
	query := `
		SELECT provider, subject_id, user_id, last_login_at
		FROM federated_identities
		WHERE provider = $1 AND subject_id = $2
	`

	var fedIdentity FederatedIdentity
	err := s.db.QueryRowContext(ctx, query, provider, subjectID).Scan(
		&fedIdentity.Provider,
		&fedIdentity.SubjectID,
		&fedIdentity.UserID,
		&fedIdentity.LastLoginAt,
	)

	if err == sql.ErrNoRows {
		return nil, ErrFederatedIdentityNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query federated identity: %w", err)
	}

	return &fedIdentity, nil
}

// updateLastLogin updates the last login timestamp for a federated identity
func (s *Service) updateLastLogin(ctx context.Context, provider, subjectID string) error {
	query := `
		UPDATE federated_identities
		SET last_login_at = $1
		WHERE provider = $2 AND subject_id = $3
	`

	_, err := s.db.ExecContext(ctx, query, time.Now(), provider, subjectID)
	if err != nil {
		return fmt.Errorf("failed to update last login: %w", err)
	}

	return nil
}
