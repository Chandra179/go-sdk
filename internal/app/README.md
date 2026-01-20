# App
do service initialization and service dependency injection here.
function must be depends on the interface not the concrete implementation for some service

## server.go
```go
package app
import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"gosdk/cfg"
	"gosdk/internal/service/auth"
	"gosdk/internal/service/event"
	"gosdk/internal/service/session"
	"gosdk/pkg/cache"
	"gosdk/pkg/db"
	"gosdk/pkg/kafka"
	"gosdk/pkg/logger"
	"gosdk/pkg/oauth2"

	"github.com/gin-gonic/gin"
)

type Server struct {
	config               *cfg.Config
	httpServer           *http.Server
	router               *gin.Engine
	logger               *logger.AppLogger
	db                   db.DB
	cache                cache.Cache
	sessionStore         session.Client
	oauth2Manager        *oauth2.Manager
	authService          *auth.Service
	kafkaClient          kafka.Client
	messageBrokerSvc     *event.Service
	messageBrokerHandler *event.Handler
	shutdown             func(context.Context) error
}

func NewServer(ctx context.Context, config *cfg.Config) (*Server, error) {
	s := &Server{
		config: config,
	}

	shutdown, err := setupObservability(ctx, &config.Observability)
	if err != nil {
		return nil, fmt.Errorf("observability setup: %w", err)
	}
	s.shutdown = shutdown

	s.logger = logger.NewLogger(config.AppEnv)
	s.logger.Info(ctx, "Initializing server...")

	if err := s.initDatabase(); err != nil {
		return nil, fmt.Errorf("database init: %w", err)
	}

	if err := s.initCache(); err != nil {
		return nil, fmt.Errorf("cache init: %w", err)
	}

	s.sessionStore = session.NewRedisStore(s.cache)

	if err := s.initOAuth2(ctx); err != nil {
		return nil, fmt.Errorf("oauth2 init: %w", err)
	}

	if err := s.initEvent(); err != nil {
		return nil, fmt.Errorf("event init: %w", err)
	}

	s.initServices()
	s.setupRoutes()

	s.logger.Info(ctx, "Server initialized successfully")
	return s, nil
}

// Register internal service here
func (s *Server) initServices() {
	s.authService = auth.NewService(
		s.oauth2Manager,
		s.sessionStore,
		s.db,
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

## routes.go
```go
func setupInfraRoutes(r *gin.Engine, hc *HealthChecker) {
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
	r.GET("/docs", docsHandler)
	r.GET("/healthz", hc.Liveness)
	r.GET("/readyz", hc.Readiness)
}

func setupAuthRoutes(r *gin.Engine, handler *auth.Handler, oauth2mgr *oauth2.Manager) {
	auth := r.Group("/auth")
	{
		auth.POST("/login", handler.LoginHandler())
		auth.POST("/logout", handler.LogoutHandler())
		auth.GET("/callback/google", oauth2.GoogleCallbackHandler(oauth2mgr))
	}
}

func setupMessageBrokerRoutes(r *gin.Engine, handler *event.Handler) {
	mb := r.Group("/messages")
	{
		mb.POST("/publish", handler.PublishHandler())
		mb.POST("/subscribe", handler.SubscribeHandler())
		mb.DELETE("/subscribe/:id", handler.UnsubscribeHandler())
	}
}
```