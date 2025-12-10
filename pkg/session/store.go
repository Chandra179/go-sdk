package session

import (
	"errors"
	"sync"
	"time"
)

var (
	ErrSessionNotFound = errors.New("session not found")
	ErrSessionExpired  = errors.New("session expired")
)

// Session represents a generic session with binary data
type Session struct {
	ID           string    `json:"id"`
	Data         []byte    `json:"data"` // Generic binary data
	CreatedAt    time.Time `json:"created_at"`
	ExpiresAt    time.Time `json:"expires_at"`
	LastAccessed time.Time `json:"last_accessed"`
}

// Store interface for managing sessions
type Store interface {
	Create(data []byte, ttl time.Duration) (*Session, error)
	Get(sessionID string) (*Session, error)
	Update(sessionID string, data []byte) error
	Delete(sessionID string) error
}

// InMemoryStore implements Store interface
type InMemoryStore struct {
	mu       sync.RWMutex
	sessions map[string]*Session
	done     chan struct{}
	once     sync.Once
}

// NewInMemoryStore creates a new in-memory session store
func NewInMemoryStore() *InMemoryStore {
	store := &InMemoryStore{
		sessions: make(map[string]*Session),
		done:     make(chan struct{}),
	}
	go store.cleanupRoutine()
	return store
}

// Create creates a new session with the given data and TTL
func (s *InMemoryStore) Create(data []byte, ttl time.Duration) (*Session, error) {
	sessionID, err := generateRandomString(32)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	session := &Session{
		ID:           sessionID,
		Data:         data,
		CreatedAt:    now,
		ExpiresAt:    now.Add(ttl),
		LastAccessed: now,
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[sessionID] = session

	return session, nil
}

// Get retrieves a session by ID and updates last accessed time
func (s *InMemoryStore) Get(sessionID string) (*Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	session, exists := s.sessions[sessionID]
	if !exists {
		return nil, ErrSessionNotFound
	}

	if time.Now().After(session.ExpiresAt) {
		delete(s.sessions, sessionID)
		return nil, ErrSessionExpired
	}

	// Update last accessed time
	session.LastAccessed = time.Now()

	return session, nil
}

// Update updates session data
func (s *InMemoryStore) Update(sessionID string, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	session, exists := s.sessions[sessionID]
	if !exists {
		return ErrSessionNotFound
	}

	if time.Now().After(session.ExpiresAt) {
		delete(s.sessions, sessionID)
		return ErrSessionExpired
	}

	session.Data = data
	session.LastAccessed = time.Now()

	return nil
}

// Delete removes a session
func (s *InMemoryStore) Delete(sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, sessionID)
	return nil
}

// cleanupRoutine periodically removes expired sessions
func (s *InMemoryStore) cleanupRoutine() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.removeExpired()
		case <-s.done:
			return
		}
	}
}

// removeExpired removes all expired sessions
func (s *InMemoryStore) removeExpired() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for id, session := range s.sessions {
		if now.After(session.ExpiresAt) {
			delete(s.sessions, id)
		}
	}
}
