package store

import (
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

type Store interface {
	Agent() Agent
	Source() Source
	InitialMigration() error
	Close() error
}

type DataStore struct {
	agent  Agent
	source Source
	db     *gorm.DB
}

func NewStore(db *gorm.DB, log logrus.FieldLogger) Store {
	return &DataStore{
		agent:  NewAgentSource(db, log),
		source: NewSource(db, log),
		db:     db,
	}
}

func (s *DataStore) Source() Source {
	return s.source
}

func (s *DataStore) Agent() Agent {
	return s.agent
}

func (s *DataStore) InitialMigration() error {
	if err := s.Source().InitialMigration(); err != nil {
		return err
	}
	if err := s.Agent().InitialMigration(); err != nil {
		return err
	}
	return nil
}

func (s *DataStore) Close() error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}
