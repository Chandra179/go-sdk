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

type CallbackInfo struct {
	SessionCookieName string
	UserID            string
	SessionID         string
	CookieMaxAge      int
}

// CallbackHandler is invoked after successful OAuth2 authentication
// It receives the provider name, user info, and token set
// Returns custom data that can be used by the caller (e.g., session data)
type CallbackHandler func(ctx context.Context, provider string, userInfo *UserInfo, tokenSet *TokenSet) (*CallbackInfo, error)

// Manager manages OAuth2/OIDC providers and authentication flow
type Manager struct {
	providers       map[string]Provider
	stateStorage    StateStorage
	CallbackHandler CallbackHandler
	stateTimeout    time.Duration
}

// NewManager creates a new manager with providers from configuration
func NewManager(ctx context.Context, cfg *cfg.Oauth2Config) (*Manager, error) {
	mgr := &Manager{
		providers:    make(map[string]Provider),
		stateStorage: NewInMemoryStorage(),
		stateTimeout: 10 * time.Minute,
	}

	if cfg.GoogleClientID != "" && cfg.GoogleClientSecret != "" {
		googleProvider, err := NewGoogleOIDCProvider(
			ctx,
			cfg.GoogleClientID,
			cfg.GoogleClientSecret,
			cfg.GoogleRedirectUrl,
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

// HandleCallback handles OAuth2/OIDC callback and invokes the callback handler
// Returns userInfo, tokenSet, and any custom result from the callback handler
func (m *Manager) HandleCallback(ctx context.Context, providerName, code, state string) (*CallbackInfo, error) {
	provider, exists := m.providers[providerName]
	if !exists {
		return nil, ErrProviderNotFound
	}

	stateData, err := m.stateStorage.GetStateData(state)
	if err != nil {
		return nil, fmt.Errorf("invalid state: %w", err)
	}

	// Delete state after retrieval (one-time use)
	defer m.stateStorage.DeleteState(state)

	userInfo, tokenSet, err := provider.HandleCallback(ctx, code, state, stateData.Nonce, stateData.CodeVerifier)
	if err != nil {
		return nil, fmt.Errorf("callback failed: %w", err)
	}

	// Invoke callback handler
	info, err := m.CallbackHandler(ctx, providerName, userInfo, tokenSet)
	if err != nil {
		return nil, fmt.Errorf("callback handler failed: %w", err)
	}

	return info, nil
}

// GetProvider returns a provider by name
func (m *Manager) GetProvider(providerName string) (Provider, error) {
	provider, exists := m.providers[providerName]
	if !exists {
		return nil, ErrProviderNotFound
	}
	return provider, nil
}

// Cleanup cleans up storage resources
func (m *Manager) Cleanup() {
	m.stateStorage.Cleanup()
}
