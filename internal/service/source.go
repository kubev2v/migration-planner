package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/internal/image"
	"github.com/kubev2v/migration-planner/internal/opa"
	"github.com/kubev2v/migration-planner/internal/service/mappers"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/internal/store/model"
	"github.com/kubev2v/migration-planner/internal/util"
)

type SourceService struct {
	store        store.Store
	opaValidator *opa.Validator
}

func NewSourceService(store store.Store, opaValidator *opa.Validator) *SourceService {
	return &SourceService{
		store:        store,
		opaValidator: opaValidator,
	}
}

// TODO should be moved to ImageService (to be created)
func (s *SourceService) GetSourceDownloadURL(ctx context.Context, id uuid.UUID) (string, time.Time, error) {
	source, err := s.store.Source().Get(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrRecordNotFound) {
			return "", time.Now(), NewErrSourceNotFound(id)
		}
		return "", time.Time{}, err
	}

	// FIXME: refactor the environment vars + config.yaml
	baseUrl := util.GetEnv("MIGRATION_PLANNER_IMAGE_URL", "http://localhost:11443")

	url, expireAt, err := image.GenerateDownloadURLByToken(baseUrl, source)
	if err != nil {
		return "", time.Time{}, err
	}

	return url, time.Time(*expireAt), err
}

func (s *SourceService) ListSources(ctx context.Context, filter *SourceFilter) ([]model.Source, error) {
	storeFilter := store.NewSourceQueryFilter().ByOrgID(filter.OrgID)

	userResult, err := s.store.Source().List(ctx, storeFilter)
	if err != nil {
		return nil, err
	}

	return userResult, nil
}

func (s *SourceService) GetSource(ctx context.Context, id uuid.UUID) (*model.Source, error) {
	source, err := s.store.Source().Get(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrRecordNotFound) {
			return nil, NewErrSourceNotFound(id)
		}
		return nil, err
	}

	return source, nil
}

func (s *SourceService) CreateSource(ctx context.Context, sourceForm mappers.SourceCreateForm) (model.Source, error) {
	// Generate a signing key for tokens for the source
	imageTokenKey, err := image.HMACKey(32)
	if err != nil {
		return model.Source{}, err
	}

	ctx, err = s.store.NewTransactionContext(ctx)
	if err != nil {
		return model.Source{}, err
	}

	result, err := s.store.Source().Create(ctx, sourceForm.ToSource())
	if err != nil {
		_, _ = store.Rollback(ctx)
		return model.Source{}, err
	}

	imageInfra := sourceForm.ToImageInfra(result.ID, imageTokenKey)
	if _, err := s.store.ImageInfra().Create(ctx, imageInfra); err != nil {
		_, _ = store.Rollback(ctx)
		return model.Source{}, err
	}

	if _, err := store.Commit(ctx); err != nil {
		return model.Source{}, err
	}

	return *result, nil
}

func (s *SourceService) DeleteSources(ctx context.Context) error {
	if err := s.store.Source().DeleteAll(ctx); err != nil {
		return err
	}

	return nil
}

func (s *SourceService) DeleteSource(ctx context.Context, id uuid.UUID) error {
	if err := s.store.Source().Delete(ctx, id); err != nil {
		return err
	}

	return nil
}

func (s *SourceService) UpdateSource(ctx context.Context, id uuid.UUID, form mappers.SourceUpdateForm) (*model.Source, error) {
	ctx, err := s.store.NewTransactionContext(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		_, _ = store.Rollback(ctx)
	}()

	source, err := s.store.Source().Get(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrRecordNotFound) {
			return nil, NewErrSourceNotFound(id)
		}
		return nil, err
	}

	// Update source fields
	form.ToSource(source)
	if _, err := s.store.Source().Update(ctx, *source); err != nil {
		return nil, err
	}

	// Update ImageInfra
	form.ToImageInfra(&source.ImageInfra)
	if _, err := s.store.ImageInfra().Update(ctx, source.ImageInfra); err != nil {
		return nil, err
	}

	// Update labels
	if labels := form.ToLabels(); labels != nil {
		if err := s.store.Label().UpdateLabels(ctx, source.ID, labels); err != nil {
			return nil, err
		}
	}

	if _, err := store.Commit(ctx); err != nil {
		return nil, err
	}

	// Re-fetch source to get all updated associations
	updatedSource, err := s.store.Source().Get(ctx, id)
	if err != nil {
		return nil, err
	}

	return updatedSource, nil
}

func (s *SourceService) UpdateInventory(ctx context.Context, form mappers.InventoryUpdateForm) (model.Source, error) {
	ctx, err := s.store.NewTransactionContext(ctx)
	if err != nil {
		return model.Source{}, err
	}

	source, err := s.store.Source().Get(ctx, form.SourceID)
	if err != nil {
		if errors.Is(err, store.ErrRecordNotFound) {
			return model.Source{}, NewErrSourceNotFound(form.SourceID)
		}
		return model.Source{}, err
	}

	// create the agent if missing
	var agent *model.Agent
	for _, a := range source.Agents {
		if a.ID == form.AgentId {
			agent = &a
			break
		}
	}

	if agent == nil {
		newAgent := model.NewAgentForSource(uuid.New(), *source)
		if _, err := s.store.Agent().Create(ctx, newAgent); err != nil {
			return model.Source{}, err
		}
	}

	if source.VCenterID != "" && source.VCenterID != form.Inventory.Vcenter.Id {
		_, _ = store.Rollback(ctx)
		return model.Source{}, NewErrInvalidVCenterID(form.SourceID, form.Inventory.Vcenter.Id)
	}

	source.OnPremises = true
	source.VCenterID = form.Inventory.Vcenter.Id
	source.Inventory = model.MakeJSONField(form.Inventory)

	if _, err = s.store.Source().Update(ctx, *source); err != nil {
		_, _ = store.Rollback(ctx)
		return model.Source{}, err
	}

	if _, err := store.Commit(ctx); err != nil {
		return model.Source{}, err
	}

	return *source, nil
}

type SourceFilterFunc func(s *SourceFilter)

type SourceFilter struct {
	OrgID string
	ID    uuid.UUID
}

func NewSourceFilter(filters ...SourceFilterFunc) *SourceFilter {
	s := &SourceFilter{}
	for _, f := range filters {
		f(s)
	}
	return s
}

func (s *SourceFilter) WithOption(o SourceFilterFunc) *SourceFilter {
	o(s)
	return s
}

func WithSourceID(id uuid.UUID) SourceFilterFunc {
	return func(s *SourceFilter) {
		s.ID = id
	}
}

func WithOrgID(orgID string) SourceFilterFunc {
	return func(s *SourceFilter) {
		s.OrgID = orgID
	}
}
