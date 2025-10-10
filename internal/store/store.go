package store

import (
	"context"
	"errors"

	"github.com/authzed/authzed-go/v1"
	"gorm.io/gorm"

	"github.com/kubev2v/migration-planner/internal/store/model"
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
	Statistics(ctx context.Context) (model.InventoryStats, error)
	Close() error
}

type DataStore struct {
	agent      Agent
	db         *gorm.DB
	source     Source
	authz      Authz
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

func NewStoreWithAuthz(db *gorm.DB, spiceDbClient *authzed.Client) Store {
	return &DataStore{
		agent:      NewAgentSource(db),
		authz:      NewAuthzStore(NewZedTokenStore(db), spiceDbClient, db),
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
	assessments, err := s.Assessment().List(ctx, NewAssessmentQueryFilter())
	if err != nil {
		return model.InventoryStats{}, err
	}
	return model.NewInventoryStats(assessments), nil
}

func (s *DataStore) Authz() Authz {
	return s.authz
}

func (s *DataStore) Close() error {
	var returnErr error

	if s.authz != nil {
		if err := s.authz.Close(); err != nil {
			returnErr = err
		}
	}

	sqlDB, err := s.db.DB()
	if err != nil {
		return errors.Join(returnErr, err)
	}

	if err := sqlDB.Close(); err != nil {
		return errors.Join(returnErr, err)
	}

	return nil

}
