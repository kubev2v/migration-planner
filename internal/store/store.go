package store

import (
	"context"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/internal/store/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Store interface {
	NewTransactionContext(ctx context.Context) (context.Context, error)
	Agent() Agent
	Source() Source
	ImageInfra() ImageInfra
	Seed() error
	InitialMigration() error
	Close() error
}

type DataStore struct {
	agent      Agent
	db         *gorm.DB
	source     Source
	imageInfra ImageInfra
}

func NewStore(db *gorm.DB) Store {
	return &DataStore{
		agent:      NewAgentSource(db),
		source:     NewSource(db),
		imageInfra: NewImageInfraStore(db),
		db:         db,
	}
}

func (s *DataStore) NewTransactionContext(ctx context.Context) (context.Context, error) {
	return newTransactionContext(ctx, s.db)
}

func (s *DataStore) Source() Source {
	return s.source
}

func (s *DataStore) Agent() Agent {
	return s.agent
}

func (s *DataStore) ImageInfra() ImageInfra {
	return s.imageInfra
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

	if err := s.ImageInfra().InitialMigration(ctx); err != nil {
		return err
	}

	_, err = Commit(ctx)
	return err
}

func (s *DataStore) Seed() error {
	sourceUuid := uuid.UUID{}

	tx, err := newTransaction(s.db)
	if err != nil {
		return err
	}
	// Create/update default source
	source := model.Source{
		ID:        sourceUuid,
		Name:      "Example",
		Inventory: model.MakeJSONField(GenerateDefaultInventory()),
	}

	if err := tx.tx.Clauses(clause.OnConflict{
		UpdateAll: true,
	}).Create(&source).Error; err != nil {
		_ = tx.Rollback()
	}

	if err := tx.Commit(); err != nil {
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
