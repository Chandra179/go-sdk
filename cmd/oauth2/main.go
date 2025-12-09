package main

import (
	"context"
	"fmt"
	"gosdk/cfg"
	user "gosdk/internal/userservice"
	"gosdk/pkg/db"
	"gosdk/pkg/oauth2"
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
	// User Service
	// ============
	userSvc := user.NewService(dbClient)

	// ============
	// Oauth2
	// ============
	oauth2mgr, err := oauth2.NewManager(context.Background(), &config.OAuth2, userSvc.ResolveUser)
	if err != nil {
		log.Fatal(err)
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
		auth.POST("/login", oauth2.LoginHandler(oauth2mgr))
		auth.POST("/logout", oauth2.LogoutHandler(oauth2mgr))
		auth.GET("/callback/google", oauth2.GoogleCallbackHandler(oauth2mgr))
	}

	if err := r.Run(":8080"); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
