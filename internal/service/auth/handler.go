package auth

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{
		service: service,
	}
}

// LoginHandler initiates OAuth2 login flow
// @Summary Login with OAuth2 provider
// @Description Redirects to OAuth2 provider authorization page
// @Tags auth
// @Accept json
// @Produce json
// @Param request body LoginRequest true "Login request"
// @Success 302 {string} string "Redirect to OAuth2 provider"
// @Failure 400 {object} map[string]string "Bad request"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /auth/login [post]
func (h *Handler) LoginHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req LoginRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "provider is required"})
			return
		}

		authURL, err := h.service.InitiateLogin(req.Provider)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.Redirect(http.StatusFound, authURL)
	}
}

// LogoutHandler logs out the user
// @Summary Logout
// @Description Deletes user session and clears cookie
// @Tags auth
// @Produce json
// @Success 200 {object} map[string]string "Logged out successfully"
// @Router /auth/logout [post]
func (h *Handler) LogoutHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		sessionID, err := c.Cookie(SessionCookieName)

		if err == nil {
			h.service.DeleteSession(sessionID)
		}

		c.SetCookie(
			SessionCookieName,
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

// AuthMiddleware validates session and proactively refreshes tokens
func (h *Handler) AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		sessionID, err := c.Cookie(SessionCookieName)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "no session found"})
			c.Abort()
			return
		}

		// Validate and refresh session if needed
		sessionData, err := h.service.ValidateAndRefreshSession(c.Request.Context(), sessionID)
		if err != nil {
			// Clear cookie and return 401
			c.SetCookie(SessionCookieName, "", -1, "/", "", true, true)
			c.JSON(http.StatusUnauthorized, gin.H{"error": "session expired, please re-login"})
			c.Abort()
			return
		}

		// Store session data in context for downstream handlers
		c.Set("user_id", sessionData.UserID)
		c.Next()
	}
}
