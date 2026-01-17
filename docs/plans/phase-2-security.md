# Phase 2: Security - Middleware, Validation, Error Handling

## Overview

This phase addresses security vulnerabilities and input handling issues. Current code lacks essential HTTP middleware, has no structured validation beyond basic JSON binding, and returns generic error messages that may leak sensitive information.

## Objectives

- Implement essential HTTP middleware (CORS, Security Headers, Rate Limiting, Request ID, Logging)
- Add comprehensive input validation with custom rules
- Create structured error handling system with proper error types

---

## 1. HTTP Middleware

### Current State

**File:** `internal/app/routes.go`

Only `gin.Recovery()` middleware exists:

```go
func (s *Server) setupRoutes() {
    r := gin.New()
    r.Use(gin.Recovery())
    // ... route setup ...
}
```

**Issues:**
- No CORS handling
- No security headers
- No request ID tracking
- No rate limiting
- No request logging
- `AuthMiddleware()` is defined but never applied

### Implementation Plan

#### 1.1 Create Middleware Package Directory

```bash
mkdir -p internal/app/middleware
```

#### 1.2 Create CORS Middleware

**File:** `internal/app/middleware/cors.go`

```go
package middleware

import (
    "github.com/gin-gonic/gin"
)

type CORSConfig struct {
    AllowedOrigins   string
    AllowedMethods   string
    AllowedHeaders   string
    ExposeHeaders    string
    AllowCredentials bool
    MaxAge           int
}

func CORS(config CORSConfig) gin.HandlerFunc {
    return func(c *gin.Context) {
        c.Header("Access-Control-Allow-Origin", config.AllowedOrigins)
        c.Header("Access-Control-Allow-Methods", config.AllowedMethods)
        c.Header("Access-Control-Allow-Headers", config.AllowedHeaders)

        if config.ExposeHeaders != "" {
            c.Header("Access-Control-Expose-Headers", config.ExposeHeaders)
        }

        if config.AllowCredentials {
            c.Header("Access-Control-Allow-Credentials", "true")
        }

        if config.MaxAge > 0 {
            c.Header("Access-Control-Max-Age", string(config.MaxAge))
        }

        if c.Request.Method == "OPTIONS" {
            c.AbortWithStatus(204)
            return
        }

        c.Next()
    }
}
```

#### 1.3 Create Request ID Middleware

**File:** `internal/app/middleware/request_id.go`

```go
package middleware

import (
    "github.com/gin-gonic/gin"
    "github.com/google/uuid"
)

const RequestIDKey = "request_id"

func RequestID() gin.HandlerFunc {
    return func(c *gin.Context) {
        rid := c.GetHeader("X-Request-ID")
        if rid == "" {
            rid = uuid.New().String()
        }

        c.Set(RequestIDKey, rid)
        c.Writer.Header().Set("X-Request-ID", rid)

        c.Next()
    }
}

func GetRequestID(c *gin.Context) string {
    if rid, exists := c.Get(RequestIDKey); exists {
        return rid.(string)
    }
    return ""
}
```

#### 1.4 Create Security Headers Middleware

**File:** `internal/app/middleware/security.go`

```go
package middleware

import (
    "github.com/gin-gonic/gin"
)

func SecurityHeaders() gin.HandlerFunc {
    return func(c *gin.Context) {
        c.Header("X-Content-Type-Options", "nosniff")
        c.Header("X-Frame-Options", "DENY")
        c.Header("X-XSS-Protection", "1; mode=block")
        c.Header("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
        c.Header("Content-Security-Policy", "default-src 'self'")
        c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
        c.Header("Permissions-Policy", "geolocation=(), microphone=()")

        c.Next()
    }
}
```

#### 1.5 Create Rate Limiting Middleware

**File:** `internal/app/middleware/rate_limit.go`

```go
package middleware

import (
    "fmt"
    "strconv"
    "time"

    "github.com/gin-gonic/gin"
    "gosdk/pkg/cache"
)

type RateLimiterConfig struct {
    Requests int
    Window   time.Duration
    KeyFunc  func(*gin.Context) string
}

func RateLimit(cache cache.Cache, config RateLimiterConfig) gin.HandlerFunc {
    if config.KeyFunc == nil {
        config.KeyFunc = func(c *gin.Context) string {
            return fmt.Sprintf("ratelimit:%s", c.ClientIP())
        }
    }

    return func(c *gin.Context) {
        key := config.KeyFunc(c)

        val, err := cache.Get(c.Request.Context(), key)
        if err != nil && err != cache.ErrNotFound {
            c.Next()
            return
        }

        if val != "" {
            count, _ := strconv.Atoi(val)
            if count >= config.Requests {
                c.AbortWithStatusJSON(429, gin.H{
                    "error": "too many requests",
                })
                return
            }
        }

        // Increment counter
        if err := cache.Incr(c.Request.Context(), key); err != nil {
            c.Next()
            return
        }

        // Set expiration if new key
        if val == "" {
            cache.Set(c.Request.Context(), key, "1", config.Window)
        }

        c.Next()
    }
}
```

#### 1.6 Add Incr Method to Cache Interface

**File:** `pkg/cache/client.go`

```go
type Cache interface {
    Set(ctx context.Context, key string, value string, ttl time.Duration) error
    SetNX(ctx context.Context, key string, value string, ttl time.Duration) (bool, error)
    Get(ctx context.Context, key string) (string, error)
    Del(ctx context.Context, key string) error
    Incr(ctx context.Context, key string) error
}
```

**File:** `pkg/cache/redis_cache.go`

```go
func (c *RedisCache) Incr(ctx context.Context, key string) error {
    return c.client.Incr(ctx, key).Err()
}
```

#### 1.7 Create Request Logging Middleware

**File:** `internal/app/middleware/logging.go`

```go
package middleware

import (
    "time"

    "github.com/gin-gonic/gin"
    "gosdk/pkg/logger"
)

type RequestLogger struct {
    logger logger.Logger
}

func NewRequestLogger(logger logger.Logger) *RequestLogger {
    return &RequestLogger{logger: logger}
}

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

#### 1.8 Update Config to Include CORS Settings

**File:** `cfg/config.go`

```go
type Config struct {
    AppEnv        string
    Redis         RedisConfig
    Postgres      PostgresConfig
    OAuth2        Oauth2Config
    Observability  ObservabilityConfig
    Kafka         KafkaConfig
    CORS          CORSConfig
    RateLimit     RateLimitConfig
    ShutdownTimeout time.Duration
}

type CORSConfig struct {
    AllowedOrigins   string
    AllowedMethods   string
    AllowedHeaders   string
    ExposeHeaders    string
    AllowCredentials bool
    MaxAge           int
}

type RateLimitConfig struct {
    Enabled  bool
    Requests int
    Window   time.Duration
}
```

**File:** `cfg/config.go` (in `Load()` function)

```go
func Load() (*Config, error) {
    // ... existing code ...

    cors := CORSConfig{
        AllowedOrigins:   getEnvOrDefault("CORS_ALLOWED_ORIGINS", "*"),
        AllowedMethods:   getEnvOrDefault("CORS_ALLOWED_METHODS", "GET, POST, PUT, DELETE, OPTIONS"),
        AllowedHeaders:   getEnvOrDefault("CORS_ALLOWED_HEADERS", "Authorization, Content-Type, X-Request-ID"),
        ExposeHeaders:    getEnvOrDefault("CORS_EXPOSE_HEADERS", ""),
        AllowCredentials: getEnvOrDefault("CORS_ALLOW_CREDENTIALS", "false") == "true",
        MaxAge:           86400,
    }

    rateLimit := RateLimitConfig{
        Enabled:  getEnvOrDefault("RATE_LIMIT_ENABLED", "false") == "true",
        Requests: 100,
        Window:   time.Minute,
    }

    if rateLimit.Enabled {
        requestsStr := getEnvOrDefault("RATE_LIMIT_REQUESTS", "100")
        requests, err := strconv.Atoi(requestsStr)
        if err != nil {
            errs = append(errs, errors.New("invalid RATE_LIMIT_REQUESTS: "+requestsStr))
        }
        rateLimit.Requests = requests

        windowStr := getEnvOrDefault("RATE_LIMIT_WINDOW", "1m")
        window, err := time.ParseDuration(windowStr)
        if err != nil {
            errs = append(errs, errors.New("invalid RATE_LIMIT_WINDOW: "+windowStr))
        }
        rateLimit.Window = window
    }

    // ... existing validation ...

    return &Config{
        // ... existing config ...
        CORS:      cors,
        RateLimit: rateLimit,
    }, nil
}
```

#### 1.9 Apply Middleware to Routes

**File:** `internal/app/routes.go`

```go
package app

import (
    "gosdk/internal/app/middleware"
    "gosdk/internal/service/auth"
    "gosdk/internal/service/event"
    "gosdk/pkg/oauth2"

    "github.com/gin-gonic/gin"
)

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

    setupInfraRoutes(r)

    authHandler := auth.NewHandler(s.authService)
    setupAuthRoutes(r, authHandler, s.oauth2Manager)

    setupMessageBrokerRoutes(r, s.messageBrokerHandler)

    s.router = r
}
```

### Testing

**File:** `internal/app/middleware/cors_test.go`

```go
package middleware

import (
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/gin-gonic/gin"
    "github.com/stretchr/testify/assert"
)

func TestCORS(t *testing.T) {
    gin.SetMode(gin.TestMode)

    config := CORSConfig{
        AllowedOrigins:   "*",
        AllowedMethods:   "GET, POST",
        AllowedHeaders:   "Authorization",
        AllowCredentials: false,
    }

    router := gin.New()
    router.Use(CORS(config))
    router.GET("/test", func(c *gin.Context) {
        c.JSON(200, gin.H{"ok": true})
    })

    t.Run("OPTIONS request", func(t *testing.T) {
        req := httptest.NewRequest(http.MethodOptions, "/test", nil)
        w := httptest.NewRecorder()
        router.ServeHTTP(w, req)

        assert.Equal(t, 204, w.Code)
        assert.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"))
        assert.Equal(t, "GET, POST", w.Header().Get("Access-Control-Allow-Methods"))
    })

    t.Run("GET request", func(t *testing.T) {
        req := httptest.NewRequest(http.MethodGet, "/test", nil)
        w := httptest.NewRecorder()
        router.ServeHTTP(w, req)

        assert.Equal(t, 200, w.Code)
        assert.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"))
    })
}
```

---

## 2. Input Validation

### Current State

**File:** `internal/service/auth/types.go`

Only basic JSON binding exists:

```go
type LoginRequest struct {
    Provider string `json:"provider" binding:"required"`
}
```

**Issues:**
- No email format validation
- No string length constraints
- No provider name validation
- No custom validation rules

### Implementation Plan

#### 2.1 Update LoginRequest with Validation

**File:** `internal/service/auth/types.go`

```go
package auth

import (
    "errors"
    "gosdk/pkg/oauth2"
    "time"

    "github.com/go-playground/validator/v10"
)

var validate = validator.New()

func ValidateStruct(s interface{}) error {
    return validate.Struct(s)
}

type LoginRequest struct {
    Provider string `json:"provider" binding:"required" validate:"required,oneof=google github"`
}

type RegisterRequest struct {
    Email    string `json:"email" binding:"required" validate:"required,email,max=255"`
    Password string `json:"password" binding:"required" validate:"required,min=8,max=128"`
    Name     string `json:"name" validate:"omitempty,max=100"`
}

type RefreshTokenRequest struct {
    RefreshToken string `json:"refresh_token" binding:"required" validate:"required"`
}

const (
    SessionCookieName  = "session_id"
    CookieMaxAge       = 86400           // 24 hours
    TokenRefreshLeeway = 5 * time.Minute // Refresh tokens 5 minutes before expiry
    SessionTimeout     = 24 * time.Hour  // Session timeout
)

var (
    ErrUserNotFound       = errors.New("user not found")
    ErrInvalidProvider   = errors.New("invalid provider")
    ErrInvalidEmail     = errors.New("invalid email format")
    ErrPasswordTooShort  = errors.New("password must be at least 8 characters")
    ErrPasswordTooLong  = errors.New("password must be at most 128 characters")
)

type User struct {
    ID        string    `json:"id" validate:"required,uuid4"`
    Provider  string    `json:"provider" validate:"required,oneof=google github"`
    SubjectID string    `json:"subject_id" validate:"required"`
    Email     string    `json:"email" validate:"required,email"`
    FullName  string    `json:"full_name" validate:"omitempty,max=100"`
    CreatedAt time.Time `json:"created_at"`
    UpdatedAt time.Time `json:"updated_at"`
}
```

#### 2.2 Create Custom Validator for UUID

**File:** `internal/service/auth/validator.go`

```go
package auth

import (
    "github.com/go-playground/validator/v10"
    "github.com/google/uuid"
)

func RegisterCustomValidators(v *validator.Validate) error {
    if err := v.RegisterValidation("uuid4", validateUUID4); err != nil {
        return err
    }
    return nil
}

func validateUUID4(fl validator.FieldLevel) bool {
    id := fl.Field().String()
    _, err := uuid.Parse(id)
    return err == nil
}
```

#### 2.3 Initialize Validators in Service

**File:** `internal/service/auth/service.go`

```go
func init() {
    if err := RegisterCustomValidators(validate); err != nil {
        panic(err)
    }
}
```

#### 2.4 Update LoginHandler to Use Validation

**File:** `internal/service/auth/handler.go`

```go
func (h *Handler) LoginHandler() gin.HandlerFunc {
    return func(c *gin.Context) {
        var req LoginRequest
        if err := c.ShouldBindJSON(&req); err != nil {
            c.JSON(http.StatusBadRequest, gin.H{
                "error": "invalid request body",
            })
            return
        }

        if err := ValidateStruct(&req); err != nil {
            c.JSON(http.StatusBadRequest, gin.H{
                "error": err.Error(),
            })
            return
        }

        authURL, err := h.service.InitiateLogin(req.Provider)
        if err != nil {
            c.JSON(http.StatusInternalServerError, gin.H{
                "error": "failed to initiate login",
            })
            return
        }

        c.Redirect(http.StatusFound, authURL)
    }
}
```

#### 2.5 Create Validation Error Formatter

**File:** `internal/app/validator.go`

```go
package app

import (
    "strings"

    "github.com/gin-gonic/gin"
    "github.com/go-playground/validator/v10"
)

type ValidationError struct {
    Field   string `json:"field"`
    Message string `json:"message"`
}

func HandleValidationError(c *gin.Context, err error) {
    if validationErr, ok := err.(validator.ValidationErrors); ok {
        var errors []ValidationError
        for _, e := range validationErr {
            errors = append(errors, ValidationError{
                Field:   strings.ToLower(e.Field()),
                Message: getValidationMessage(e),
            })
        }
        c.JSON(http.StatusBadRequest, gin.H{
            "error":  "validation failed",
            "fields": errors,
        })
        return
    }

    c.JSON(http.StatusBadRequest, gin.H{
        "error": "invalid request",
    })
}

func getValidationMessage(e validator.FieldError) string {
    switch e.Tag() {
    case "required":
        return "this field is required"
    case "email":
        return "must be a valid email address"
    case "min":
        return "must be at least " + e.Param() + " characters"
    case "max":
        return "must be at most " + e.Param() + " characters"
    case "oneof":
        return "must be one of: " + e.Param()
    case "uuid4":
        return "must be a valid UUID"
    default:
        return "invalid value"
    }
}
```

### Testing

**File:** `internal/service/auth/validator_test.go`

```go
package auth

import (
    "testing"

    "github.com/stretchr/testify/assert"
)

func TestValidateStruct(t *testing.T) {
    tests := []struct {
        name    string
        input   interface{}
        wantErr bool
    }{
        {
            name: "valid login request",
            input: LoginRequest{
                Provider: "google",
            },
            wantErr: false,
        },
        {
            name: "invalid provider",
            input: LoginRequest{
                Provider: "facebook",
            },
            wantErr: true,
        },
        {
            name: "missing provider",
            input: LoginRequest{},
            wantErr: true,
        },
        {
            name: "valid register request",
            input: RegisterRequest{
                Email:    "test@example.com",
                Password: "password123",
            },
            wantErr: false,
        },
        {
            name: "invalid email",
            input: RegisterRequest{
                Email:    "invalid-email",
                Password: "password123",
            },
            wantErr: true,
        },
        {
            name: "password too short",
            input: RegisterRequest{
                Email:    "test@example.com",
                Password: "short",
            },
            wantErr: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := ValidateStruct(tt.input)
            if tt.wantErr {
                assert.Error(t, err)
            } else {
                assert.NoError(t, err)
            }
        })
    }
}
```

---

## 3. Error Handling

### Current State

**File:** `internal/service/auth/handler.go`

Generic error responses:

```go
c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
```

**Issues:**
- Exposes internal errors to clients
- No structured error types
- No error middleware
- Sensitive information may leak

### Implementation Plan

#### 3.1 Create Error Types Package

**File:** `internal/app/errors/types.go`

```go
package errors

import "net/http"

type AppError struct {
    Code    string `json:"code"`
    Message string `json:"message"`
    Details any    `json:"details,omitempty"`
    Status  int    `json:"-"`
    Err     error  `json:"-"`
}

func (e *AppError) Error() string {
    if e.Err != nil {
        return e.Err.Error()
    }
    return e.Message
}

func (e *AppError) Unwrap() error {
    return e.Err
}

func NewBadRequest(code, message string) *AppError {
    return &AppError{
        Code:    code,
        Message: message,
        Status:  http.StatusBadRequest,
    }
}

func NewBadRequestWithDetails(code, message string, details any) *AppError {
    return &AppError{
        Code:    code,
        Message: message,
        Details: details,
        Status:  http.StatusBadRequest,
    }
}

func NewUnauthorized(code, message string) *AppError {
    return &AppError{
        Code:    code,
        Message: message,
        Status:  http.StatusUnauthorized,
    }
}

func NewForbidden(code, message string) *AppError {
    return &AppError{
        Code:    code,
        Message: message,
        Status:  http.StatusForbidden,
    }
}

func NewNotFound(code, message string) *AppError {
    return &AppError{
        Code:    code,
        Message: message,
        Status:  http.StatusNotFound,
    }
}

func NewConflict(code, message string) *AppError {
    return &AppError{
        Code:    code,
        Message: message,
        Status:  http.StatusConflict,
    }
}

func NewTooManyRequests(code, message string) *AppError {
    return &AppError{
        Code:    code,
        Message: message,
        Status:  http.StatusTooManyRequests,
    }
}

func NewInternal(code, message string) *AppError {
    return &AppError{
        Code:    code,
        Message: message,
        Status:  http.StatusInternalServerError,
    }
}

func Wrap(err error, code, message string) *AppError {
    return &AppError{
        Code:    code,
        Message: message,
        Status:  http.StatusInternalServerError,
        Err:     err,
    }
}
```

#### 3.2 Create Common Error Codes

**File:** `internal/app/errors/codes.go`

```go
package errors

const (
    // Validation errors (400)
    ErrCodeValidationFailed   = "VALIDATION_FAILED"
    ErrCodeInvalidRequest    = "INVALID_REQUEST"
    ErrCodeInvalidCredentials = "INVALID_CREDENTIALS"

    // Authentication errors (401)
    ErrCodeUnauthorized      = "UNAUTHORIZED"
    ErrCodeInvalidToken     = "INVALID_TOKEN"
    ErrCodeTokenExpired     = "TOKEN_EXPIRED"
    ErrCodeSessionExpired   = "SESSION_EXPIRED"

    // Authorization errors (403)
    ErrCodeForbidden        = "FORBIDDEN"

    // Not found errors (404)
    ErrCodeNotFound         = "NOT_FOUND"
    ErrCodeUserNotFound    = "USER_NOT_FOUND"

    // Conflict errors (409)
    ErrCodeConflict        = "CONFLICT"
    ErrCodeUserExists      = "USER_EXISTS"

    // Rate limiting (429)
    ErrCodeTooManyRequests = "TOO_MANY_REQUESTS"

    // Internal errors (500)
    ErrCodeInternal        = "INTERNAL_ERROR"
    ErrCodeDatabaseError   = "DATABASE_ERROR"
    ErrCodeCacheError      = "CACHE_ERROR"
    ErrCodeKafkaError     = "KAFKA_ERROR"
)
```

#### 3.3 Create Error Middleware

**File:** `internal/app/middleware/errors.go`

```go
package middleware

import (
    "errors"
    "net/http"

    "github.com/gin-gonic/gin"
    "gosdk/internal/app/errors"
    "gosdk/pkg/logger"
)

type ErrorHandler struct {
    logger logger.Logger
}

func NewErrorHandler(logger logger.Logger) *ErrorHandler {
    return &ErrorHandler{logger: logger}
}

func (eh *ErrorHandler) Middleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        c.Next()

        if len(c.Errors) == 0 {
            return
        }

        // Handle the first error
        err := c.Errors[0].Err

        if appErr, ok := err.(*errors.AppError); ok {
            eh.logError(c, appErr)
            eh.sendResponse(c, appErr)
            return
        }

        // Handle generic errors
        eh.logError(c, err)
        eh.sendResponse(c, errors.NewInternal(errors.ErrCodeInternal, "internal server error"))
    }
}

func (eh *ErrorHandler) logError(c *gin.Context, err error) {
    fields := []logger.Field{
        {Key: "error", Value: err.Error()},
        {Key: "path", Value: c.Request.URL.Path},
        {Key: "method", Value: c.Request.Method},
    }

    if rid := GetRequestID(c); rid != "" {
        fields = append(fields, logger.Field{Key: "request_id", Value: rid})
    }

    eh.logger.Error(c.Request.Context(), "Request error", fields...)
}

func (eh *ErrorHandler) sendResponse(c *gin.Context, appErr *errors.AppError) {
    // If response was already written, don't overwrite
    if c.Writer.Written() {
        return
    }

    c.JSON(appErr.Status, appErr)
}
```

#### 3.4 Update Service to Use AppError

**File:** `internal/service/auth/service.go`

```go
package auth

import (
    "errors"
    "fmt"

    apperrors "gosdk/internal/app/errors"
    "gosdk/internal/service/session"
    "gosdk/pkg/db"
    "gosdk/pkg/oauth2"
)

var (
    ErrUserNotFound  = apperrors.NewNotFound(apperrors.ErrCodeUserNotFound, "user not found")
    ErrInvalidToken = apperrors.NewUnauthorized(apperrors.ErrCodeInvalidToken, "invalid token")
)

func (s *Service) HandleCallback(ctx context.Context, provider string,
    userInfo *oauth2.UserInfo, tokenSet *oauth2.TokenSet) (*oauth2.CallbackInfo, error) {
    internalUserID, err := s.GetOrCreateUser(ctx, provider, userInfo.ID, userInfo.Email, userInfo.Name)
    if err != nil {
        return nil, apperrors.Wrap(err, apperrors.ErrCodeInternal, "failed to resolve user")
    }

    // ... rest of implementation ...
}

func (s *Service) RefreshToken(ctx context.Context, sessionID string) error {
    sessionData, err := s.GetSessionData(sessionID)
    if err != nil {
        return apperrors.Wrap(err, apperrors.ErrCodeSessionExpired, "session not found")
    }

    if sessionData.TokenSet.RefreshToken == "" {
        return apperrors.NewUnauthorized(apperrors.ErrCodeInvalidToken, "no refresh token available")
    }

    provider, err := s.oauth2Manager.GetProvider(sessionData.Provider)
    if err != nil {
        return apperrors.Wrap(err, apperrors.ErrCodeInternal, "failed to get provider")
    }

    googleProvider, ok := provider.(*oauth2.GoogleOIDCProvider)
    if !ok {
        return apperrors.NewBadRequest(apperrors.ErrCodeInvalidRequest, "provider does not support token refresh")
    }

    newTokenSet, err := googleProvider.RefreshToken(ctx, sessionData.TokenSet.RefreshToken)
    if err != nil {
        s.sessionStore.Delete(sessionID)
        return apperrors.Wrap(err, apperrors.ErrCodeInternal, "failed to refresh token")
    }

    sessionData.TokenSet = newTokenSet
    data, err := json.Marshal(sessionData)
    if err != nil {
        return apperrors.Wrap(err, apperrors.ErrCodeInternal, "failed to marshal session data")
    }

    return s.sessionStore.Update(sessionID, data)
}
```

#### 3.5 Update Handlers to Use AppError

**File:** `internal/service/auth/handler.go`

```go
package auth

import (
    "net/http"

    apperrors "gosdk/internal/app/errors"

    "github.com/gin-gonic/gin"
)

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
```

#### 3.6 Apply Error Middleware to Routes

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

    // Error handler must be last to catch all errors
    errorHandler := middleware.NewErrorHandler(s.logger)
    r.Use(errorHandler.Middleware())

    // ... route setup ...
}
```

### Testing

**File:** `internal/app/errors/types_test.go`

```go
package errors

import (
    "errors"
    "net/http"
    "testing"

    "github.com/stretchr/testify/assert"
)

func TestAppError(t *testing.T) {
    t.Run("NewBadRequest", func(t *testing.T) {
        err := NewBadRequest(ErrCodeInvalidRequest, "bad request")
        assert.Equal(t, ErrCodeInvalidRequest, err.Code)
        assert.Equal(t, "bad request", err.Message)
        assert.Equal(t, http.StatusBadRequest, err.Status)
    })

    t.Run("Wrap", func(t *testing.T) {
        original := errors.New("original error")
        wrapped := Wrap(original, ErrCodeInternal, "wrapped error")

        assert.Equal(t, ErrCodeInternal, wrapped.Code)
        assert.Equal(t, "wrapped error", wrapped.Message)
        assert.ErrorIs(t, wrapped, original)
    })

    t.Run("Unwrap", func(t *testing.T) {
        original := errors.New("original error")
        wrapped := Wrap(original, ErrCodeInternal, "wrapped error")

        unwrapped := errors.Unwrap(wrapped)
        assert.Equal(t, original, unwrapped)
    })
}
```

---

## Dependencies

New dependencies required:

- `github.com/go-playground/validator/v10` - for structured validation

Add to `go.mod`:

```bash
go get github.com/go-playground/validator/v10
make ins
```

---

## Estimated Effort

| Task                 | Effort |
|---------------------|--------|
| HTTP Middleware     | 6 hours |
| Input Validation    | 4 hours |
| Error Handling      | 5 hours |
| **Total**           | **15 hours** |

---

## Rollback Plan

If any feature causes issues:

1. **Middleware:**
   - Remove middleware from `routes.go`
   - Delete `internal/app/middleware/` directory
   - Keep `gin.Recovery()` only

2. **Validation:**
   - Remove validation tags from structs
   - Remove `ValidateStruct()` calls from handlers
   - Keep basic `binding:"required"` only

3. **Error Handling:**
   - Revert to generic error responses
   - Delete `internal/app/errors/` package
   - Remove error middleware from routes

---

## Success Criteria

After completing Phase 2:

- [ ] CORS middleware handles preflight requests correctly
- [ ] Security headers are present on all responses
- [ ] Request ID is generated and returned on all requests
- [ ] Rate limiting prevents abuse when enabled
- [ ] All HTTP requests are logged with request ID
- [ ] Input validation catches invalid emails, passwords, provider names
- [ ] Validation errors return structured JSON with field-level details
- [ ] Internal errors don't expose sensitive information
- [ ] Error middleware catches and logs all errors
- [ ] Standard error codes are used across all endpoints
- [ ] AuthMiddleware is applied to protected routes
