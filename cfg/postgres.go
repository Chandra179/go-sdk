package cfg

type PostgresConfig struct {
	Host          string
	Port          string
	User          string
	Password      string
	DBName        string
	SSLMode       string
	MigrationPath string
}

func (l *Loader) loadPostgres() PostgresConfig {
	return PostgresConfig{
		Host:          l.requireEnv("POSTGRES_HOST"),
		Port:          l.requireEnv("POSTGRES_PORT"),
		User:          l.requireEnv("POSTGRES_USER"),
		Password:      l.requireEnv("POSTGRES_PASSWORD"),
		DBName:        l.requireEnv("POSTGRES_DB"),
		SSLMode:       l.requireEnv("POSTGRES_SSLMODE"),
		MigrationPath: l.requireEnv("MIGRATION_PATH"),
	}
}
