package main

import (
	"context"
	"fmt"
	"gosdk/cfg"
	"gosdk/internal/authservice"
	"gosdk/pkg/db"
	"gosdk/pkg/oauth2"
	"gosdk/pkg/session"
	"log"

	"github.com/gin-gonic/gin"
	"github.com/golang-migrate/migrate/v4"
)

func main() {
	// ============
	// config
	// ============
	config, errCfg := cfg.Load()
	if errCfg != nil {
		log.Fatal(errCfg)
	}

	// ============
	// Postgres
	// ============
	pg := config.Postgres
	pgDSN := fmt.Sprintf(
		"postgres://%s:%s@%s:%s/%s?sslmode=%s",
		pg.User,
		pg.Password,
		pg.Host,
		pg.Port,
		pg.DBName,
		pg.SSLMode,
	)

	// =========
	// Migrate
	// =========
	m, err := migrate.New("file://db/migrations", pgDSN)
	if err != nil {
		log.Fatal(err)
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		log.Fatal(err)
	}

	// ============
	// Sql Executor
	// ============
	dbClient, err := db.NewSQLClient("postgres", pgDSN)
	if err != nil {
		log.Fatal(err)
	}

	// ============
	// Session Store
	// ============
	sessionStore := session.NewInMemoryStore()

	// ============
	// Oauth2
	// ============
	oauth2mgr, err := oauth2.NewManager(context.Background(), &config.OAuth2)
	if err != nil {
		log.Fatal(err)
	}

	// ============
	// Auth Service
	// ============
	authSvc := authservice.NewService(oauth2mgr, sessionStore, dbClient)
	authHandler := authservice.NewHandler(authSvc)
	oauth2mgr.CallbackHandler = func(ctx context.Context, provider string, userInfo *oauth2.UserInfo,
		tokenSet *oauth2.TokenSet) (*oauth2.CallbackInfo, error) {
		return authSvc.HandleCallback(ctx, provider, userInfo, tokenSet)
	}

	// ============
	// Gin Engine
	// ============
	r := gin.Default()

	// ============
	// Auth Endpoints
	// ============
	auth := r.Group("/auth")
	{
		auth.POST("/login", authHandler.LoginHandler())
		auth.POST("/logout", authHandler.LogoutHandler())
		auth.GET("/callback/google", oauth2.GoogleCallbackHandler(oauth2mgr))
	}

	if err := r.Run(":8080"); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
