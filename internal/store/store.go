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
	PrivateKey() PrivateKey
	ShareToken() ShareToken
	Seed() error
	Statistics(ctx context.Context) (model.InventoryStats, error)
	Close() error
}

type DataStore struct {
	agent      Agent
	db         *gorm.DB
	source     Source
	imageInfra ImageInfra
	privateKey PrivateKey
	shareToken ShareToken
}

func NewStore(db *gorm.DB) Store {
	return &DataStore{
		agent:      NewAgentSource(db),
		source:     NewSource(db),
		imageInfra: NewImageInfraStore(db),
		privateKey: NewCacheKeyStore(NewPrivateKey(db)),
		shareToken: NewShareTokenStore(db),
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

func (s *DataStore) PrivateKey() PrivateKey {
	return s.privateKey
}

func (s *DataStore) ShareToken() ShareToken {
	return s.shareToken
}

func (s *DataStore) ImageInfra() ImageInfra {
	return s.imageInfra
}

func (s *DataStore) Statistics(ctx context.Context) (model.InventoryStats, error) {
	sources, err := s.Source().List(ctx, NewSourceQueryFilter().WithoutDefaultInventory())
	if err != nil {
		return model.InventoryStats{}, err
	}
	return model.NewInventoryStats(sources), nil
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
		Columns:   []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{"inventory"}),
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
