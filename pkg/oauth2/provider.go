// Package oauth2 provides OAuth2 and OIDC authentication support.
//
// This package implements secure OAuth2 flows with PKCE (Proof Key for Code Exchange)
// and OIDC (OpenID Connect) support. It is designed to be extensible for multiple
// identity providers.
//
// Basic usage:
//
//	config := &oauth2.ManagerConfig{
//	    StateTimeout:    10 * time.Minute,
//	    CallbackHandler: myCallbackHandler,
//	}
//
//	manager, err := oauth2.NewManagerWithGoogle(ctx, config, clientID, clientSecret, redirectURL)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Generate auth URL
//	authURL, err := manager.GetAuthURL("google")
//
//	// In your HTTP handler
//	router.GET("/auth/callback/google", oauth2.GoogleCallbackHandler(manager, nil))
//
// Security features:
//   - PKCE (RFC 7636) protection against authorization code interception
//   - State parameter for CSRF protection
//   - Nonce validation for replay attack prevention
//   - Secure, HTTP-only cookies for session management
//   - Thread-safe operations
//
// Thread safety:
//   - Manager methods are safe for concurrent use
//   - StateStorage implementations are safe for concurrent use
//   - Provider implementations should be safe for concurrent use
package oauth2

import (
	"context"
	"time"
)

// Provider defines the interface for OAuth2/OIDC authentication providers.
// Implementations must be safe for concurrent use.
type Provider interface {
	// GetName returns the unique identifier for this provider (e.g., "google", "github")
	GetName() string

	// GetAuthURL generates the authorization URL for the OAuth2 flow.
	// Parameters:
	//   - state: CSRF protection token
	//   - nonce: OIDC nonce for replay attack prevention
	//   - codeChallenge: PKCE code challenge (S256 method)
	// Returns the complete authorization URL
	GetAuthURL(state string, nonce string, codeChallenge string) string

	// HandleCallback processes the OAuth2 callback after user authorization.
	// Parameters:
	//   - ctx: context for cancellation and timeouts
	//   - code: authorization code from OAuth2 provider
	//   - state: state parameter for CSRF validation
	//   - nonce: nonce for ID token validation
	//   - codeVerifier: PKCE code verifier
	// Returns user information and token set on success
	HandleCallback(ctx context.Context, code string, state string, nonce string, codeVerifier string) (*UserInfo, *TokenSet, error)

	// RefreshToken exchanges a refresh token for a new access token.
	// Parameters:
	//   - ctx: context for cancellation and timeouts
	//   - refreshToken: the refresh token obtained from initial authentication
	// Returns new token set on success
	RefreshToken(ctx context.Context, refreshToken string) (*TokenSet, error)
}

// UserInfo represents unified user information across all OAuth2 providers.
// All fields are JSON-serializable for API responses.
type UserInfo struct {
	// ID is the unique identifier from the provider (e.g., Google sub claim)
	ID string `json:"id"`

	// Email is the user's email address (if available)
	Email string `json:"email"`

	// Name is the user's display name (if available)
	Name string `json:"name"`

	// Provider identifies which OAuth2 provider authenticated this user
	Provider string `json:"provider"`

	// CreatedAt is the timestamp when this UserInfo was created
	CreatedAt time.Time `json:"created_at"`
}

// TokenSet represents the complete OAuth2 token response from providers.
// This includes access tokens, refresh tokens, and OIDC ID tokens.
type TokenSet struct {
	// AccessToken is the OAuth2 access token for API calls
	AccessToken string `json:"access_token"`

	// TokenType is typically "Bearer"
	TokenType string `json:"token_type"`

	// RefreshToken is used to obtain new access tokens (optional)
	RefreshToken string `json:"refresh_token,omitempty"`

	// IDToken is the OIDC ID token containing user claims (optional)
	IDToken string `json:"id_token,omitempty"`

	// ExpiresAt is the access token expiration time
	ExpiresAt time.Time `json:"expires_at"`
}

// IsExpired returns true if the access token has expired
func (t *TokenSet) IsExpired() bool {
	return time.Now().After(t.ExpiresAt)
}

// IsValid returns true if the token set contains a non-empty access token
func (t *TokenSet) IsValid() bool {
	return t != nil && t.AccessToken != ""
}
