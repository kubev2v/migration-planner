package store

import (
	"context"
	"fmt"
	"io"
	"math/rand"

	v1pb "github.com/authzed/authzed-go/proto/authzed/api/v1"
	"github.com/authzed/authzed-go/v1"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/kubev2v/migration-planner/internal/store/model"
)

type ZedTokenKey struct{}

type Authz interface {
	WriteRelationships(ctx context.Context, relationships ...model.RelationshipFn) error
	DeleteRelationships(ctx context.Context, resource model.Resource) error
	ListResources(ctx context.Context, userID string, permission model.Permission, resourceType model.ResourceType) ([]string, error)
	GetPermissions(ctx context.Context, assessmentIDs []string, userID string) (map[string][]model.Permission, error)
}

type AuthzStore struct {
	ZedToken *v1pb.ZedToken // should be public to allow unit test to use it
	client   *authzed.Client
	zedStore *ZedTokenStore
	db       *gorm.DB
}

func NewAuthzStore(zedTokenStore *ZedTokenStore, client *authzed.Client, db *gorm.DB) Authz {
	return &AuthzStore{
		client:   client,
		zedStore: zedTokenStore,
		db:       db,
	}
}

// WriteRelationships writes multiple relationships to SpiceDB using relationship functions.
// This method allows batch creation of relationships for better performance and atomicity.
//
// Parameters:
//   - ctx: Context for the request
//   - relationships: Variadic list of RelationshipFn functions that define relationships to create
//
// Returns:
//   - error: Error if the operation fails, nil on success
//
// Example:
//
//	userSubject := model.Subject{Kind: model.User, Id: "user123"}
//	orgSubject := model.Subject{Kind: model.Organization, Id: "org456"}
//
//	err := authzService.WriteRelationships(ctx,
//	    AddUserToOrganization("user123", "org456"),
//	    WithOwnerRelationship("assessment789", userSubject),
//	    WithReaderRelationship("assessment789", orgSubject),
//	)
//	if err != nil {
//	    log.Printf("Failed to write relationships: %v", err)
//	}
func (a *AuthzStore) WriteRelationships(ctx context.Context, relationships ...model.RelationshipFn) error {
	callerID := fmt.Sprintf("write-%d", rand.Int31())
	zap.S().Debugw("WriteRelationships: acquiring exclusive lock", "callerID", callerID, "relationshipCount", len(relationships))
	if err := a.zedStore.AcquireGlobalLock(ctx); err != nil {
		zap.S().Errorw("WriteRelationships: failed to acquire exclusive lock", "callerID", callerID, "error", err)
		return err
	}

	zap.S().Debugw("WriteRelationships: executing relationship writes", "callerID", callerID)
	relationshipsUpdate := []*v1pb.RelationshipUpdate{}
	relationshipModels := make(map[model.RelationshipOp][]model.Relationship)

	if len(relationships) == 0 {
		return nil
	}

	for _, fn := range relationships {
		var rel model.Relationship
		var op model.RelationshipOp
		relationshipsUpdate, rel, op = fn(relationshipsUpdate)
		relationshipModels[op] = append(relationshipModels[op], rel)
	}

	resp, err := a.client.WriteRelationships(ctx, &v1pb.WriteRelationshipsRequest{
		Updates: relationshipsUpdate,
	})
	if err != nil {
		zap.S().Errorw("WriteRelationships: failed to write relationships", "error", err)
		return err
	}

	// Persist relationships to database
	for _, rel := range relationshipModels[model.RelationshipOpUpdate] {
		relModel := rel.ToModel()
		if err := a.db.WithContext(ctx).Create(&relModel).Error; err != nil {
			zap.S().Errorw("WriteRelationships: failed to create relationship in database", "error", err, "relationID", relModel.RelationID)
			return err
		}
	}

	// Delete relationships from database
	for _, rel := range relationshipModels[model.RelationshipOpDelete] {
		relModel := rel.ToModel()
		if err := a.db.WithContext(ctx).Where("relation_id = ?", relModel.RelationID).Delete(&model.RelationshipModel{}).Error; err != nil {
			zap.S().Errorw("WriteRelationships: failed to delete relationship from database", "error", err, "relationID", relModel.RelationID)
			return err
		}
	}

	zap.S().Debugw("WriteRelationships: writing zed token to store", "token", resp.WrittenAt.Token, "callerID", callerID)
	return a.zedStore.Write(ctx, resp.WrittenAt.Token)
}

// DeleteRelationships removes all relationships for a resource from SpiceDB.
// This method deletes all relationships associated with the given resource.
//
// Parameters:
//   - ctx: Context for the request
//   - resource: The resource to delete relationships for (contains ResourceType and ID)
//
// Returns:
//   - error: Error if the operation fails, nil on success
//
// Example:
//
//	// Delete assessment relationships
//	resource := model.Resource{ID: "assessment789", ResourceType: model.AssessmentObject}
//	err := authzService.DeleteRelationships(ctx, resource)
//	if err != nil {
//	    log.Printf("Failed to delete relationships: %v", err)
//	}
//
//	// Delete platform relationships
//	resource := model.Resource{ID: "platform123", ResourceType: model.PlatformObject}
//	err := authzService.DeleteRelationships(ctx, resource)
func (a *AuthzStore) DeleteRelationships(ctx context.Context, resource model.Resource) error {
	callerID := fmt.Sprintf("delete-%d", rand.Int31())
	zap.S().Debugw("DeleteRelationships: acquiring exclusive lock", "callerID", callerID, "resourceType", resource.ResourceType, "resourceID", resource.ID)
	if err := a.zedStore.AcquireGlobalLock(ctx); err != nil {
		zap.S().Errorw("DeleteRelationships: failed to acquire exclusive lock", "callerID", callerID, "error", err)
		return err
	}

	zap.S().Debugw("DeleteRelationships: executing relationship deletions", "resourceType", resource.ResourceType, "resourceID", resource.ID)

	req := &v1pb.DeleteRelationshipsRequest{
		RelationshipFilter: &v1pb.RelationshipFilter{
			ResourceType: resource.ResourceType.String(),
		},
	}
	if resource.ID != "" {
		req.RelationshipFilter.OptionalResourceId = resource.ID
	}

	resp, err := a.client.DeleteRelationships(ctx, req)
	if err != nil {
		zap.S().Errorw("DeleteRelationships: failed to delete relationships from SpiceDB", "error", err, "resourceType", resource.ResourceType, "resourceID", resource.ID)
		return err
	}

	// Delete relationships from database only for assessments (platform relationships are not tracked in DB)
	if resource.ResourceType == model.AssessmentResource {
		if resource.ID != "" {
			if err := a.db.WithContext(ctx).Where("assessment_id = ?", resource.ID).Delete(&model.RelationshipModel{}).Error; err != nil {
				zap.S().Errorw("DeleteRelationships: failed to delete relationships from database", "error", err, "resource_type", resource.ResourceType.String(), "resource_id", resource.ID)
				return nil // don't care for this as long as relationships have been removed from spicedb
			}
		} else {
			if err := a.db.WithContext(ctx).Where("assessment_id IS NOT NULL").Delete(&model.RelationshipModel{}).Error; err != nil {
				zap.S().Errorw("DeleteRelationships: failed to delete relationships from database", "error", err, "resource_type", resource.ResourceType.String())
				return nil // don't care for this as long as relationships have been removed from spicedb
			}
		}
	}

	zap.S().Debugw("DeleteRelationships: writing zed token to store", "token", resp.DeletedAt.Token, "callerID", callerID)
	return a.zedStore.Write(ctx, resp.DeletedAt.Token)
}

// ListResources returns a list of resources that the user has access to with specific permission.
// This method discovers all resources of the specified type the user can access.
//
// Parameters:
//   - ctx: Context for the request
//   - userID: The ID of the user to check resources for
//   - permission: The permission to check for
//   - resourceType: The type of resource to list (assessment or platform)
//
// Returns:
//   - []string: A slice of resource IDs
//   - error: Error if the operation fails, nil on success
//
// Example:
//
//	resources, err := authzService.ListResources(ctx, "user123", model.ReadPermission, model.AssessmentResource)
//	if err != nil {
//	    log.Printf("Failed to list resources: %v", err)
//	    return
//	}
//
//	for _, resourceID := range resources {
//	    fmt.Printf("Resource ID: %s\n", resourceID)
//	}
func (a *AuthzStore) ListResources(ctx context.Context, userID string, permission model.Permission, resourceType model.ResourceType) ([]string, error) {
	zap.S().Debugw("ListResources: acquiring shared lock", "userID", userID, "resourceType", resourceType)
	if err := a.zedStore.AcquireSharedLock(ctx); err != nil {
		zap.S().Errorw("ListResources: failed to acquire shared lock", "error", err, "userID", userID)
		return []string{}, err
	}
	zap.S().Debugw("ListResources: shared lock acquired")

	zap.S().Debugw("ListResources: reading zed token from store")
	token, err := a.zedStore.Read(ctx)
	if err != nil {
		zap.S().Errorw("ListResources: failed to read zed token", "error", err)
		return []string{}, err
	}

	subject := model.NewUserSubject(userID)
	zap.S().Debugw("ListResources: executing resource lookup", "userID", userID, "resourceType", resourceType)

	// Lookup resources for which the user has the specified permission
	req := &v1pb.LookupResourcesRequest{
		ResourceObjectType: resourceType.String(),
		Permission:         permission.String(),
		Subject: &v1pb.SubjectReference{
			Object: &v1pb.ObjectReference{
				ObjectType: model.UserObject,
				ObjectId:   subject.GeneratedID,
			},
		},
	}

	// Use token for at least as fresh consistency
	if token != nil {
		req.Consistency = &v1pb.Consistency{
			Requirement: &v1pb.Consistency_AtLeastAsFresh{
				AtLeastAsFresh: &v1pb.ZedToken{Token: *token},
			},
		}
	}

	resp, err := a.client.LookupResources(ctx, req)
	if err != nil {
		return nil, err
	}

	ids := []string{}
	for {
		resource, err := resp.Recv()
		if err != nil {
			// Check if we've reached the end of the stream
			if err == io.EOF {
				break
			}
			return nil, err
		}

		ids = append(ids, resource.ResourceObjectId)
	}

	return ids, nil
}

// GetPermissions returns a map of permissions that a user has on multiple assessments.
// This method checks all possible permissions for each assessment and returns only those that the user actually has.
//
// Parameters:
//   - ctx: Context for the request
//   - assessmentIDs: A slice of assessment IDs to check permissions for
//   - userID: The ID of the user to check permissions for
//
// Returns:
//   - map[string][]model.Permission: A map where keys are assessment IDs and values are slices of permissions
//   - error: Error if the operation fails, nil on success
//
// Example:
//
//	assessmentIDs := []string{"assessment789", "assessment456"}
//	permissionsMap, err := authzService.GetPermissions(ctx, assessmentIDs, "user123")
//	if err != nil {
//	    log.Printf("Failed to get permissions: %v", err)
//	    return
//	}
//
//	for assessmentID, permissions := range permissionsMap {
//	    fmt.Printf("Assessment %s permissions: ", assessmentID)
//	    for _, perm := range permissions {
//	        fmt.Printf("%s ", perm.String())
//	    }
//	    fmt.Println()
//	}
func (a *AuthzStore) GetPermissions(ctx context.Context, assessmentIDs []string, userID string) (map[string][]model.Permission, error) {
	zap.S().Debugw("GetPermissions: acquiring shared lock", "userID", userID, "assessmentCount", len(assessmentIDs))
	if err := a.zedStore.AcquireSharedLock(ctx); err != nil {
		zap.S().Errorw("GetPermissions: failed to acquire shared lock", "error", err, "userID", userID)
		return map[string][]model.Permission{}, err
	}

	// read the token first
	zap.S().Debugw("GetPermissions: reading zed token from store")
	token, err := a.zedStore.Read(ctx)
	if err != nil {
		zap.S().Errorw("GetPermissions: failed to read zed token", "error", err)
		return map[string][]model.Permission{}, err
	}

	subject := model.NewUserSubject(userID)

	zap.S().Debugw("GetPermissions: executing bulk permission checks", "userID", userID, "assessmentIDs", assessmentIDs)
	return a.getBulkPermissions(a.TokenToContext(ctx, token), subject, assessmentIDs)
}

func (a *AuthzStore) TokenToContext(ctx context.Context, token *string) context.Context {
	if token == nil {
		return ctx
	}
	return context.WithValue(ctx, ZedTokenKey{}, &v1pb.ZedToken{Token: *token})
}

// Private methods

func (a *AuthzStore) getZedToken(ctx context.Context) *v1pb.ZedToken {
	// check if service has the token already
	if a.ZedToken != nil {
		return a.ZedToken
	}
	// look into the context (used for testing mainly)
	val := ctx.Value(ZedTokenKey{})
	if val == nil {
		return nil
	}
	token, ok := val.(*v1pb.ZedToken)
	if ok {
		return token
	}
	return nil
}

// getBulkPermissions checks all possible permissions for a user on multiple assessments using bulk check
func (a *AuthzStore) getBulkPermissions(ctx context.Context, subject model.Subject, assessmentIDs []string) (map[string][]model.Permission, error) {
	if len(assessmentIDs) == 0 {
		return make(map[string][]model.Permission), nil
	}

	token := a.getZedToken(ctx)

	allPermissions := []model.Permission{
		model.ReadPermission,
		model.EditPermission,
		model.SharePermission,
		model.DeletePermission,
	}

	// Build bulk permission check request for all assessment-permission combinations
	var items []*v1pb.CheckBulkPermissionsRequestItem
	var itemIndex []struct {
		assessmentID string
		permission   model.Permission
	}

	for _, assessmentID := range assessmentIDs {
		for _, perm := range allPermissions {
			items = append(items, &v1pb.CheckBulkPermissionsRequestItem{
				Resource: &v1pb.ObjectReference{
					ObjectType: model.AssessmentObject,
					ObjectId:   assessmentID,
				},
				Permission: perm.String(),
				Subject: &v1pb.SubjectReference{
					Object: &v1pb.ObjectReference{
						ObjectType: model.UserObject,
						ObjectId:   subject.GeneratedID,
					},
				},
			})
			itemIndex = append(itemIndex, struct {
				assessmentID string
				permission   model.Permission
			}{assessmentID, perm})
		}
	}

	req := &v1pb.CheckBulkPermissionsRequest{Items: items}
	if token != nil {
		req.Consistency = &v1pb.Consistency{
			Requirement: &v1pb.Consistency_AtLeastAsFresh{
				AtLeastAsFresh: token,
			},
		}
	}

	resp, err := a.client.CheckBulkPermissions(ctx, req)
	if err != nil {
		return nil, err
	}

	// Build result map
	result := make(map[string][]model.Permission)
	for i, pair := range resp.Pairs {
		assessmentID := itemIndex[i].assessmentID
		permission := itemIndex[i].permission

		// Initialize slice if not exists
		if _, exists := result[assessmentID]; !exists {
			result[assessmentID] = []model.Permission{}
		}

		// Check if the response is an item (not an error)
		if item := pair.GetItem(); item != nil {
			if item.Permissionship == v1pb.CheckPermissionResponse_PERMISSIONSHIP_HAS_PERMISSION {
				result[assessmentID] = append(result[assessmentID], permission)
			}
		}
		// If there's an error for this permission check, we skip it
	}

	return result, nil
}
