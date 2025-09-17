package store

import (
	"context"
	"time"

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
	Label() Label
	Assessment() Assessment
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
	label      Label
	assessment Assessment
}

func NewStore(db *gorm.DB) Store {
	return &DataStore{
		agent:      NewAgentSource(db),
		source:     NewSource(db),
		imageInfra: NewImageInfraStore(db),
		privateKey: NewCacheKeyStore(NewPrivateKey(db)),
		label:      NewLabelStore(db),
		assessment: NewAssessmentStore(db),
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

func (s *DataStore) ImageInfra() ImageInfra {
	return s.imageInfra
}

func (s *DataStore) Label() Label {
	return s.label
}

func (s *DataStore) Assessment() Assessment {
	return s.assessment
}

func (s *DataStore) Statistics(ctx context.Context) (model.InventoryStats, error) {
	sources, err := s.Source().List(ctx, NewSourceQueryFilter())
	if err != nil {
		return model.InventoryStats{}, err
	}
	return model.NewInventoryStats(sources), nil
}

func (s *DataStore) Seed() error {
	assessmentId := uuid.MustParse(exampleAssessmentID)

	tx, err := newTransaction(s.db)
	if err != nil {
		return err
	}

	assessment := model.Assessment{
		ID:         assessmentId,
		CreatedAt:  time.Now(),
		Name:       "Example",
		OrgID:      "example",
		SourceType: "inventory",
	}

	if err := tx.tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoNothing: true,
	}).Create(&assessment).Error; err != nil {
		_ = tx.Rollback()
		return err
	}

	// backwards compability: find if we already have a snapshot for this assessment
	foundId := 0
	_ = tx.tx.Select("id").Table("snapshots").Where("assessment_id = ?", assessmentId).Scan(&foundId) // ignore error

	snapshot := model.Snapshot{
		AssessmentID: assessmentId,
		Inventory:    model.MakeJSONField(GenerateDefaultInventory()),
	}

	if foundId > 0 {
		snapshot.ID = uint(foundId)
	}

	if err := tx.tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{"inventory"}),
	}).Create(&snapshot).Error; err != nil {
		_ = tx.Rollback()
		return err
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
