// Package temporal provides infrastructure support for Temporal workflow engine.
// It includes client initialization, worker management, and health checking.
package temporal

import (
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

// Config holds all configuration for Temporal client and workers.
// All non-sensitive configuration values should be set via YAML in internal/cfg/temporal.yaml.
type Config struct {
	// Connection settings
	Host      string
	Port      string
	Namespace string

	// Security - TLS
	TLSEnabled bool
	TLSConfig  *tls.Config

	// Authentication
	APIKey string

	// Worker settings
	MaxConcurrentActivityExecutionSize     int
	MaxConcurrentWorkflowTaskExecutionSize int
	WorkerStopTimeout                      time.Duration

	// Client options
	Logger *slog.Logger
}

// Validate checks if the configuration is valid.
func (c *Config) Validate() error {
	if c.Host == "" {
		return errors.New("temporal host is required")
	}

	if c.Port == "" {
		return errors.New("temporal port is required")
	}

	if c.Namespace == "" {
		return errors.New("temporal namespace is required")
	}

	return nil
}

// Address returns the full Temporal server address.
func (c *Config) Address() string {
	return fmt.Sprintf("%s:%s", c.Host, c.Port)
}
