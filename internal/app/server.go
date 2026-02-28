package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"gosdk/internal/app/health"
	"gosdk/internal/app/middleware"
	"gosdk/internal/app/routes"
	"gosdk/internal/cfg"
	"gosdk/internal/service/auth"
	"gosdk/pkg/logger"
	"gosdk/pkg/oauth2"

	"github.com/gin-gonic/gin"
)

// Server is the HTTP server that handles all incoming requests.
// It acts as the transport layer, delegating all business logic to the service layer.
type Server struct {
	config     *cfg.Config
	provider   *Provider
	httpServer *http.Server
	router     *gin.Engine
	logger     *logger.AppLogger
}

// NewServer creates a new HTTP server with all dependencies provided by the Provider.
// The Provider handles all infrastructure and service initialization.
func NewServer(provider *Provider) (*Server, error) {
	s := &Server{
		config:   provider.Config,
		provider: provider,
		logger:   provider.Infra.Logger,
	}

	s.logger.Info(context.Background(), "Creating HTTP server...")

	s.setupRoutes()
	s.setupHTTPServer()

	s.logger.Info(context.Background(), "HTTP server created successfully")
	return s, nil
}

// setupRoutes configures all HTTP routes for the application.
func (s *Server) setupRoutes() {
	r := gin.New()

	// Global middleware
	r.Use(gin.Recovery())
	r.Use(middleware.RequestIDMiddleware())
	r.Use(middleware.LoggingMiddleware(s.logger))
	r.Use(middleware.CORSMiddleware())

	// Health and infrastructure routes
	healthChecker := health.NewChecker(
		s.provider.Infra.DB,
		s.provider.Infra.Cache,
		s.logger,
	)
	routes.SetupInfra(r, healthChecker, s.provider.Infra.MetricsHandler)

	// Auth routes
	authHandler := auth.NewHandler(s.provider.Services.Auth, s.config)
	handlerConfig := &oauth2.HandlerConfig{
		SecureCookies:  s.config.AppEnv == "production",
		CookiePath:     "/",
		RequestTimeout: 30 * time.Second,
	}
	routes.SetupAuth(r, authHandler, s.provider.Infra.OAuth2Manager, handlerConfig)

	s.router = r
}

// setupHTTPServer creates the underlying HTTP server.
func (s *Server) setupHTTPServer() {
	s.httpServer = &http.Server{
		Addr:         ":" + s.config.HTTPServer.Port,
		Handler:      s.router,
		ReadTimeout:  s.config.HTTPServer.ReadTimeout,
		WriteTimeout: s.config.HTTPServer.WriteTimeout,
	}
}

// Run starts the HTTP server and blocks until it shuts down.
func (s *Server) Run() error {
	s.logger.Info(context.Background(), "HTTP server listening",
		logger.Field{Key: "addr", Value: ":" + s.config.HTTPServer.Port})

	err := s.httpServer.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("HTTP server error: %w", err)
	}

	return nil
}

// Shutdown gracefully shuts down the HTTP server.
// Infrastructure resources are managed separately by the Provider.
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info(ctx, "Shutting down HTTP server")

	if s.httpServer != nil {
		if err := s.httpServer.Shutdown(ctx); err != nil {
			return fmt.Errorf("HTTP server shutdown: %w", err)
		}
	}

	s.logger.Info(ctx, "HTTP server shutdown complete")
	return nil
}
