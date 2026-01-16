package event

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

func (h *Handler) PublishHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req PublishRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if err := h.service.PublishMessage(c.Request.Context(), req.Topic, req.Key, req.Value, nil); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "message published successfully"})
	}
}

func (h *Handler) SubscribeHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req SubscribeRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		subscriptionID, err := h.service.SubscribeToTopic(c.Request.Context(), req.Topic, req.GroupID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message":         "subscribed successfully",
			"subscription_id": subscriptionID,
		})
	}
}

func (h *Handler) UnsubscribeHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		subscriptionID := c.Param("id")

		if err := h.service.Unsubscribe(subscriptionID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "unsubscribed successfully"})
	}
}
