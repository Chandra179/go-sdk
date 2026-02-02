package kafka

import (
	"context"
	"fmt"
	"time"
)

// HealthStatus represents the overall health status
type HealthStatus struct {
	Status    string                 `json:"status"` // "healthy", "degraded", "unhealthy"
	Checks    map[string]HealthCheck `json:"checks"`
	Timestamp time.Time              `json:"timestamp"`
}

// HealthCheck represents an individual health check
type HealthCheck struct {
	Status    string        `json:"status"` // "healthy", "degraded", "unhealthy"
	Message   string        `json:"message,omitempty"`
	Latency   time.Duration `json:"latency,omitempty"`
	LastError time.Time     `json:"last_error,omitempty"`
}

// HealthChecker provides health checking for Kafka components
type HealthChecker struct {
	client  Client
	timeout time.Duration
}

// NewHealthChecker creates a new health checker
func NewHealthChecker(client Client, timeout time.Duration) *HealthChecker {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &HealthChecker{
		client:  client,
		timeout: timeout,
	}
}

// Check performs a comprehensive health check
func (h *HealthChecker) Check(ctx context.Context) HealthStatus {
	status := HealthStatus{
		Status:    "healthy",
		Checks:    make(map[string]HealthCheck),
		Timestamp: time.Now(),
	}

	// Check connectivity
	connectivityCheck := h.checkConnectivity(ctx)
	status.Checks["connectivity"] = connectivityCheck
	if connectivityCheck.Status != "healthy" {
		status.Status = "unhealthy"
	}

	// Check producer health
	producerCheck := h.checkProducer(ctx)
	status.Checks["producer"] = producerCheck
	if producerCheck.Status != "healthy" && status.Status == "healthy" {
		status.Status = "degraded"
	}

	return status
}

// CheckLiveness returns a simple liveness check (can we connect?)
func (h *HealthChecker) CheckLiveness(ctx context.Context) HealthStatus {
	status := HealthStatus{
		Status:    "healthy",
		Checks:    make(map[string]HealthCheck),
		Timestamp: time.Now(),
	}

	check := h.checkConnectivity(ctx)
	status.Checks["liveness"] = check
	if check.Status != "healthy" {
		status.Status = "unhealthy"
	}

	return status
}

// CheckReadiness returns readiness check (is everything operational?)
func (h *HealthChecker) CheckReadiness(ctx context.Context) HealthStatus {
	return h.Check(ctx)
}

func (h *HealthChecker) checkConnectivity(ctx context.Context) HealthCheck {
	start := time.Now()

	pingCtx, cancel := context.WithTimeout(ctx, h.timeout)
	defer cancel()

	err := h.client.Ping(pingCtx)
	if err != nil {
		return HealthCheck{
			Status:    "unhealthy",
			Message:   fmt.Sprintf("Ping failed: %v", err),
			Latency:   time.Since(start),
			LastError: time.Now(),
		}
	}

	return HealthCheck{
		Status:  "healthy",
		Latency: time.Since(start),
	}
}

func (h *HealthChecker) checkProducer(ctx context.Context) HealthCheck {
	start := time.Now()

	producer, err := h.client.Producer()
	if err != nil {
		return HealthCheck{
			Status:    "degraded",
			Message:   fmt.Sprintf("Failed to get producer: %v", err),
			Latency:   time.Since(start),
			LastError: time.Now(),
		}
	}

	// Try to produce a test message to a dummy topic
	testMsg := Message{
		Topic: "__health_check__",
		Key:   []byte("health"),
		Value: []byte(fmt.Sprintf("check-%d", time.Now().UnixNano())),
	}

	pubCtx, cancel := context.WithTimeout(ctx, h.timeout)
	defer cancel()

	err = producer.Publish(pubCtx, testMsg)
	if err != nil {
		// This is expected to fail if __health_check__ topic doesn't exist
		// but the connection itself should work
		if err == ErrTopicNotFound || isTopicNotFound(err) {
			return HealthCheck{
				Status:  "healthy",
				Message: "Producer available (test topic doesn't exist)",
				Latency: time.Since(start),
			}
		}
		return HealthCheck{
			Status:    "degraded",
			Message:   fmt.Sprintf("Producer publish failed: %v", err),
			Latency:   time.Since(start),
			LastError: time.Now(),
		}
	}

	return HealthCheck{
		Status:  "healthy",
		Latency: time.Since(start),
	}
}

// isTopicNotFound checks if an error indicates topic not found
func isTopicNotFound(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	// Common Kafka error messages for unknown topic
	return contains(errStr, "Unknown topic") ||
		contains(errStr, "unknown topic") ||
		contains(errStr, "TOPIC_AUTHORIZATION_FAILED") ||
		contains(errStr, "InvalidTopicException")
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || (len(s) > len(substr) && containsInternal(s, substr)))
}

func containsInternal(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Healthy returns true if status is healthy
func (s HealthStatus) Healthy() bool {
	return s.Status == "healthy"
}

// Degraded returns true if status is degraded
func (s HealthStatus) Degraded() bool {
	return s.Status == "degraded"
}

// Unhealthy returns true if status is unhealthy
func (s HealthStatus) Unhealthy() bool {
	return s.Status == "unhealthy"
}

// String returns a string representation of the health status
func (s HealthStatus) String() string {
	return fmt.Sprintf("Status: %s, Checks: %d, Timestamp: %s",
		s.Status, len(s.Checks), s.Timestamp.Format(time.RFC3339))
}
