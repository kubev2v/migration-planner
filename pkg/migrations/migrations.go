package migrations

import (
	"context"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pressly/goose/v3"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/kubev2v/migration-planner/internal/config"
)

func MigrateStore(db *gorm.DB, migrationFolder string) error {
	goose.SetLogger(&logger{})

	fi, err := os.Stat(migrationFolder)
	if err != nil {
		return err
	}

	if !fi.Mode().IsDir() {
		return fmt.Errorf("failed to open migration folder: %s is not a folder", migrationFolder)
	}

	goose.SetBaseFS(os.DirFS(migrationFolder))

	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}

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

// MigrateRiver runs River's database migrations for job queue tables.
func MigrateRiver(ctx context.Context, cfg *config.Config) error {
	dsn := fmt.Sprintf("postgres://%s:%s@%s:%s/%s",
		cfg.Database.User,
		cfg.Database.Password,
		cfg.Database.Hostname,
		cfg.Database.Port,
		cfg.Database.Name,
	)

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return fmt.Errorf("creating pgx pool for River migration: %w", err)
	}
	defer pool.Close()

	migrator, err := rivermigrate.New(riverpgxv5.New(pool), nil)
	if err != nil {
		return fmt.Errorf("creating River migrator: %w", err)
	}

	_, err = migrator.Migrate(ctx, rivermigrate.DirectionUp, nil)
	if err != nil {
		return fmt.Errorf("running River migration: %w", err)
	}
	zap.S().Infof("River migration completed")
	return nil
}
