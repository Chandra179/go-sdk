package cfg

import (
	"errors"
	"os"
	"time"

	"github.com/goccy/go-yaml"
)

// Postgres YAML config file path
const postgresYAMLPath = "internal/cfg/postgres.yaml"

// Environment variable names for Postgres secrets
const (
	envPostgresPassword = "POSTGRES_PASSWORD"
)

// PostgresConfig holds database configuration
type PostgresConfig struct {
	Host          string
	Port          string
	User          string
	Password      string
	DBName        string
	SSLMode       string
	MigrationPath string

	// Connection pool settings
	MaxOpenConns    int           // Maximum number of open connections
	MaxIdleConns    int           // Maximum number of idle connections
	ConnMaxLifetime time.Duration // Maximum lifetime of a connection
	ConnMaxIdleTime time.Duration // Maximum idle time before closing

	// Query timeout
	QueryTimeout time.Duration
}

// PostgresYAMLConfig represents the YAML configuration structure
type PostgresYAMLConfig struct {
	Host                   string `yaml:"host"`
	Port                   string `yaml:"port"`
	User                   string `yaml:"user"`
	Database               string `yaml:"database"`
	SSLMode                string `yaml:"ssl_mode"`
	MigrationPath          string `yaml:"migration_path"`
	MaxOpenConns           int    `yaml:"max_open_conns"`
	MaxIdleConns           int    `yaml:"max_idle_conns"`
	ConnMaxLifetimeSeconds int    `yaml:"conn_max_lifetime_seconds"`
	ConnMaxIdleTimeSeconds int    `yaml:"conn_max_idle_time_seconds"`
	QueryTimeoutSeconds    int    `yaml:"query_timeout_seconds"`
}

func (l *Loader) loadPostgres() PostgresConfig {
	// Load YAML config
	yamlCfg, err := l.loadPostgresYAML()
	if err != nil {
		l.errs = append(l.errs, errors.New("failed to load postgres yaml config: "+err.Error()))
		return PostgresConfig{}
	}

	// Password always comes from env (secret)
	password := l.requireEnv(envPostgresPassword)

	return PostgresConfig{
		Host:            yamlCfg.Host,
		Port:            yamlCfg.Port,
		User:            yamlCfg.User,
		Password:        password,
		DBName:          yamlCfg.Database,
		SSLMode:         yamlCfg.SSLMode,
		MigrationPath:   yamlCfg.MigrationPath,
		MaxOpenConns:    yamlCfg.MaxOpenConns,
		MaxIdleConns:    yamlCfg.MaxIdleConns,
		ConnMaxLifetime: time.Duration(yamlCfg.ConnMaxLifetimeSeconds) * time.Second,
		ConnMaxIdleTime: time.Duration(yamlCfg.ConnMaxIdleTimeSeconds) * time.Second,
		QueryTimeout:    time.Duration(yamlCfg.QueryTimeoutSeconds) * time.Second,
	}
}

// loadPostgresYAML loads Postgres configuration from internal/cfg/postgres.yaml
func (l *Loader) loadPostgresYAML() (*PostgresYAMLConfig, error) {
	yamlData, err := os.ReadFile(postgresYAMLPath)
	if err != nil {
		return nil, errors.New("failed to read " + postgresYAMLPath + ": " + err.Error())
	}

	var cfg PostgresYAMLConfig
	if err := yaml.Unmarshal(yamlData, &cfg); err != nil {
		return nil, errors.New("failed to parse " + postgresYAMLPath + ": " + err.Error())
	}

	return &cfg, nil
}
