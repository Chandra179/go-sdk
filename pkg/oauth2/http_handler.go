package oauth2

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

// GoogleAuthHandler starts the Google OAuth2 flow
// @Summary Start Google OAuth2 login
// @Description Redirects user to Google OAuth2 login page
// @Tags oauth2
// @Produce json
// @Success 302 {string} string "Redirect"
// @Router /auth/google [get]
func GoogleAuthHandler(manager *Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		authURL, err := manager.GetAuthURL("google")
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.Redirect(http.StatusTemporaryRedirect, authURL)
	}
}

// GithubAuthHandler starts the GitHub OAuth2 flow
// @Summary Start GitHub OAuth2 login
// @Description Redirects user to GitHub OAuth2 login page
// @Tags oauth2
// @Produce json
// @Success 302 {string} string "Redirect"
// @Router /auth/github [get]
func GithubAuthHandler(manager *Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		authURL, err := manager.GetAuthURL("github")
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.Redirect(http.StatusTemporaryRedirect, authURL)
	}
}

// GoogleCallbackHandler handles Google OAuth2 callback
// @Summary Google OAuth2 callback
// @Description Handles Google OAuth2 callback and issues JWT
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

		jwt, userInfo, err := manager.HandleCallback(context.Background(), "google", code, state)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.SetCookie("auth_token", jwt, 3600, "/", "", true, true)
		c.JSON(http.StatusOK, gin.H{
			"message": fmt.Sprintf("Authenticated as: %s (%s)", userInfo.Name, userInfo.Email),
		})
	}
}

// GithubCallbackHandler handles GitHub OAuth2 callback
// @Summary GitHub OAuth2 callback
// @Description Handles GitHub OAuth2 callback and issues JWT
// @Tags oauth2
// @Produce json
// @Param code query string true "OAuth2 code"
// @Param state query string true "OAuth2 state"
// @Success 200 {object} map[string]string "Authenticated"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Router /auth/callback/github [get]
func GithubCallbackHandler(manager *Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		code := c.Query("code")
		state := c.Query("state")

		jwt, userInfo, err := manager.HandleCallback(context.Background(), "github", code, state)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.SetCookie("auth_token", jwt, 3600, "/", "", true, true)
		c.JSON(http.StatusOK, gin.H{
			"message": fmt.Sprintf("Authenticated as: %s (%s)", userInfo.Name, userInfo.Email),
		})
	}
}

// MeHandler returns authenticated user info
// @Summary Get authenticated user info
// @Description Returns user info from JWT token
// @Tags oauth2
// @Produce json
// @Success 200 {object} map[string]string "User info"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Router /api/me [get]
func MeHandler(manager *Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		cookie, err := c.Cookie("auth_token")
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}

		claims, err := manager.ValidateToken(cookie)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"name":     claims.Name,
			"email":    claims.Email,
			"provider": claims.Provider,
		})
	}
}
