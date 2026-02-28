package bootstrap

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log/slog"
	"os"

	"gosdk/internal/cfg"
	"gosdk/pkg/temporal"
)

// InitTemporal initializes the Temporal client and worker manager.
// Returns nil if Temporal is not enabled in configuration.
func InitTemporal(config *cfg.TemporalConfig, logger *slog.Logger) (*temporal.Client, *temporal.WorkerManager, error) {
	if config == nil || !config.Enabled {
		logger.Info("temporal is disabled, skipping initialization")
		return nil, nil, nil
	}

	logger.Info("initializing temporal client", "host", config.Host, "port", config.Port, "namespace", config.Namespace)

	// Create temporal configuration from cfg.TemporalConfig
	// All defaults come from YAML (internal/cfg/temporal.yaml)
	temporalCfg := &temporal.Config{
		Host:                                   config.Host,
		Port:                                   config.Port,
		Namespace:                              config.Namespace,
		TLSEnabled:                             config.TLSEnabled,
		Logger:                                 logger,
		MaxConcurrentActivityExecutionSize:     config.MaxConcurrentActivityExecutionSize,
		MaxConcurrentWorkflowTaskExecutionSize: config.MaxConcurrentWorkflowTaskExecutionSize,
		WorkerStopTimeout:                      config.WorkerStopTimeout,
	}

	// Configure TLS if enabled
	if config.TLSEnabled {
		tlsConfig, err := loadTemporalTLSConfig(config, logger)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to load temporal TLS config: %w", err)
		}
		temporalCfg.TLSConfig = tlsConfig
	}

	// Create client
	client, err := temporal.NewClient(temporalCfg)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create temporal client: %w", err)
	}

	// Create worker manager
	workerManager := temporal.NewWorkerManager(client, temporalCfg)

	logger.Info("temporal client and worker manager initialized successfully")

	return client, workerManager, nil
}

// loadTemporalTLSConfig loads TLS configuration from certificate files
func loadTemporalTLSConfig(config *cfg.TemporalConfig, logger *slog.Logger) (*tls.Config, error) {
	if config.TLSCertFile == "" || config.TLSKeyFile == "" {
		logger.Warn("TLS is enabled but certificate files are not configured, using system defaults")
		return &tls.Config{MinVersion: tls.VersionTLS12}, nil
	}

	// Load client certificate
	cert, err := tls.LoadX509KeyPair(config.TLSCertFile, config.TLSKeyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load client certificate: %w", err)
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}

	// Load CA certificate if provided
	if config.TLSCAFile != "" {
		caCert, err := os.ReadFile(config.TLSCAFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA certificate: %w", err)
		}

		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to parse CA certificate")
		}
		tlsConfig.RootCAs = caCertPool
	}

	logger.Info("TLS configuration loaded successfully")
	return tlsConfig, nil
}
