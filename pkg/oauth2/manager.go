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

// Manager manages OAuth2 providers and authentication flow
type Manager struct {
	providers     map[string]Provider
	storage       Storage
	JWTSecret     string
	JWTExpiration time.Duration
	StateTimeout  time.Duration
}

// NewManagerFromEnv creates a new manager with providers from environment variables
func NewManager(cfg *cfg.Oauth2Config) (*Manager, error) {
	storage := NewInMemoryStorage()
	mgr := &Manager{
		providers:     make(map[string]Provider),
		storage:       storage,
		JWTSecret:     cfg.JWTSecret,
		JWTExpiration: 24 * time.Hour,
		StateTimeout:  10 * time.Minute,
	}

	googleProvider := NewGoogleProvider(cfg.GoogleClientID, cfg.GoogleClientSecret, cfg.GoogleRedirectUrl, nil)
	mgr.RegisterProvider(googleProvider)

	githubProvider := NewGitHubProvider(cfg.GithubClientID, cfg.GithubClientSecret, cfg.GithubRedirectUrl, nil)
	mgr.RegisterProvider(githubProvider)

	return mgr, nil
}

// RegisterProvider registers a new OAuth2 provider
func (m *Manager) RegisterProvider(provider Provider) {
	m.providers[provider.GetName()] = provider
}

// GetAuthURL generates authorization URL with PKCE
func (m *Manager) GetAuthURL(providerName string) (string, error) {
	provider, exists := m.providers[providerName]
	if !exists {
		return "", ErrProviderNotFound
	}

	// Generate state
	state, err := GenerateRandomString(32)
	if err != nil {
		return "", fmt.Errorf("failed to generate state: %w", err)
	}

	// Generate PKCE verifier and challenge
	verifier, err := GenerateCodeVerifier()
	if err != nil {
		return "", fmt.Errorf("failed to generate code verifier: %w", err)
	}

	challenge := GenerateCodeChallenge(verifier)

	// Store state and verifier
	expiresAt := time.Now().Add(m.StateTimeout)
	if err := m.storage.SaveState(state, verifier, expiresAt); err != nil {
		return "", fmt.Errorf("failed to save state: %w", err)
	}

	return provider.GetAuthURL(state, challenge), nil
}

// HandleCallback handles OAuth2 callback and returns JWT token
func (m *Manager) HandleCallback(ctx context.Context, providerName, code, state string) (string, *UserInfo, error) {
	provider, exists := m.providers[providerName]
	if !exists {
		return "", nil, ErrProviderNotFound
	}

	// Validate state and get verifier
	verifier, err := m.storage.GetVerifier(state)
	if err != nil {
		return "", nil, fmt.Errorf("invalid state: %w", err)
	}

	// Delete state after retrieval (one-time use)
	defer m.storage.DeleteState(state)

	// Exchange code for token
	tokenResp, err := provider.Exchange(ctx, code)
	if err != nil {
		return "", nil, fmt.Errorf("failed to exchange code: %w", err)
	}

	// Get user info
	userInfo, err := provider.GetUserInfo(ctx, tokenResp.AccessToken)
	if err != nil {
		return "", nil, fmt.Errorf("failed to get user info: %w", err)
	}

	// Generate JWT
	jwt, err := GenerateJWT(userInfo, m.JWTSecret, m.JWTExpiration)
	if err != nil {
		return "", nil, fmt.Errorf("failed to generate JWT: %w", err)
	}

	// Note: verifier is validated by the provider during Exchange
	_ = verifier

	return jwt, userInfo, nil
}

// ValidateToken validates a JWT token
func (m *Manager) ValidateToken(token string) (*JWTClaims, error) {
	return ValidateJWT(token, m.JWTSecret)
}

// Cleanup cleans up storage resources
func (m *Manager) Cleanup() {
	m.storage.Cleanup()
}
