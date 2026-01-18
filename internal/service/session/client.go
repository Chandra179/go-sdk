package session

import (
	"errors"
	"time"
)

var (
	ErrSessionNotFound = errors.New("session not found")
	ErrSessionExpired  = errors.New("session expired")
)

type Session struct {
	ID           string    `json:"id"`
	Data         []byte    `json:"data"`
	CreatedAt    time.Time `json:"created_at"`
	ExpiresAt    time.Time `json:"expires_at"`
	LastAccessed time.Time `json:"last_accessed"`
}

type Client interface {
	Create(data []byte, ttl time.Duration) (*Session, error)
	Get(sessionID string) (*Session, error)
	Update(sessionID string, data []byte) error
	Delete(sessionID string) error
}
