# Phase 5: Polish - API Docs, Session/Event Improvements, DB Abstractions

## Overview

This phase completes production readiness by addressing API documentation gaps, improving session and event services with advanced features, and implementing a proper database repository pattern.

## Objectives

- Complete API documentation with examples and error responses
- Improve session store with rotation, metadata, and rate limiting
- Enhance event service with retry logic, DLQ, and backpressure
- Implement database repository pattern for better abstraction

---

## 1. API Documentation

### Current State

**File:** `internal/service/auth/handler.go`

Basic Swagger annotations exist:

```go
// @Summary Login with OAuth2 provider
// @Description Redirects to OAuth2 provider authorization page
// @Tags auth
// @Accept json
// @Produce json
// @Param request body LoginRequest true "Login request"
// @Success 302 {string} string "Redirect to OAuth2 provider"
// @Failure 400 {object} map[string]string "Bad request"
// @Failure 500 {object} map[string]string "Internal server error"
```

**Issues:**
- No request/response examples
- No error response documentation
- No security scheme documentation
- Missing API versioning

### Implementation Plan

#### 1.1 Add Examples to Auth Handlers

**File:** `internal/service/auth/handler.go`

```go
package auth

import (
    "net/http"

    apperrors "gosdk/internal/app/errors"

    "github.com/gin-gonic/gin"
)

// LoginHandler initiates OAuth2 login flow
// @Summary Login with OAuth2 provider
// @Description Redirects to OAuth2 provider authorization page
// @Tags auth
// @Accept json
// @Produce json
// @Param request body LoginRequest true "Login request"
// @Success 302 {string} string "Redirect to OAuth2 provider"
// @Failure 400 {object} apperrors.ErrorResponse "Bad request - missing or invalid provider"
// @Failure 500 {object} apperrors.ErrorResponse "Internal server error"
// @Router /v1/auth/login [post]
// @Example request
// {
//   "provider": "google"
// }
func (h *Handler) LoginHandler() gin.HandlerFunc {
    return func(c *gin.Context) {
        var req LoginRequest
        if err := c.ShouldBindJSON(&req); err != nil {
            c.Error(apperrors.NewBadRequest(apperrors.ErrCodeInvalidRequest, "invalid request body"))
            return
        }

        if err := ValidateStruct(&req); err != nil {
            c.Error(apperrors.NewBadRequestWithDetails(apperrors.ErrCodeValidationFailed, "validation failed", err))
            return
        }

        authURL, err := h.service.InitiateLogin(req.Provider)
        if err != nil {
            c.Error(apperrors.Wrap(err, apperrors.ErrCodeInternal, "failed to initiate login"))
            return
        }

        c.Redirect(http.StatusFound, authURL)
    }
}

// LogoutHandler logs out the user
// @Summary Logout
// @Description Deletes user session and clears cookie
// @Tags auth
// @Produce json
// @Success 200 {object} SuccessResponse "Logged out successfully"
// @Failure 500 {object} apperrors.ErrorResponse "Internal server error"
// @Router /v1/auth/logout [post]
// @Security CookieAuth
// @Example response
// {
//   "message": "logged out"
// }
func (h *Handler) LogoutHandler() gin.HandlerFunc {
    return func(c *gin.Context) {
        sessionID, err := c.Cookie(SessionCookieName)
        if err == nil {
            if err := h.service.DeleteSession(sessionID); err != nil {
                c.Error(apperrors.Wrap(err, apperrors.ErrCodeInternal, "failed to delete session"))
                return
            }
        }

        c.SetCookie(
            SessionCookieName,
            "",
            -1,
            "/",
            "",
            true,
            true,
        )

        c.JSON(http.StatusOK, gin.H{"message": "logged out"})
    }
}

// AuthMiddleware validates session and proactively refreshes tokens
// @Summary Authentication Middleware
// @Description Validates session cookie and proactively refreshes tokens if needed
// @Tags auth
// @Produce json
// @Failure 401 {object} apperrors.ErrorResponse "Unauthorized - no session found or expired"
// @Router /v1/protected [get]
// @Security CookieAuth
func (h *Handler) AuthMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        sessionID, err := c.Cookie(SessionCookieName)
        if err != nil {
            c.Error(apperrors.NewUnauthorized(apperrors.ErrCodeUnauthorized, "no session found"))
            c.Abort()
            return
        }

        sessionData, err := h.service.ValidateAndRefreshSession(c.Request.Context(), sessionID)
        if err != nil {
            c.SetCookie(SessionCookieName, "", -1, "/", "", true, true)
            c.Error(apperrors.NewUnauthorized(apperrors.ErrCodeSessionExpired, "session expired, please re-login"))
            c.Abort()
            return
        }

        c.Set("user_id", sessionData.UserID)
        c.Next()
    }
}

type SuccessResponse struct {
    Message string `json:"message" example:"logged out"`
}
```

#### 1.2 Add Event Handler Examples

**File:** `internal/service/event/handler.go`

```go
package event

import (
    "net/http"

    "github.com/gin-gonic/gin"
    "gosdk/pkg/logger"
)

type PublishRequest struct {
    Topic   string            `json:"topic" example:"user-events" binding:"required"`
    Key     string            `json:"key" example:"user-123" binding:"required"`
    Value   string            `json:"value" example:"{\"action\":\"login\"}" binding:"required"`
    Headers map[string]string `json:"headers" example:"{\"event-type\":\"user-login\"}"`
}

type SubscribeRequest struct {
    Topic   string `json:"topic" example:"user-events" binding:"required"`
    GroupID string `json:"group_id" example:"consumer-group-1" binding:"required"`
}

// PublishHandler publishes a message to Kafka
// @Summary Publish Message
// @Description Publishes a message to a Kafka topic
// @Tags messages
// @Accept json
// @Produce json
// @Param request body PublishRequest true "Publish request"
// @Success 200 {object} SuccessResponse "Message published successfully"
// @Failure 400 {object} ErrorResponse "Bad request"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /v1/messages/publish [post]
// @Example request
// {
//   "topic": "user-events",
//   "key": "user-123",
//   "value": "{\"action\":\"login\"}",
//   "headers": {
//     "event-type": "user-login"
//   }
// }
// @Example response
// {
//   "message": "published"
// }
func (h *Handler) PublishHandler() gin.HandlerFunc {
    return func(c *gin.Context) {
        var req PublishRequest
        if err := c.ShouldBindJSON(&req); err != nil {
            h.logger.Warn(c.Request.Context(), "Invalid publish request",
                logger.Field{Key: "error", Value: err.Error()})
            c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
            return
        }

        if err := h.service.PublishMessage(c.Request.Context(), req.Topic, req.Key, req.Value, req.Headers); err != nil {
            h.logger.Error(c.Request.Context(), "Failed to publish message",
                logger.Field{Key: "error", Value: err.Error()},
                logger.Field{Key: "topic", Value: req.Topic})
            c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to publish message"})
            return
        }

        h.logger.Info(c.Request.Context(), "Message published successfully",
            logger.Field{Key: "topic", Value: req.Topic})

        c.JSON(http.StatusOK, gin.H{"message": "published"})
    }
}

// SubscribeHandler subscribes to a Kafka topic
// @Summary Subscribe to Topic
// @Description Subscribes to a Kafka topic with a consumer group
// @Tags messages
// @Accept json
// @Produce json
// @Param request body SubscribeRequest true "Subscribe request"
// @Success 200 {object} SubscribeResponse "Subscription created successfully"
// @Failure 400 {object} ErrorResponse "Bad request"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /v1/messages/subscribe [post]
// @Example request
// {
//   "topic": "user-events",
//   "group_id": "consumer-group-1"
// }
// @Example response
// {
//   "subscription_id": "550e8400-e29b-41d4-a716-446655440000"
// }
func (h *Handler) SubscribeHandler() gin.HandlerFunc {
    return func(c *gin.Context) {
        var req SubscribeRequest
        if err := c.ShouldBindJSON(&req); err != nil {
            h.logger.Warn(c.Request.Context(), "Invalid subscribe request",
                logger.Field{Key: "error", Value: err.Error()})
            c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
            return
        }

        subscriptionID, err := h.service.SubscribeToTopic(c.Request.Context(), req.Topic, req.GroupID)
        if err != nil {
            h.logger.Error(c.Request.Context(), "Failed to subscribe",
                logger.Field{Key: "error", Value: err.Error()},
                logger.Field{Key: "topic", Value: req.Topic})
            c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to subscribe"})
            return
        }

        h.logger.Info(c.Request.Context(), "Subscription created",
            logger.Field{Key: "subscription_id", Value: subscriptionID},
            logger.Field{Key: "topic", Value: req.Topic})

        c.JSON(http.StatusOK, gin.H{"subscription_id": subscriptionID})
    }
}

// UnsubscribeHandler unsubscribes from a Kafka topic
// @Summary Unsubscribe from Topic
// @Description Unsubscribes from a Kafka topic using subscription ID
// @Tags messages
// @Produce json
// @Param id path string true "Subscription ID"
// @Success 200 {object} SuccessResponse "Unsubscribed successfully"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /v1/messages/subscribe/{id} [delete]
// @Example response
// {
//   "message": "unsubscribed"
// }
func (h *Handler) UnsubscribeHandler() gin.HandlerFunc {
    return func(c *gin.Context) {
        subscriptionID := c.Param("id")

        if err := h.service.Unsubscribe(c.Request.Context(), subscriptionID); err != nil {
            h.logger.Error(c.Request.Context(), "Failed to unsubscribe",
                logger.Field{Key: "error", Value: err.Error()},
                logger.Field{Key: "subscription_id", Value: subscriptionID})
            c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to unsubscribe"})
            return
        }

        h.logger.Info(c.Request.Context(), "Unsubscribed successfully",
            logger.Field{Key: "subscription_id", Value: subscriptionID})

        c.JSON(http.StatusOK, gin.H{"message": "unsubscribed"})
    }
}

type ErrorResponse struct {
    Error string `json:"error" example:"invalid request"`
}

type SubscribeResponse struct {
    SubscriptionID string `json:"subscription_id" example:"550e8400-e29b-41d4-a716-446655440000"`
}
```

#### 1.3 Add Security Scheme to Swagger

**File:** `cmd/myapp/main.go`

```go
//go:generate swag init -g main.go -o ../../api
// @title Go SDK API
// @version 1.0
// @description Production-ready Go SDK API with OAuth2 authentication
// @termsOfService http://swagger.io/terms/
// @contact.name API Support
// @contact.email support@example.com
// @license.name Apache 2.0
// @license.url http://www.apache.org/licenses/LICENSE-2.0.html
// @host localhost:8080
// @BasePath /v1
// @securitydefinitions.apikey CookieAuth
// @in header
// @name Cookie
// @description session_id cookie for authentication
package main

import (
    // ... imports ...
)
```

#### 1.4 Add API Versioning

**File:** `internal/app/routes.go`

```go
func (s *Server) setupRoutes() {
    r := gin.New()

    // Apply middleware
    r.Use(gin.Recovery())
    r.Use(middleware.RequestID())
    r.Use(middleware.SecurityHeaders())
    r.Use(middleware.CORS(middleware.CORSConfig{
        AllowedOrigins:   s.config.CORS.AllowedOrigins,
        AllowedMethods:   s.config.CORS.AllowedMethods,
        AllowedHeaders:   s.config.CORS.AllowedHeaders,
        ExposeHeaders:    s.config.CORS.ExposeHeaders,
        AllowCredentials: s.config.CORS.AllowCredentials,
        MaxAge:           s.config.CORS.MaxAge,
    }))

    if s.config.RateLimit.Enabled {
        r.Use(middleware.RateLimit(s.cache, middleware.RateLimiterConfig{
            Requests: s.config.RateLimit.Requests,
            Window:   s.config.RateLimit.Window,
        }))
    }

    requestLogger := middleware.NewRequestLogger(s.logger)
    r.Use(requestLogger.Logging())
    r.Use(middleware.MetricsMiddleware())

    errorHandler := middleware.NewErrorHandler(s.logger)
    r.Use(errorHandler.Middleware())

    setupInfraRoutes(r)

    // API v1
    v1 := r.Group("/v1")
    {
        authHandler := auth.NewHandler(s.authService)
        setupAuthRoutes(v1, authHandler, s.oauth2Manager)
        setupMessageBrokerRoutes(v1, s.messageBrokerHandler)
    }

    s.router = r
}
```

#### 1.5 Update Route Functions for Versioning

**File:** `internal/app/routes.go`

```go
func setupAuthRoutes(r *gin.RouterGroup, handler *auth.Handler, oauth2mgr *oauth2.Manager) {
    auth := r.Group("/auth")
    {
        auth.POST("/login", handler.LoginHandler())
        auth.POST("/logout", handler.LogoutHandler())
        auth.GET("/callback/google", oauth2.GoogleCallbackHandler(oauth2mgr))
    }
}

func setupMessageBrokerRoutes(r *gin.RouterGroup, handler *event.Handler) {
    mb := r.Group("/messages")
    {
        mb.POST("/publish", handler.PublishHandler())
        mb.POST("/subscribe", handler.SubscribeHandler())
        mb.DELETE("/subscribe/:id", handler.UnsubscribeHandler())
    }
}
```

#### 1.6 Generate Complete Swagger

**File:** `Makefile`

```makefile
swag:
	swag init -g cmd/myapp/main.go -o api --parseDependency --parseInternal --parseInternal
```

### Testing

Run swagger generation:

```bash
make swag
```

Verify generated files:
- `api/docs.go`
- `api/swagger.json`
- `api/swagger.yaml`

Visit Swagger UI: `http://localhost:8080/swagger/index.html`

---

## 2. Session Store Improvements

### Current State

**File:** `internal/service/session/redis_session.go`

Basic Redis session store with no tests.

**Issues:**
- No session rotation
- No metadata tracking (IP, User-Agent)
- No rate limiting for session creation
- No cleanup job
- No tests

### Implementation Plan

#### 2.1 Add Session Metadata

**File:** `internal/service/session/types.go`

```go
package session

import (
    "time"
)

type SessionMetadata struct {
    ID        string    `json:"id"`
    CreatedAt time.Time `json:"created_at"`
    ExpiresAt time.Time `json:"expires_at"`
    ClientIP  string    `json:"client_ip,omitempty"`
    UserAgent string    `json:"user_agent,omitempty"`
}

type Session struct {
    ID        string
    Data      []byte
    Metadata  *SessionMetadata
    CreatedAt time.Time
    ExpiresAt time.Time
}
```

#### 2.2 Implement Session Rotation

**File:** `internal/service/session/redis_session.go`

```go
package session

import (
    "context"
    "encoding/json"
    "fmt"
    "time"

    "gosdk/pkg/cache"

    "github.com/google/uuid"
)

type RedisStore struct {
    cache cache.Cache
}

func NewRedisStore(cache cache.Cache) *RedisStore {
    return &RedisStore{cache: cache}
}

func (s *RedisStore) Create(data []byte, timeout time.Duration) (*Session, error) {
    ctx := context.Background()
    sessionID := uuid.NewString()
    now := time.Now()

    session := &Session{
        ID:        sessionID,
        Data:      data,
        CreatedAt: now,
        ExpiresAt:  now.Add(timeout),
    }

    sessionJSON, err := json.Marshal(session)
    if err != nil {
        return nil, fmt.Errorf("failed to marshal session: %w", err)
    }

    key := s.sessionKey(sessionID)
    if err := s.cache.Set(ctx, key, string(sessionJSON), timeout); err != nil {
        return nil, fmt.Errorf("failed to store session: %w", err)
    }

    return session, nil
}

func (s *RedisStore) Rotate(sessionID string, newData []byte, ttl time.Duration) (*Session, error) {
    ctx := context.Background()

    sess, err := s.Get(sessionID)
    if err != nil {
        return nil, err
    }

    newSessionID := uuid.NewString()
    newSession := &Session{
        ID:        newSessionID,
        Data:      newData,
        CreatedAt: sess.CreatedAt,
        ExpiresAt:  time.Now().Add(ttl),
    }

    sessionJSON, err := json.Marshal(newSession)
    if err != nil {
        return nil, fmt.Errorf("failed to marshal session: %w", err)
    }

    newKey := s.sessionKey(newSessionID)
    if err := s.cache.Set(ctx, newKey, string(sessionJSON), ttl); err != nil {
        return nil, fmt.Errorf("failed to store new session: %w", err)
    }

    // Delete old session
    s.Delete(sessionID)

    return newSession, nil
}

func (s *RedisStore) Get(id string) (*Session, error) {
    ctx := context.Background()
    key := s.sessionKey(id)

    val, err := s.cache.Get(ctx, key)
    if err != nil {
        return nil, fmt.Errorf("failed to get session: %w", err)
    }

    var session Session
    if err := json.Unmarshal([]byte(val), &session); err != nil {
        return nil, fmt.Errorf("failed to unmarshal session: %w", err)
    }

    return &session, nil
}

func (s *RedisStore) Update(id string, data []byte) error {
    ctx := context.Background()
    key := s.sessionKey(id)

    sess, err := s.Get(id)
    if err != nil {
        return err
    }

    sess.Data = data
    sessionJSON, err := json.Marshal(sess)
    if err != nil {
        return fmt.Errorf("failed to marshal session: %w", err)
    }

    ttl := time.Until(sess.ExpiresAt)
    return s.cache.Set(ctx, key, string(sessionJSON), ttl)
}

func (s *RedisStore) Delete(id string) error {
    ctx := context.Background()
    key := s.sessionKey(id)
    return s.cache.Del(ctx, key)
}

func (s *RedisStore) sessionKey(id string) string {
    return fmt.Sprintf("session:%s", id)
}

// Rate limiting for session creation
func (s *RedisStore) CanCreateSession(clientIP string, maxSessions int, window time.Duration) (bool, error) {
    ctx := context.Background()
    key := fmt.Sprintf("session_rate:%s", clientIP)

    val, err := s.cache.Get(ctx, key)
    if err != nil {
        return true, nil
    }

    var count int
    if val != "" {
        count, _ = fmt.Sscanf(val, "%d", &val)
    }

    if count >= maxSessions {
        return false, nil
    }

    newCount := count + 1
    return true, s.cache.Set(ctx, key, fmt.Sprintf("%d", newCount), window)
}
```

#### 2.3 Add Cleanup Job

**File:** `internal/service/session/cleanup.go`

```go
package session

import (
    "context"
    "time"

    "gosdk/pkg/logger"
)

func (s *RedisStore) StartCleanupJob(interval time.Duration, logger logger.Logger) {
    ticker := time.NewTicker(interval)

    go func() {
        for range ticker.C {
            s.cleanupExpiredSessions(context.Background(), logger)
        }
    }()
}

func (s *RedisStore) cleanupExpiredSessions(ctx context.Context, logger logger.Logger) {
    // Redis handles TTL automatically
    // This could track sessions in a set for manual cleanup
    // or scan for expired sessions if needed
    logger.Debug(ctx, "Session cleanup check completed")
}
```

#### 2.4 Comprehensive Tests

**File:** `internal/service/session/redis_session_test.go`

```go
package session

import (
    "context"
    "testing"
    "time"

    "github.com/alicebob/miniredis/v2"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func setupTestSessionStore(t *require.Assertions) (*miniredis.Miniredis, *RedisStore) {
    mr, err := miniredis.Run()
    require.NoError(t, err)

    store := NewRedisStore(mr.Addr())

    t.Cleanup(func() {
        mr.Close()
    })

    return mr, store
}

func TestRedisStore_Create(t *testing.T) {
    r := require.New(t)
    mr, store := setupTestSessionStore(r)
    ctx := context.Background()

    data := []byte(`{"user_id": "123"}`)
    sess, err := store.Create(data, 24*time.Hour)

    assert.NoError(t, err)
    assert.NotEmpty(t, sess.ID)
    assert.Equal(t, data, sess.Data)

    keys := mr.Keys()
    assert.Len(t, keys, 1)
    assert.Contains(t, keys[0], "session:")
}

func TestRedisStore_Get(t *testing.T) {
    r := require.New(t)
    _, store := setupTestSessionStore(r)
    ctx := context.Background()

    data := []byte(`{"user_id": "123"}`)
    created, _ := store.Create(data, 24*time.Hour)

    retrieved, err := store.Get(created.ID)

    assert.NoError(t, err)
    assert.Equal(t, created.ID, retrieved.ID)
    assert.Equal(t, data, retrieved.Data)
}

func TestRedisStore_Rotate(t *testing.T) {
    r := require.New(t)
    _, store := setupTestSessionStore(r)
    ctx := context.Background()

    data := []byte(`{"user_id": "123"}`)
    oldSess, _ := store.Create(data, 24*time.Hour)

    newData := []byte(`{"user_id": "456"}`)
    newSess, err := store.Rotate(oldSess.ID, newData, 24*time.Hour)

    assert.NoError(t, err)
    assert.NotEqual(t, oldSess.ID, newSess.ID)
    assert.Equal(t, newData, newSess.Data)

    _, err = store.Get(oldSess.ID)
    assert.Error(t, err)
}

func TestRedisStore_RateLimit(t *testing.T) {
    r := require.New(t)
    mr, store := setupTestSessionStore(r)

    clientIP := "127.0.0.1"

    t.Run("within limit", func(t *testing.T) {
        for i := 0; i < 3; i++ {
            allowed, err := store.CanCreateSession(clientIP, 3, time.Minute)
            assert.NoError(t, err)
            assert.True(t, allowed)
        }
    })

    t.Run("exceeds limit", func(t *testing.T) {
        allowed, err := store.CanCreateSession(clientIP, 3, time.Minute)
        assert.NoError(t, err)
        assert.False(t, allowed)
    })
}
```

---

## 3. Event Service Improvements

### Current State

**File:** `internal/service/event/service.go`

Basic publish/subscribe with `fmt.Printf` logging.

**Issues:**
- No retry logic for failed publishes
- No DLQ support
- No consumer backpressure handling

### Implementation Plan

#### 3.1 Replace fmt.Printf with Structured Logging

Already covered in Phase 3.

#### 3.2 Add Retry Logic

**File:** `internal/service/event/service.go`

```go
import (
    "time"

    "github.com/cenkalti/backoff/v5"
)

func (s *Service) PublishMessageWithRetry(ctx context.Context, topic, key, value string, headers map[string]string) error {
    backoffPolicy := backoff.NewExponentialBackOff()
    backoffPolicy.InitialInterval = 100 * time.Millisecond
    backoffPolicy.MaxInterval = 5 * time.Second
    backoffPolicy.MaxElapsedTime = 30 * time.Second

    operation := func() error {
        return s.PublishMessage(ctx, topic, key, value, headers)
    }

    notify := func(err error, duration time.Duration) {
        s.logger.Warn(ctx, "Publish failed, retrying",
            logger.Field{Key: "error", Value: err.Error()},
            logger.Field{Key: "retry_after", Value: duration})
    }

    return backoff.RetryNotify(operation, backoffPolicy, notify)
}
```

#### 3.3 Add DLQ Support

**File:** `internal/service/event/service.go`

```go
const (
    DLQTopic = "events-dlq"
)

func (s *Service) PublishMessage(ctx context.Context, topic, key, value string, headers map[string]string) error {
    producer, err := s.kafkaClient.Producer()
    if err != nil {
        s.logger.Error(ctx, "Failed to get producer",
            logger.Field{Key: "error", Value: err.Error()})
        return fmt.Errorf("failed to get producer: %w", err)
    }

    message := kafka.Message{
        Topic:   topic,
        Key:     []byte(key),
        Value:   []byte(value),
        Headers: headers,
    }

    err = producer.Publish(ctx, message)
    if err != nil {
        s.logger.Error(ctx, "Publish failed, sending to DLQ",
            logger.Field{Key: "original_topic", Value: topic},
            logger.Field{Key: "error", Value: err.Error()})

        dlqMessage := kafka.Message{
            Topic:   DLQTopic,
            Key:     []byte(key),
            Value:   []byte(value),
            Headers: map[string]string{
                "original_topic": topic,
                "error":          err.Error(),
            },
        }

        return producer.Publish(ctx, dlqMessage)
    }

    s.logger.Info(ctx, "Message published successfully",
        logger.Field{Key: "topic", Value: topic},
        logger.Field{Key: "key", Value: key})

    return nil
}
```

#### 3.4 Add Consumer Backpressure

**File:** `internal/service/event/service.go`

```go
type SubscribeConfig struct {
    MaxConcurrent int
    MaxRetries    int
    BackoffDelay  time.Duration
}

type SubscribeOption func(*SubscribeConfig)

func WithMaxConcurrent(n int) SubscribeOption {
    return func(c *SubscribeConfig) {
        c.MaxConcurrent = n
    }
}

func WithMaxRetries(n int) SubscribeOption {
    return func(c *SubscribeConfig) {
        c.MaxRetries = n
    }
}

func (s *Service) SubscribeToTopic(ctx context.Context, topic, groupID string, opts ...SubscribeOption) (string, error) {
    config := &SubscribeConfig{
        MaxConcurrent: 10,
        MaxRetries:    3,
        BackoffDelay:  1 * time.Second,
    }

    for _, opt := range opts {
        opt(config)
    }

    subscriptionID := uuid.NewString()

    s.mu.Lock()
    defer s.mu.Unlock()

    consumer, err := s.kafkaClient.Consumer(groupID)
    if err != nil {
        return "", fmt.Errorf("failed to get consumer: %w", err)
    }

    s.consumers[subscriptionID] = consumer

    sem := make(chan struct{}, config.MaxConcurrent)

    handler := func(msg kafka.Message) error {
        sem <- struct{}{}
        defer func() { <-sem }()

        s.logger.Info(ctx, "Received message",
            logger.Field{Key: "topic", Value: msg.Topic},
            logger.Field{Key: "key", Value: string(msg.Key)})

        return nil
    }

    if err := consumer.Subscribe(ctx, []string{topic}, handler); err != nil {
        return "", fmt.Errorf("failed to subscribe: %w", err)
    }

    return subscriptionID, nil
}
```

#### 3.5 Add Event Tests

**File:** `internal/service/event/service_test.go`

```go
package event

import (
    "context"
    "testing"
    "time"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/mock"
    "github.com/stretchr/testify/require"
)

type MockProducer struct {
    mock.Mock
}

func (m *MockProducer) Publish(ctx context.Context, msg kafka.Message) error {
    args := m.Called(ctx, msg)
    return args.Error(0)
}

func (m *MockProducer) Close() error {
    args := m.Called()
    return args.Error(0)
}

type MockKafkaClient struct {
    mock.Mock
}

func (m *MockKafkaClient) Producer() (kafka.Producer, error) {
    args := m.Called()
    return args.Get(0).(*MockProducer), args.Error(1)
}

func TestService_PublishMessage_DLQ(t *testing.T) {
    r := require.New(t)

    mockProducer := new(MockProducer)
    mockKafkaClient := new(MockKafkaClient)

    mockKafkaClient.On("Producer").Return(mockProducer, nil)

    firstCall := mockProducer.On("Publish", mock.Anything, mock.MatchedBy(func(msg kafka.Message) bool {
        return msg.Topic == "test-topic"
    })).Return(assert.AnError)

    secondCall := mockProducer.On("Publish", mock.Anything, mock.MatchedBy(func(msg kafka.Message) bool {
        return msg.Topic == DLQTopic
    })).Return(nil)

    service := NewService(mockKafkaClient, nil)

    ctx := context.Background()
    err := service.PublishMessage(ctx, "test-topic", "test-key", "test-value", nil)

    assert.NoError(t, err)
    mockKafkaClient.AssertExpectations(t)
    mockProducer.AssertExpectations(t)
}
```

---

## 4. Database Abstractions

### Current State

**File:** `internal/service/auth/service.go:138-161`

SQL queries embedded in service:

```go
insertUserQuery := `
    INSERT INTO users (id, provider, subject_id, email, full_name, created_at, updated_at)
    VALUES ($1, $2, $3, $4, $5, $6, $7)
`
_, err = s.db.ExecContext(ctx, insertUserQuery, userID, provider, subjectID, email, fullName, now, now)
```

**Issues:**
- No repository pattern
- Queries scattered across services
- No query logging abstraction
- No connection pooling configuration

### Implementation Plan

#### 4.1 Create Repository Package

**File:** `pkg/db/repositories/user.go`

```go
package repositories

import (
    "context"
    "database/sql"
    "fmt"
    "time"

    "gosdk/internal/service/auth"
)

type UserRepository interface {
    FindByProviderAndSubject(ctx context.Context, provider, subjectID string) (*auth.User, error)
    Create(ctx context.Context, user *auth.User) error
    Update(ctx context.Context, user *auth.User) error
    Delete(ctx context.Context, id string) error
    FindByID(ctx context.Context, id string) (*auth.User, error)
}

type SQLUserRepository struct {
    db db.SQLExecutor
}

func NewUserRepository(db db.SQLExecutor) UserRepository {
    return &SQLUserRepository{db: db}
}

func (r *SQLUserRepository) FindByProviderAndSubject(ctx context.Context, provider, subjectID string) (*auth.User, error) {
    query := `
        SELECT id, provider, subject_id, email, full_name, created_at, updated_at
        FROM users
        WHERE provider = $1 AND subject_id = $2
    `

    var user auth.User
    err := r.db.QueryRowContext(ctx, query, provider, subjectID).Scan(
        &user.ID,
        &user.Provider,
        &user.SubjectID,
        &user.Email,
        &user.FullName,
        &user.CreatedAt,
        &user.UpdatedAt,
    )

    if err == sql.ErrNoRows {
        return nil, auth.ErrUserNotFound
    }
    if err != nil {
        return nil, fmt.Errorf("failed to query user: %w", err)
    }

    return &user, nil
}

func (r *SQLUserRepository) Create(ctx context.Context, user *auth.User) error {
    query := `
        INSERT INTO users (id, provider, subject_id, email, full_name, created_at, updated_at)
        VALUES ($1, $2, $3, $4, $5, $6, $7)
    `

    _, err := r.db.ExecContext(ctx, query,
        user.ID, user.Provider, user.SubjectID,
        user.Email, user.FullName, user.CreatedAt, user.UpdatedAt)

    if err != nil {
        return fmt.Errorf("failed to insert user: %w", err)
    }

    return nil
}

func (r *SQLUserRepository) Update(ctx context.Context, user *auth.User) error {
    query := `
        UPDATE users
        SET email = $2, full_name = $3, updated_at = $4
        WHERE id = $1
    `

    _, err := r.db.ExecContext(ctx, query,
        user.ID, user.Email, user.FullName, user.UpdatedAt)

    if err != nil {
        return fmt.Errorf("failed to update user: %w", err)
    }

    return nil
}

func (r *SQLUserRepository) Delete(ctx context.Context, id string) error {
    query := `DELETE FROM users WHERE id = $1`

    _, err := r.db.ExecContext(ctx, query, id)

    if err != nil {
        return fmt.Errorf("failed to delete user: %w", err)
    }

    return nil
}

func (r *SQLUserRepository) FindByID(ctx context.Context, id string) (*auth.User, error) {
    query := `
        SELECT id, provider, subject_id, email, full_name, created_at, updated_at
        FROM users
        WHERE id = $1
    `

    var user auth.User
    err := r.db.QueryRowContext(ctx, query, id).Scan(
        &user.ID,
        &user.Provider,
        &user.SubjectID,
        &user.Email,
        &user.FullName,
        &user.CreatedAt,
        &user.UpdatedAt,
    )

    if err == sql.ErrNoRows {
        return nil, auth.ErrUserNotFound
    }
    if err != nil {
        return nil, fmt.Errorf("failed to query user: %w", err)
    }

    return &user, nil
}
```

#### 4.2 Update Auth Service to Use Repository

**File:** `internal/service/auth/service.go`

```go
package auth

import (
    "context"
    "database/sql"
    "encoding/json"
    "errors"
    "fmt"
    "time"

    "gosdk/internal/service/session"
    "gosdk/pkg/db"
    "gosdk/pkg/oauth2"
    "gosdk/pkg/db/repositories"

    "github.com/google/uuid"
)

type Service struct {
    oauth2Manager *oauth2.Manager
    sessionStore  session.Client
    userRepo      repositories.UserRepository
}

func NewService(oauth2Manager *oauth2.Manager, sessionStore session.Client, userRepo repositories.UserRepository) *Service {
    return &Service{
        oauth2Manager: oauth2Manager,
        sessionStore:  sessionStore,
        userRepo:      userRepo,
    }
}

func (s *Service) GetOrCreateUser(ctx context.Context, provider, subjectID, email, fullName string) (string, error) {
    user, err := s.userRepo.FindByProviderAndSubject(ctx, provider, subjectID)
    if err == nil {
        return user.ID, nil
    }

    if !errors.Is(err, ErrUserNotFound) {
        return "", fmt.Errorf("failed to check user: %w", err)
    }

    userID := uuid.NewString()
    now := time.Now()

    newUser := &User{
        ID:        userID,
        Provider:  provider,
        SubjectID: subjectID,
        Email:     email,
        FullName:  fullName,
        CreatedAt: now,
        UpdatedAt: now,
    }

    if err := s.userRepo.Create(ctx, newUser); err != nil {
        return "", fmt.Errorf("failed to create user: %w", err)
    }

    return userID, nil
}

// Remove getUserByProviderAndSubject (now handled by repository)
```

#### 4.3 Update Server Initialization

**File:** `internal/app/server.go`

```go
func (s *Server) initServices() {
    userRepo := repositories.NewUserRepository(s.db)
    instrumentedDB := db.NewInstrumentedExecutor(s.db, s.logger)

    s.authService = auth.NewService(
        s.oauth2Manager,
        s.sessionStore,
        repositories.NewUserRepository(instrumentedDB),
    )

    s.oauth2Manager.CallbackHandler = func(
        ctx context.Context,
        provider string,
        userInfo *oauth2.UserInfo,
        tokenSet *oauth2.TokenSet,
    ) (*oauth2.CallbackInfo, error) {
        return s.authService.HandleCallback(ctx, provider, userInfo, tokenSet)
    }
}
```

### Testing

**File:** `pkg/db/repositories/user_test.go`

```go
package repositories

import (
    "context"
    "testing"
    "time"

    "gosdk/internal/service/auth"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestSQLUserRepository_Create(t *testing.T) {
    r := require.New(t)
    mockDB := testutil.NewMockDB()

    mockDB.On("ExecContext", mock.Anything, mock.Anything, mock.Anything).Return(&sql.Result{RowsAffected: 1}, nil)

    repo := NewUserRepository(mockDB)

    user := &auth.User{
        ID:        "test-id",
        Provider:  "google",
        SubjectID: "123",
        Email:     "test@example.com",
        FullName:  "Test User",
        CreatedAt: time.Now(),
        UpdatedAt: time.Now(),
    }

    err := repo.Create(context.Background(), user)
    assert.NoError(t, err)
    mockDB.AssertExpectations(t)
}

func TestSQLUserRepository_FindByProviderAndSubject(t *testing.T) {
    r := require.New(t)
    mockDB := testutil.NewMockDB()

    row := &testutil.MockableRow{}
    row.On("Scan", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)

    mockDB.On("QueryRowContext", mock.Anything, mock.Anything, "google", "123").Return(row)

    repo := NewUserRepository(mockDB)

    user, err := repo.FindByProviderAndSubject(context.Background(), "google", "123")
    assert.NoError(t, err)
    assert.NotNil(t, user)

    mockDB.AssertExpectations(t)
    row.AssertExpectations(t)
}
```

---

## Dependencies

New dependencies required:

- `github.com/cenkalti/backoff/v5` - for retry logic

Add to `go.mod`:

```bash
go get github.com/cenkalti/backoff/v5
make ins
```

---

## Estimated Effort

| Task                          | Effort |
|-------------------------------|--------|
| API Documentation             | 4 hours |
| Session Store Improvements   | 6 hours |
| Event Service Improvements   | 5 hours |
| Database Abstractions       | 5 hours |
| **Total**                    | **20 hours** |

---

## Rollback Plan

If any feature causes issues:

1. **API Documentation:**
   - Revert Swagger annotations
   - Remove versioning from routes
   - Keep existing docs

2. **Session Store:**
   - Remove rotation logic
   - Remove rate limiting
   - Remove cleanup job
   - Keep basic implementation

3. **Event Service:**
   - Remove retry logic
   - Remove DLQ support
   - Remove backpressure
   - Keep basic publish/subscribe

4. **Database Abstractions:**
   - Revert auth service to use direct DB queries
   - Remove repository package
   - Keep SQL in services

---

## Success Criteria

After completing Phase 5:

- [ ] Swagger UI displays correctly at `/swagger/index.html`
- [ ] All endpoints have request/response examples
- [ ] All endpoints have error response documentation
- [ ] Security scheme is documented (CookieAuth)
- [ ] API is versioned (`/v1/` prefix)
- [ ] Swagger JSON/YAML generated successfully
- [ ] Session rotation works correctly
- [ ] Session rate limiting prevents abuse
- [ ] Session cleanup job runs without errors
- [ ] Event retry logic handles transient failures
- [ ] DLQ receives failed messages
- [ ] Consumer backpressure limits concurrent processing
- [ ] Repository pattern abstracts database queries
- [ ] Auth service uses repository instead of direct DB
- [ ] Query logging captures all repository operations
- [ ] All new features have tests
- [ ] Overall codebase is production-ready

---

## Final Summary

After completing all 5 phases, the codebase will be:

1. **Phase 1 - Foundation:**
   - Graceful shutdown for all services
   - Health check endpoints for Kubernetes
   - CI/CD pipeline with automated testing
   - Code quality tooling with pre-commit hooks

2. **Phase 2 - Security:**
   - Essential HTTP middleware (CORS, security headers, rate limiting)
   - Comprehensive input validation
   - Structured error handling system

3. **Phase 3 - Observability:**
   - Configuration validation
   - Structured logging throughout
   - Custom metrics and distributed tracing

4. **Phase 4 - Quality:**
   - 80%+ test coverage
   - Reusable test utilities
   - Integration tests

5. **Phase 5 - Polish:**
   - Complete API documentation
   - Advanced session and event features
   - Database repository pattern

**Total Estimated Effort Across All Phases: 90 hours**

The codebase will be production-ready, secure, observable, well-tested, and fully documented.
