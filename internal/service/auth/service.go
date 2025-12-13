package auth

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

// GetOrCreateUser finds existing user by provider and subject_id or creates new user
func (s *Service) GetOrCreateUser(ctx context.Context, provider, subjectID, email, fullName string) (string, error) {
	user, err := s.getUserByProviderAndSubject(ctx, provider, subjectID)
	if err == nil {
		return user.ID, nil
	}

	if !errors.Is(err, ErrUserNotFound) {
		return "", fmt.Errorf("failed to check user: %w", err)
	}

	userID := uuid.NewString()
	now := time.Now()

	insertUserQuery := `
		INSERT INTO users (id, provider, subject_id, email, full_name, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`
	_, err = s.db.ExecContext(ctx, insertUserQuery, userID, provider, subjectID, email, fullName, now, now)
	if err != nil {
		return "", fmt.Errorf("failed to insert user: %w", err)
	}

	return userID, nil
}

// getUserByProviderAndSubject retrieves a user by provider and subject_id
func (s *Service) getUserByProviderAndSubject(ctx context.Context, provider, subjectID string) (*User, error) {
	query := `
		SELECT id, provider, subject_id, email, full_name, created_at, updated_at
		FROM users
		WHERE provider = $1 AND subject_id = $2
	`

	var user User
	err := s.db.QueryRowContext(ctx, query, provider, subjectID).Scan(
		&user.ID,
		&user.Provider,
		&user.SubjectID,
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

// DeleteSession deletes a session (logout)
func (s *Service) DeleteSession(sessionID string) error {
	return s.sessionStore.Delete(sessionID)
}
