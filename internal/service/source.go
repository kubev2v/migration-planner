package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/internal/service/mappers"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/internal/store/model"
	"github.com/kubev2v/migration-planner/pkg/opa"
	"github.com/kubev2v/migration-planner/pkg/version"
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

func (s *SourceService) ListSources(ctx context.Context, filter *SourceFilter) ([]model.Source, error) {
	storeFilter := store.NewSourceQueryFilter().ByUsername(filter.Username).ByOrgID(filter.OrgID)

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
	imageTokenKey, err := HMACKey(32)
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

		if errors.Is(err, store.ErrDuplicateKey) {
			return model.Source{}, NewErrSourceDuplicateName(sourceForm.Name)
		}

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

	result.ImageInfra = imageInfra
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
		if a.ID == form.AgentID {
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

	if source.VCenterID != "" && source.VCenterID != form.VCenterID {
		_, _ = store.Rollback(ctx)
		return model.Source{}, NewErrInvalidVCenterID(form.SourceID, form.VCenterID)
	}

	source.OnPremises = true
	source.VCenterID = form.VCenterID
	source.Inventory = form.Inventory

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
	Username string
	OrgID    string
	ID       uuid.UUID
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

func WithUsername(username string) SourceFilterFunc {
	return func(s *SourceFilter) {
		s.Username = username
	}
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

// CheckAgentVersionWarning compares stored agent version with current and returns warning if they differ.
func CheckAgentVersionWarning(imageInfra *model.ImageInfra) *string {
	versionInfo := version.Get()
	if !version.IsValidAgentVersion(versionInfo.AgentVersionName) {
		return nil
	}
	currentVersion := versionInfo.AgentVersionName

	// Handle missing version (edge case if migration hasn't run)
	if imageInfra == nil || imageInfra.AgentVersion == nil || *imageInfra.AgentVersion == "" {
		message := "No version information available for this OVA. Current system version: " + currentVersion +
			". Consider downloading a new OVA to ensure compatibility."
		return &message
	}

	storedVersion := *imageInfra.AgentVersion
	if !version.IsValidAgentVersion(storedVersion) {
		return nil
	}

	if storedVersion == currentVersion {
		return nil
	}

	message := "Agent version mismatch detected. The OVA was downloaded with version " + storedVersion +
		", but the current system version is " + currentVersion +
		". Consider downloading a new OVA to ensure compatibility."
	return &message
}

// HMACKey generates a hex string representing n random bytes
//
// This string is intended to be used as a private key for signing and
// verifying jwt tokens. Specifically ones used for downloading images
// when using rhsso auth and the image service.
func HMACKey(n int) (string, error) {
	buf := make([]byte, n)
	_, err := rand.Read(buf)
	if err != nil {
		return "", err
	}

	return hex.EncodeToString(buf), nil
}
