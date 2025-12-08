package oauth2

import (
	"context"
	"errors"
	"fmt"
	"gosdk/cfg"
	"time"
)

var (
	ErrProviderNotFound = errors.New("provider not found")
	ErrInvalidState     = errors.New("invalid state")
)

// UserResolver is a callback function that resolves federated identity to internal user ID
type UserResolver func(ctx context.Context, provider, subjectID, email, fullName string) (string, error)

// Manager manages OAuth2/OIDC providers and authentication flow
type Manager struct {
	providers      map[string]Provider
	stateStorage   StateStorage
	sessionStore   SessionStore
	userResolver   UserResolver
	stateTimeout   time.Duration
	sessionTimeout time.Duration
}

// NewManager creates a new manager with providers from configuration
func NewManager(ctx context.Context, cfg *cfg.Oauth2Config, userResolver UserResolver) (*Manager, error) {
	mgr := &Manager{
		providers:      make(map[string]Provider),
		stateStorage:   NewInMemoryStorage(),
		sessionStore:   NewInMemorySessionStore(),
		userResolver:   userResolver,
		stateTimeout:   10 * time.Minute,
		sessionTimeout: 24 * time.Hour,
	}

	if cfg.GoogleClientID != "" && cfg.GoogleClientSecret != "" {
		googleProvider, err := NewGoogleOIDCProvider(
			ctx,
			cfg.GoogleClientID,
			cfg.GoogleClientSecret,
			cfg.GoogleRedirectUrl,
			cfg.GoogleLogoutUrl,
			nil,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create Google provider: %w", err)
		}
		mgr.RegisterProvider(googleProvider)
	}

	return mgr, nil
}

// RegisterProvider registers a new authentication provider
func (m *Manager) RegisterProvider(provider Provider) {
	m.providers[provider.GetName()] = provider
}

// GetAuthURL generates authorization URL with state, nonce, and PKCE
func (m *Manager) GetAuthURL(providerName string) (string, error) {
	provider, exists := m.providers[providerName]
	if !exists {
		return "", ErrProviderNotFound
	}

	state, err := GenerateRandomString(32)
	if err != nil {
		return "", fmt.Errorf("failed to generate state: %w", err)
	}

	nonce, err := GenerateRandomString(32)
	if err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	codeVerifier, err := GenerateCodeVerifier()
	if err != nil {
		return "", fmt.Errorf("failed to generate code verifier: %w", err)
	}
	codeChallenge := GenerateCodeChallenge(codeVerifier)

	// Store state, nonce, and code verifier
	expiresAt := time.Now().Add(m.stateTimeout)
	if err := m.stateStorage.SaveState(state, nonce, codeVerifier, expiresAt); err != nil {
		return "", fmt.Errorf("failed to save state: %w", err)
	}

	return provider.GetAuthURL(state, nonce, codeChallenge), nil
}

// HandleCallback handles OAuth2/OIDC callback and creates a session
func (m *Manager) HandleCallback(ctx context.Context, providerName, code, state string) (string, *UserInfo, error) {
	provider, exists := m.providers[providerName]
	if !exists {
		return "", nil, ErrProviderNotFound
	}

	stateData, err := m.stateStorage.GetStateData(state)
	if err != nil {
		return "", nil, fmt.Errorf("invalid state: %w", err)
	}

	// Delete state after retrieval (one-time use)
	defer m.stateStorage.DeleteState(state)

	userInfo, tokenSet, err := provider.HandleCallback(ctx, code, state, stateData.Nonce, stateData.CodeVerifier)
	if err != nil {
		return "", nil, fmt.Errorf("callback failed: %w", err)
	}

	// Resolve federated identity to internal user ID via callback
	if m.userResolver != nil {
		internalUserID, err := m.userResolver(ctx, providerName, userInfo.ID, userInfo.Email, userInfo.Name)
		if err != nil {
			return "", nil, fmt.Errorf("failed to resolve user: %w", err)
		}
		userInfo.ID = internalUserID
	}

	session, err := m.sessionStore.Create(userInfo, tokenSet, m.sessionTimeout)
	if err != nil {
		return "", nil, fmt.Errorf("failed to create session: %w", err)
	}

	return session.ID, userInfo, nil
}

// GetSession retrieves a session by ID
func (m *Manager) GetSession(sessionID string) (*Session, error) {
	return m.sessionStore.Get(sessionID)
}

// RefreshSession refreshes tokens for a session (only for OIDC providers)
func (m *Manager) RefreshSession(ctx context.Context, sessionID string) error {
	session, err := m.sessionStore.Get(sessionID)
	if err != nil {
		return err
	}

	if session.TokenSet.RefreshToken == "" {
		return errors.New("no refresh token available")
	}

	provider, exists := m.providers[session.UserInfo.Provider]
	if !exists {
		return ErrProviderNotFound
	}

	googleProvider, ok := provider.(*GoogleOIDCProvider)
	if !ok {
		return errors.New("provider does not support token refresh")
	}

	newTokenSet, err := googleProvider.RefreshToken(ctx, session.TokenSet.RefreshToken)
	if err != nil {
		// Refresh failed - delete session and force re-login
		// TODO: need to be detailed on this (logging, tracing why it failed, could be using retry if neccesary)
		m.DeleteSession(sessionID)
		return fmt.Errorf("failed to refresh token (session deleted): %w", err)
	}

	return m.sessionStore.Update(sessionID, newTokenSet)
}

// DeleteSession deletes a session (logout)
func (m *Manager) DeleteSession(sessionID string) error {
	return m.sessionStore.Delete(sessionID)
}

// Cleanup cleans up storage resources
func (m *Manager) Cleanup() {
	m.stateStorage.Cleanup()
	m.sessionStore.Cleanup()
}
