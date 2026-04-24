package service

import (
	"context"
	"fmt"
	"slices"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/internal/service/mappers"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/internal/store/model"
)

type AuthzAssessmentService struct {
	inner       AssessmentServicer
	store       store.Store
	accountsSrv *AccountsService
}

func NewAuthzAssessmentService(inner AssessmentServicer, s store.Store, accountsSrv *AccountsService) AssessmentServicer {
	return &AuthzAssessmentService{inner: inner, store: s, accountsSrv: accountsSrv}
}

func (a *AuthzAssessmentService) ListAssessments(ctx context.Context, filter *AssessmentFilter) ([]model.Assessment, error) {
	user := auth.MustHaveUser(ctx)

	identity, err := a.accountsSrv.GetIdentity(ctx, user)
	if err != nil {
		return []model.Assessment{}, err
	}

	resources, err := a.store.Authz().ListResources(ctx, user.Username, model.AssessmentResource)
	if err != nil {
		return nil, fmt.Errorf("authz: failed to list resources: %w", err)
	}

	if len(resources) == 0 {
		return []model.Assessment{}, nil
	}

	permsByID := make(map[string][]model.Permission, len(resources))
	ids := make([]uuid.UUID, 0, len(resources))
	for _, r := range resources {
		id, err := uuid.Parse(r.ID)
		if err != nil {
			continue
		}
		ids = append(ids, id)
		permsByID[r.ID] = r.Permissions
	}

	filter.IDs = ids
	filter.Username = ""
	filter.OrgID = ""

	assessments, err := a.inner.ListAssessments(ctx, filter)
	if err != nil {
		return nil, err
	}

	var ownedIDs []string
	for i, assessment := range assessments {
		assessments[i].Permissions = permsByID[assessment.ID.String()]
		if assessment.Username == user.Username {
			ownedIDs = append(ownedIDs, assessment.ID.String())
		}
	}

	var relsByID map[string][]model.Relationship
	if len(ownedIDs) > 0 {
		relsByID, err = a.store.Authz().ListBulkRelationship(ctx, ownedIDs)
		if err != nil {
			return nil, fmt.Errorf("authz: failed to list relationships: %w", err)
		}
	}

	for i, assessment := range assessments {
		if assessment.Username == user.Username {
			assessments[i].Sharing = buildOwnerSharing(relsByID[assessment.ID.String()])
			assessments[i].Permissions = slices.DeleteFunc(assessments[i].Permissions, func(s model.Permission) bool {
				if identity.Kind == KindRegular || identity.Kind == KindPartner {
					return s.String() == "share" || s.String() == "edit"
				}
				return s.String() == "edit"
			})
		} else {
			assessments[i].Sharing = buildViewerSharing(assessment.Username)
		}
	}

	// per design, the regular user cannot share so remove the permission

	return assessments, nil
}

func (a *AuthzAssessmentService) GetAssessment(ctx context.Context, id uuid.UUID) (*model.Assessment, error) {
	user := auth.MustHaveUser(ctx)

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

	assessment.Permissions = resource.Permissions

	if assessment.Username == user.Username {
		rels, err := a.store.Authz().ListRelationships(ctx, model.NewAssessmentResource(id.String()))
		if err != nil {
			return nil, fmt.Errorf("authz: failed to list relationships: %w", err)
		}
		assessment.Sharing = buildOwnerSharing(rels)

		identity, err := a.accountsSrv.GetIdentity(ctx, user)
		if err != nil {
			return nil, err
		}
		assessment.Permissions = slices.DeleteFunc(assessment.Permissions, func(s model.Permission) bool {
			if identity.Kind == KindRegular || identity.Kind == KindPartner {
				return s.String() == "share" || s.String() == "edit"
			}
			return s.String() == "edit"
		})
	} else {
		assessment.Sharing = buildViewerSharing(assessment.Username)
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

func (a *AuthzAssessmentService) ShareAssessment(ctx context.Context, id uuid.UUID) error {
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

	if !model.SharePermission.In(resource.Permissions) {
		return NewErrForbidden("assessment", id.String())
	}

	return a.inner.ShareAssessment(ctx, id)
}

func (a *AuthzAssessmentService) UnshareAssessment(ctx context.Context, id uuid.UUID) error {
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

	if !model.SharePermission.In(resource.Permissions) {
		return NewErrForbidden("assessment", id.String())
	}

	return a.inner.UnshareAssessment(ctx, id)
}

func buildOwnerSharing(rels []model.Relationship) *model.Sharing {
	shared := make([]model.SharingSubject, 0, len(rels))
	for _, r := range rels {
		if r.Relation == model.OwnerRelation {
			continue
		}
		st := string(r.Subject.Kind)
		if r.Subject.Kind == model.OrgSubject {
			st = "group"
		}
		shared = append(shared, model.SharingSubject{Type: st, ID: r.Subject.ID})
	}
	return &model.Sharing{
		IsShared:   len(shared) > 0,
		SharedWith: shared,
	}
}

func buildViewerSharing(ownerUsername string) *model.Sharing {
	return &model.Sharing{
		IsShared:   true,
		SharedWith: []model.SharingSubject{},
		SharedBy:   &model.SharingSubject{Type: "username", ID: ownerUsername},
	}
}
