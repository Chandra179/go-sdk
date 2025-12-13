package app

import (
	"gosdk/internal/service/auth"
	"gosdk/pkg/oauth2"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

func setupInfraRoutes(r *gin.Engine) {
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
	r.GET("/docs", docsHandler)
}

func setupAuthRoutes(r *gin.Engine, handler *auth.Handler, oauth2mgr *oauth2.Manager) {
	auth := r.Group("/auth")
	{
		auth.POST("/login", handler.LoginHandler())
		auth.POST("/logout", handler.LogoutHandler())
		auth.GET("/callback/google", oauth2.GoogleCallbackHandler(oauth2mgr))
	}
}

func docsHandler(c *gin.Context) {
	c.Header("Content-Type", "text/html; charset=utf-8")
	html := `<!DOCTYPE html>...`
	c.String(200, html)
}
