package health

import (
	"context"
	"net/http"
	"time"

	"gosdk/pkg/logger"

	"github.com/gin-gonic/gin"
)

type Checker struct {
	db     DBChecker
	cache  CacheChecker
	kafka  KafkaChecker
	logger logger.Logger
}

type DBChecker interface {
	PingContext(ctx context.Context) error
	Close() error
}

type CacheChecker interface {
	Ping(ctx context.Context) error
	Close() error
}

type KafkaChecker interface {
	Ping(ctx context.Context) error
	Close() error
}

func NewChecker(db DBChecker, cache CacheChecker, kafka KafkaChecker, logger logger.Logger) *Checker {
	return &Checker{
		db:     db,
		cache:  cache,
		kafka:  kafka,
		logger: logger,
	}
}

type Status struct {
	Status    string            `json:"status"`
	Timestamp string            `json:"timestamp"`
	Checks    map[string]string `json:"checks,omitempty"`
}

func (h *Checker) Liveness(c *gin.Context) {
	c.JSON(http.StatusOK, Status{
		Status:    "ok",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
}

func (h *Checker) Readiness(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	checks := make(map[string]string)
	healthy := true

	if h.db != nil {
		if err := h.db.PingContext(ctx); err != nil {
			checks["database"] = "unhealthy: " + err.Error()
			healthy = false
		} else {
			checks["database"] = "healthy"
		}
	}

	if h.cache != nil {
		if err := h.cache.Ping(ctx); err != nil {
			checks["cache"] = "unhealthy: " + err.Error()
			healthy = false
		} else {
			checks["cache"] = "healthy"
		}
	}

	if h.kafka != nil {
		if err := h.kafka.Ping(ctx); err != nil {
			checks["kafka"] = "unhealthy: " + err.Error()
			healthy = false
		} else {
			checks["kafka"] = "healthy"
		}
	}

	if healthy {
		c.JSON(http.StatusOK, Status{
			Status:    "ready",
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Checks:    checks,
		})
	} else {
		c.JSON(http.StatusServiceUnavailable, Status{
			Status:    "not_ready",
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Checks:    checks,
		})
	}
}
