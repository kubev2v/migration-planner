package store

import (
	"context"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

type Store interface {
	NewTransactionContext(ctx context.Context) (context.Context, error)
	Agent() Agent
	Source() Source
	InitialMigration() error
	Close() error
}

type DataStore struct {
	agent  Agent
	db     *gorm.DB
	source Source
	log    logrus.FieldLogger
}

func NewStore(db *gorm.DB, log logrus.FieldLogger) Store {
	return &DataStore{
		agent:  NewAgentSource(db, log),
		source: NewSource(db, log),
		db:     db,
		log:    log,
	}
}

func (s *DataStore) NewTransactionContext(ctx context.Context) (context.Context, error) {
	return newTransactionContext(ctx, s.db, s.log)
}

func (s *DataStore) Source() Source {
	return s.source
}

func (s *DataStore) Agent() Agent {
	return s.agent
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

	if err := s.Agent().InitialMigration(ctx); err != nil {
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
