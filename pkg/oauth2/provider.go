package oauth2

import (
	"context"
	"time"
)

// Provider defines the interface for OAuth2 providers
type Provider interface {
	GetAuthURL(state, codeChallenge string) string
	Exchange(ctx context.Context, code string) (*TokenResponse, error)
	GetUserInfo(ctx context.Context, accessToken string) (*UserInfo, error)
	GetName() string
}

// UserInfo represents unified user information across providers
type UserInfo struct {
	ID            string    `json:"id"`
	Email         string    `json:"email"`
	EmailVerified bool      `json:"email_verified"`
	Name          string    `json:"name"`
	Picture       string    `json:"picture"`
	Provider      string    `json:"provider"`
	CreatedAt     time.Time `json:"created_at"`
}

// TokenResponse represents OAuth2 token response
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token,omitempty"`
	IDToken      string `json:"id_token,omitempty"`
}
