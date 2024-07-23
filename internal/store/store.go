package store

import (
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

type Store interface {
	Source() Source
	InitialMigration() error
	Close() error
}

type DataStore struct {
	source Source
	db     *gorm.DB
}

func NewStore(db *gorm.DB, log logrus.FieldLogger) Store {
	return &DataStore{
		source: NewSource(db, log),
		db:     db,
	}
}

func (s *DataStore) Source() Source {
	return s.source
}

func (s *DataStore) InitialMigration() error {
	if err := s.Source().InitialMigration(); err != nil {
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
