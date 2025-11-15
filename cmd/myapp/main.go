package main

import (
	"fmt"
	"gosdk/cfg"
	"gosdk/pkg/cache"
	"gosdk/pkg/logger"
	"gosdk/pkg/oauth2"
	"log"

	_ "gosdk/api" // swagger docs

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
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
	// logger
	// ============
	zlogger := logger.NewZeroLog(config.AppEnv)
	fmt.Println(zlogger)

	// ============
	// cache
	// ============
	redisCache := cache.NewRedisCache(config.Redis.Host + ":" + config.Redis.Port)
	fmt.Println(redisCache)

	// ============
	// oauth2
	// ============
	oauth2mgr, err := oauth2.NewManager(&config.OAuth2)
	if err != nil {
		log.Fatal(err)
	}

	r := gin.Default()

	// ============
	// swagger & redoc
	// ============
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// Serve Redoc
	r.GET("/docs", func(c *gin.Context) {
		c.Header("Content-Type", "text/html; charset=utf-8")
		html := `<!DOCTYPE html>
<html>
<head>
    <title>API Documentation</title>
    <meta charset="utf-8"/>
    <meta name="viewport" content="width=device-width, initial-scale=1">
</head>
<body>
    <script id="api-reference" data-url="/swagger/doc.json"></script>
    <script src="https://cdn.jsdelivr.net/npm/@scalar/api-reference"></script>
</body>
</html>`
		c.String(200, html)
	})

	r.GET("/rapidoc", func(c *gin.Context) {
		c.Header("Content-Type", "text/html; charset=utf-8")
		html := `<!DOCTYPE html>
<html>
<head>
    <title>API Documentation</title>
    <meta charset="utf-8"/>
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <script type="module" src="https://unpkg.com/rapidoc/dist/rapidoc-min.js"></script>
</head>
<body>
    <rapi-doc 
        spec-url="/swagger/doc.json"
        theme="dark"
        render-style="read"
        show-header="false"
        allow-try="true"
        allow-server-selection="true"
    ></rapi-doc>
</body>
</html>`
		c.String(200, html)
	})

	// ============
	// HTTP handler
	// ============
	auth := r.Group("/auth")
	{
		auth.GET("/google", oauth2.GoogleAuthHandler(oauth2mgr))
		auth.GET("/callback/google", oauth2.GoogleCallbackHandler(oauth2mgr))
		auth.GET("/github", oauth2.GithubAuthHandler(oauth2mgr))
		auth.GET("/callback/github", oauth2.GithubCallbackHandler(oauth2mgr))
	}

	api := r.Group("/api")
	api.GET("/me", oauth2.MeHandler(oauth2mgr))

	r.Run(":8080")
}
