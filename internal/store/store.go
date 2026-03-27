package store

import (
	"context"

	"github.com/kubev2v/migration-planner/internal/store/model"
	"gorm.io/gorm"
)

type Store interface {
	NewTransactionContext(ctx context.Context) (context.Context, error)
	Agent() Agent
	Authz() Authz
	Source() Source
	ImageInfra() ImageInfra
	PrivateKey() PrivateKey
	Label() Label
	Assessment() Assessment
	Job() Job
	Accounts() Accounts
	Statistics(ctx context.Context) (model.InventoryStats, error)
	Close() error
}

type DataStore struct {
	agent      Agent
	authz      Authz
	db         *gorm.DB
	source     Source
	imageInfra ImageInfra
	privateKey PrivateKey
	label      Label
	assessment Assessment
	job        Job
	accounts   Accounts
}

func NewStore(db *gorm.DB) Store {
	return &DataStore{
		agent:      NewAgentSource(db),
		source:     NewSource(db),
		imageInfra: NewImageInfraStore(db),
		privateKey: NewCacheKeyStore(NewPrivateKey(db)),
		label:      NewLabelStore(db),
		assessment: NewAssessmentStore(db),
		job:        NewJobStore(db),
		authz:      NewAuthzStore(db),
		accounts:   NewAccountsStore(db),
		db:         db,
	}
}

func (s *DataStore) NewTransactionContext(ctx context.Context) (context.Context, error) {
	return newTransactionContext(ctx, s.db)
}

func (s *DataStore) Authz() Authz {
	return s.authz
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

func (s *DataStore) Job() Job {
	return s.job
}

func (s *DataStore) Accounts() Accounts {
	return s.accounts
}

func (s *DataStore) Statistics(ctx context.Context) (model.InventoryStats, error) {
	assessments, err := s.Assessment().List(ctx, NewAssessmentQueryFilter())
	if err != nil {
		return model.InventoryStats{}, err
	}
	return model.NewInventoryStats(assessments), nil
}

func (s *DataStore) Close() error {
	if s.authz != nil {
		if err := s.authz.Close(); err != nil {
			return err
		}
	}

	sqlDB, err := s.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}
