package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"gosdk/pkg/logger"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

type MockDBChecker struct {
	pingErr error
}

func (m *MockDBChecker) PingContext(ctx context.Context) error {
	return m.pingErr
}

func (m *MockDBChecker) Close() error {
	return nil
}

type MockCacheChecker struct {
	pingErr error
}

func (m *MockCacheChecker) Ping(ctx context.Context) error {
	return m.pingErr
}

func (m *MockCacheChecker) Close() error {
	return nil
}

func TestHealthChecker_Liveness(t *testing.T) {
	gin.SetMode(gin.TestMode)
	hc := NewHealthChecker(&MockDBChecker{}, &MockCacheChecker{}, nil, logger.NewLogger("test"))

	router := gin.New()
	router.GET("/healthz", hc.Liveness)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"status":"ok"`)
}

func TestHealthChecker_Readiness(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("all healthy", func(t *testing.T) {
		hc := NewHealthChecker(&MockDBChecker{}, &MockCacheChecker{}, nil, logger.NewLogger("test"))

		router := gin.New()
		router.GET("/readyz", hc.Readiness)

		req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), `"status":"ready"`)
	})

	t.Run("database unhealthy", func(t *testing.T) {
		hc := NewHealthChecker(&MockDBChecker{pingErr: assert.AnError}, &MockCacheChecker{}, nil, logger.NewLogger("test"))

		router := gin.New()
		router.GET("/readyz", hc.Readiness)

		req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusServiceUnavailable, w.Code)
		assert.Contains(t, w.Body.String(), `"status":"not_ready"`)
	})
}
