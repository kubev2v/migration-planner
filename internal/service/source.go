package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/internal/image"
	"github.com/kubev2v/migration-planner/internal/opa"
	"github.com/kubev2v/migration-planner/internal/rvtools"
	"github.com/kubev2v/migration-planner/internal/service/mappers"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/internal/store/model"
	"github.com/kubev2v/migration-planner/internal/util"
	"go.uber.org/zap"
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

	if filter.IncludeDefault {
		// Get default content
		defaultResult, err := s.store.Source().List(ctx, store.NewSourceQueryFilter().ByDefaultInventory())
		if err != nil {
			return nil, err
		}
		return append(userResult, defaultResult...), nil
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

func (s *SourceService) UploadRvtoolsFile(ctx context.Context, sourceID uuid.UUID, reader io.Reader) error {
	source, err := s.store.Source().Get(ctx, sourceID)
	if err != nil {
		if errors.Is(err, store.ErrRecordNotFound) {
			return NewErrSourceNotFound(sourceID)
		}
		return err
	}

	rvtoolsContent, err := io.ReadAll(reader)
	if err != nil {
		return fmt.Errorf("failed to read uploaded file content: %w", err)
	}

	if rvtoolsContent == nil {
		return errors.New("no file was found in the request")
	}

	if len(rvtoolsContent) == 0 {
		return errors.New("empty file uploaded")
	}

	zap.S().Infow("received RVTools data", "size [bytes]", len(rvtoolsContent))

	//TODO: support csv files
	if !rvtools.IsExcelFile(rvtoolsContent) {
		return NewErrExcelFileNotValid()
	}

	inventory, err := rvtools.ParseRVTools(ctx, rvtoolsContent, s.opaValidator)
	if err != nil {
		return fmt.Errorf("Error parsing RVTools file: %v", err)
	}

	if source.VCenterID != "" && source.VCenterID != inventory.Vcenter.Id {
		return NewErrInvalidVCenterID(sourceID, inventory.Vcenter.Id)
	}

	ctx, err = s.store.NewTransactionContext(ctx)
	if err != nil {
		return err
	}

	var rvtoolsAgent *model.Agent
	if len(source.Agents) > 0 {
		rvtoolsAgent = &source.Agents[0]
	} else {
		newAgent := model.NewAgentForSource(uuid.New(), *source)

		if _, err := s.store.Agent().Create(ctx, newAgent); err != nil {
			_, _ = store.Rollback(ctx)
			return err
		}
		rvtoolsAgent = &newAgent
	}

	source.OnPremises = true
	source.VCenterID = inventory.Vcenter.Id
	source.Inventory = model.MakeJSONField(*inventory)

	if _, err = s.store.Source().Update(ctx, *source); err != nil {
		_, _ = store.Rollback(ctx)
		return err
	}

	rvtoolsAgent.StatusInfo = "Last updated via RVTools upload on " + time.Now().Format(time.RFC3339)

	if _, err = s.store.Agent().Update(ctx, *rvtoolsAgent); err != nil {
		_, _ = store.Rollback(ctx)
		return err
	}

	if _, err := store.Commit(ctx); err != nil {
		return err
	}

	return nil
}

type SourceFilterFunc func(s *SourceFilter)

type SourceFilter struct {
	OrgID          string
	ID             uuid.UUID
	IncludeDefault bool
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

func WithDefaultInventory() SourceFilterFunc {
	return func(s *SourceFilter) {
		s.IncludeDefault = true
	}
}
