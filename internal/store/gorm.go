package store

import (
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/stdlib"
	"github.com/ngrok/sqlmw"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"

	"github.com/kubev2v/migration-planner/internal/config"
	"github.com/kubev2v/migration-planner/internal/store/model"
	"github.com/kubev2v/migration-planner/pkg/log"
)

var (
	registerSync sync.Once
)

func InitDB(cfg *config.Config) (*gorm.DB, error) {
	var dia gorm.Dialector

	if cfg.Database.Type == "pgsql" {
		dsn := fmt.Sprintf("host=%s user=%s password=%s port=%s",
			cfg.Database.Hostname,
			cfg.Database.User,
			cfg.Database.Password,
			cfg.Database.Port,
		)
		if cfg.Database.Name != "" {
			dsn = fmt.Sprintf("%s dbname=%s", dsn, cfg.Database.Name)
		}

		registerSync.Do(func() {
			sql.Register("postgres", sqlmw.Driver(stdlib.GetDefaultDriver(), new(metricInterceptor)))
		})
		dia = postgres.New(postgres.Config{
			DriverName: "postgres",
			DSN:        dsn,
		})
	} else {
		dia = sqlite.Open(cfg.Database.Name)
	}

	newLogger := log.NewGormLogger("database", logger.Config{
		SlowThreshold:             time.Second, // Slow SQL threshold
		LogLevel:                  logger.Info, // Log level
		IgnoreRecordNotFoundError: true,        // Ignore ErrRecordNotFound error for logger
		ParameterizedQueries:      true,        // Don't include params in the SQL log
		Colorful:                  false,       // Disable color
	})

	newDB, err := gorm.Open(dia, &gorm.Config{Logger: newLogger, TranslateError: true})
	if err != nil {
		zap.S().Named("gorm").Fatalf("failed to connect database: %v", err)
		return nil, err
	}
	schema.RegisterSerializer("key_serializer", model.KeySerializer{})

	sqlDB, err := newDB.DB()
	if err != nil {
		zap.S().Named("gorm").Fatalf("failed to configure connections: %v", err)
		return nil, err
	}
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)

	if cfg.Database.Type == "pgsql" {
		var minorVersion string
		if result := newDB.Raw("SELECT version()").Scan(&minorVersion); result.Error != nil {
			zap.S().Named("gorm").Infoln(result.Error.Error())
			return nil, result.Error
		}

		zap.S().Named("gorm").Infof("PostgreSQL information: '%s'", minorVersion)
	}

	return newDB, nil
}
