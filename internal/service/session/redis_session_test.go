package session

import (
	"context"
	"testing"
	"time"

	"gosdk/pkg/cache"

	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestRedisSession(t *testing.T) (*miniredis.Miniredis, Client) {
	t.Helper()

	mr, err := miniredis.Run()
	require.NoError(t, err)

	redisCache := cache.NewRedisCache(mr.Addr())
	sessionStore := NewRedisStore(redisCache)

	return mr, sessionStore
}

func TestRedisStore_Create(t *testing.T) {
	t.Run("successful create", func(t *testing.T) {
		mr, store := setupTestRedisSession(t)
		defer mr.Close()

		data := []byte("test-session-data")
		ttl := time.Hour

		session, err := store.Create(data, ttl)
		assert.NoError(t, err)
		assert.NotEmpty(t, session.ID)
		assert.Equal(t, data, session.Data)
		assert.False(t, session.ExpiresAt.IsZero())
	})

	t.Run("create with default TTL", func(t *testing.T) {
		mr, store := setupTestRedisSession(t)
		defer mr.Close()

		data := []byte("test-session-data")

		session, err := store.Create(data, 0)
		assert.NoError(t, err)
		expectedExpiry := time.Now().Add(24 * time.Hour)
		assert.WithinDuration(t, expectedExpiry, session.ExpiresAt, time.Minute)
	})
}

func TestRedisStore_Get(t *testing.T) {
	t.Run("successful get", func(t *testing.T) {
		mr, store := setupTestRedisSession(t)
		defer mr.Close()

		data := []byte("test-session-data")
		session, err := store.Create(data, time.Hour)
		require.NoError(t, err)

		retrieved, err := store.Get(session.ID)
		assert.NoError(t, err)
		assert.Equal(t, session.ID, retrieved.ID)
		assert.Equal(t, data, retrieved.Data)
	})

	t.Run("get non-existent session", func(t *testing.T) {
		mr, store := setupTestRedisSession(t)
		defer mr.Close()

		_, err := store.Get("non-existent-id")
		assert.Error(t, err)
		assert.Equal(t, ErrSessionNotFound, err)
	})

	t.Run("get expired session", func(t *testing.T) {
		mr, store := setupTestRedisSession(t)
		defer mr.Close()

		data := []byte("test-session-data")
		session, err := store.Create(data, 100*time.Millisecond)
		require.NoError(t, err)

		mr.FastForward(200 * time.Millisecond)

		_, err = store.Get(session.ID)
		assert.Error(t, err)
		assert.Equal(t, ErrSessionNotFound, err)
	})
}

func TestRedisStore_Update(t *testing.T) {
	t.Run("successful update", func(t *testing.T) {
		mr, store := setupTestRedisSession(t)
		defer mr.Close()

		data1 := []byte("original-data")
		session, err := store.Create(data1, time.Hour)
		require.NoError(t, err)

		data2 := []byte("updated-data")
		err = store.Update(session.ID, data2)
		assert.NoError(t, err)

		retrieved, err := store.Get(session.ID)
		assert.NoError(t, err)
		assert.Equal(t, data2, retrieved.Data)
	})

	t.Run("update non-existent session", func(t *testing.T) {
		mr, store := setupTestRedisSession(t)
		defer mr.Close()

		data := []byte("test-data")
		err := store.Update("non-existent-id", data)
		assert.Error(t, err)
		assert.Equal(t, ErrSessionNotFound, err)
	})
}

func TestRedisStore_Delete(t *testing.T) {
	t.Run("successful delete", func(t *testing.T) {
		mr, store := setupTestRedisSession(t)
		defer mr.Close()

		data := []byte("test-session-data")
		session, err := store.Create(data, time.Hour)
		require.NoError(t, err)

		err = store.Delete(session.ID)
		assert.NoError(t, err)

		_, err = store.Get(session.ID)
		assert.Error(t, err)
		assert.Equal(t, ErrSessionNotFound, err)
	})

	t.Run("delete non-existent session", func(t *testing.T) {
		mr, store := setupTestRedisSession(t)
		defer mr.Close()

		err := store.Delete("non-existent-id")
		assert.NoError(t, err)
	})
}

func TestRedisStore_GetWithContext(t *testing.T) {
	t.Run("successful get with context", func(t *testing.T) {
		mr, store := setupTestRedisSession(t)
		defer mr.Close()

		data := []byte("test-session-data")
		session, err := store.Create(data, time.Hour)
		require.NoError(t, err)

		ctx := context.Background()
		retrieved, err := store.(*RedisStore).GetWithContext(ctx, session.ID)
		assert.NoError(t, err)
		assert.Equal(t, session.ID, retrieved.ID)
	})

	t.Run("get with cancelled context", func(t *testing.T) {
		mr, store := setupTestRedisSession(t)
		defer mr.Close()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := store.(*RedisStore).GetWithContext(ctx, "any-id")
		assert.Error(t, err)
	})
}
