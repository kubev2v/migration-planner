package migrations

import (
	"fmt"
	"os"

	"github.com/pressly/goose/v3"
	"go.uber.org/zap"
	"gorm.io/gorm"
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
