package cfg

import (
	"errors"
	"os"
	"time"

	"github.com/goccy/go-yaml"
)

// Temporal YAML config file path
const temporalYAMLPath = "internal/cfg/temporal.yaml"

// Environment variable names for TLS secrets
const (
	envTemporalTLSCertFile = "TEMPORAL_TLS_CERT_FILE"
	envTemporalTLSKeyFile  = "TEMPORAL_TLS_KEY_FILE"
	envTemporalTLSCAFile   = "TEMPORAL_TLS_CA_FILE"
)

// TemporalYAMLConfig represents the YAML configuration structure
type TemporalYAMLConfig struct {
	Enabled    bool   `yaml:"enabled"`
	Host       string `yaml:"host"`
	Port       string `yaml:"port"`
	Namespace  string `yaml:"namespace"`
	TLSEnabled bool   `yaml:"tls_enabled"`
	Worker     struct {
		MaxConcurrentActivityExecutionSize     int `yaml:"max_concurrent_activity_execution_size"`
		MaxConcurrentWorkflowTaskExecutionSize int `yaml:"max_concurrent_workflow_task_execution_size"`
		StopTimeoutSeconds                     int `yaml:"stop_timeout_seconds"`
	} `yaml:"worker"`
	DefaultTaskQueue string `yaml:"default_task_queue"`
}

// TemporalConfig holds configuration for Temporal workflow engine.
type TemporalConfig struct {
	Enabled    bool
	Host       string
	Port       string
	Namespace  string
	TLSEnabled bool

	// TLS certificate file paths (loaded from env)
	TLSCertFile string
	TLSKeyFile  string
	TLSCAFile   string

	// Worker settings (loaded from YAML)
	MaxConcurrentActivityExecutionSize     int
	MaxConcurrentWorkflowTaskExecutionSize int
	WorkerStopTimeout                      time.Duration
	DefaultTaskQueue                       string
}

// loadTemporal loads Temporal configuration from YAML and environment variables.
func (l *Loader) loadTemporal() *TemporalConfig {
	// Load YAML configuration
	yamlCfg, err := l.loadTemporalYAML()
	if err != nil {
		l.errs = append(l.errs, errors.New("failed to load temporal yaml config: "+err.Error()))
		return nil
	}

	return &TemporalConfig{
		Enabled:    yamlCfg.Enabled,
		Host:       yamlCfg.Host,
		Port:       yamlCfg.Port,
		Namespace:  yamlCfg.Namespace,
		TLSEnabled: yamlCfg.TLSEnabled,

		// TLS certificate files from env (secrets)
		TLSCertFile: l.getEnvWithDefault(envTemporalTLSCertFile, ""),
		TLSKeyFile:  l.getEnvWithDefault(envTemporalTLSKeyFile, ""),
		TLSCAFile:   l.getEnvWithDefault(envTemporalTLSCAFile, ""),

		// Worker settings from YAML
		MaxConcurrentActivityExecutionSize:     yamlCfg.Worker.MaxConcurrentActivityExecutionSize,
		MaxConcurrentWorkflowTaskExecutionSize: yamlCfg.Worker.MaxConcurrentWorkflowTaskExecutionSize,
		WorkerStopTimeout:                      time.Duration(yamlCfg.Worker.StopTimeoutSeconds) * time.Second,
		DefaultTaskQueue:                       yamlCfg.DefaultTaskQueue,
	}
}

// loadTemporalYAML loads the YAML configuration file
func (l *Loader) loadTemporalYAML() (*TemporalYAMLConfig, error) {
	yamlData, err := os.ReadFile(temporalYAMLPath)
	if err != nil {
		return nil, errors.New("failed to read " + temporalYAMLPath + ": " + err.Error())
	}

	var cfg TemporalYAMLConfig
	if err := yaml.Unmarshal(yamlData, &cfg); err != nil {
		return nil, errors.New("failed to parse " + temporalYAMLPath + ": " + err.Error())
	}

	return &cfg, nil
}
