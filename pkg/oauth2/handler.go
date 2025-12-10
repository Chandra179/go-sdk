package oauth2

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// GoogleCallbackHandler handles Google OAuth2 callback
// Note: This handler does NOT set cookies - that's handled by authservice
// @Summary Google OAuth2 callback
// @Description Handles Google OAuth2 callback and returns user info
// @Tags oauth2
// @Produce json
// @Param code query string true "OAuth2 code"
// @Param state query string true "OAuth2 state"
// @Success 200 {object} map[string]interface{} "Authenticated"
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
		info, err := manager.HandleCallback(c, "google", code, state)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.SetSameSite(http.SameSiteLaxMode)
		c.SetCookie(
			info.SessionCookieName,
			info.SessionID,
			info.CookieMaxAge,
			"/",
			"",
			false, // Secure: false for development (set true in production)
			true,  // HttpOnly: not accessible via JavaScript
		)

		c.Status(http.StatusOK)
	}
}
