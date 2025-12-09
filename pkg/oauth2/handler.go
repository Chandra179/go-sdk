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

// LoginRequest represents the login request body
type LoginRequest struct {
	Provider string `json:"provider" binding:"required"` // "google" or "github"
}

// LoginHandler initiates OAuth2 login flow
// @Summary Login with OAuth2 provider
// @Description Redirects to OAuth2 provider authorization page
// @Tags oauth2
// @Accept json
// @Produce json
// @Param request body LoginRequest true "Login request"
// @Success 302 {string} string "Redirect to OAuth2 provider"
// @Failure 400 {object} map[string]string "Bad request"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /auth/login [post]
func LoginHandler(manager *Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req LoginRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "provider is required"})
			return
		}

		// Get authorization URL from OAuth2 manager
		authURL, err := manager.GetAuthURL(req.Provider)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		// Redirect user to OAuth2 provider
		c.Redirect(http.StatusFound, authURL)
	}
}

// LogoutHandler logs out the user
// @Summary Logout
// @Description Deletes user session, clears cookie, and performs upstream provider logout
// @Tags oauth2
// @Produce json
// @Router /auth/logout [post]
func LogoutHandler(manager *Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		sessionID, err := c.Cookie(sessionCookieName)

		if err == nil {
			manager.DeleteSession(sessionID)
		}

		c.SetCookie(
			sessionCookieName,
			"",
			-1,
			"/",
			"",
			true,
			true,
		)

		c.JSON(http.StatusOK, gin.H{"message": "logged out"})
	}
}

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
