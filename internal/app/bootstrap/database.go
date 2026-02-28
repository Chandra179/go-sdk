package bootstrap

import (
	"fmt"

	"gosdk/pkg/db"

	"github.com/golang-migrate/migrate/v4"
)

func InitDatabase(dsn string, migrationPath string, connConfig db.ConnectionConfig) (db.DB, error) {
	dbClient, err := db.NewPostgresClient(dsn, connConfig)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}

	if err := runMigrations(dsn, migrationPath); err != nil {
		dbClient.Close()
		return nil, fmt.Errorf("migrations: %w", err)
	}

	return dbClient, nil
}

func runMigrations(dsn, migrationPath string) error {
	m, err := migrate.New("file://"+migrationPath, dsn)
	if err != nil {
		return err
	}
	defer m.Close()

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return err
	}

	return nil
}
