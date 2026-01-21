package health

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestChecker_Liveness(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mockLogger := &MockLogger{}
	hc := NewChecker(nil, nil, nil, mockLogger)

	router := gin.New()
	router.GET("/healthz", hc.Liveness)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"status":"ok"`)
}

func TestChecker_Readiness(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("all healthy", func(t *testing.T) {
		mockDB := &MockDBChecker{}
		mockDB.On("PingContext", mock.Anything).Return(nil)
		mockCache := &MockCacheChecker{}
		mockCache.On("Ping", mock.Anything).Return(nil)
		mockKafka := &MockKafkaChecker{}
		mockKafka.On("Ping", mock.Anything).Return(nil)
		mockLogger := &MockLogger{}
		hc := NewChecker(mockDB, mockCache, mockKafka, mockLogger)

		router := gin.New()
		router.GET("/readyz", hc.Readiness)

		req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), `"status":"ready"`)
	})

	t.Run("database unhealthy", func(t *testing.T) {
		mockDB := &MockDBChecker{}
		mockDB.On("PingContext", mock.Anything).Return(errors.New("db error"))
		mockCache := &MockCacheChecker{}
		mockCache.On("Ping", mock.Anything).Return(nil)
		mockKafka := &MockKafkaChecker{}
		mockKafka.On("Ping", mock.Anything).Return(nil)
		mockLogger := &MockLogger{}
		hc := NewChecker(mockDB, mockCache, mockKafka, mockLogger)

		router := gin.New()
		router.GET("/readyz", hc.Readiness)

		req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusServiceUnavailable, w.Code)
		assert.Contains(t, w.Body.String(), `"status":"not_ready"`)
	})
}
