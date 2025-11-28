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
)

func MigrateStore(db *gorm.DB, migrationFolder string, pgxPool *pgxpool.Pool) error {
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

	if err := migrateRiver(pgxPool); err != nil {
		return fmt.Errorf("river migrations: %w", err)
	}

	return nil
}

func migrateRiver(pool *pgxpool.Pool) error {
	migrator, err := rivermigrate.New(riverpgxv5.New(pool), nil)
	if err != nil {
		return err
	}
	_, err = migrator.Migrate(context.Background(), rivermigrate.DirectionUp, nil)
	return err
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
