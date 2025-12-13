package app

import (
	"context"
	"fmt"
	"log"

	"gosdk/cfg"
	"gosdk/internal/service/auth"
	"gosdk/pkg/cache"
	"gosdk/pkg/db"
	"gosdk/pkg/logger"
	"gosdk/pkg/oauth2"
	"gosdk/pkg/session"

	"github.com/gin-gonic/gin"
)

// Server holds all application dependencies
type Server struct {
	config        *cfg.Config
	router        *gin.Engine
	logger        *logger.AppLogger
	db            *db.SQLClient
	cache         cache.Cache
	sessionStore  session.Store
	oauth2Manager *oauth2.Manager
	authService   *auth.Service
	shutdown      func(context.Context) error
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

	s.sessionStore = session.NewInMemoryStore()

	if err := s.initOAuth2(ctx); err != nil {
		return nil, fmt.Errorf("oauth2 init: %w", err)
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

	// Infrastructure endpoints
	setupInfraRoutes(r)

	// Business logic endpoints
	authHandler := auth.NewHandler(s.authService)
	setupAuthRoutes(r, authHandler, s.oauth2Manager)

	s.router = r
}

// Run starts the HTTP server
func (s *Server) Run(addr string) error {
	log.Printf("Server listening on %s", addr)
	return s.router.Run(addr)
}

// Shutdown gracefully shuts down the server
func (s *Server) Shutdown(ctx context.Context) error {
	if s.shutdown != nil {
		if err := s.shutdown(ctx); err != nil {
			return fmt.Errorf("observability shutdown: %w", err)
		}
	}
	return nil
}
