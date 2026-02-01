package routes

import (
	"gosdk/internal/service/auth"
	"gosdk/pkg/oauth2"

	"github.com/gin-gonic/gin"
)

func SetupAuth(r *gin.Engine, handler *auth.Handler, oauth2mgr *oauth2.Manager, handlerConfig *oauth2.HandlerConfig) {
	auth := r.Group("/auth")
	{
		auth.POST("/login", handler.LoginHandler())
		auth.POST("/logout", handler.LogoutHandler())
		auth.GET("/callback/google", oauth2.GoogleCallbackHandler(oauth2mgr, handlerConfig))
	}
}
