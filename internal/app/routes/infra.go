package routes

import (
	"net/http"

	"gosdk/internal/app/health"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

func SetupInfra(r *gin.Engine, hc *health.Checker, metricsHandler http.Handler) {
	// Use the OTEL metrics handler which exposes metrics from the OTEL SDK
	r.GET("/metrics", gin.WrapH(metricsHandler))
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
