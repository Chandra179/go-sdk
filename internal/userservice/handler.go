package user

import (
	"gosdk/pkg/oauth2"
	"net/http"

	"github.com/gin-gonic/gin"
)

const (
	sessionCookieName = "session_id"
)

// Handler handles user-related HTTP endpoints
type Handler struct {
	oauth2Manager *oauth2.Manager
}

// NewHandler creates a new user handler
func NewHandler(oauth2Manager *oauth2.Manager) *Handler {
	return &Handler{
		oauth2Manager: oauth2Manager,
	}
}

// LoginRequest represents the login request body
type LoginRequest struct {
	Provider string `json:"provider" binding:"required"` // "google" or "github"
}

// LoginHandler initiates OAuth2 login flow
// @Summary Login with OAuth2 provider
// @Description Redirects to OAuth2 provider authorization page
// @Tags user
// @Accept json
// @Produce json
// @Param request body LoginRequest true "Login request"
// @Success 302 {string} string "Redirect to OAuth2 provider"
// @Failure 400 {object} map[string]string "Bad request"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /api/user/login [post]
func (h *Handler) LoginHandler(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "provider is required"})
		return
	}

	// Get authorization URL from OAuth2 manager
	authURL, err := h.oauth2Manager.GetAuthURL(req.Provider)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Redirect user to OAuth2 provider
	c.Redirect(http.StatusFound, authURL)
}

// LogoutHandler logs out the user
// @Summary Logout
// @Description Deletes user session, clears cookie, and performs upstream provider logout
// @Tags user
// @Produce json
// @Router /user/logout [post]
func (h *Handler) LogoutHandler(c *gin.Context) {
	sessionID, err := c.Cookie(sessionCookieName)

	if err == nil {
		h.oauth2Manager.DeleteSession(sessionID)
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
