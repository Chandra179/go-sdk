package oauth2

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	sessionCookieName  = "session_id"
	cookieMaxAge       = 86400           // 24 hours
	tokenRefreshLeeway = 5 * time.Minute // Refresh tokens 5 minutes before expiry
)

// GoogleCallbackHandler handles Google OAuth2 callback
// @Summary Google OAuth2 callback
// @Description Handles Google OAuth2 callback and creates session
// @Tags oauth2
// @Produce json
// @Param code query string true "OAuth2 code"
// @Param state query string true "OAuth2 state"
// @Success 200 {object} map[string]string "Authenticated"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Router /auth/callback/google [get]
func GoogleCallbackHandler(manager *Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		code := c.Query("code")
		state := c.Query("state")

		if code == "" || state == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "missing code or state"})
			return
		}

		// TODO: context with timeout
		sessionID, userInfo, err := manager.HandleCallback(context.Background(), "google", code, state)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.SetSameSite(http.SameSiteLaxMode)
		c.SetCookie(
			sessionCookieName,
			sessionID,
			cookieMaxAge,
			"/",
			"",
			false, // Secure: false for development (set true in production)
			true,  // HttpOnly: not accessible via JavaScript
		)

		c.JSON(http.StatusOK, gin.H{
			"message": fmt.Sprintf("Authenticated as: %s (%s)", userInfo.Name, userInfo.Email),
			"user":    userInfo,
		})
	}
}

// AuthMiddleware is a middleware that validates session and proactively refreshes tokens
func AuthMiddleware(manager *Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		sessionID, err := c.Cookie(sessionCookieName)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "no session found"})
			c.Abort()
			return
		}

		session, err := manager.GetSession(sessionID)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired session"})
			c.Abort()
			return
		}

		// This covers BOTH:
		// - Already expired: timeUntilExpiry is negative (< 5 min)
		// - Expires soon: timeUntilExpiry is 0-5 minutes
		if time.Until(session.TokenSet.ExpiresAt) <= tokenRefreshLeeway {
			if err := manager.RefreshSession(context.Background(), sessionID); err != nil {
				// Refresh failed - session has been deleted by manager
				// Clear cookie and return 401
				c.SetCookie(sessionCookieName, "", -1, "/", "", true, true)
				c.JSON(http.StatusUnauthorized, gin.H{"error": "session expired, please re-login"})
				c.Abort()
				return
			}

			session, err = manager.GetSession(sessionID)
			if err != nil {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "session error after refresh"})
				c.Abort()
				return
			}
		}

		// Store session in context for downstream handlers
		c.Set("session", session)

		c.Next()
	}
}
