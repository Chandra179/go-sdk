package auth

import (
	"errors"
	"gosdk/pkg/oauth2"
	"time"
)

// SessionData represents the data stored in a session
// This is what gets marshaled/unmarshaled to/from the session store
type SessionData struct {
	UserID   string           `json:"user_id"`
	TokenSet *oauth2.TokenSet `json:"token_set"`
	Provider string           `json:"provider"`
}

// LoginRequest represents the login request body
type LoginRequest struct {
	Provider string `json:"provider" binding:"required"` // "google" or "github"
}

const (
	SessionCookieName  = "session_id"
	CookieMaxAge       = 86400           // 24 hours
	TokenRefreshLeeway = 5 * time.Minute // Refresh tokens 5 minutes before expiry
	SessionTimeout     = 24 * time.Hour  // Session timeout
)

var (
	ErrUserNotFound = errors.New("user not found")
)

// User represents a user in the system
type User struct {
	ID        string    `json:"id"`
	Provider  string    `json:"provider"`
	SubjectID string    `json:"subject_id"`
	Email     string    `json:"email"`
	FullName  string    `json:"full_name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
