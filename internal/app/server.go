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

func (s *Server) initDatabase() error {
	pg := s.config.Postgres
	dsn := fmt.Sprintf(
		"postgres://%s:%s@%s:%s/%s?sslmode=%s",
		pg.User, pg.Password, pg.Host, pg.Port, pg.DBName, pg.SSLMode,
	)

	dbClient, err := db.NewSQLClient("postgres", dsn)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	s.db = dbClient

	if err := runMigrations(dsn); err != nil {
		return fmt.Errorf("migrations: %w", err)
	}

	return nil
}

func (s *Server) initCache() error {
	addr := s.config.Redis.Host + ":" + s.config.Redis.Port
	s.cache = cache.NewRedisCache(addr)
	return nil
}

func (s *Server) initOAuth2(ctx context.Context) error {
	mgr, err := oauth2.NewManager(ctx, &s.config.OAuth2)
	if err != nil {
		return err
	}
	s.oauth2Manager = mgr
	return nil
}

func (s *Server) initEvent() error {
	s.kafkaClient = kafka.NewClient(s.config.Kafka.Brokers)
	s.messageBrokerSvc = event.NewService(s.kafkaClient)
	s.messageBrokerHandler = event.NewHandler(s.messageBrokerSvc)
	return nil
}

func (s *Server) initServices() {
	s.authService = auth.NewService(
		s.oauth2Manager,
		s.sessionStore,
		s.db,
	)

	// Wire up OAuth2 callback
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

	healthChecker := NewHealthChecker(s.db, s.cache, s.kafkaClient, s.logger)
	setupInfraRoutes(r, healthChecker)

	authHandler := auth.NewHandler(s.authService, s.config)
	setupAuthRoutes(r, authHandler, s.oauth2Manager)

	setupMessageBrokerRoutes(r, s.messageBrokerHandler)

	s.router = r
}

// Run starts the HTTP server
func (s *Server) Run(addr string) error {
	s.httpServer = &http.Server{
		Addr:    addr,
		Handler: s.router,
	}

	s.logger.Info(context.Background(), "Server listening", logger.Field{Key: "addr", Value: addr})

	err := s.httpServer.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("server error: %w", err)
	}

	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info(ctx, "Starting graceful shutdown")

	var errs []error

	if s.httpServer != nil {
		s.logger.Info(ctx, "Shutting down HTTP server")
		if err := s.httpServer.Shutdown(ctx); err != nil {
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
