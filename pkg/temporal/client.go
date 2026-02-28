package temporal

import (
	"context"
	"fmt"
	"log/slog"

	"go.temporal.io/sdk/client"
)

// Client wraps the Temporal SDK client to provide a simplified interface.
type Client struct {
	client client.Client
	config *Config
	logger *slog.Logger
}

// NewClient creates a new Temporal client with the provided configuration.
func NewClient(cfg *Config) (*Client, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid temporal config: %w", err)
	}

	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	clientOptions := client.Options{
		HostPort:  cfg.Address(),
		Namespace: cfg.Namespace,
		Logger:    &SlogAdapter{logger: logger},
	}

	// Configure TLS if enabled
	if cfg.TLSEnabled && cfg.TLSConfig != nil {
		clientOptions.ConnectionOptions = client.ConnectionOptions{
			TLS: cfg.TLSConfig,
		}
	}

	// Configure API key authentication if provided
	if cfg.APIKey != "" {
		clientOptions.Credentials = client.NewAPIKeyStaticCredentials(cfg.APIKey)
	}

	temporalClient, err := client.Dial(clientOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to temporal server: %w", err)
	}

	logger.Info("connected to temporal server", "address", cfg.Address(), "namespace", cfg.Namespace)

	return &Client{
		client: temporalClient,
		config: cfg,
		logger: logger,
	}, nil
}

// ExecuteWorkflow starts a workflow execution.
func (c *Client) ExecuteWorkflow(ctx context.Context, options client.StartWorkflowOptions, workflow interface{}, args ...interface{}) (client.WorkflowRun, error) {
	if c.client == nil {
		return nil, fmt.Errorf("temporal client is not initialized")
	}

	return c.client.ExecuteWorkflow(ctx, options, workflow, args...)
}

// SignalWorkflow sends a signal to a running workflow.
func (c *Client) SignalWorkflow(ctx context.Context, workflowID string, runID string, signalName string, arg interface{}) error {
	if c.client == nil {
		return fmt.Errorf("temporal client is not initialized")
	}

	return c.client.SignalWorkflow(ctx, workflowID, runID, signalName, arg)
}

// CancelWorkflow cancels a running workflow.
func (c *Client) CancelWorkflow(ctx context.Context, workflowID string, runID string) error {
	if c.client == nil {
		return fmt.Errorf("temporal client is not initialized")
	}

	return c.client.CancelWorkflow(ctx, workflowID, runID)
}

// TerminateWorkflow terminates a running workflow.
func (c *Client) TerminateWorkflow(ctx context.Context, workflowID string, runID string, reason string, details ...interface{}) error {
	if c.client == nil {
		return fmt.Errorf("temporal client is not initialized")
	}

	return c.client.TerminateWorkflow(ctx, workflowID, runID, reason, details...)
}

// QueryWorkflow queries a workflow for its current state.
func (c *Client) QueryWorkflow(ctx context.Context, workflowID string, runID string, queryType string, args ...interface{}) (*client.QueryWorkflowWithOptionsResponse, error) {
	if c.client == nil {
		return nil, fmt.Errorf("temporal client is not initialized")
	}

	return c.client.QueryWorkflowWithOptions(ctx, &client.QueryWorkflowWithOptionsRequest{
		WorkflowID: workflowID,
		RunID:      runID,
		QueryType:  queryType,
		Args:       args,
	})
}

// ListNamespaces lists all available namespaces.
func (c *Client) ListNamespaces(ctx context.Context) ([]string, error) {
	if c.client == nil {
		return nil, fmt.Errorf("temporal client is not initialized")
	}

	// This is a simple health check - in production you might want to use operator service
	// For now, we just return the current namespace as a simple check
	return []string{c.config.Namespace}, nil
}

// Close gracefully closes the Temporal client connection.
func (c *Client) Close() error {
	if c.client == nil {
		return nil
	}

	c.logger.Info("closing temporal client connection")
	c.client.Close()
	return nil
}

// UnderlyingClient returns the underlying Temporal SDK client.
// Use this for advanced operations not covered by the wrapper.
func (c *Client) UnderlyingClient() client.Client {
	return c.client
}

// SlogAdapter adapts slog.Logger to Temporal's logger interface.
type SlogAdapter struct {
	logger *slog.Logger
}

// Debug logs a debug message.
func (s *SlogAdapter) Debug(msg string, keyvals ...interface{}) {
	s.logger.Debug(msg, keyvals...)
}

// Info logs an info message.
func (s *SlogAdapter) Info(msg string, keyvals ...interface{}) {
	s.logger.Info(msg, keyvals...)
}

// Warn logs a warning message.
func (s *SlogAdapter) Warn(msg string, keyvals ...interface{}) {
	s.logger.Warn(msg, keyvals...)
}

// Error logs an error message.
func (s *SlogAdapter) Error(msg string, keyvals ...interface{}) {
	s.logger.Error(msg, keyvals...)
}
