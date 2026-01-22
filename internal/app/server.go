package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"gosdk/cfg"
	"gosdk/internal/app/bootstrap"
	"gosdk/internal/app/health"
	"gosdk/internal/app/middleware"
	"gosdk/internal/app/routes"
	"gosdk/internal/service/auth"
	"gosdk/internal/service/session"
	"gosdk/pkg/cache"
	"gosdk/pkg/db"
	"gosdk/pkg/kafka"
	"gosdk/pkg/logger"
	"gosdk/pkg/oauth2"

	"github.com/gin-gonic/gin"
)

type Server struct {
	config        *cfg.Config
	httpServer    *http.Server
	router        *gin.Engine
	logger        *logger.AppLogger
	db            db.DB
	cache         cache.Cache
	sessionStore  session.Client
	oauth2Manager *oauth2.Manager
	authService   *auth.Service
	kafkaClient   kafka.Client
	shutdown      func(context.Context) error
	httpShutdown  func(context.Context) error
}

func NewServer(ctx context.Context, config *cfg.Config) (*Server, error) {
	s := &Server{
		config: config,
	}

	shutdown, err := bootstrap.InitObservability(ctx, &config.Observability, config.Observability.SamplerRatio)
	if err != nil {
		return nil, fmt.Errorf("observability setup: %w", err)
	}
	s.shutdown = shutdown

	s.logger = logger.NewLogger(config.AppEnv)
	s.logger.Info(ctx, "Initializing server...")

	dsn := fmt.Sprintf(
		"postgres://%s:%s@%s:%s/%s?sslmode=%s",
		config.Postgres.User, config.Postgres.Password, config.Postgres.Host, config.Postgres.Port, config.Postgres.DBName, config.Postgres.SSLMode,
	)

	s.db, err = bootstrap.InitDatabase(dsn, config.Postgres.MigrationPath)
	if err != nil {
		return nil, fmt.Errorf("database init: %w", err)
	}

	s.cache = bootstrap.InitCache(config.Redis.Host, config.Redis.Port)

	s.oauth2Manager, err = bootstrap.InitOAuth2(ctx, &config.OAuth2)
	if err != nil {
		return nil, fmt.Errorf("oauth2 init: %w", err)
	}

	s.initServices()
	s.setupRoutes()

	s.logger.Info(ctx, "Server initialized successfully")
	return s, nil
}

func (s *Server) initServices() {
	s.sessionStore = session.NewRedisStore(s.cache)

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

func (s *Server) setupRoutes() {
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.RequestIDMiddleware())
	r.Use(middleware.LoggingMiddleware(s.logger))
	r.Use(middleware.CORSMiddleware())

	healthChecker := health.NewChecker(s.db, s.cache, s.kafkaClient, s.logger)
	routes.SetupInfra(r, healthChecker)

	authHandler := auth.NewHandler(s.authService, s.config)
	routes.SetupAuth(r, authHandler, s.oauth2Manager)

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
