package bootstrap

import (
	"fmt"

	"gosdk/pkg/db"
)

func InitDatabase(dsn string, connConfig db.ConnectionConfig) (db.DB, error) {
	dbClient, err := db.NewPostgresClient(dsn, connConfig)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}

	if err := RunMigrations(dsn); err != nil {
		dbClient.Close()
		return nil, fmt.Errorf("migrations: %w", err)
	}

	return dbClient, nil
}
