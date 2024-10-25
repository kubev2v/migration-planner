package store

import (
	"context"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

type Store interface {
	NewTransactionContext(ctx context.Context) (context.Context, error)
	Source() Source
	InitialMigration() error
	Close() error
}

type DataStore struct {
	db     *gorm.DB
	source Source
	log    logrus.FieldLogger
}

func NewStore(db *gorm.DB, log logrus.FieldLogger) Store {
	return &DataStore{
		db:     db,
		log:    log,
		source: NewSource(db, log),
	}
}

func (s *DataStore) NewTransactionContext(ctx context.Context) (context.Context, error) {
	return newTransactionContext(ctx, s.db, s.log)
}

func (s *DataStore) Source() Source {
	return s.source
}

func (s *DataStore) InitialMigration() error {
	ctx, err := s.NewTransactionContext(context.Background())
	if err != nil {
		return err
	}

	if err := s.Source().InitialMigration(ctx); err != nil {
		_, _ = Rollback(ctx)
		return err
	}

	_, err = Commit(ctx)
	return err
}

func (s *DataStore) Close() error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}
