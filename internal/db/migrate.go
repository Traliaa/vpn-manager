package db

import (
	"fmt"
	"strings"

	"github.com/Traliaa/vpn-manager/internal/config"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

type MigrateParams struct {
	fx.In

	Config *config.Config
	Logger *zap.Logger
}

func RunMigrations(p MigrateParams) error {
	p.Logger.Info("applying database migrations",
		zap.String("dir", p.Config.Database.MigrationsDir),
	)

	// golang-migrate/pgx v5 driver uses pgx5:// scheme
	dsn := strings.Replace(p.Config.Database.DSN, "postgres://", "pgx5://", 1)

	m, err := migrate.New(
		fmt.Sprintf("file://%s", p.Config.Database.MigrationsDir),
		dsn,
	)
	if err != nil {
		return fmt.Errorf("init migration: %w", err)
	}
	defer m.Close()

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("run migration: %w", err)
	}

	p.Logger.Info("database migrations applied successfully")
	return nil
}

var MigrateModule = fx.Module("migrations",
	fx.Invoke(RunMigrations),
)
