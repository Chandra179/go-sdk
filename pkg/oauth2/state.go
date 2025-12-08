package oauth2

import (
	"errors"
	"sync"
	"time"
)

var (
	ErrStateNotFound = errors.New("state not found")
	ErrStateExpired  = errors.New("state expired")
)

// StateStorage interface for state, nonce, and PKCE verifier management
type StateStorage interface {
	SaveState(state string, nonce string, codeVerifier string, expiresAt time.Time) error
	GetStateData(state string) (*StateData, error)
	DeleteState(state string) error
	Cleanup()
}

// StateData holds all security parameters for OAuth2 flow
type StateData struct {
	Nonce        string
	CodeVerifier string
	ExpiresAt    time.Time
}

// InMemoryStorage implements Storage interface
type InMemoryStorage struct {
	mu   sync.RWMutex
	data map[string]*StateData
	done chan struct{}
	once sync.Once
}

func NewInMemoryStorage() *InMemoryStorage {
	s := &InMemoryStorage{
		data: make(map[string]*StateData),
		done: make(chan struct{}),
	}
	go s.cleanupRoutine()
	return s
}

func (s *InMemoryStorage) SaveState(state string, nonce string, codeVerifier string, expiresAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[state] = &StateData{
		Nonce:        nonce,
		CodeVerifier: codeVerifier,
		ExpiresAt:    expiresAt,
	}
	return nil
}

func (s *InMemoryStorage) GetStateData(state string) (*StateData, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, exists := s.data[state]
	if !exists {
		return nil, ErrStateNotFound
	}

	if time.Now().After(data.ExpiresAt) {
		return nil, ErrStateExpired
	}

	return data, nil
}

func (s *InMemoryStorage) DeleteState(state string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, state)
	return nil
}

func (s *InMemoryStorage) Cleanup() {
	s.once.Do(func() {
		close(s.done)
	})
}

func (s *InMemoryStorage) cleanupRoutine() {
	ticker := time.NewTicker(5 * time.Minute)
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

func (s *InMemoryStorage) removeExpired() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for state, data := range s.data {
		if now.After(data.ExpiresAt) {
			delete(s.data, state)
		}
	}
}
