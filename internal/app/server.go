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

// Server holds all application dependencies
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
	httpShutdown         func(context.Context) error
}

// NewServer creates and initializes a new server instance
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

func (s *Server) setupRoutes() {
	r := gin.New()
	r.Use(gin.Recovery())

	healthChecker := NewHealthChecker(s.db, s.cache, s.kafkaClient, s.logger)
	setupInfraRoutes(r, healthChecker)

	authHandler := auth.NewHandler(s.authService, s.config)
	setupAuthRoutes(r, authHandler, s.oauth2Manager)

	setupMessageBrokerRoutes(r, s.messageBrokerHandler)

	s.router = r
}

func (s *Server) setupHTTPServer() {
	s.httpServer = &http.Server{
		Addr:         ":" + s.config.HTTPServer.Port,
		Handler:      s.router,
		ReadTimeout:  s.config.HTTPServer.ReadTimeout,
		WriteTimeout: s.config.HTTPServer.WriteTimeout,
	}

	s.httpShutdown = func(ctx context.Context) error {
		s.logger.Info(ctx, "Shutting down HTTP server")
		return s.httpServer.Shutdown(ctx)
	}
}

func (s *Server) Run() error {
	s.setupHTTPServer()

	s.logger.Info(context.Background(), "HTTP server listening", logger.Field{Key: "addr", Value: ":" + s.config.HTTPServer.Port})
	err := s.httpServer.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("HTTP server: %w", err)
	}
	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info(ctx, "Starting graceful shutdown")

	var errs []error

	if s.httpShutdown != nil {
		if err := s.httpShutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("HTTP server shutdown: %w", err))
		}
	}

	if s.kafkaClient != nil {
		s.logger.Info(ctx, "Closing Kafka connections")
		if err := s.kafkaClient.Close(); err != nil {
			errs = append(errs, fmt.Errorf("kafka shutdown: %w", err))
		}
	}

	if s.db != nil {
		s.logger.Info(ctx, "Closing database connections")
		if err := s.db.Close(); err != nil {
			errs = append(errs, fmt.Errorf("database shutdown: %w", err))
		}
	}

	if s.cache != nil {
		s.logger.Info(ctx, "Closing Redis connections")
		if err := s.cache.Close(); err != nil {
			errs = append(errs, fmt.Errorf("cache shutdown: %w", err))
		}
	}

	if s.shutdown != nil {
		s.logger.Info(ctx, "Shutting down observability")
		if err := s.shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("observability shutdown: %w", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("shutdown errors: %w", errors.Join(errs...))
	}

	s.logger.Info(ctx, "Shutdown completed successfully")
	return nil
}
