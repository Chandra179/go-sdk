package bootstrap

import (
	"fmt"

	"gosdk/pkg/db"
)

func InitDatabase(dsn string, migrationPath string, connConfig db.ConnectionConfig) (db.DB, error) {
	dbClient, err := db.NewSQLClient("postgres", dsn, connConfig)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}

	if err := RunMigrations(dsn, migrationPath); err != nil {
		dbClient.Close()
		return nil, fmt.Errorf("migrations: %w", err)
	}

	return dbClient, nil
}
