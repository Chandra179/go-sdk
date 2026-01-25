package routes

import (
	"gosdk/internal/app/health"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

func SetupInfra(r *gin.Engine, hc *health.Checker) {
	// Use the legacy prometheus handler for now - this should be replaced with OTEL metrics
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
	r.GET("/docs", docsHandler)
	r.GET("/healthz", hc.Liveness)
	r.GET("/readyz", hc.Readiness)
}

func docsHandler(c *gin.Context) {
	c.Header("Content-Type", "text/html; charset=utf-8")
	html := `<!DOCTYPE html>...`
	c.String(200, html)
}
