package service

import (
	"context"
	"fmt"
	"slices"

	"go.uber.org/zap"

	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/internal/store/model"
	"github.com/kubev2v/migration-planner/pkg/metrics"
)

const (
	redhatPlatform = "redhat"
)

// Authz defines the interface for authorization operations.
// All methods require transactions because the zed_token store uses PostgreSQL
// advisory transaction locks to ensure consistency between SpiceDB operations
// and the stored zed token.
type Authz interface {
	// CreateUser creates a new user in the authorization system.
	// Acquires: Global Lock (Exclusive) - writes relationships to SpiceDB.
	CreateUser(ctx context.Context, user auth.User) error

	// CreateAssessmentRelationship creates ownership and editor relationships for an assessment.
	// It is called when the assessment is created and writes the initials relationships
	// Acquires: Global Lock (Exclusive) - writes relationships to SpiceDB.
	CreateAssessmentRelationship(ctx context.Context, assessmentID string, user auth.User) error

	// InitilizePlatform recreates the relationships for platform.
	// The old relationships are, always, deleted.
	// This establishes platform-level roles (admin, editor, viewer) that cascade permissions to assessments.
	// Acquires: Global Lock (Exclusive) - writes relationships to SpiceDB.
	//
	// Parameters:
	//   - ctx: Context for the request
	//   - users: A map where keys are role names ("admin", "editor", "viewer") and values are slices of userIDs
	//
	// Returns:
	//   - error: Error if the operation fails, nil on success
	InitilizePlatform(ctx context.Context, users map[string][]string) error

	// ListAssessments returns a list of assessment IDs that the user has read access to.
	// Acquires: Shared Lock - reads from SpiceDB using stored zed token.
	ListAssessments(ctx context.Context, user auth.User) ([]string, error)

	// WriteRelationships creates relationships (owner, editor, or reader) for an assessment.
	// Acquires: Global Lock (Exclusive) - writes relationships to SpiceDB.
	WriteRelationships(ctx context.Context, relationships ...model.Relationship) error

	// DeleteAssessmentAllRelationships removes all relationships for an assessment.
	// Acquires: Global Lock (Exclusive) - writes/deletes relationships to/from SpiceDB.
	DeleteAllRelationships(ctx context.Context, assessmentID string) error

	// DeleteAssessmentRelationship removes a specific relationship or all relationships for an assessment.
	// If subject is nil, all relationships for the assessment are removed (used when deleting an assessment).
	// If subject and relationship are provided, only that specific relationship is removed (used for unsharing).
	// Acquires: Global Lock (Exclusive) - writes/deletes relationships to/from SpiceDB.
	DeleteRelationships(ctx context.Context, relationships ...model.Relationship) error

	// GetPermissions returns the permissions a user has for a specific assessment.
	// Acquires: Shared Lock - reads from SpiceDB using stored zed token.
	GetPermissions(ctx context.Context, assessmentID string, user auth.User) ([]model.Permission, error)

	// GetBulkPermissions returns the permissions a user has for multiple assessments.
	// Acquires: Shared Lock - reads from SpiceDB using stored zed token.
	GetBulkPermissions(ctx context.Context, assessmentIds []string, user auth.User) (map[string][]model.Permission, error)

	// HasPermission checks if a user has a specific permission for an assessment.
	// Acquires: Shared Lock - reads from SpiceDB using stored zed token.
	HasPermission(ctx context.Context, assessmentID string, user auth.User, permission model.Permission) (bool, error)

	// ListRelationships returns all relationships for a specific assessment.
	// Acquires: Shared Lock - reads from SpiceDB using stored zed token.
	ListRelationships(ctx context.Context, assessmentID string) ([]model.Relationship, error)
}

// AuthzService provides authorization functionality. All methods require transactions
// because the zed_token store uses PostgreSQL advisory transaction locks to ensure consistency
// between SpiceDB operations and the stored zed token.
//
// Lock Types:
//   - Global Lock (Exclusive): pg_advisory_xact_lock() - Used for write operations (WriteRelationships, DeleteRelationships)
//     that modify relationships. Only one global lock can be held at a time.
//   - Shared Lock: pg_advisory_xact_lock_shared() - Used for read operations (ListResources, GetPermissions)
//     that query permissions. Multiple shared locks can be held concurrently, but not with a global lock.
//
// The advisory locks are transaction-scoped and automatically released when the transaction
// commits or rolls back, ensuring proper cleanup and preventing deadlocks.
type AuthzService struct {
	s store.Store
}

func NewAuthzService(s store.Store) *AuthzService {
	return &AuthzService{
		s: s,
	}
}

// CreateUser creates a new user in the authorization system.
// Acquires: Global Lock (Exclusive) - writes relationships to SpiceDB.
func (a *AuthzService) CreateUser(ctx context.Context, user auth.User) error {
	// absolutly, we need a transaction here for the lock to be acquired
	ctx, err := a.s.NewTransactionContext(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_, _ = store.Rollback(ctx)
	}()

	relationships := []model.RelationshipFn{
		store.WithMemberRelationship(model.NewUserSubject(user.Username), model.NewOrganizationSubject(user.Organization)),
	}

	if err := a.s.Authz().WriteRelationships(ctx, relationships...); err != nil {
		return err
	}

	if _, err := store.Commit(ctx); err != nil {
		return err
	}

	return nil
}

// CreateAssessmentRelationship creates ownership and editor relationships for an assessment.
// It is called when the assessment is created and writes the initials relationships
// Acquires: Global Lock (Exclusive) - writes relationships to SpiceDB.
func (a *AuthzService) CreateAssessmentRelationship(ctx context.Context, assessmentID string, user auth.User) error {
	ctx, err := a.s.NewTransactionContext(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_, _ = store.Rollback(ctx)
	}()

	relationships := []model.RelationshipFn{
		store.WithOwnerRelationship(assessmentID, model.NewUserSubject(user.Username)),
		store.WithParentRelationship(assessmentID, model.NewPlatformSubject(redhatPlatform)),
	}

	if err := a.s.Authz().WriteRelationships(ctx, relationships...); err != nil {
		return err
	}

	if _, err := store.Commit(ctx); err != nil {
		return err
	}

	return nil
}

// InitilizePlatform recreates the relationships for platform.
// The old relationships are, always, deleted.
// This establishes platform-level roles (admin, editor, viewer) that cascade permissions to assessments.
// Acquires: Global Lock (Exclusive) - writes relationships to SpiceDB.
//
// Parameters:
//   - ctx: Context for the request
//   - platformID: The ID of the platform to assign roles for
//   - users: A map where keys are role names ("admin", "editor", "viewer") and values are slices of userIDs
//
// Returns:
//   - error: Error if the operation fails, nil on success
//
// Example:
//
//	users := map[string][]string{
//	    "admin": ["user1", "user2"],
//	    "editor": ["user3"],
//	    "viewer": ["user4", "user5"],
//	}
//	err := authzService.CreatePlatformRelationships(ctx, "platform-id", users)
func (a *AuthzService) InitilizePlatform(ctx context.Context, users map[string][]string) error {
	ctx, err := a.s.NewTransactionContext(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_, _ = store.Rollback(ctx)
	}()

	// remove everything first
	if err := a.s.Authz().DeleteRelationships(ctx, model.NewPlatformResource(redhatPlatform)); err != nil {
		return err
	}

	// Convert userIDs to subjects map
	subjectsMap := make(map[string][]model.Subject)
	for role, userIDs := range users {
		if len(userIDs) == 0 {
			continue
		}

		switch role {
		case model.AdminPlatformRelationshipKind.String():
		case model.EditorPlatformRelationshipKind.String():
		case model.ViewerPlatformRelationshipKind.String():
		default:
			zap.S().Debugw("unknown platform relationship", "relationship", role)
			continue
		}

		subjects := make([]model.Subject, 0, len(userIDs))
		for _, userID := range userIDs {
			subjects = append(subjects, model.NewUserSubject(userID))
		}
		subjectsMap[role] = subjects
	}

	if err := a.s.Authz().WriteRelationships(ctx, store.WithPlatformRelationship(model.NewPlatformSubject(redhatPlatform), subjectsMap)); err != nil {
		return err
	}

	if _, err := store.Commit(ctx); err != nil {
		return err
	}

	return nil
}

// ListAssessments returns a list of assessment IDs that the user has read access to.
// Acquires: Shared Lock - reads from SpiceDB using stored zed token.
func (a *AuthzService) ListAssessments(ctx context.Context, user auth.User) ([]string, error) {
	ctx, err := a.s.NewTransactionContext(ctx)
	if err != nil {
		return []string{}, err
	}
	defer func() {
		_, _ = store.Rollback(ctx)
	}()

	// list assessments for which the current user has **at least** read permission
	allowedAssessments, err := a.s.Authz().ListResources(ctx, user.Username, model.ReadPermission, model.AssessmentResource)
	if err != nil {
		return []string{}, err
	}

	if _, err := store.Commit(ctx); err != nil {
		return []string{}, err
	}

	return allowedAssessments, nil
}

// WriteRelationships creates multiple relationships for an assessment.
// Acquires: Global Lock (Exclusive) - writes relationships to SpiceDB.
func (a *AuthzService) WriteRelationships(ctx context.Context, relationships ...model.Relationship) error {
	ctx, err := a.s.NewTransactionContext(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_, _ = store.Rollback(ctx)
	}()

	relationshipFns := make([]model.RelationshipFn, 0, len(relationships))
	for _, relationship := range relationships {
		var relationshipFn model.RelationshipFn
		switch relationship.Kind {
		case model.OwnerRelationshipKind:
			relationshipFn = store.WithOwnerRelationship(relationship.AssessmentID, relationship.Subject)
		case model.OrganizationRelationshipKind:
			relationshipFn = store.WithOrganizationRelationship(relationship.AssessmentID, relationship.Subject)
		case model.ViewerRelationshipKind:
			relationshipFn = store.WithViewerRelationship(relationship.AssessmentID, relationship.Subject)
		case model.EditorRelationshipKind:
			relationshipFn = store.WithEditorRelationship(relationship.AssessmentID, relationship.Subject)
		default:
			return fmt.Errorf("unsupported relationship kind: %s", relationship.Kind)
		}
		relationshipFns = append(relationshipFns, relationshipFn)
	}

	metrics.IncreaseAuthzTotalRelationshipsMetric(len(relationships))
	if err := a.s.Authz().WriteRelationships(ctx, relationshipFns...); err != nil {
		return err
	}

	if _, err := store.Commit(ctx); err != nil {
		return err
	}

	metrics.IncreaseAuthzValidRelationshipsMetric(len(relationships))
	return nil
}

func (a *AuthzService) DeleteAllRelationships(ctx context.Context, assessmentID string) error {
	ctx, err := a.s.NewTransactionContext(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_, _ = store.Rollback(ctx)
	}()

	metrics.IncreaseAuthzTotalRelationshipsMetric(1)
	if err := a.s.Authz().DeleteRelationships(ctx, model.NewAssessmentResource(assessmentID)); err != nil {
		return err
	}
	if _, err := store.Commit(ctx); err != nil {
		return err
	}
	metrics.IncreaseAuthzValidRelationshipsMetric(1)
	return nil
}

// DeleteRelationships removes multiple relationships for an assessment.
// Acquires: Global Lock (Exclusive) - writes/deletes relationships to/from SpiceDB.
func (a *AuthzService) DeleteRelationships(ctx context.Context, relationships ...model.Relationship) error {
	ctx, err := a.s.NewTransactionContext(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_, _ = store.Rollback(ctx)
	}()

	relationshipFns := make([]model.RelationshipFn, 0, len(relationships))
	for _, r := range relationships {
		var relationshipFn model.RelationshipFn
		switch r.Kind {
		case model.OwnerRelationshipKind:
			relationshipFn = store.WithoutOwnerRelationship(r.AssessmentID, r.Subject)
		case model.OrganizationRelationshipKind:
			relationshipFn = store.WithoutOrganizationRelationship(r.AssessmentID, r.Subject)
		case model.ViewerRelationshipKind:
			relationshipFn = store.WithoutViewerRelationship(r.AssessmentID, r.Subject)
		case model.EditorRelationshipKind:
			relationshipFn = store.WithoutEditorRelationship(r.AssessmentID, r.Subject)
		default:
			return fmt.Errorf("unsupported relationship kind: %s", r)
		}
		relationshipFns = append(relationshipFns, relationshipFn)
	}

	metrics.IncreaseAuthzTotalRelationshipsMetric(len(relationships))
	if err := a.s.Authz().WriteRelationships(ctx, relationshipFns...); err != nil {
		return err
	}

	if _, err := store.Commit(ctx); err != nil {
		return err
	}

	metrics.IncreaseAuthzValidRelationshipsMetric(len(relationships))
	return nil
}

// GetPermissions returns the permissions a user has for a specific assessment.
// Acquires: Shared Lock - reads from SpiceDB using stored zed token.
func (a *AuthzService) GetPermissions(ctx context.Context, assessmentID string, user auth.User) ([]model.Permission, error) {
	ctx, err := a.s.NewTransactionContext(ctx)
	if err != nil {
		return []model.Permission{}, err
	}
	defer func() {
		_, _ = store.Rollback(ctx)
	}()

	permissionsMap, err := a.s.Authz().GetPermissions(ctx, []string{assessmentID}, user.Username)
	if err != nil {
		return []model.Permission{}, err
	}

	if permissions, ok := permissionsMap[assessmentID]; ok {
		if _, err := store.Commit(ctx); err != nil {
			return []model.Permission{}, err
		}
		return permissions, nil
	}

	if _, err := store.Commit(ctx); err != nil {
		return []model.Permission{}, err
	}

	return []model.Permission{}, nil
}

// GetBulkPermissions returns the permissions a user has for multiple assessments.
// Acquires: Shared Lock - reads from SpiceDB using stored zed token.
func (a *AuthzService) GetBulkPermissions(ctx context.Context, assessmentIds []string, user auth.User) (map[string][]model.Permission, error) {
	ctx, err := a.s.NewTransactionContext(ctx)
	if err != nil {
		return map[string][]model.Permission{}, err
	}
	defer func() {
		_, _ = store.Rollback(ctx)
	}()

	permissions, err := a.s.Authz().GetPermissions(ctx, assessmentIds, user.Username)
	if err != nil {
		return map[string][]model.Permission{}, err
	}

	if _, err := store.Commit(ctx); err != nil {
		return map[string][]model.Permission{}, err
	}

	return permissions, nil
}

// HasPermission checks if a user has a specific permission for an assessment.
// Acquires: Shared Lock - reads from SpiceDB using stored zed token.
func (a *AuthzService) HasPermission(ctx context.Context, assessmentID string, user auth.User, permission model.Permission) (bool, error) {
	ctx, err := a.s.NewTransactionContext(ctx)
	if err != nil {
		return false, err
	}
	defer func() {
		_, _ = store.Rollback(ctx)
	}()

	permissionsMap, err := a.s.Authz().GetPermissions(ctx, []string{assessmentID}, user.Username)
	if err != nil {
		return false, err
	}

	if permissions, ok := permissionsMap[assessmentID]; ok {
		if _, err := store.Commit(ctx); err != nil {
			return false, err
		}
		return slices.Contains(permissions, permission), nil
	}

	if _, err := store.Commit(ctx); err != nil {
		return false, err
	}

	return false, nil
}

// ListRelationships returns all relationships for a specific assessment.
// Acquires: Shared Lock - reads from SpiceDB using stored zed token.
func (a *AuthzService) ListRelationships(ctx context.Context, assessmentID string) ([]model.Relationship, error) {
	ctx, err := a.s.NewTransactionContext(ctx)
	if err != nil {
		return []model.Relationship{}, err
	}
	defer func() {
		_, _ = store.Rollback(ctx)
	}()

	relationships, err := a.s.Authz().ListRelationships(ctx, assessmentID)
	if err != nil {
		return []model.Relationship{}, err
	}

	if _, err := store.Commit(ctx); err != nil {
		return []model.Relationship{}, err
	}

	return relationships, nil
}

// NoopAuthzService is an implementation of the Authz interface
// that performs no op or legacy authz.
// The only op is in ListAssessments which returns all the assessments ids found in db.
// This service is used when MIGRATION_PLANNER_AUTHZ_ENABLED = false
type NoopAuthzService struct {
	s        store.Store
	isLegacy bool
}

// NewNoopAuthzService creates a new instance of NoopAuthzService.
func NewNoopAuthzService(s store.Store) *NoopAuthzService {
	return &NoopAuthzService{s: s}
}

func NewLegacyAuthzService(s store.Store) *NoopAuthzService {
	return &NoopAuthzService{s: s, isLegacy: true}
}

// CreateUser is a no-op implementation that returns nil.
func (n *NoopAuthzService) CreateUser(ctx context.Context, user auth.User) error {
	return nil
}

// CreateAssessmentRelationship is a no-op implementation that returns nil.
func (n *NoopAuthzService) CreateAssessmentRelationship(ctx context.Context, assessmentID string, user auth.User) error {
	return nil
}

// InitilizePlatform is a no-op implementation that returns nil.
func (n *NoopAuthzService) InitilizePlatform(ctx context.Context, users map[string][]string) error {
	return nil
}

// ListAssessments returns all ids from DB in "none" authz kind.
// For "legacy" filters out ids by org_id.
func (n *NoopAuthzService) ListAssessments(ctx context.Context, user auth.User) ([]string, error) {
	filter := store.NewAssessmentQueryFilter()

	if n.isLegacy {
		filter = filter.WithOrgID(user.Organization)
	}

	assessments, err := n.s.Assessment().List(ctx, filter)
	if err != nil {
		return []string{}, err
	}

	ids := make([]string, 0, len(assessments))
	for _, a := range assessments {
		ids = append(ids, a.ID.String())
	}

	return ids, nil
}

// WriteRelationships is a no-op implementation that returns nil.
func (n *NoopAuthzService) WriteRelationships(ctx context.Context, relationships ...model.Relationship) error {
	return nil
}

// DeletetAllRelationships is a no-op implementation that returns nil.
func (n *NoopAuthzService) DeleteAllRelationships(ctx context.Context, assessmentID string) error {
	return nil
}

// DeletetRelationships is a no-op implementation that returns nil.
func (n *NoopAuthzService) DeleteRelationships(ctx context.Context, relationships ...model.Relationship) error {
	return nil
}

// GetPermissions is a no-op implementation that returns all permissions.
func (n *NoopAuthzService) GetPermissions(ctx context.Context, assessmentID string, user auth.User) ([]model.Permission, error) {
	return []model.Permission{
		model.ReadPermission,
		model.EditPermission,
		model.SharePermission,
		model.DeletePermission,
	}, nil
}

// GetBulkPermissions is a no-op implementation that returns a map with all permissions for each assessment.
func (n *NoopAuthzService) GetBulkPermissions(ctx context.Context, assessmentIds []string, user auth.User) (map[string][]model.Permission, error) {
	permissions := make(map[string][]model.Permission)
	for _, id := range assessmentIds {
		permissions[id] = []model.Permission{
			model.ReadPermission,
			model.EditPermission,
			model.SharePermission,
			model.DeletePermission,
		}
	}

	return permissions, nil
}

// HasPermission is a no-op implementation that returns true.
func (n *NoopAuthzService) HasPermission(ctx context.Context, assessmentID string, user auth.User, permission model.Permission) (bool, error) {
	return true, nil
}

// ListRelationships is a no-op implementation that returns an empty slice.
func (n *NoopAuthzService) ListRelationships(ctx context.Context, assessmentID string) ([]model.Relationship, error) {
	return []model.Relationship{}, nil
}
