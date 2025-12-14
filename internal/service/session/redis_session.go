package session

import (
	"context"
	"encoding/json"
	"time"

	"gosdk/pkg/cache"

	"github.com/google/uuid"
)

const (
	sessionPrefix = "session:"
	defaultTTL    = 24 * time.Hour
)

// RedisStore implements Store interface using Redis cache
type RedisStore struct {
	cache cache.Cache
	ctx   context.Context
}

// NewRedisStore creates a new Redis-backed session store
func NewRedisStore(cache cache.Cache) Client {
	return &RedisStore{
		cache: cache,
		ctx:   context.Background(),
	}
}

// Create creates a new session with the given data and TTL
func (s *RedisStore) Create(data []byte, ttl time.Duration) (*Session, error) {
	if ttl == 0 {
		ttl = defaultTTL
	}

	now := time.Now()
	session := &Session{
		ID:           uuid.New().String(),
		Data:         data,
		CreatedAt:    now,
		ExpiresAt:    now.Add(ttl),
		LastAccessed: now,
	}

	// Serialize session to JSON
	sessionData, err := json.Marshal(session)
	if err != nil {
		return nil, err
	}

	// Store in Redis with TTL
	key := sessionPrefix + session.ID
	if err := s.cache.Set(s.ctx, key, string(sessionData), ttl); err != nil {
		return nil, err
	}

	return session, nil
}

// Get retrieves a session by ID
func (s *RedisStore) Get(sessionID string) (*Session, error) {
	key := sessionPrefix + sessionID

	// Retrieve from Redis
	data, err := s.cache.Get(s.ctx, key)
	if err != nil {
		return nil, ErrSessionNotFound
	}

	// Deserialize session
	var session Session
	if err := json.Unmarshal([]byte(data), &session); err != nil {
		return nil, err
	}

	// Check if session is expired
	if time.Now().After(session.ExpiresAt) {
		s.cache.Del(s.ctx, key)
		return nil, ErrSessionExpired
	}

	// Update last accessed time
	session.LastAccessed = time.Now()

	// Save updated session back to Redis (with remaining TTL)
	remainingTTL := time.Until(session.ExpiresAt)
	if remainingTTL > 0 {
		sessionData, err := json.Marshal(session)
		if err != nil {
			return nil, err
		}
		s.cache.Set(s.ctx, key, string(sessionData), remainingTTL)
	}

	return &session, nil
}

// Update updates the data of an existing session
func (s *RedisStore) Update(sessionID string, data []byte) error {
	// Get existing session
	session, err := s.Get(sessionID)
	if err != nil {
		return err
	}

	// Update the data
	session.Data = data
	session.LastAccessed = time.Now()

	// Serialize and save
	sessionData, err := json.Marshal(session)
	if err != nil {
		return err
	}

	key := sessionPrefix + sessionID
	remainingTTL := time.Until(session.ExpiresAt)
	if remainingTTL <= 0 {
		return ErrSessionExpired
	}

	return s.cache.Set(s.ctx, key, string(sessionData), remainingTTL)
}

// Delete removes a session from the store
func (s *RedisStore) Delete(sessionID string) error {
	key := sessionPrefix + sessionID
	return s.cache.Del(s.ctx, key)
}

// GetWithContext retrieves a session with a custom context
func (s *RedisStore) GetWithContext(ctx context.Context, sessionID string) (*Session, error) {
	key := sessionPrefix + sessionID

	data, err := s.cache.Get(ctx, key)
	if err != nil {
		return nil, ErrSessionNotFound
	}

	var session Session
	if err := json.Unmarshal([]byte(data), &session); err != nil {
		return nil, err
	}

	if time.Now().After(session.ExpiresAt) {
		s.cache.Del(ctx, key)
		return nil, ErrSessionExpired
	}

	session.LastAccessed = time.Now()
	remainingTTL := time.Until(session.ExpiresAt)
	if remainingTTL > 0 {
		sessionData, err := json.Marshal(session)
		if err != nil {
			return nil, err
		}
		s.cache.Set(ctx, key, string(sessionData), remainingTTL)
	}

	return &session, nil
}
