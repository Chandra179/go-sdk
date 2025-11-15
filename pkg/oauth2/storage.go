package oauth2

import (
	"errors"
	"sync"
	"time"
)

var (
	ErrStateNotFound    = errors.New("state not found")
	ErrVerifierNotFound = errors.New("verifier not found")
	ErrStateExpired     = errors.New("state expired")
)

// Storage interface for state and code verifier management
type Storage interface {
	SaveState(state string, verifier string, expiresAt time.Time) error
	GetVerifier(state string) (string, error)
	DeleteState(state string) error
	Cleanup()
}

// InMemoryStorage implements Storage interface
type InMemoryStorage struct {
	mu   sync.RWMutex
	data map[string]*stateData
	done chan struct{}
	once sync.Once
}

type stateData struct {
	verifier  string
	expiresAt time.Time
}

func NewInMemoryStorage() *InMemoryStorage {
	s := &InMemoryStorage{
		data: make(map[string]*stateData),
		done: make(chan struct{}),
	}
	go s.cleanupRoutine()
	return s
}

func (s *InMemoryStorage) SaveState(state string, verifier string, expiresAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[state] = &stateData{
		verifier:  verifier,
		expiresAt: expiresAt,
	}
	return nil
}

func (s *InMemoryStorage) GetVerifier(state string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, exists := s.data[state]
	if !exists {
		return "", ErrStateNotFound
	}

	if time.Now().After(data.expiresAt) {
		return "", ErrStateExpired
	}

	return data.verifier, nil
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
		if now.After(data.expiresAt) {
			delete(s.data, state)
		}
	}
}
