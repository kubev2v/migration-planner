package migrations

import (
	"context"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kubev2v/migration-planner/internal/config"
	"github.com/pressly/goose/v3"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func MigrateStore(db *gorm.DB, cfg *config.Config) error {
	goose.SetLogger(&logger{})

	fi, err := os.Stat(cfg.Service.MigrationFolder)
	if err != nil {
		return err
	}

	if !fi.Mode().IsDir() {
		return fmt.Errorf("failed to open migration folder: %s is not a folder", cfg.Service.MigrationFolder)
	}

	goose.SetBaseFS(os.DirFS(cfg.Service.MigrationFolder))

	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}

	// Run River migrations to create necessary tables
	zap.S().Info("Running River migrations...")

	// Create a connection pool specifically for migrations
	dsn := fmt.Sprintf("host=%s user=%s password=%s port=%s dbname=%s",
		cfg.Database.Hostname,
		cfg.Database.User,
		cfg.Database.Password,
		cfg.Database.Port,
		cfg.Database.Name,
	)

	ctx := context.Background()
	dbPool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return fmt.Errorf("failed to create pgx pool for river migrations: %w", err)
	}
	defer dbPool.Close()

	migrator, err := rivermigrate.New(riverpgxv5.New(dbPool), nil)
	if err != nil {
		return fmt.Errorf("failed to create river migrator: %w", err)
	}

	_, err = migrator.Migrate(ctx, rivermigrate.DirectionUp, &rivermigrate.MigrateOpts{})
	if err != nil {
		return fmt.Errorf("failed to run river migrations: %w", err)
	}
	zap.S().Info("River migrations completed")

	sqlDB, err := db.DB()
	if err != nil {
		return err
	}

	if err := goose.Up(sqlDB, "."); err != nil {
		return err
	}

	return nil
}

/*
logger implements goose.Logger interface

	type Logger interface {
		Fatalf(format string, v ...interface{})
		Printf(format string, v ...interface{})
	}
*/
type logger struct{}

func (m *logger) Printf(format string, v ...interface{}) { zap.S().Infof(format, v...) }
func (m *logger) Fatalf(format string, v ...interface{}) { zap.S().Fatalf(format, v...) }
