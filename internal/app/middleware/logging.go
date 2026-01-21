package middleware

import (
	"time"

	"gosdk/pkg/logger"

	"github.com/gin-gonic/gin"
)

func LoggingMiddleware(l logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()

		l.Info(c.Request.Context(), "request completed",
			logger.Field{Key: "path", Value: path},
			logger.Field{Key: "query", Value: query},
			logger.Field{Key: "method", Value: c.Request.Method},
			logger.Field{Key: "status", Value: status},
			logger.Field{Key: "latency", Value: latency},
		)
	}
}
