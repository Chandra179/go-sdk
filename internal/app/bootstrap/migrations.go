// internal/app/bootstrap/migrations.go
package bootstrap

import (
	"errors"
	"fmt"

	appdb "gosdk/internal/db"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

func RunMigrations(dsn string) error {
	src, err := iofs.New(appdb.MigrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("loading embedded migrations: %w", err)
	}

	m, err := migrate.NewWithSourceInstance("iofs", src, dsn)
	if err != nil {
		return fmt.Errorf("creating migrator: %w", err)
	}
	defer m.Close()

	if err := m.Up(); err != nil {
		if errors.Is(err, migrate.ErrNoChange) {
			return nil
		}

		version, dirty, vErr := m.Version()
		if vErr == nil && dirty {
			return fmt.Errorf("migration %d is dirty (failed mid-run): %w", version, err)
		}

		return fmt.Errorf("running migrations: %w", err)
	}

	return nil
}
