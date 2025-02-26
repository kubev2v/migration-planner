package store

import (
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type Config struct {
	Type     string `envconfig:"DB_TYPE" default:"pgsql"`
	Hostname string `envconfig:"DB_HOST" default:"localhost"`
	Port     string `envconfig:"DB_PORT" default:"5432"`
	Name     string `envconfig:"DB_NAME" default:"planner"`
	User     string `envconfig:"DB_USER" default:"admin"`
	Password string `envconfig:"DB_PASS" default:"adminpass"`
}

func InitDB(cfg *Config) (*gorm.DB, error) {
	var dia gorm.Dialector

	if cfg.Type == "pgsql" {
		dsn := fmt.Sprintf("host=%s user=%s password=%s port=%s",
			cfg.Hostname,
			cfg.User,
			cfg.Password,
			cfg.Port,
		)
		if cfg.Name != "" {
			dsn = fmt.Sprintf("%s dbname=%s", dsn, cfg.Name)
		}
		dia = postgres.Open(dsn)
	} else {
		dia = sqlite.Open(cfg.Name)
	}

	newLogger := logger.New(
		logrus.New(),
		logger.Config{
			SlowThreshold:             time.Second, // Slow SQL threshold
			LogLevel:                  logger.Warn, // Log level
			IgnoreRecordNotFoundError: true,        // Ignore ErrRecordNotFound error for logger
			ParameterizedQueries:      true,        // Don't include params in the SQL log
			Colorful:                  false,       // Disable color
		},
	)

	newDB, err := gorm.Open(dia, &gorm.Config{Logger: newLogger, TranslateError: true})
	if err != nil {
		zap.S().Named("gorm").Fatalf("failed to connect database: %v", err)
		return nil, err
	}

	sqlDB, err := newDB.DB()
	if err != nil {
		zap.S().Named("gorm").Fatalf("failed to configure connections: %v", err)
		return nil, err
	}
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)

	if cfg.Type == "pgsql" {
		var minorVersion string
		if result := newDB.Raw("SELECT version()").Scan(&minorVersion); result.Error != nil {
			zap.S().Named("gorm").Infoln(result.Error.Error())
			return nil, result.Error
		}

		zap.S().Named("gorm").Infof("PostgreSQL information: '%s'", minorVersion)
	}

	return newDB, nil
}
