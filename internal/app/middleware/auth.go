package middleware

import (
	"gosdk/internal/service/auth"

	"github.com/gin-gonic/gin"
)

func AuthMiddleware(authService *auth.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		sessionID, err := c.Cookie("session_id")
		if err != nil {
			c.AbortWithStatusJSON(401, gin.H{"error": "unauthorized"})
			return
		}

		sessionData, err := authService.ValidateAndRefreshSession(c.Request.Context(), sessionID)
		if err != nil {
			c.AbortWithStatusJSON(401, gin.H{"error": "unauthorized"})
			return
		}

		c.Set("user_id", sessionData.UserID)
		c.Next()
	}
}
