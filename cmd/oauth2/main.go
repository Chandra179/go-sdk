package main

import (
	"gosdk/cfg"
	"gosdk/pkg/oauth2"
	"log"

	"github.com/gin-gonic/gin"
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
	// Oauth2
	// ============
	oauth2mgr, err := oauth2.NewManager(&config.OAuth2)
	if err != nil {
		log.Fatal(err)
	}

	// ============
	// HTTP
	// ============
	r := gin.Default()
	auth := r.Group("/auth")
	{
		auth.GET("/google", oauth2.GoogleAuthHandler(oauth2mgr))
		auth.GET("/callback/google", oauth2.GoogleCallbackHandler(oauth2mgr))
		auth.GET("/github", oauth2.GithubAuthHandler(oauth2mgr))
		auth.GET("/callback/github", oauth2.GithubCallbackHandler(oauth2mgr))
	}

	api := r.Group("/api")
	api.GET("/me", oauth2.MeHandler(oauth2mgr))

	if err := r.Run(":8080"); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
