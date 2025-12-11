package authservice

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gosdk/pkg/db"
	"gosdk/pkg/oauth2"
	"gosdk/pkg/session"

	"github.com/google/uuid"
)

type Service struct {
	oauth2Manager *oauth2.Manager
	sessionStore  session.Store
	db            db.SQLExecutor
}

func NewService(oauth2Manager *oauth2.Manager, sessionStore session.Store, db db.SQLExecutor) *Service {
	return &Service{
		oauth2Manager: oauth2Manager,
		sessionStore:  sessionStore,
		db:            db,
	}
}

// InitiateLogin generates the OAuth2 authorization URL
func (s *Service) InitiateLogin(provider string) (string, error) {
	return s.oauth2Manager.GetAuthURL(provider)
}

// HandleCallback processes OAuth2 callback and creates a session
// This is the callback handler that gets invoked by oauth2.Manager
func (s *Service) HandleCallback(ctx context.Context, provider string,
	userInfo *oauth2.UserInfo, tokenSet *oauth2.TokenSet) (*oauth2.CallbackInfo, error) {
	internalUserID, err := s.GetOrCreateUser(ctx, provider, userInfo.ID, userInfo.Email, userInfo.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve user: %w", err)
	}

	sessionData := &SessionData{
		UserID: internalUserID,
	}

	data, err := json.Marshal(sessionData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal session data: %w", err)
	}

	sess, err := s.sessionStore.Create(data, SessionTimeout)
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	return &oauth2.CallbackInfo{
		SessionID:         sess.ID,
		UserID:            internalUserID,
		SessionCookieName: SessionCookieName,
		CookieMaxAge:      CookieMaxAge,
	}, nil
}

// GetSessionData retrieves and unmarshals session data
func (s *Service) GetSessionData(sessionID string) (*SessionData, error) {
	sess, err := s.sessionStore.Get(sessionID)
	if err != nil {
		return nil, err
	}

	var sessionData SessionData
	if err := json.Unmarshal(sess.Data, &sessionData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal session data: %w", err)
	}

	return &sessionData, nil
}

// RefreshToken refreshes the access token for a session
func (s *Service) RefreshToken(ctx context.Context, sessionID string) error {
	sessionData, err := s.GetSessionData(sessionID)
	if err != nil {
		return err
	}

	if sessionData.TokenSet.RefreshToken == "" {
		return errors.New("no refresh token available")
	}

	provider, err := s.oauth2Manager.GetProvider(sessionData.Provider)
	if err != nil {
		return err
	}

	googleProvider, ok := provider.(*oauth2.GoogleOIDCProvider)
	if !ok {
		return errors.New("provider does not support token refresh")
	}

	newTokenSet, err := googleProvider.RefreshToken(ctx, sessionData.TokenSet.RefreshToken)
	if err != nil {
		s.sessionStore.Delete(sessionID)
		return fmt.Errorf("failed to refresh token (session deleted): %w", err)
	}

	sessionData.TokenSet = newTokenSet
	data, err := json.Marshal(sessionData)
	if err != nil {
		return fmt.Errorf("failed to marshal session data: %w", err)
	}

	return s.sessionStore.Update(sessionID, data)
}

// ValidateAndRefreshSession validates a session and refreshes tokens if needed
func (s *Service) ValidateAndRefreshSession(ctx context.Context, sessionID string) (*SessionData, error) {
	sessionData, err := s.GetSessionData(sessionID)
	if err != nil {
		return nil, err
	}

	if time.Until(sessionData.TokenSet.ExpiresAt) <= TokenRefreshLeeway {
		if err := s.RefreshToken(ctx, sessionID); err != nil {
			return nil, err
		}

		sessionData, err = s.GetSessionData(sessionID)
		if err != nil {
			return nil, err
		}
	}

	return sessionData, nil
}

// GetOrCreateUser finds existing user by federated identity or creates new user with federated identity
func (s *Service) GetOrCreateUser(ctx context.Context, provider, subjectID, email, fullName string) (string, error) {
	fedIdentity, err := s.getFederatedIdentity(ctx, provider, subjectID)
	if err == nil {
		user, err := s.getUserByID(ctx, fedIdentity.UserID)
		if err != nil {
			return "", fmt.Errorf("failed to get user: %w", err)
		}
		return user.ID, nil
	}

	if !errors.Is(err, ErrFederatedIdentityNotFound) {
		return "", fmt.Errorf("failed to check federated identity: %w", err)
	}

	// No existing identity found, create new user and federated identity in transaction
	var userID string
	err = s.db.WithTransaction(ctx, sql.LevelReadCommitted, func(ctx context.Context, tx *sql.Tx) error {
		userID := uuid.NewString()
		now := time.Now()

		insertUserQuery := `
			INSERT INTO users (id, email, full_name, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5)
		`
		_, err := tx.ExecContext(ctx, insertUserQuery, userID, email, fullName, now, now)
		if err != nil {
			return fmt.Errorf("failed to insert user: %w", err)
		}

		insertFedIdentityQuery := `
			INSERT INTO federated_identities (provider, subject_id, user_id, last_login_at)
			VALUES ($1, $2, $3, $4)
		`
		_, err = tx.ExecContext(ctx, insertFedIdentityQuery, provider, subjectID, userID, now)
		if err != nil {
			return fmt.Errorf("failed to insert federated identity: %w", err)
		}

		return nil
	})

	if err != nil {
		return "", err
	}

	return userID, nil
}

// getUserByID retrieves a user by their internal ID
func (s *Service) getUserByID(ctx context.Context, userID string) (*User, error) {
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
		SELECT provider, subject_id, user_id
		FROM federated_identities
		WHERE provider = $1 AND subject_id = $2
	`

	var fedIdentity FederatedIdentity
	err := s.db.QueryRowContext(ctx, query, provider, subjectID).Scan(
		&fedIdentity.Provider,
		&fedIdentity.SubjectID,
		&fedIdentity.UserID,
	)

	if err == sql.ErrNoRows {
		return nil, ErrFederatedIdentityNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query federated identity: %w", err)
	}

	return &fedIdentity, nil
}

// DeleteSession deletes a session (logout)
func (s *Service) DeleteSession(sessionID string) error {
	return s.sessionStore.Delete(sessionID)
}
