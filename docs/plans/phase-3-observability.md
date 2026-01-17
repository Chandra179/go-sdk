# Phase 3: Observability - Config, Logging, Metrics

## Overview

This phase addresses observability gaps preventing effective monitoring and debugging. Current code lacks configuration validation, uses inconsistent logging (some `fmt.Printf`), and has no custom metrics or tracing for business logic.

## Objectives

- Add comprehensive configuration validation
- Implement structured logging throughout the application
- Add custom metrics for HTTP, OAuth2, database, and cache operations
- Add distributed tracing for critical business flows

---

## 1. Configuration Validation

### Current State

**File:** `cfg/config.go`

Only checks for missing environment variables:

```go
func mustEnv(key string, errs *[]error) string {
    value, exists := os.LookupEnv(key)
    if !exists || value == "" {
        *errs = append(*errs, errors.New("missing env: "+key))
    }
    return value
}
```

**Issues:**
- No validation of JWT secret length
- No port number range validation
- No URL format validation
- No duration format validation beyond what's already done
- No support for configuration files (YAML/TOML)
- No `--validate-config` flag

### Implementation Plan

#### 1.1 Create Validator Package

**File:** `cfg/validator.go`

```go
package cfg

import (
    "errors"
    "fmt"
    "net"
    "net/url"
    "regexp"
    "strconv"
    "time"
)

var (
    ErrJWTSecretTooShort = errors.New("JWT_SECRET must be at least 32 characters")
    ErrInvalidPort       = errors.New("invalid port number (must be 1-65535)")
    ErrInvalidURL       = errors.New("invalid URL format")
)

func (c *Config) Validate() error {
    var errs []error

    if err := c.ValidateJWT(); err != nil {
        errs = append(errs, err)
    }

    if err := c.ValidatePorts(); err != nil {
        errs = append(errs, err)
    }

    if err := c.ValidateURLs(); err != nil {
        errs = append(errs, err)
    }

    if err := c.ValidateDurations(); err != nil {
        errs = append(errs, err)
    }

    if len(errs) > 0 {
        return fmt.Errorf("configuration validation failed: %w", errors.Join(errs...))
    }

    return nil
}

func (c *Config) ValidateJWT() error {
    if len(c.OAuth2.JWTSecret) < 32 {
        return ErrJWTSecretTooShort
    }
    return nil
}

func (c *Config) ValidatePorts() error {
    if err := validatePort(c.Redis.Port, "REDIS_PORT"); err != nil {
        return err
    }
    if err := validatePort(c.Postgres.Port, "POSTGRES_PORT"); err != nil {
        return err
    }
    return nil
}

func validatePort(portStr, name string) error {
    port, err := strconv.Atoi(portStr)
    if err != nil {
        return fmt.Errorf("%s is not a valid number: %w", name, err)
    }
    if port < 1 || port > 65535 {
        return fmt.Errorf("%w: %s", ErrInvalidPort, name)
    }
    return nil
}

func (c *Config) ValidateURLs() error {
    if err := validateURL(c.OAuth2.GoogleRedirectUrl, "GOOGLE_REDIRECT_URL"); err != nil {
        return err
    }
    if err := validateURL(c.OAuth2.GoogleLogoutUrl, "GOOGLE_LOGOUT_URL"); err != nil {
        return err
    }
    if c.Observability.OTLPEndpoint != "" {
        if err := validateURL(c.Observability.OTLPEndpoint, "OTEL_EXPORTER_OTLP_ENDPOINT"); err != nil {
            return err
        }
    }
    return nil
}

func validateURL(urlStr, name string) error {
    if urlStr == "" {
        return nil
    }
    _, err := url.Parse(urlStr)
    if err != nil {
        return fmt.Errorf("%w: %s - %s", ErrInvalidURL, name, err.Error())
    }
    return nil
}

func (c *Config) ValidateDurations() error {
    // Durations are already parsed in Load(), but we can add bounds checking
    if c.OAuth2.JWTExpiration < time.Minute {
        return errors.New("JWT_EXPIRATION must be at least 1 minute")
    }
    if c.OAuth2.JWTExpiration > 30*24*time.Hour {
        return errors.New("JWT_EXPIRATION must be at most 30 days")
    }
    if c.ShutdownTimeout < time.Second {
        return errors.New("SHUTDOWN_TIMEOUT must be at least 1 second")
    }
    return nil
}
```

#### 1.2 Add Configuration File Support

**File:** `cfg/config.go`

```go
package cfg

import (
    "os"
    "strings"
    "time"

    "github.com/spf13/viper"
)
```

**File:** `cfg/config.go` (updated Load function)

```go
func Load() (*Config, error) {
    v := viper.New()

    // Set defaults
    v.SetDefault("app_env", "development")
    v.SetDefault("redis.password", "")
    v.SetDefault("postgres.sslmode", "disable")
    v.SetDefault("cors.allowed_origins", "*")
    v.SetDefault("cors.allowed_methods", "GET, POST, PUT, DELETE, OPTIONS")
    v.SetDefault("cors.allowed_headers", "Authorization, Content-Type, X-Request-ID")
    v.SetDefault("shutdown_timeout", "30s")
    v.SetDefault("rate_limit.enabled", "false")
    v.SetDefault("rate_limit.requests", 100)
    v.SetDefault("rate_limit.window", "1m")

    // Load from config file if exists
    v.SetConfigName("config")
    v.SetConfigType("yaml")
    v.AddConfigPath(".")
    v.AddConfigPath("/etc/gosdk/")
    v.AddConfigPath("$HOME/.gosdk/")

    if err := v.ReadInConfig(); err != nil {
        if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
            return nil, fmt.Errorf("failed to read config file: %w", err)
        }
    }

    // Override with environment variables
    v.AutomaticEnv()
    v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

    var config Config
    if err := v.Unmarshal(&config); err != nil {
        return nil, fmt.Errorf("failed to unmarshal config: %w", err)
    }

    return &config, nil
}
```

#### 1.3 Create Example Configuration File

**File:** `config.yaml.example`

```yaml
# Application Environment
app_env: development

# Redis Configuration
redis:
  host: localhost
  port: "6379"
  password: ""

# OAuth2 Configuration
oauth2:
  google_client_id: your-client-id
  google_client_secret: your-client-secret
  google_redirect_url: http://localhost:8080/auth/callback/google
  google_logout_url: https://accounts.google.com/o/oauth2/revoke
  jwt_secret: your-secret-key-at-least-32-characters
  jwt_expiration: 24h
  state_timeout: 10m

# PostgreSQL Configuration
postgres:
  host: localhost
  port: "5432"
  user: postgres
  password: postgres
  db_name: gosdk
  sslmode: disable

# Observability Configuration
observability:
  otlp_endpoint: http://localhost:4317
  service_name: gosdk

# Kafka Configuration
kafka:
  brokers:
    - localhost:9092

# CORS Configuration
cors:
  allowed_origins: "*"
  allowed_methods: "GET, POST, PUT, DELETE, OPTIONS"
  allowed_headers: "Authorization, Content-Type, X-Request-ID"
  expose_headers: ""
  allow_credentials: false
  max_age: 86400

# Rate Limiting Configuration
rate_limit:
  enabled: false
  requests: 100
  window: 1m

# Shutdown Configuration
shutdown_timeout: 30s
```

#### 1.4 Add Validate Flag to main.go

**File:** `cmd/myapp/main.go`

```go
package main

import (
    "context"
    "flag"
    "log"
    "os"
    "os/signal"
    "syscall"

    "gosdk/cfg"
    "gosdk/internal/app"
)

var validateConfig bool

func init() {
    flag.BoolVar(&validateConfig, "validate-config", false, "validate configuration and exit")
}

func main() {
    flag.Parse()

    config, err := cfg.Load()
    if err != nil {
        log.Fatalf("Failed to load config: %v", err)
    }

    if validateConfig {
        if err := config.Validate(); err != nil {
            log.Fatalf("Configuration validation failed: %v", err)
        }
        log.Println("Configuration is valid")
        os.Exit(0)
    }

    ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
    defer stop()

    server, err := app.NewServer(ctx, config)
    if err != nil {
        log.Fatalf("Failed to initialize server: %v", err)
    }
    defer server.Shutdown(ctx)

    go func() {
        <-ctx.Done()
        log.Println("Shutdown signal received, gracefully shutting down...")
        shutdownCtx, cancel := context.WithTimeout(context.Background(), config.ShutdownTimeout)
        defer cancel()
        if err := server.Shutdown(shutdownCtx); err != nil {
            log.Printf("Error during shutdown: %v", err)
        }
    }()

    if err := server.Run(":8080"); err != nil {
        log.Fatalf("Server error: %v", err)
    }

    log.Println("Server stopped")
}
```

#### 1.5 Add Validation to NewServer

**File:** `internal/app/server.go`

```go
func NewServer(ctx context.Context, config *cfg.Config) (*Server, error) {
    if err := config.Validate(); err != nil {
        return nil, fmt.Errorf("config validation failed: %w", err)
    }

    s := &Server{
        config: config,
    }
    // ... rest of implementation ...
}
```

### Testing

**File:** `cfg/validator_test.go`

```go
package cfg

import (
    "testing"
    "time"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestConfig_Validate(t *testing.T) {
    tests := []struct {
        name    string
        config  *Config
        wantErr bool
        errMsg  string
    }{
        {
            name: "valid config",
            config: &Config{
                OAuth2: Oauth2Config{
                    JWTSecret:     "this-is-a-secret-key-that-is-long-enough",
                    JWTExpiration: 24 * time.Hour,
                    StateTimeout:  10 * time.Minute,
                },
                Redis: RedisConfig{
                    Host: "localhost",
                    Port: "6379",
                },
                Postgres: PostgresConfig{
                    Port: "5432",
                },
                ShutdownTimeout: 30 * time.Second,
            },
            wantErr: false,
        },
        {
            name: "JWT secret too short",
            config: &Config{
                OAuth2: Oauth2Config{
                    JWTSecret: "short",
                },
            },
            wantErr: true,
            errMsg:  "JWT_SECRET",
        },
        {
            name: "invalid port",
            config: &Config{
                Redis: RedisConfig{
                    Port: "99999",
                },
            },
            wantErr: true,
            errMsg:  "invalid port",
        },
        {
            name: "invalid URL",
            config: &Config{
                OAuth2: Oauth2Config{
                    GoogleRedirectUrl: "not-a-url",
                },
            },
            wantErr: true,
            errMsg:  "invalid URL",
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := tt.config.Validate()
            if tt.wantErr {
                require.Error(t, err)
                assert.Contains(t, err.Error(), tt.errMsg)
            } else {
                assert.NoError(t, err)
            }
        })
    }
}
```

---

## 2. Structured Logging

### Current State

**File:** `internal/service/event/service.go:58`

Uses `fmt.Printf` instead of structured logging:

```go
handler := func(msg kafka.Message) error {
    fmt.Printf("Received message: topic=%s, key=%s, value=%s\n", msg.Topic, msg.Key, msg.Value)
    if msg.Headers != nil {
        fmt.Printf("Headers: %v\n", msg.Headers)
    }
    return nil
}
```

**Issues:**
- No request ID in logs
- No correlation ID for distributed tracing
- Inconsistent logging (some use logger, some use fmt.Printf)
- Critical events not logged (login attempts, token refresh failures)

### Implementation Plan

#### 2.1 Replace fmt.Printf in Event Service

**File:** `internal/service/event/service.go`

```go
package event

import (
    "context"
    "fmt"
    "sync"
    "time"

    "gosdk/internal/app/middleware"
    "gosdk/pkg/kafka"
    "gosdk/pkg/logger"

    "github.com/google/uuid"
)

type Service struct {
    kafkaClient kafka.Client
    consumers   map[string]*kafka.KafkaConsumer
    mu          sync.RWMutex
    logger      logger.Logger
}

func NewService(kafkaClient kafka.Client, logger logger.Logger) *Service {
    return &Service{
        kafkaClient: kafkaClient,
        consumers:   make(map[string]*kafka.KafkaConsumer),
        logger:      logger,
    }
}

func (s *Service) PublishMessage(ctx context.Context, topic, key, value string, headers map[string]string) error {
    producer, err := s.kafkaClient.Producer()
    if err != nil {
        s.logger.Error(ctx, "Failed to get producer",
            logger.Field{Key: "error", Value: err.Error()},
        )
        return fmt.Errorf("failed to get producer: %w", err)
    }

    message := kafka.Message{
        Topic:   topic,
        Key:     []byte(key),
        Value:   []byte(value),
        Headers: headers,
    }

    s.logger.Info(ctx, "Publishing message",
        logger.Field{Key: "topic", Value: topic},
        logger.Field{Key: "key", Value: key},
    )

    return producer.Publish(ctx, message)
}

func (s *Service) SubscribeToTopic(ctx context.Context, topic, groupID string) (string, error) {
    subscriptionID := uuid.NewString()

    s.mu.Lock()
    defer s.mu.Unlock()

    if _, exists := s.consumers[subscriptionID]; !exists {
        consumer, err := s.kafkaClient.Consumer(groupID)
        if err != nil {
            s.logger.Error(ctx, "Failed to get consumer",
                logger.Field{Key: "error", Value: err.Error()},
                logger.Field{Key: "group_id", Value: groupID},
            )
            return "", fmt.Errorf("failed to get consumer: %w", err)
        }

        s.consumers[subscriptionID] = consumer.(*kafka.KafkaConsumer)
    }

    handler := func(msg kafka.Message) error {
        s.logger.Info(ctx, "Received message",
            logger.Field{Key: "topic", Value: msg.Topic},
            logger.Field{Key: "key", Value: string(msg.Key)},
            logger.Field{Key: "value", Value: string(msg.Value)},
            logger.Field{Key: "headers", Value: msg.Headers},
        )
        return nil
    }

    consumer := s.consumers[subscriptionID]
    if err := consumer.Subscribe(ctx, []string{topic}, handler); err != nil {
        s.logger.Error(ctx, "Failed to subscribe",
            logger.Field{Key: "error", Value: err.Error()},
            logger.Field{Key: "topic", Value: topic},
        )
        return "", fmt.Errorf("failed to subscribe: %w", err)
    }

    s.logger.Info(ctx, "Successfully subscribed to topic",
        logger.Field{Key: "subscription_id", Value: subscriptionID},
        logger.Field{Key: "topic", Value: topic},
        logger.Field{Key: "group_id", Value: groupID},
    )

    return subscriptionID, nil
}

func (s *Service) Unsubscribe(ctx context.Context, subscriptionID string) error {
    s.mu.Lock()
    defer s.mu.Unlock()

    s.logger.Info(ctx, "Unsubscribing",
        logger.Field{Key: "subscription_id", Value: subscriptionID},
    )

    if consumer, exists := s.consumers[subscriptionID]; exists {
        if err := consumer.Close(); err != nil {
            s.logger.Error(ctx, "Failed to close consumer",
                logger.Field{Key: "error", Value: err.Error()},
                logger.Field{Key: "subscription_id", Value: subscriptionID},
            )
            return fmt.Errorf("failed to close consumer: %w", err)
        }
        delete(s.consumers, subscriptionID)
    }

    return nil
}
```

#### 2.2 Add Request ID to Auth Service Logging

**File:** `internal/service/auth/service.go`

```go
func (s *Service) GetOrCreateUser(ctx context.Context, provider, subjectID, email, fullName string) (string, error) {
    user, err := s.getUserByProviderAndSubject(ctx, provider, subjectID)
    if err == nil {
        s.logger.Info(ctx, "User authenticated",
            logger.Field{Key: "user_id", Value: user.ID},
            logger.Field{Key: "provider", Value: provider},
            logger.Field{Key: "email", Value: user.Email},
        )
        return user.ID, nil
    }

    if !errors.Is(err, ErrUserNotFound) {
        s.logger.Error(ctx, "Failed to check user",
            logger.Field{Key: "error", Value: err.Error()},
            logger.Field{Key: "provider", Value: provider},
            logger.Field{Key: "subject_id", Value: subjectID},
        )
        return "", fmt.Errorf("failed to check user: %w", err)
    }

    userID := uuid.NewString()
    now := time.Now()

    insertUserQuery := `
        INSERT INTO users (id, provider, subject_id, email, full_name, created_at, updated_at)
        VALUES ($1, $2, $3, $4, $5, $6, $7)
    `
    _, err = s.db.ExecContext(ctx, insertUserQuery, userID, provider, subjectID, email, fullName, now, now)
    if err != nil {
        s.logger.Error(ctx, "Failed to create user",
            logger.Field{Key: "error", Value: err.Error()},
            logger.Field{Key: "email", Value: email},
        )
        return "", fmt.Errorf("failed to insert user: %w", err)
    }

    s.logger.Info(ctx, "New user registered",
        logger.Field{Key: "user_id", Value: userID},
        logger.Field{Key: "provider", Value: provider},
        logger.Field{Key: "email", Value: email},
        logger.Field{Key: "full_name", Value: fullName},
    )

    return userID, nil
}

func (s *Service) RefreshToken(ctx context.Context, sessionID string) error {
    sessionData, err := s.GetSessionData(sessionID)
    if err != nil {
        s.logger.Warn(ctx, "Session not found for token refresh",
            logger.Field{Key: "session_id", Value: sessionID},
            logger.Field{Key: "error", Value: err.Error()},
        )
        return err
    }

    if sessionData.TokenSet.RefreshToken == "" {
        s.logger.Warn(ctx, "No refresh token available",
            logger.Field{Key: "session_id", Value: sessionID},
        )
        return errors.New("no refresh token available")
    }

    provider, err := s.oauth2Manager.GetProvider(sessionData.Provider)
    if err != nil {
        s.logger.Error(ctx, "Failed to get provider",
            logger.Field{Key: "error", Value: err.Error()},
            logger.Field{Key: "provider", Value: sessionData.Provider},
        )
        return err
    }

    googleProvider, ok := provider.(*oauth2.GoogleOIDCProvider)
    if !ok {
        s.logger.Warn(ctx, "Provider does not support token refresh",
            logger.Field{Key: "provider", Value: sessionData.Provider},
        )
        return errors.New("provider does not support token refresh")
    }

    s.logger.Info(ctx, "Refreshing token",
        logger.Field{Key: "session_id", Value: sessionID},
        logger.Field{Key: "provider", Value: sessionData.Provider},
    )

    newTokenSet, err := googleProvider.RefreshToken(ctx, sessionData.TokenSet.RefreshToken)
    if err != nil {
        s.logger.Error(ctx, "Failed to refresh token",
            logger.Field{Key: "error", Value: err.Error()},
            logger.Field{Key: "session_id", Value: sessionID},
        )
        s.sessionStore.Delete(sessionID)
        return fmt.Errorf("failed to refresh token (session deleted): %w", err)
    }

    sessionData.TokenSet = newTokenSet
    data, err := json.Marshal(sessionData)
    if err != nil {
        s.logger.Error(ctx, "Failed to marshal session data",
            logger.Field{Key: "error", Value: err.Error()},
        )
        return fmt.Errorf("failed to marshal session data: %w", err)
    }

    if err := s.sessionStore.Update(sessionID, data); err != nil {
        s.logger.Error(ctx, "Failed to update session",
            logger.Field{Key: "error", Value: err.Error()},
            logger.Field{Key: "session_id", Value: sessionID},
        )
        return err
    }

    s.logger.Info(ctx, "Token refreshed successfully",
        logger.Field{Key: "session_id", Value: sessionID},
    )

    return nil
}
```

#### 2.3 Update Middleware Logging

**File:** `internal/app/middleware/logging.go`

```go
func (rl *RequestLogger) Logging() gin.HandlerFunc {
    return func(c *gin.Context) {
        start := time.Now()
        path := c.Request.URL.Path
        query := c.Request.URL.RawQuery

        c.Next()

        latency := time.Since(start)
        status := c.Writer.Status()

        fields := []logger.Field{
            {Key: "request_id", Value: GetRequestID(c)},
            {Key: "method", Value: c.Request.Method},
            {Key: "path", Value: path},
            {Key: "query", Value: query},
            {Key: "status", Value: status},
            {Key: "latency_ms", Value: latency.Milliseconds()},
            {Key: "client_ip", Value: c.ClientIP()},
            {Key: "user_agent", Value: c.Request.UserAgent()},
        }

        if len(c.Errors) > 0 {
            rl.logger.Error(c.Request.Context(), "HTTP request completed with errors", fields...)
        } else {
            rl.logger.Info(c.Request.Context(), "HTTP request completed", fields...)
        }
    }
}
```

#### 2.4 Update Event Service Initialization

**File:** `internal/app/server.go`

```go
func (s *Server) initEvent() error {
    s.kafkaClient = kafka.NewClient(s.config.Kafka.Brokers)
    s.messageBrokerSvc = event.NewService(s.kafkaClient, s.logger)
    s.messageBrokerHandler = event.NewHandler(s.messageBrokerSvc, s.logger)
    return nil
}
```

#### 2.5 Update Event Handler

**File:** `internal/service/event/handler.go`

```go
package event

import (
    "net/http"
    "gosdk/pkg/logger"
    "github.com/gin-gonic/gin"
)

type Handler struct {
    service *Service
    logger  logger.Logger
}

func NewHandler(service *Service, logger logger.Logger) *Handler {
    return &Handler{
        service: service,
        logger:  logger,
    }
}

func (h *Handler) PublishHandler() gin.HandlerFunc {
    return func(c *gin.Context) {
        var req struct {
            Topic   string            `json:"topic" binding:"required"`
            Key     string            `json:"key" binding:"required"`
            Value   string            `json:"value" binding:"required"`
            Headers map[string]string `json:"headers"`
        }

        if err := c.ShouldBindJSON(&req); err != nil {
            h.logger.Warn(c.Request.Context(), "Invalid publish request",
                logger.Field{Key: "error", Value: err.Error()},
            )
            c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
            return
        }

        if err := h.service.PublishMessage(c.Request.Context(), req.Topic, req.Key, req.Value, req.Headers); err != nil {
            h.logger.Error(c.Request.Context(), "Failed to publish message",
                logger.Field{Key: "error", Value: err.Error()},
                logger.Field{Key: "topic", Value: req.Topic},
            )
            c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to publish message"})
            return
        }

        h.logger.Info(c.Request.Context(), "Message published successfully",
            logger.Field{Key: "topic", Value: req.Topic},
        )

        c.JSON(http.StatusOK, gin.H{"message": "published"})
    }
}

func (h *Handler) SubscribeHandler() gin.HandlerFunc {
    return func(c *gin.Context) {
        var req struct {
            Topic   string `json:"topic" binding:"required"`
            GroupID string `json:"group_id" binding:"required"`
        }

        if err := c.ShouldBindJSON(&req); err != nil {
            h.logger.Warn(c.Request.Context(), "Invalid subscribe request",
                logger.Field{Key: "error", Value: err.Error()},
            )
            c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
            return
        }

        subscriptionID, err := h.service.SubscribeToTopic(c.Request.Context(), req.Topic, req.GroupID)
        if err != nil {
            h.logger.Error(c.Request.Context(), "Failed to subscribe",
                logger.Field{Key: "error", Value: err.Error()},
                logger.Field{Key: "topic", Value: req.Topic},
            )
            c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to subscribe"})
            return
        }

        h.logger.Info(c.Request.Context(), "Subscription created",
            logger.Field{Key: "subscription_id", Value: subscriptionID},
            logger.Field{Key: "topic", Value: req.Topic},
        )

        c.JSON(http.StatusOK, gin.H{"subscription_id": subscriptionID})
    }
}

func (h *Handler) UnsubscribeHandler() gin.HandlerFunc {
    return func(c *gin.Context) {
        subscriptionID := c.Param("id")

        if err := h.service.Unsubscribe(c.Request.Context(), subscriptionID); err != nil {
            h.logger.Error(c.Request.Context(), "Failed to unsubscribe",
                logger.Field{Key: "error", Value: err.Error()},
                logger.Field{Key: "subscription_id", Value: subscriptionID},
            )
            c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to unsubscribe"})
            return
        }

        h.logger.Info(c.Request.Context(), "Unsubscribed successfully",
            logger.Field{Key: "subscription_id", Value: subscriptionID},
        )

        c.JSON(http.StatusOK, gin.H{"message": "unsubscribed"})
    }
}
```

### Testing

Verify logs contain request ID and proper structure by running the application and checking logs.

---

## 3. Metrics & Tracing

### Current State

**File:** `internal/app/routes.go:15`

Metrics endpoint exists but no custom metrics:

```go
r.GET("/metrics", gin.WrapH(promhttp.Handler()))
```

**Issues:**
- No custom HTTP metrics
- No OAuth2 flow metrics
- No token refresh metrics
- No database query metrics
- No cache metrics
- No tracing for business logic

### Implementation Plan

#### 3.1 Create Metrics Package

**File:** `internal/app/metrics/metrics.go`

```go
package metrics

import (
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promauto"
)

var (
    // HTTP metrics
    HTTPRequests = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "http_requests_total",
            Help: "Total number of HTTP requests",
        },
        []string{"method", "path", "status"},
    )

    HTTPDuration = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "http_request_duration_seconds",
            Help:    "HTTP request latency",
            Buckets: prometheus.DefBuckets,
        },
        []string{"method", "path"},
    )

    HTTPRequestsInFlight = promauto.NewGauge(
        prometheus.GaugeOpts{
            Name: "http_requests_in_flight",
            Help: "Current number of HTTP requests being served",
        },
    )

    // OAuth2 metrics
    OAuth2Logins = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "oauth2_login_total",
            Help: "Total OAuth2 login attempts",
        },
        []string{"provider", "status"},
    )

    TokenRefreshes = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "token_refresh_total",
            Help: "Total token refresh attempts",
        },
        []string{"status"},
    )

    // Database metrics
    DBQueryDuration = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "db_query_duration_seconds",
            Help:    "Database query latency",
            Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.5, 1},
        },
        []string{"operation"},
    )

    DBConnectionsActive = promauto.NewGauge(
        prometheus.GaugeOpts{
            Name: "db_connections_active",
            Help: "Number of active database connections",
        },
    )

    DBConnectionsIdle = promauto.NewGauge(
        prometheus.GaugeOpts{
            Name: "db_connections_idle",
            Help: "Number of idle database connections",
        },
    )

    // Cache metrics
    CacheHits = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "cache_hits_total",
            Help: "Cache hit count",
        },
        []string{"operation"},
    )

    CacheMisses = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "cache_misses_total",
            Help: "Cache miss count",
        },
        []string{"operation"},
    )

    CacheErrors = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "cache_errors_total",
            Help: "Cache error count",
        },
        []string{"operation"},
    )

    // Kafka metrics
    KafkaMessagesPublished = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "kafka_messages_published_total",
            Help: "Total Kafka messages published",
        },
        []string{"topic", "status"},
    )

    KafkaMessagesConsumed = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "kafka_messages_consumed_total",
            Help: "Total Kafka messages consumed",
        },
        []string{"topic"},
    )
)
```

#### 3.2 Create HTTP Metrics Middleware

**File:** `internal/app/middleware/metrics.go`

```go
package middleware

import (
    "strconv"
    "time"

    "github.com/gin-gonic/gin"
    "gosdk/internal/app/metrics"
)

func MetricsMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        start := time.Now()

        metrics.HTTPRequestsInFlight.Inc()

        c.Next()

        status := strconv.Itoa(c.Writer.Status())
        path := c.FullPath()
        if path == "" {
            path = c.Request.URL.Path
        }

        metrics.HTTPRequests.WithLabelValues(c.Request.Method, path, status).Inc()
        metrics.HTTPDuration.WithLabelValues(c.Request.Method, path).Observe(time.Since(start).Seconds())
        metrics.HTTPRequestsInFlight.Dec()
    }
}
```

#### 3.3 Add OAuth2 Metrics to Auth Service

**File:** `internal/service/auth/service.go`

```go
import (
    "gosdk/internal/app/metrics"
)

func (s *Service) HandleCallback(ctx context.Context, provider string,
    userInfo *oauth2.UserInfo, tokenSet *oauth2.TokenSet) (*oauth2.CallbackInfo, error) {
    metrics.OAuth2Logins.WithLabelValues(provider, "started").Inc()

    internalUserID, err := s.GetOrCreateUser(ctx, provider, userInfo.ID, userInfo.Email, userInfo.Name)
    if err != nil {
        metrics.OAuth2Logins.WithLabelValues(provider, "failed_user").Inc()
        return nil, apperrors.Wrap(err, apperrors.ErrCodeInternal, "failed to resolve user")
    }

    sessionData := &SessionData{
        UserID:   internalUserID,
        TokenSet: tokenSet,
        Provider: provider,
    }

    data, err := json.Marshal(sessionData)
    if err != nil {
        metrics.OAuth2Logins.WithLabelValues(provider, "failed_marshal").Inc()
        return nil, apperrors.Wrap(err, apperrors.ErrCodeInternal, "failed to marshal session data")
    }

    sess, err := s.sessionStore.Create(data, SessionTimeout)
    if err != nil {
        metrics.OAuth2Logins.WithLabelValues(provider, "failed_session").Inc()
        return nil, apperrors.Wrap(err, apperrors.ErrCodeInternal, "failed to create session")
    }

    metrics.OAuth2Logins.WithLabelValues(provider, "success").Inc()

    return &oauth2.CallbackInfo{
        SessionID:         sess.ID,
        UserID:            internalUserID,
        SessionCookieName: SessionCookieName,
        CookieMaxAge:      CookieMaxAge,
    }, nil
}

func (s *Service) RefreshToken(ctx context.Context, sessionID string) error {
    metrics.TokenRefreshes.WithLabelValues("started").Inc()

    sessionData, err := s.GetSessionData(sessionID)
    if err != nil {
        metrics.TokenRefreshes.WithLabelValues("failed_session_not_found").Inc()
        return apperrors.Wrap(err, apperrors.ErrCodeSessionExpired, "session not found")
    }

    // ... rest of implementation ...

    if err := s.sessionStore.Update(sessionID, data); err != nil {
        metrics.TokenRefreshes.WithLabelValues("failed_update").Inc()
        return err
    }

    metrics.TokenRefreshes.WithLabelValues("success").Inc()
    return nil
}
```

#### 3.4 Add Database Metrics

**File:** `pkg/db/instrumented.go`

```go
package db

import (
    "context"
    "database/sql"
    "time"

    "gosdk/internal/app/metrics"
)

type InstrumentedExecutor struct {
    executor SQLExecutor
}

func NewInstrumentedExecutor(executor SQLExecutor) SQLExecutor {
    return &InstrumentedExecutor{executor: executor}
}

func (i *InstrumentedExecutor) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
    start := time.Now()
    result, err := i.executor.ExecContext(ctx, query, args...)

    metrics.DBQueryDuration.WithLabelValues("exec").Observe(time.Since(start).Seconds())

    return result, err
}

func (i *InstrumentedExecutor) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
    start := time.Now()
    rows, err := i.executor.QueryContext(ctx, query, args...)

    metrics.DBQueryDuration.WithLabelValues("query").Observe(time.Since(start).Seconds())

    return rows, err
}

func (i *InstrumentedExecutor) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
    start := time.Now()
    row := i.executor.QueryRowContext(ctx, query, args...)

    metrics.DBQueryDuration.WithLabelValues("query_row").Observe(time.Since(start).Seconds())

    return row
}
```

#### 3.5 Add Cache Metrics

**File:** `pkg/cache/metrics.go`

```go
package cache

import (
    "context"
    "time"

    "gosdk/internal/app/metrics"
)

type InstrumentedCache struct {
    cache Cache
}

func NewInstrumentedCache(cache Cache) Cache {
    return &InstrumentedCache{cache: cache}
}

func (i *InstrumentedCache) Set(ctx context.Context, key string, value string, ttl time.Duration) error {
    err := i.cache.Set(ctx, key, value, ttl)
    if err != nil {
        metrics.CacheErrors.WithLabelValues("set").Inc()
    }
    return err
}

func (i *InstrumentedCache) SetNX(ctx context.Context, key string, value string, ttl time.Duration) (bool, error) {
    ok, err := i.cache.SetNX(ctx, key, value, ttl)
    if err != nil {
        metrics.CacheErrors.WithLabelValues("setnx").Inc()
    }
    return ok, err
}

func (i *InstrumentedCache) Get(ctx context.Context, key string) (string, error) {
    value, err := i.cache.Get(ctx, key)
    if err == ErrNotFound {
        metrics.CacheMisses.WithLabelValues("get").Inc()
        return "", err
    }
    if err != nil {
        metrics.CacheErrors.WithLabelValues("get").Inc()
        return "", err
    }
    metrics.CacheHits.WithLabelValues("get").Inc()
    return value, nil
}

func (i *InstrumentedCache) Del(ctx context.Context, key string) error {
    err := i.cache.Del(ctx, key)
    if err != nil {
        metrics.CacheErrors.WithLabelValues("del").Inc()
    }
    return err
}
```

#### 3.6 Add Kafka Metrics

**File:** `pkg/kafka/metrics.go`

```go
package kafka

import (
    "gosdk/internal/app/metrics"
)

type InstrumentedProducer struct {
    producer Producer
}

func NewInstrumentedProducer(producer Producer) Producer {
    return &InstrumentedProducer{producer: producer}
}

func (i *InstrumentedProducer) Publish(ctx context.Context, msg Message) error {
    err := i.producer.Publish(ctx, msg)
    if err != nil {
        metrics.KafkaMessagesPublished.WithLabelValues(msg.Topic, "failed").Inc()
    } else {
        metrics.KafkaMessagesPublished.WithLabelValues(msg.Topic, "success").Inc()
    }
    return err
}
```

#### 3.7 Apply Metrics Middleware

**File:** `internal/app/routes.go`

```go
func (s *Server) setupRoutes() {
    r := gin.New()

    // Apply middleware
    r.Use(gin.Recovery())
    r.Use(middleware.RequestID())
    r.Use(middleware.SecurityHeaders())
    r.Use(middleware.CORS(middleware.CORSConfig{...}))

    if s.config.RateLimit.Enabled {
        r.Use(middleware.RateLimit(s.cache, middleware.RateLimiterConfig{...}))
    }

    requestLogger := middleware.NewRequestLogger(s.logger)
    r.Use(requestLogger.Logging())
    r.Use(middleware.MetricsMiddleware())

    errorHandler := middleware.NewErrorHandler(s.logger)
    r.Use(errorHandler.Middleware())

    setupInfraRoutes(r)
    // ... rest of route setup ...
}
```

#### 3.8 Update Server Initialization to Use Instrumented Clients

**File:** `internal/app/server.go`

```go
func (s *Server) initDatabase() error {
    pg := s.config.Postgres
    dsn := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s",
        pg.User, pg.Password, pg.Host, pg.Port, pg.DBName, pg.SSLMode)

    dbClient, err := db.NewSQLClient("postgres", dsn)
    if err != nil {
        return fmt.Errorf("connect: %w", err)
    }

    s.db = db.NewInstrumentedExecutor(dbClient)

    if err := runMigrations(dsn); err != nil {
        return fmt.Errorf("migrations: %w", err)
    }

    return nil
}

func (s *Server) initCache() error {
    addr := s.config.Redis.Host + ":" + s.config.Redis.Port
    rawCache := cache.NewRedisCache(addr)
    s.cache = cache.NewInstrumentedCache(rawCache)
    return nil
}

func (s *Server) initEvent() error {
    s.kafkaClient = kafka.NewClient(s.config.Kafka.Brokers)
    s.messageBrokerSvc = event.NewService(s.kafkaClient, s.logger)
    s.messageBrokerHandler = event.NewHandler(s.messageBrokerSvc, s.logger)
    return nil
}
```

### Testing

Verify metrics are exported at `/metrics` by:

1. Run the application
2. Make HTTP requests to trigger metrics
3. Visit `http://localhost:8080/metrics`
4. Verify custom metrics appear with correct labels

---

## Dependencies

New dependencies required:

- `github.com/spf13/viper` - for configuration file support

Add to `go.mod`:

```bash
go get github.com/spf13/viper
make ins
```

---

## Estimated Effort

| Task                 | Effort |
|---------------------|--------|
| Configuration Validation | 5 hours |
| Structured Logging    | 4 hours |
| Metrics & Tracing   | 8 hours |
| **Total**           | **17 hours** |

---

## Rollback Plan

If any feature causes issues:

1. **Configuration Validation:**
   - Remove `config.Validate()` call from `NewServer()`
   - Keep validation code but comment out enforcement
   - Use existing env-only configuration

2. **Structured Logging:**
   - Revert `fmt.Printf` calls if needed
   - Remove logger from event service
   - Remove request ID from logs

3. **Metrics & Tracing:**
   - Remove metrics middleware from routes
   - Remove metric calls from services
   - Delete `internal/app/metrics/` directory
   - Use non-instrumented DB/cache clients

---

## Success Criteria

After completing Phase 3:

- [ ] Configuration validates JWT secret length >= 32
- [ ] Configuration validates port ranges 1-65535
- [ ] Configuration validates URL formats
- [ ] `--validate-config` flag validates and exits
- [ ] Configuration file (YAML) loads with env overrides
- [ ] All `fmt.Printf` statements replaced with logger
- [ ] Request ID appears in all logs
- [ ] Critical events are logged (login, token refresh, errors)
- [ ] Custom HTTP metrics appear at `/metrics`
- [ ] OAuth2 metrics track login attempts by status
- [ ] Token refresh metrics track success/failure
- [ ] Database query duration metrics appear
- [ ] Cache hit/miss/error metrics appear
- [ ] Kafka message metrics appear
- [ ] Metrics middleware is applied to all routes
