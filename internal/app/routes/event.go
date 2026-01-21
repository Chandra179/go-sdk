package routes

import (
	"gosdk/internal/service/event"

	"github.com/gin-gonic/gin"
)

func SetupEvent(r *gin.Engine, handler *event.Handler) {
	mb := r.Group("/messages")
	{
		mb.POST("/publish", handler.PublishHandler())
		mb.POST("/subscribe", handler.SubscribeHandler())
		mb.DELETE("/subscribe/:id", handler.UnsubscribeHandler())
	}
}
