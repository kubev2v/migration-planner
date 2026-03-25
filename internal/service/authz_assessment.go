package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/internal/service/mappers"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/internal/store/model"
)

type AuthzAssessmentService struct {
	inner AssessmentServicer
	store store.Store
}

func NewAuthzAssessmentService(inner AssessmentServicer, s store.Store) AssessmentServicer {
	return &AuthzAssessmentService{inner: inner, store: s}
}

func (a *AuthzAssessmentService) ListAssessments(ctx context.Context, filter *AssessmentFilter) ([]model.Assessment, error) {
	user := auth.MustHaveUser(ctx)

	resources, err := a.store.Authz().ListResources(ctx, user.Username, model.AssessmentResource)
	if err != nil {
		return nil, fmt.Errorf("authz: failed to list resources: %w", err)
	}

	if len(resources) == 0 {
		return []model.Assessment{}, nil
	}

	ids := make([]uuid.UUID, 0, len(resources))
	for _, r := range resources {
		id, err := uuid.Parse(r.ID)
		if err != nil {
			continue
		}
		ids = append(ids, id)
	}

	filter.IDs = ids
	filter.Username = ""
	filter.OrgID = ""

	return a.inner.ListAssessments(ctx, filter)
}

func (a *AuthzAssessmentService) GetAssessment(ctx context.Context, id uuid.UUID) (*model.Assessment, error) {
	user := auth.MustHaveUser(ctx)

	// get assessment first to capture the 404 if any
	assessment, err := a.inner.GetAssessment(ctx, id)
	if err != nil {
		return nil, err
	}

	resource, err := a.store.Authz().GetPermissions(ctx, user.Username, model.NewAssessmentResource(id.String()))
	if err != nil {
		return nil, fmt.Errorf("authz: failed to get permissions: %w", err)
	}

	if !model.ReadPermission.In(resource.Permissions) {
		return nil, NewErrForbidden("assessment", id.String())
	}

	return assessment, nil
}

func (a *AuthzAssessmentService) CreateAssessment(ctx context.Context, createForm mappers.AssessmentCreateForm) (*model.Assessment, error) {
	user := auth.MustHaveUser(ctx)

	ctx, err := a.store.NewTransactionContext(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		_, _ = store.Rollback(ctx)
	}()

	assessment, err := a.inner.CreateAssessment(ctx, createForm)
	if err != nil {
		return nil, err
	}

	updates := store.NewRelationshipBuilder().
		With(model.NewAssessmentResource(assessment.ID.String()), model.OwnerRelation, model.NewUserSubject(user.Username)).
		Build()

	if err := a.store.Authz().WriteRelationships(ctx, updates); err != nil {
		return nil, fmt.Errorf("authz: failed to write owner relation: %w", err)
	}

	if _, err := store.Commit(ctx); err != nil {
		return nil, err
	}

	return assessment, nil
}

func (a *AuthzAssessmentService) UpdateAssessment(ctx context.Context, id uuid.UUID, name *string) (*model.Assessment, error) {
	user := auth.MustHaveUser(ctx)

	// get assessment first to capture the 404 if any
	_, err := a.inner.GetAssessment(ctx, id)
	if err != nil {
		return nil, err
	}

	resource, err := a.store.Authz().GetPermissions(ctx, user.Username, model.NewAssessmentResource(id.String()))
	if err != nil {
		return nil, fmt.Errorf("authz: failed to get permissions: %w", err)
	}

	if !model.EditPermission.In(resource.Permissions) {
		return nil, NewErrForbidden("assessment", id.String())
	}

	return a.inner.UpdateAssessment(ctx, id, name)
}

func (a *AuthzAssessmentService) DeleteAssessment(ctx context.Context, id uuid.UUID) error {
	user := auth.MustHaveUser(ctx)

	// get assessment first to capture the 404 if any
	_, err := a.inner.GetAssessment(ctx, id)
	if err != nil {
		return err
	}

	resource, err := a.store.Authz().GetPermissions(ctx, user.Username, model.NewAssessmentResource(id.String()))
	if err != nil {
		return fmt.Errorf("authz: failed to get permissions: %w", err)
	}

	if !model.DeletePermission.In(resource.Permissions) {
		return NewErrForbidden("assessment", id.String())
	}

	ctx, err = a.store.NewTransactionContext(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_, _ = store.Rollback(ctx)
	}()

	if err := a.inner.DeleteAssessment(ctx, id); err != nil {
		return err
	}

	if err := a.store.Authz().DeleteRelationships(ctx, model.NewAssessmentResource(id.String())); err != nil {
		return fmt.Errorf("authz: failed to delete relations: %w", err)
	}

	if _, err := store.Commit(ctx); err != nil {
		return err
	}

	return nil
}
