package temporal

import (
	"context"
	"fmt"

	"go.temporal.io/api/workflowservice/v1"
)

// HealthChecker defines the interface for Temporal health checking.
type HealthChecker interface {
	// Check verifies the health of the Temporal connection.
	Check(ctx context.Context) error
}

// healthChecker implements HealthChecker for Temporal.
type healthChecker struct {
	client *Client
}

// NewHealthChecker creates a new health checker for the Temporal client.
func NewHealthChecker(client *Client) HealthChecker {
	return &healthChecker{client: client}
}

// Check verifies the Temporal connection by checking the namespace.
// This operation makes an actual server call to verify connectivity.
func (h *healthChecker) Check(ctx context.Context) error {
	if h.client == nil {
		return fmt.Errorf("temporal client is not initialized")
	}

	// Use the underlying connection to make a direct gRPC call
	// This verifies the server is reachable and the namespace exists
	conn := h.client.UnderlyingClient()
	if conn == nil {
		return fmt.Errorf("temporal underlying client is not initialized")
	}

	// Try to describe the namespace as a lightweight health check
	// This requires server connectivity and validates namespace exists
	_, err := conn.WorkflowService().DescribeNamespace(ctx, &workflowservice.DescribeNamespaceRequest{
		Namespace: h.client.config.Namespace,
	})
	if err != nil {
		return fmt.Errorf("temporal health check failed: %w", err)
	}

	return nil
}
