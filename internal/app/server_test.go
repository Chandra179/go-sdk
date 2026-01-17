package app

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"gosdk/pkg/cache"
	"gosdk/pkg/db"
	"gosdk/pkg/kafka"
	"gosdk/pkg/logger"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockCloseableCache struct {
	cache.Cache
	closeCalled bool
	closeError  error
}

func (m *mockCloseableCache) Close() error {
	m.closeCalled = true
	return m.closeError
}

type mockCache struct {
	cache.Cache
}

type mockProducer struct {
	kafka.Producer
}

func (m *mockProducer) Publish(ctx context.Context, msg kafka.Message) error {
	return nil
}

type mockConsumer struct {
	kafka.Consumer
}

func (m *mockConsumer) Subscribe(ctx context.Context, topics []string, handler kafka.ConsumerHandler) error {
	return nil
}

type mockCloseableKafka struct {
	closeCalled bool
	closeError  error
}

func (m *mockCloseableKafka) Close() error {
	m.closeCalled = true
	return m.closeError
}

func (m *mockCloseableKafka) Producer() (kafka.Producer, error) {
	return &mockProducer{}, nil
}

func (m *mockCloseableKafka) Consumer(groupID string) (kafka.Consumer, error) {
	return &mockConsumer{}, nil
}

type mockCloseableDB struct {
	closeCalled bool
	closeError  error
}

func (m *mockCloseableDB) Close() error {
	m.closeCalled = true
	return m.closeError
}

func (m *mockCloseableDB) WithTransaction(ctx context.Context, isolation sql.IsolationLevel, fn db.TxFunc) error {
	return nil
}

func (m *mockCloseableDB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return nil, nil
}

func (m *mockCloseableDB) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return nil, nil
}

func (m *mockCloseableDB) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return nil
}

func TestServer_Shutdown_HTTPServer(t *testing.T) {
	t.Run("shuts down HTTP server", func(t *testing.T) {
		router := gin.New()
		router.GET("/test", func(c *gin.Context) {
			c.String(http.StatusOK, "ok")
		})

		server := httptest.NewServer(router)
		defer server.Close()

		s := &Server{
			logger:     logger.NewLogger("test"),
			httpServer: &http.Server{Addr: server.Listener.Addr().String()},
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		err := s.Shutdown(ctx)
		assert.NoError(t, err)
	})
}

func TestServer_Shutdown_Kafka(t *testing.T) {
	t.Run("closes Kafka connections", func(t *testing.T) {
		mockKafka := &mockCloseableKafka{}

		s := &Server{
			logger:      logger.NewLogger("test"),
			kafkaClient: mockKafka,
		}

		err := s.Shutdown(context.Background())
		assert.NoError(t, err)
		assert.True(t, mockKafka.closeCalled, "Kafka Close should be called")
	})

	t.Run("returns error when Kafka close fails", func(t *testing.T) {
		mockKafka := &mockCloseableKafka{closeError: errors.New("kafka error")}

		s := &Server{
			logger:      logger.NewLogger("test"),
			kafkaClient: mockKafka,
		}

		err := s.Shutdown(context.Background())
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "kafka shutdown")
	})
}

func TestServer_Shutdown_Cache(t *testing.T) {
	t.Run("closes Redis cache when Close method exists", func(t *testing.T) {
		mockCache := &mockCloseableCache{}

		s := &Server{
			logger: logger.NewLogger("test"),
			cache:  mockCache,
		}

		err := s.Shutdown(context.Background())
		assert.NoError(t, err)
		assert.True(t, mockCache.closeCalled, "Cache Close should be called")
	})

	t.Run("continues when cache has no Close method", func(t *testing.T) {
		mockCache := &mockCache{}

		s := &Server{
			logger: logger.NewLogger("test"),
			cache:  mockCache,
		}

		err := s.Shutdown(context.Background())
		assert.NoError(t, err)
	})

	t.Run("returns error when cache close fails", func(t *testing.T) {
		mockCache := &mockCloseableCache{closeError: errors.New("cache error")}

		s := &Server{
			logger: logger.NewLogger("test"),
			cache:  mockCache,
		}

		err := s.Shutdown(context.Background())
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cache shutdown")
	})
}

func TestServer_Shutdown_Database(t *testing.T) {
	t.Run("closes database connections", func(t *testing.T) {
		mockDB := &mockCloseableDB{}

		s := &Server{
			logger: logger.NewLogger("test"),
			db:     mockDB,
		}

		err := s.Shutdown(context.Background())
		assert.NoError(t, err)
		assert.True(t, mockDB.closeCalled, "DB Close should be called")
	})

	t.Run("returns error when database close fails", func(t *testing.T) {
		mockDB := &mockCloseableDB{closeError: errors.New("database error")}

		s := &Server{
			logger: logger.NewLogger("test"),
			db:     mockDB,
		}

		err := s.Shutdown(context.Background())
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "database shutdown")
	})
}

func TestServer_Shutdown_Observability(t *testing.T) {
	t.Run("calls observability shutdown function", func(t *testing.T) {
		shutdownCalled := false
		mockShutdown := func(ctx context.Context) error {
			shutdownCalled = true
			return nil
		}

		s := &Server{
			logger:   logger.NewLogger("test"),
			shutdown: mockShutdown,
		}

		err := s.Shutdown(context.Background())
		assert.NoError(t, err)
		assert.True(t, shutdownCalled, "Observability shutdown should be called")
	})

	t.Run("returns error when observability shutdown fails", func(t *testing.T) {
		mockShutdown := func(ctx context.Context) error {
			return errors.New("observability error")
		}

		s := &Server{
			logger:   logger.NewLogger("test"),
			shutdown: mockShutdown,
		}

		err := s.Shutdown(context.Background())
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "observability shutdown")
	})
}

func TestServer_Shutdown_MultipleErrors(t *testing.T) {
	t.Run("aggregates multiple shutdown errors", func(t *testing.T) {
		mockKafka := &mockCloseableKafka{closeError: errors.New("kafka error")}
		mockCache := &mockCloseableCache{closeError: errors.New("cache error")}

		s := &Server{
			logger:      logger.NewLogger("test"),
			kafkaClient: mockKafka,
			cache:       mockCache,
		}

		err := s.Shutdown(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "shutdown errors")
	})
}

func TestServer_Shutdown_NilResources(t *testing.T) {
	t.Run("handles nil resources gracefully", func(t *testing.T) {
		s := &Server{
			logger:      logger.NewLogger("test"),
			httpServer:  nil,
			kafkaClient: nil,
			db:          nil,
			cache:       nil,
			shutdown:    nil,
		}

		err := s.Shutdown(context.Background())
		assert.NoError(t, err)
	})
}
