package kafka

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// HealthStatus represents the overall health status
type HealthStatus struct {
	Status    string                 `json:"status"` // "healthy", "degraded", "unhealthy"
	Checks    map[string]HealthCheck `json:"checks"`
	Timestamp time.Time              `json:"timestamp"`
	Details   *PingResult            `json:"details,omitempty"` // Detailed broker status
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

// Check performs a comprehensive health check with detailed broker status
func (h *HealthChecker) Check(ctx context.Context) HealthStatus {
	status := HealthStatus{
		Status:    "healthy",
		Checks:    make(map[string]HealthCheck),
		Timestamp: time.Now(),
	}

	// Check connectivity with detailed broker status
	connectivityCheck, pingResult := h.checkConnectivityDetailed(ctx)
	status.Checks["connectivity"] = connectivityCheck
	status.Details = pingResult

	if connectivityCheck.Status == "unhealthy" {
		status.Status = "unhealthy"
	} else if connectivityCheck.Status == "degraded" {
		status.Status = "degraded"
	}

	// Check producer health
	producerCheck := h.checkProducer(ctx)
	status.Checks["producer"] = producerCheck
	if producerCheck.Status != "healthy" && status.Status == "healthy" {
		status.Status = "degraded"
	}

	return status
}

// CheckLiveness returns a simple liveness check (can we connect to any broker?)
func (h *HealthChecker) CheckLiveness(ctx context.Context) HealthStatus {
	status := HealthStatus{
		Status:    "healthy",
		Checks:    make(map[string]HealthCheck),
		Timestamp: time.Now(),
	}

	check, result := h.checkConnectivityDetailed(ctx)
	status.Checks["liveness"] = check
	status.Details = result

	if check.Status == "unhealthy" {
		status.Status = "unhealthy"
	}

	return status
}

// CheckReadiness returns readiness check (is cluster fully operational?)
func (h *HealthChecker) CheckReadiness(ctx context.Context) HealthStatus {
	return h.Check(ctx)
}

// checkConnectivityDetailed checks connectivity and returns detailed broker status
func (h *HealthChecker) checkConnectivityDetailed(ctx context.Context) (HealthCheck, *PingResult) {
	start := time.Now()

	pingCtx, cancel := context.WithTimeout(ctx, h.timeout)
	defer cancel()

	// Use the detailed ping to get per-broker status
	result := h.client.PingDetailed(pingCtx)

	if result == nil {
		return HealthCheck{
			Status:    "unhealthy",
			Message:   "Failed to get ping result",
			Latency:   time.Since(start),
			LastError: time.Now(),
		}, nil
	}

	// Determine status based on broker connectivity
	var status string
	var message string

	if result.AllHealthy {
		status = "healthy"
		message = fmt.Sprintf("All %d brokers reachable", result.Total)
	} else if result.Healthy {
		status = "degraded"
		// List unhealthy brokers
		var unhealthyBrokers []string
		for _, b := range result.Brokers {
			if !b.Healthy {
				unhealthyBrokers = append(unhealthyBrokers, b.Address)
			}
		}
		message = fmt.Sprintf("%d/%d brokers reachable. Unreachable: %s",
			result.Reachable, result.Total, strings.Join(unhealthyBrokers, ", "))
	} else {
		status = "unhealthy"
		message = fmt.Sprintf("No brokers reachable (%d/%d attempted)",
			result.Reachable, result.Total)
	}

	// Calculate average latency from healthy brokers
	var totalLatency time.Duration
	var healthyCount int
	for _, b := range result.Brokers {
		if b.Healthy {
			totalLatency += b.Latency
			healthyCount++
		}
	}

	avgLatency := time.Since(start)
	if healthyCount > 0 {
		avgLatency = totalLatency / time.Duration(healthyCount)
	}

	return HealthCheck{
		Status:  status,
		Message: message,
		Latency: avgLatency,
	}, result
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
	return strings.Contains(errStr, "Unknown topic") ||
		strings.Contains(errStr, "unknown topic") ||
		strings.Contains(errStr, "TOPIC_AUTHORIZATION_FAILED") ||
		strings.Contains(errStr, "InvalidTopicException")
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
