package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/kubev2v/migration-planner/pkg/opa"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/internal/image"
	"github.com/kubev2v/migration-planner/internal/service/mappers"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/internal/store/model"
	"github.com/kubev2v/migration-planner/internal/util"
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

// TODO should be moved to ImageService (to be created)
func (s *SourceService) GetSourceDownloadURL(ctx context.Context, id uuid.UUID) (string, time.Time, error) {
	source, err := s.store.Source().Get(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrRecordNotFound) {
			return "", time.Now(), NewErrSourceNotFound(id)
		}
		return "", time.Time{}, err
	}

	// Pre-generate and store agent JWT so all pods produce byte-identical OVAs.
	// This is required for Akamai LFO byte-range requests across load-balanced pods.
	if err := s.ensureAgentToken(ctx, source); err != nil {
		return "", time.Time{}, fmt.Errorf("failed to ensure agent token: %w", err)
	}

	// FIXME: refactor the environment vars + config.yaml
	baseUrl := util.GetEnv("MIGRATION_PLANNER_IMAGE_URL", "http://localhost:11443")

	url, expireAt, err := image.GenerateDownloadURLByToken(baseUrl, source)
	if err != nil {
		return "", time.Time{}, err
	}

	return url, time.Time(*expireAt), err
}

// ensureAgentToken generates and stores the agent JWT if it's missing or near expiration.
func (s *SourceService) ensureAgentToken(ctx context.Context, source *model.Source) error {
	if source.ImageInfra.AgentToken != nil && !isTokenNearExpiry(*source.ImageInfra.AgentToken) {
		return nil
	}

	key, err := s.store.PrivateKey().Get(ctx, source.OrgID)
	if err != nil {
		if !errors.Is(err, store.ErrRecordNotFound) {
			return err
		}
		newKey, token, err := auth.GenerateAgentJWTAndKey(source)
		if err != nil {
			return err
		}
		if _, err := s.store.PrivateKey().Create(ctx, *newKey); err != nil {
			return err
		}
		return s.store.ImageInfra().UpdateAgentToken(ctx, source.ID.String(), token)
	}

	token, err := auth.GenerateAgentJWT(key, source)
	if err != nil {
		return err
	}
	return s.store.ImageInfra().UpdateAgentToken(ctx, source.ID.String(), token)
}

// isTokenNearExpiry parses a JWT without verification and checks if it expires within 30 days.
// The agent token lifetime is 90 days, so this renews when ~1/3 of the lifetime remains.
func isTokenNearExpiry(tokenStr string) bool {
	parser := jwt.NewParser()
	token, _, err := parser.ParseUnverified(tokenStr, jwt.MapClaims{})
	if err != nil {
		return true // can't parse → treat as expired
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return true
	}
	exp, err := claims.GetExpirationTime()
	if err != nil || exp == nil {
		return true
	}
	return time.Until(exp.Time) < 30*24*time.Hour
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
