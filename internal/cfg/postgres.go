package cfg

import "time"

type PostgresConfig struct {
	Host          string
	Port          string
	User          string
	Password      string
	DBName        string
	SSLMode       string
	MigrationPath string

	// Connection pool settings
	MaxOpenConns    int           // Maximum number of open connections (default: 25)
	MaxIdleConns    int           // Maximum number of idle connections (default: 10)
	ConnMaxLifetime time.Duration // Maximum lifetime of a connection (default: 5m)
	ConnMaxIdleTime time.Duration // Maximum idle time before closing (default: 10m)

	// Query timeout (default: 5s)
	QueryTimeout time.Duration
}

func (l *Loader) loadPostgres() PostgresConfig {
	return PostgresConfig{
		Host:            l.requireEnv("POSTGRES_HOST"),
		Port:            l.requireEnv("POSTGRES_PORT"),
		User:            l.requireEnv("POSTGRES_USER"),
		Password:        l.requireEnv("POSTGRES_PASSWORD"),
		DBName:          l.requireEnv("POSTGRES_DB"),
		SSLMode:         l.requireEnv("POSTGRES_SSLMODE"),
		MigrationPath:   l.requireEnv("MIGRATION_PATH"),
		MaxOpenConns:    l.getEnvIntOrDefault("POSTGRES_MAX_OPEN_CONNS", 25),
		MaxIdleConns:    l.getEnvIntOrDefault("POSTGRES_MAX_IDLE_CONNS", 10),
		ConnMaxLifetime: l.getEnvDurationOrDefault("POSTGRES_CONN_MAX_LIFETIME", 5*time.Minute),
		ConnMaxIdleTime: l.getEnvDurationOrDefault("POSTGRES_CONN_MAX_IDLE_TIME", 10*time.Minute),
		QueryTimeout:    l.getEnvDurationOrDefault("POSTGRES_QUERY_TIMEOUT", 5*time.Second),
	}
}
