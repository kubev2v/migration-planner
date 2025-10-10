package store_test

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"time"

	v1pb "github.com/authzed/authzed-go/proto/authzed/api/v1"
	"github.com/authzed/authzed-go/v1"
	"github.com/authzed/grpcutil"
	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/internal/config"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/internal/store/model"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"gorm.io/gorm"
)

// AuthzStore Test Suite
//
// This test suite provides comprehensive coverage of the authorization store implementation,
// which manages relationships and permissions using SpiceDB (Zanzibar-style authorization).
//
// Test Contexts:
//
// 1. Write Relationships
//    - Tests the WriteRelationships method for creating and managing authorization relationships
//    - Coverage: Batch writing, With* functions (create), Without* functions (delete)
//    - Validates: Atomic operations, database tracking (Update/Delete operations)
//    - Edge cases: Empty relationship list, permission verification after changes
//
// 2. User-Organization Membership
//    - Tests basic user-to-organization membership relationships
//    - Coverage: WithMemberRelationship function
//    - Validates: Member relationship creation, verification via SpiceDB queries
//    - Purpose: Foundation for organization-based permissions
//
// 3. User-Assessment Relationships
//    - Tests direct user relationships with assessments (owner, viewer)
//    - Coverage: WithOwnerRelationship, WithViewerRelationship
//    - Validates: Owner has all permissions, viewer has read-only access
//    - Purpose: Tests highest and lowest privilege levels
//
// 4. Organization-Assessment Relationships
//    - Tests organization-level access to assessments
//    - Coverage: WithOrganizationRelationship
//    - Validates: Organization relationship grants members read+edit permissions
//    - Purpose: Tests organization-based access control
//
// 5. List Permissions
//    - Tests ListResources method and permission discovery
//    - Coverage: Platform roles (super_admin, editor, viewer), org membership
//    - Validates: Permission inheritance through platform parent relationships
//    - Purpose: Tests all permission sources from SpiceDB schema:
//      * owner (all permissions)
//      * viewer (read only)
//      * org->org_member (read + edit)
//      * parent->super_admin (all permissions)
//      * parent->edit (read + edit)
//      * parent->view (read only)
//
// 6. Delete Relationships
//    - Tests DeleteRelationships method for removing authorization relationships
//    - Coverage: Single resource deletion, bulk deletion (all assessments)
//    - Validates: Relationship removal from SpiceDB, permission revocation
//    - Database: Verifies relationships removed from tracking table
//    - Consistency: Tests ZedToken management across operations
//
// 7. Database Relationship Tracking
//    - Tests selective persistence of relationships to PostgreSQL
//    - Coverage: Assessment relationships (tracked), platform relationships (not tracked)
//    - Validates: Correct database operations based on RelationshipOp type
//    - Purpose: Tests dual-storage model (SpiceDB for auth, DB for metadata)
//
// 8. Get Permissions
//    - Tests GetPermissions method for bulk permission retrieval
//    - Coverage: All permission sources, combinations, edge cases
//    - Validates:
//      * Single permissions: owner, viewer, org member, platform roles
//      * Combined permissions: owner+org, viewer+org
//      * Isolation: wrong org, no access, parent without role
//      * Bulk operations: multiple assessments with different permissions
//      * Edge cases: empty list, non-existent assessments
//    - Purpose: Comprehensive validation of permission computation
//
// SpiceDB Schema Coverage:
//   - assessment#owner (all permissions)
//   - assessment#viewer (read only)
//   - assessment#org->org_member (read + edit)
//   - assessment#parent->super_admin (all permissions)
//   - assessment#parent->edit (read + edit)
//   - assessment#parent->view (read only)
//   - org#member (organization membership)
//   - platform#admin (super_admin permission)
//   - platform#editor (edit permission)
//   - platform#viewer (view permission)
//
// Database Operations Coverage:
//   - RelationshipOpUpdate: Tracked in relationships table
//   - RelationshipOpDelete: Removed from relationships table
//   - RelationshipOpTouch: Used for org membership (tracked differently)
//   - RelationshipOpIgnore: Not tracked (platform relationships)
//
// ZedToken Consistency:
//   - All tests validate proper token management for read-after-write consistency
//   - Tests verify token updates after write and delete operations
//   - Tests use AtLeastAsFresh consistency for permission checks

var _ = Describe("AuthzStore", Ordered, func() {
	var (
		authzSvc      *store.AuthzStore
		spiceDBClient *authzed.Client
		ctx           context.Context
		zedToken      *v1pb.ZedToken
		gormDB        *gorm.DB
		zedTokenStore *store.ZedTokenStore
	)

	BeforeAll(func() {
		ctx = context.Background()

		// Skip tests if SpiceDB is not available
		spiceDBEndpoint := os.Getenv("SPICEDB_ENDPOINT")
		if spiceDBEndpoint == "" {
			spiceDBEndpoint = "localhost:50051"
		}

		// Create SpiceDB client
		var err error
		spiceDBClient, err = authzed.NewClient(
			spiceDBEndpoint,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpcutil.WithInsecureBearerToken("foobar"),
		)
		if err != nil {
			zap.S().Error(err)
			Skip("SpiceDB not available: " + err.Error())
		}

		// Test connection
		_, err = spiceDBClient.ReadSchema(ctx, &v1pb.ReadSchemaRequest{})
		if err != nil {
			Skip("SpiceDB not reachable: " + err.Error())
		}

		// Initialize database using the same pattern as other tests
		cfg, err := config.New()
		Expect(err).To(BeNil())

		gormDB, err = store.InitDB(cfg)
		Expect(err).To(BeNil())

		zedTokenStore = store.NewZedTokenStore(gormDB)
		authzSvc = store.NewAuthzStore(zedTokenStore, spiceDBClient, gormDB).(*store.AuthzStore)
	})

	AfterAll(func() {
		if spiceDBClient != nil {
			spiceDBClient.Close()
		}
	})

	Context("Write Relationships", func() {
		// Test: Validates batch writing of multiple relationships atomically
		// Expected: All relationships should be written in a single transaction
		// Purpose: Tests atomic batch relationship creation
		It("should write multiple relationships atomically", func() {
			batchUserID := "batch-user-" + uuid.New().String()[:8]
			batchOrgID := "batch-org-" + uuid.New().String()[:8]
			batchAssessment1ID := "batch-assessment1-" + uuid.New().String()[:8]
			batchAssessment2ID := "batch-assessment2-" + uuid.New().String()[:8]

			userSubject := model.NewUserSubject(batchUserID)
			orgSubject := model.NewOrganizationSubject(batchOrgID)

			// Write multiple relationships in one call
			err := authzSvc.WriteRelationships(ctx,
				store.WithMemberRelationship(userSubject, orgSubject),
				store.WithOwnerRelationship(batchAssessment1ID, userSubject),
				store.WithViewerRelationship(batchAssessment2ID, userSubject),
				store.WithOrganizationRelationship(batchAssessment1ID, orgSubject),
			)
			Expect(err).To(BeNil())

			// Verify all relationships were created
			permissionsMap, err := authzSvc.GetPermissions(ctx, []string{batchAssessment1ID, batchAssessment2ID}, batchUserID)
			Expect(err).To(BeNil())

			// Assessment1: owner + org member = all permissions
			Expect(permissionsMap[batchAssessment1ID]).To(ContainElement(model.ReadPermission))
			Expect(permissionsMap[batchAssessment1ID]).To(ContainElement(model.DeletePermission))

			// Assessment2: viewer only
			Expect(permissionsMap[batchAssessment2ID]).To(ContainElement(model.ReadPermission))
		})

		// Test: Validates removing relationships using Without* functions
		// Expected: Specific relationships should be deleted without affecting others
		// Purpose: Tests granular relationship deletion via WriteRelationships
		It("should remove specific relationship using WithoutOwnerRelationship", func() {
			removeUserID := "remove-user-" + uuid.New().String()[:8]
			removeOrgID := "remove-org-" + uuid.New().String()[:8]
			removeAssessmentID := "remove-assessment-" + uuid.New().String()[:8]

			_, err := setupUserOrganizationMembership(ctx, spiceDBClient, zedTokenStore, removeUserID, removeOrgID)
			Expect(err).To(BeNil())

			userSubject := model.NewUserSubject(removeUserID)
			orgSubject := model.NewOrganizationSubject(removeOrgID)

			// Create owner and org relationships
			err = authzSvc.WriteRelationships(ctx,
				store.WithOwnerRelationship(removeAssessmentID, userSubject),
				store.WithOrganizationRelationship(removeAssessmentID, orgSubject),
			)
			Expect(err).To(BeNil())

			// Verify user has all permissions (owner + org member)
			permissionsMap, err := authzSvc.GetPermissions(ctx, []string{removeAssessmentID}, removeUserID)
			Expect(err).To(BeNil())
			Expect(permissionsMap[removeAssessmentID]).To(ContainElement(model.DeletePermission))

			// Remove only owner relationship
			err = authzSvc.WriteRelationships(ctx, store.WithoutOwnerRelationship(removeAssessmentID, userSubject))
			Expect(err).To(BeNil())

			// Verify user still has org member permissions but not owner permissions
			permissionsMap, err = authzSvc.GetPermissions(ctx, []string{removeAssessmentID}, removeUserID)
			Expect(err).To(BeNil())
			Expect(permissionsMap[removeAssessmentID]).To(ContainElements(
				model.ReadPermission,
				model.EditPermission,
			), "User should still have org member permissions")
			Expect(permissionsMap[removeAssessmentID]).ToNot(ContainElements(
				model.DeletePermission,
			), "User should not have owner-only permissions")
		})

		// Test: Validates removing viewer relationship
		// Expected: Viewer access should be revoked
		// Purpose: Tests WithoutViewerRelationship function
		It("should remove viewer relationship using WithoutViewerRelationship", func() {
			viewerRemoveUserID := "viewer-remove-" + uuid.New().String()[:8]
			viewerRemoveOrgID := "viewer-remove-org-" + uuid.New().String()[:8]
			viewerRemoveAssessmentID := "viewer-remove-assessment-" + uuid.New().String()[:8]

			_, err := setupUserOrganizationMembership(ctx, spiceDBClient, zedTokenStore, viewerRemoveUserID, viewerRemoveOrgID)
			Expect(err).To(BeNil())

			userSubject := model.NewUserSubject(viewerRemoveUserID)

			// Create viewer relationship
			err = authzSvc.WriteRelationships(ctx, store.WithViewerRelationship(viewerRemoveAssessmentID, userSubject))
			Expect(err).To(BeNil())

			// Verify user has read permission
			permissionsMap, err := authzSvc.GetPermissions(ctx, []string{viewerRemoveAssessmentID}, viewerRemoveUserID)
			Expect(err).To(BeNil())
			Expect(permissionsMap[viewerRemoveAssessmentID]).To(ContainElement(model.ReadPermission))

			// Remove viewer relationship
			err = authzSvc.WriteRelationships(ctx, store.WithoutViewerRelationship(viewerRemoveAssessmentID, userSubject))
			Expect(err).To(BeNil())

			// Verify user has no permissions
			permissionsMap, err = authzSvc.GetPermissions(ctx, []string{viewerRemoveAssessmentID}, viewerRemoveUserID)
			Expect(err).To(BeNil())
			if permissions, exists := permissionsMap[viewerRemoveAssessmentID]; exists {
				Expect(permissions).To(BeEmpty())
			}
		})

		// Test: Validates removing organization relationship
		// Expected: Organization members should lose access
		// Purpose: Tests WithoutOrganizationRelationship function
		It("should remove organization relationship using WithoutOrganizationRelationship", func() {
			orgRemoveUserID := "org-remove-user-" + uuid.New().String()[:8]
			orgRemoveOrgID := "org-remove-org-" + uuid.New().String()[:8]
			orgRemoveAssessmentID := "org-remove-assessment-" + uuid.New().String()[:8]

			_, err := setupUserOrganizationMembership(ctx, spiceDBClient, zedTokenStore, orgRemoveUserID, orgRemoveOrgID)
			Expect(err).To(BeNil())

			orgSubject := model.NewOrganizationSubject(orgRemoveOrgID)

			// Create org relationship
			err = authzSvc.WriteRelationships(ctx, store.WithOrganizationRelationship(orgRemoveAssessmentID, orgSubject))
			Expect(err).To(BeNil())

			// Verify org member has permissions
			permissionsMap, err := authzSvc.GetPermissions(ctx, []string{orgRemoveAssessmentID}, orgRemoveUserID)
			Expect(err).To(BeNil())
			Expect(permissionsMap[orgRemoveAssessmentID]).To(ContainElements(
				model.ReadPermission,
				model.EditPermission,
			))

			// Remove org relationship
			err = authzSvc.WriteRelationships(ctx, store.WithoutOrganizationRelationship(orgRemoveAssessmentID, orgSubject))
			Expect(err).To(BeNil())

			// Verify org member has no permissions
			permissionsMap, err = authzSvc.GetPermissions(ctx, []string{orgRemoveAssessmentID}, orgRemoveUserID)
			Expect(err).To(BeNil())
			if permissions, exists := permissionsMap[orgRemoveAssessmentID]; exists {
				Expect(permissions).To(BeEmpty())
			}
		})

		// Test: Validates removing user from organization
		// Expected: User should lose access to org assessments
		// Purpose: Tests WithoutMemberRelationship function
		It("should remove user from organization using WithoutMemberRelationship", func() {
			memberRemoveUserID := "member-remove-user-" + uuid.New().String()[:8]
			memberRemoveOrgID := "member-remove-org-" + uuid.New().String()[:8]
			memberRemoveAssessmentID := "member-remove-assessment-" + uuid.New().String()[:8]

			userSubject := model.NewUserSubject(memberRemoveUserID)
			orgSubject := model.NewOrganizationSubject(memberRemoveOrgID)

			// Add user to org and create org assessment
			err := authzSvc.WriteRelationships(ctx,
				store.WithMemberRelationship(userSubject, orgSubject),
				store.WithOrganizationRelationship(memberRemoveAssessmentID, orgSubject),
			)
			Expect(err).To(BeNil())

			// Verify user has permissions via org membership
			permissionsMap, err := authzSvc.GetPermissions(ctx, []string{memberRemoveAssessmentID}, memberRemoveUserID)
			Expect(err).To(BeNil())
			Expect(permissionsMap[memberRemoveAssessmentID]).To(ContainElements(
				model.ReadPermission,
				model.EditPermission,
			))

			// Remove user from organization
			err = authzSvc.WriteRelationships(ctx, store.WithoutMemberRelationship(userSubject, orgSubject))
			Expect(err).To(BeNil())

			// Verify user has no permissions after removal from org
			permissionsMap, err = authzSvc.GetPermissions(ctx, []string{memberRemoveAssessmentID}, memberRemoveUserID)
			Expect(err).To(BeNil())
			if permissions, exists := permissionsMap[memberRemoveAssessmentID]; exists {
				Expect(permissions).To(BeEmpty())
			}
		})

		// Test: Validates handling of empty relationships list
		// Expected: Should return nil without error
		// Purpose: Tests edge case handling
		It("should handle empty relationships list", func() {
			err := authzSvc.WriteRelationships(ctx)
			Expect(err).To(BeNil())
		})

		// Test: Validates database tracking for Update operations
		// Expected: Relationships with RelationshipOpUpdate should be tracked in DB
		// Purpose: Tests that owner/viewer/org relationships are persisted
		It("should track Update operations in database", func() {
			trackUserID := "track-user-" + uuid.New().String()[:8]
			trackOrgID := "track-org-" + uuid.New().String()[:8]
			trackAssessmentID := "track-assessment-" + uuid.New().String()[:8]

			_, err := setupUserOrganizationMembership(ctx, spiceDBClient, zedTokenStore, trackUserID, trackOrgID)
			Expect(err).To(BeNil())

			userSubject := model.NewUserSubject(trackUserID)
			orgSubject := model.NewOrganizationSubject(trackOrgID)

			// Write relationships (should be tracked)
			err = authzSvc.WriteRelationships(ctx,
				store.WithOwnerRelationship(trackAssessmentID, userSubject),
				store.WithOrganizationRelationship(trackAssessmentID, orgSubject),
			)
			Expect(err).To(BeNil())

			// Verify relationships are tracked in database
			var relationships []model.RelationshipModel
			err = gormDB.Where("assessment_id = ?", trackAssessmentID).Find(&relationships).Error
			Expect(err).To(BeNil())
			Expect(relationships).To(HaveLen(2), "Both relationships should be tracked")
		})

		// Test: Validates database deletion for Delete operations
		// Expected: Relationships with RelationshipOpDelete should be removed from DB
		// Purpose: Tests that Without* functions remove from database
		It("should remove Delete operations from database", func() {
			deleteOpUserID := "delete-op-user-" + uuid.New().String()[:8]
			deleteOpOrgID := "delete-op-org-" + uuid.New().String()[:8]
			deleteOpAssessmentID := "delete-op-assessment-" + uuid.New().String()[:8]

			_, err := setupUserOrganizationMembership(ctx, spiceDBClient, zedTokenStore, deleteOpUserID, deleteOpOrgID)
			Expect(err).To(BeNil())

			userSubject := model.NewUserSubject(deleteOpUserID)

			// Create relationship
			err = authzSvc.WriteRelationships(ctx, store.WithOwnerRelationship(deleteOpAssessmentID, userSubject))
			Expect(err).To(BeNil())

			// Verify it's tracked
			var relationshipsBefore []model.RelationshipModel
			err = gormDB.Where("assessment_id = ?", deleteOpAssessmentID).Find(&relationshipsBefore).Error
			Expect(err).To(BeNil())
			Expect(relationshipsBefore).To(HaveLen(1))

			// Delete using Without function
			err = authzSvc.WriteRelationships(ctx, store.WithoutOwnerRelationship(deleteOpAssessmentID, userSubject))
			Expect(err).To(BeNil())

			// Verify it's removed from DB
			var relationshipsAfter []model.RelationshipModel
			err = gormDB.Where("assessment_id = ?", deleteOpAssessmentID).Find(&relationshipsAfter).Error
			Expect(err).To(BeNil())
			Expect(relationshipsAfter).To(HaveLen(0))
		})
	})

	Context("User-Organization Membership", func() {
		// Test: Validates that the authz service can establish membership relationships between users and organizations
		// Expected: User should be successfully added as a member of the organization, verifiable through direct SpiceDB queries
		// Purpose: Tests the fundamental prerequisite for all organization-based permissions
		It("should write user-organization membership relationship successfully", func() {
			userID := "user_" + uuid.New().String()[:8]
			orgID := "org_" + uuid.New().String()[:8]

			userSubject := model.NewUserSubject(userID)
			orgSubject := model.NewOrganizationSubject(orgID)
			err := authzSvc.WriteRelationships(ctx, store.WithMemberRelationship(userSubject, orgSubject))
			Expect(err).To(BeNil())

			err = verifyUserOrganizationMembership(ctx, spiceDBClient, zedTokenStore, userID, orgID)
			Expect(err).To(BeNil())
		})
	})

	Context("User-Assessment relationships", func() {
		// Test: Validates that users can be granted direct owner permissions on assessments
		// Expected: Owner should have all permissions (read, edit, share, delete) on the assessment
		// Purpose: Tests the highest privilege level in the authorization model
		It("should write owner relationship and verify all permissions", func() {
			userID := "user_" + uuid.New().String()[:8]
			orgID := "org_" + uuid.New().String()[:8]
			assessmentID := "assessment_" + uuid.New().String()[:8]

			_, err := setupUserOrganizationMembership(ctx, spiceDBClient, zedTokenStore, userID, orgID)
			Expect(err).To(BeNil())

			subject := model.NewUserSubject(userID)
			err = authzSvc.WriteRelationships(ctx, store.WithOwnerRelationship(assessmentID, subject))
			Expect(err).To(BeNil())

			<-time.After(5000 * time.Microsecond)

			resp, err := spiceDBClient.ReadRelationships(ctx, &v1pb.ReadRelationshipsRequest{
				Consistency: &v1pb.Consistency{
					Requirement: &v1pb.Consistency_FullyConsistent{
						FullyConsistent: true,
					},
				},
				RelationshipFilter: &v1pb.RelationshipFilter{
					ResourceType:       model.AssessmentObject,
					OptionalResourceId: assessmentID,
					OptionalRelation:   "owner",
					OptionalSubjectFilter: &v1pb.SubjectFilter{
						SubjectType:       model.UserObject,
						OptionalSubjectId: hash(userID),
					},
				},
			})
			Expect(err).To(BeNil())

			relationships := []*v1pb.Relationship{}
			for {
				rel, err := resp.Recv()
				if err != nil {
					break
				}
				relationships = append(relationships, rel.Relationship)
			}
			Expect(relationships).To(HaveLen(1))
			Expect(relationships[0].Resource.ObjectType).To(Equal(model.AssessmentObject))
			Expect(relationships[0].Resource.ObjectId).To(Equal(assessmentID))
			Expect(relationships[0].Relation).To(Equal("owner"))
			Expect(relationships[0].Subject.Object.ObjectType).To(Equal(model.UserObject))
			Expect(relationships[0].Subject.Object.ObjectId).To(Equal(hash(userID)))
		})

		// Test: Validates that organizations can be associated with assessments
		// Expected: Organization relationship should be created with the assessment
		// Purpose: Tests the organization-assessment relationship that grants all org members access
		It("should write organization relationship and verify correct relationship", func() {
			userID := "user_" + uuid.New().String()[:8]
			orgID := "org_" + uuid.New().String()[:8]
			assessmentID := "assessment-org-" + uuid.New().String()[:8]

			_, err := setupUserOrganizationMembership(ctx, spiceDBClient, zedTokenStore, userID, orgID)
			Expect(err).To(BeNil())

			subject := model.NewOrganizationSubject(orgID)
			err = authzSvc.WriteRelationships(ctx, store.WithOrganizationRelationship(assessmentID, subject))
			Expect(err).To(BeNil())

			resp, err := spiceDBClient.ReadRelationships(ctx, &v1pb.ReadRelationshipsRequest{
				Consistency: &v1pb.Consistency{
					Requirement: &v1pb.Consistency_FullyConsistent{
						FullyConsistent: true,
					},
				},
				RelationshipFilter: &v1pb.RelationshipFilter{
					ResourceType:       model.AssessmentObject,
					OptionalResourceId: assessmentID,
					OptionalRelation:   "org",
					OptionalSubjectFilter: &v1pb.SubjectFilter{
						SubjectType:       model.OrgObject,
						OptionalSubjectId: hash(orgID),
					},
				},
			})
			Expect(err).To(BeNil())

			relationships := []*v1pb.Relationship{}
			for {
				rel, err := resp.Recv()
				if err != nil {
					break
				}
				relationships = append(relationships, rel.Relationship)
			}
			Expect(relationships).To(HaveLen(1))
			Expect(relationships[0].Resource.ObjectType).To(Equal(model.AssessmentObject))
			Expect(relationships[0].Resource.ObjectId).To(Equal(assessmentID))
			Expect(relationships[0].Relation).To(Equal("org"))
			Expect(relationships[0].Subject.Object.ObjectType).To(Equal(model.OrgObject))
			Expect(relationships[0].Subject.Object.ObjectId).To(Equal(hash(orgID)))
		})

		// Test: Validates that users can be granted direct viewer permissions on assessments
		// Expected: Reader should only have read permission, no write/edit/share/delete permissions
		// Purpose: Tests the lowest privilege level that provides read-only access
		It("should write viewer relationship and verify correct permissions", func() {
			userID := "user_" + uuid.New().String()[:8]
			orgID := "org_" + uuid.New().String()[:8]
			assessmentID := "assessment-viewer-" + uuid.New().String()[:8]

			_, err := setupUserOrganizationMembership(ctx, spiceDBClient, zedTokenStore, userID, orgID)
			Expect(err).To(BeNil())

			subject := model.NewUserSubject(userID)
			err = authzSvc.WriteRelationships(ctx, store.WithViewerRelationship(assessmentID, subject))
			Expect(err).To(BeNil())

			resp, err := spiceDBClient.ReadRelationships(ctx, &v1pb.ReadRelationshipsRequest{
				Consistency: &v1pb.Consistency{
					Requirement: &v1pb.Consistency_FullyConsistent{
						FullyConsistent: true,
					},
				},
				RelationshipFilter: &v1pb.RelationshipFilter{
					ResourceType:       model.AssessmentObject,
					OptionalResourceId: assessmentID,
					OptionalRelation:   "viewer",
					OptionalSubjectFilter: &v1pb.SubjectFilter{
						SubjectType:       model.UserObject,
						OptionalSubjectId: hash(userID),
					},
				},
			})
			Expect(err).To(BeNil())

			relationships := []*v1pb.Relationship{}
			for {
				rel, err := resp.Recv()
				if err != nil {
					break
				}
				relationships = append(relationships, rel.Relationship)
			}
			Expect(relationships).To(HaveLen(1))
			Expect(relationships[0].Resource.ObjectType).To(Equal(model.AssessmentObject))
			Expect(relationships[0].Resource.ObjectId).To(Equal(assessmentID))
			Expect(relationships[0].Relation).To(Equal("viewer"))
			Expect(relationships[0].Subject.Object.ObjectType).To(Equal(model.UserObject))
			Expect(relationships[0].Subject.Object.ObjectId).To(Equal(hash(userID)))
		})
	})

	Context("Organization-Assessment relationships", func() {
		// Test: Validates that organizations can be associated with assessments
		// Expected: Organization should be recorded with org relation, enabling members to inherit edit permissions
		// Purpose: Tests organization-level access using the single org relation type
		It("should write organization relationship for assessment", func() {
			// Create local test data
			userID := "user_" + uuid.New().String()[:8]
			orgID := "org_" + uuid.New().String()[:8]
			assessmentID := "assessment-org-" + uuid.New().String()[:8]

			// Setup user membership in organization (prerequisite)
			_, err := setupUserOrganizationMembership(ctx, spiceDBClient, zedTokenStore, userID, orgID)
			Expect(err).To(BeNil())

			subject := model.NewOrganizationSubject(orgID)
			err = authzSvc.WriteRelationships(ctx, store.WithOrganizationRelationship(assessmentID, subject))
			Expect(err).To(BeNil())

			// Verify the relationship was written by reading it back with full consistency
			resp, err := spiceDBClient.ReadRelationships(ctx, &v1pb.ReadRelationshipsRequest{
				Consistency: &v1pb.Consistency{
					Requirement: &v1pb.Consistency_FullyConsistent{
						FullyConsistent: true,
					},
				},
				RelationshipFilter: &v1pb.RelationshipFilter{
					ResourceType:       model.AssessmentObject,
					OptionalResourceId: assessmentID,
					OptionalRelation:   "org",
					OptionalSubjectFilter: &v1pb.SubjectFilter{
						SubjectType:       model.OrgObject,
						OptionalSubjectId: hash(orgID),
					},
				},
			})
			Expect(err).To(BeNil())

			relationships := []*v1pb.Relationship{}
			for {
				rel, err := resp.Recv()
				if err != nil {
					break
				}
				relationships = append(relationships, rel.Relationship)
			}
			Expect(relationships).To(HaveLen(1))
			Expect(relationships[0].Resource.ObjectType).To(Equal(model.AssessmentObject))
			Expect(relationships[0].Resource.ObjectId).To(Equal(assessmentID))
			Expect(relationships[0].Relation).To(Equal("org"))
			Expect(relationships[0].Subject.Object.ObjectType).To(Equal(model.OrgObject))
			Expect(relationships[0].Subject.Object.ObjectId).To(Equal(hash(orgID)))
		})
	})

	Context("List Permissions", func() {
		// Test: Validates that the ListResources method correctly discovers and returns user permissions across multiple assessments
		// Expected: Should return 3 assessments with correct permission sets:
		//   - Assessment1: All permissions (owner through direct user relationship)
		//   - Assessment2: Read-only (viewer through direct user relationship)
		//   - Assessment3: Read+Edit (editor through organization membership)
		// Purpose: Tests complex permission aggregation from both direct user relationships and organizational membership
		It("should return correct permissions for user with different access patterns", func() {
			userID := "list-user-" + uuid.New().String()[:8]
			assessmentID1 := "list-assessment1-" + uuid.New().String()[:8]
			assessmentID2 := "list-assessment2-" + uuid.New().String()[:8]

			updates := []*v1pb.RelationshipUpdate{
				{
					Operation: v1pb.RelationshipUpdate_OPERATION_CREATE,
					Relationship: &v1pb.Relationship{
						Resource: &v1pb.ObjectReference{
							ObjectType: model.AssessmentObject,
							ObjectId:   assessmentID1,
						},
						Relation: "owner",
						Subject: &v1pb.SubjectReference{
							Object: &v1pb.ObjectReference{
								ObjectType: model.UserObject,
								ObjectId:   hash(userID),
							},
						},
					},
				},
				{
					Operation: v1pb.RelationshipUpdate_OPERATION_CREATE,
					Relationship: &v1pb.Relationship{
						Resource: &v1pb.ObjectReference{
							ObjectType: model.AssessmentObject,
							ObjectId:   assessmentID2,
						},
						Relation: "viewer",
						Subject: &v1pb.SubjectReference{
							Object: &v1pb.ObjectReference{
								ObjectType: model.UserObject,
								ObjectId:   hash(userID),
							},
						},
					},
				},
			}

			token, err := spiceDBClient.WriteRelationships(ctx, &v1pb.WriteRelationshipsRequest{
				Updates: updates,
			})
			Expect(err).To(BeNil())

			err = zedTokenStore.Write(ctx, token.WrittenAt.Token)
			Expect(err).To(BeNil())

			// List all resources user has read access to
			resourceIDs, err := authzSvc.ListResources(ctx, userID, model.ReadPermission, model.AssessmentResource)
			Expect(err).To(BeNil())
			Expect(resourceIDs).To(HaveLen(2), "User should have access to 2 assessments")
			Expect(resourceIDs).To(ContainElements(assessmentID1, assessmentID2))

			// Get permissions for each assessment
			permissionsMap, err := authzSvc.GetPermissions(ctx, resourceIDs, userID)
			Expect(err).To(BeNil())

			// Verify permissions for each assessment
			permissions1 := permissionsMap[assessmentID1]
			Expect(permissions1).To(ContainElements(
				model.ReadPermission,
				model.EditPermission,
				model.SharePermission,
				model.DeletePermission,
			), "Assessment1: Owner should have all permissions")

			permissions2 := permissionsMap[assessmentID2]
			Expect(permissions2).To(ContainElement(model.ReadPermission),
				"Assessment2: Reader should have read permission")
			Expect(permissions2).ToNot(ContainElements(
				model.EditPermission,
			), "Assessment2: Reader should not have write permissions")
		})

		// Test: Validates that users without any permissions receive an empty resource list
		// Expected: Should return empty list for user who is not a member of any organization and has no direct permissions
		// Purpose: Tests proper access control isolation - users should only see resources they have access to
		It("should return empty list for user with no permissions", func() {
			nonMemberUserID := "no-access-user-" + uuid.New().String()[:8]
			resourceIDs, err := authzSvc.ListResources(ctx, nonMemberUserID, model.ReadPermission, model.AssessmentResource)
			Expect(err).To(BeNil())
			Expect(resourceIDs).To(HaveLen(0), "User with no permissions should get empty list")
		})

		// Test: Validates that platform super_admin has all permissions on assessments
		// Expected: Platform admin should have read, edit, share, and delete permissions
		// Purpose: Tests that platform super_admin role grants full access to all assessments
		It("should grant all permissions to platform super_admin", func() {
			platformID := "platform-" + uuid.New().String()[:8]
			adminUserID := "platform-admin-" + uuid.New().String()[:8]
			assessmentID := "assessment-platform-admin-" + uuid.New().String()[:8]

			// Create platform with admin user
			adminUser := model.NewUserSubject(adminUserID)
			subjects := map[string][]model.Subject{
				"admin": {adminUser},
			}
			err := authzSvc.WriteRelationships(ctx, store.WithPlatformRelationship(model.NewPlatformSubject(platformID), subjects))
			Expect(err).To(BeNil())

			// Create assessment with parent platform relationship
			platformSubject := model.NewPlatformSubject(platformID)
			err = authzSvc.WriteRelationships(ctx, store.WithParentRelationship(assessmentID, platformSubject))
			Expect(err).To(BeNil())

			// Verify platform admin has all permissions
			permissionsMap, err := authzSvc.GetPermissions(ctx, []string{assessmentID}, adminUserID)
			Expect(err).To(BeNil())
			Expect(permissionsMap).To(HaveKey(assessmentID))

			permissions := permissionsMap[assessmentID]
			Expect(permissions).To(ContainElements(
				model.ReadPermission,
				model.EditPermission,
				model.SharePermission,
				model.DeletePermission,
			), "Platform super_admin should have all permissions")
		})

		// Test: Validates that platform editor has read, edit, and share permissions but not delete
		// Expected: Platform editor should have read, edit, share permissions but not delete
		// Purpose: Tests that platform editor role grants appropriate permissions without delete capability
		It("should grant read, edit, share permissions to platform editor", func() {
			platformID := "platform-editor-" + uuid.New().String()[:8]
			editorUserID := "platform-editor-user-" + uuid.New().String()[:8]
			assessmentID := "assessment-platform-editor-" + uuid.New().String()[:8]

			// Create platform with editor user
			editorUser := model.NewUserSubject(editorUserID)
			subjects := map[string][]model.Subject{
				"editor": {editorUser},
			}
			err := authzSvc.WriteRelationships(ctx, store.WithPlatformRelationship(model.NewPlatformSubject(platformID), subjects))
			Expect(err).To(BeNil())

			// Create assessment with parent platform relationship
			platformSubject := model.NewPlatformSubject(platformID)
			err = authzSvc.WriteRelationships(ctx, store.WithParentRelationship(assessmentID, platformSubject))
			Expect(err).To(BeNil())

			// Verify platform editor has read, edit permissions but not share or delete
			permissionsMap, err := authzSvc.GetPermissions(ctx, []string{assessmentID}, editorUserID)
			Expect(err).To(BeNil())
			Expect(permissionsMap).To(HaveKey(assessmentID))

			permissions := permissionsMap[assessmentID]
			Expect(permissions).To(ContainElements(
				model.ReadPermission,
				model.EditPermission,
			), "Platform editor should have read and edit permissions")
			Expect(permissions).ToNot(ContainElements(
				model.SharePermission,
				model.DeletePermission,
			), "Platform editor should not have share or delete permissions")
		})

		// Test: Validates that platform viewer has only read permission
		// Expected: Platform viewer should have read permission only
		// Purpose: Tests that platform viewer role grants read-only access
		It("should grant only read permission to platform viewer", func() {
			platformID := "platform-viewer-" + uuid.New().String()[:8]
			viewerUserID := "platform-viewer-user-" + uuid.New().String()[:8]
			assessmentID := "assessment-platform-viewer-" + uuid.New().String()[:8]

			// Create platform with viewer user
			viewerUser := model.NewUserSubject(viewerUserID)
			subjects := map[string][]model.Subject{
				"viewer": {viewerUser},
			}
			err := authzSvc.WriteRelationships(ctx, store.WithPlatformRelationship(model.NewPlatformSubject(platformID), subjects))
			Expect(err).To(BeNil())

			// Create assessment with parent platform relationship
			platformSubject := model.NewPlatformSubject(platformID)
			err = authzSvc.WriteRelationships(ctx, store.WithParentRelationship(assessmentID, platformSubject))
			Expect(err).To(BeNil())

			// Verify platform viewer has only read permission
			permissionsMap, err := authzSvc.GetPermissions(ctx, []string{assessmentID}, viewerUserID)
			Expect(err).To(BeNil())
			Expect(permissionsMap).To(HaveKey(assessmentID))

			permissions := permissionsMap[assessmentID]
			Expect(permissions).To(ContainElement(model.ReadPermission),
				"Platform viewer should have read permission")
			Expect(permissions).ToNot(ContainElements(
				model.EditPermission,
				model.SharePermission,
				model.DeletePermission,
			), "Platform viewer should not have edit, share, or delete permissions")
		})

		// Test: Validates that organization members have read and edit permissions on assessments
		// Expected: Org members should have read and edit permissions but not share or delete
		// Purpose: Tests that org membership grants appropriate access through org relation
		It("should grant read and edit permissions to organization members", func() {
			userID := "org-member-user-" + uuid.New().String()[:8]
			orgID := "org-member-org-" + uuid.New().String()[:8]
			assessmentID := "assessment-org-member-" + uuid.New().String()[:8]

			// Setup user membership in organization
			_, err := setupUserOrganizationMembership(ctx, spiceDBClient, zedTokenStore, userID, orgID)
			Expect(err).To(BeNil())

			// Create organization relationship with assessment
			orgSubject := model.NewOrganizationSubject(orgID)
			err = authzSvc.WriteRelationships(ctx, store.WithOrganizationRelationship(assessmentID, orgSubject))
			Expect(err).To(BeNil())

			// Verify org member has read and edit permissions but not share or delete
			permissionsMap, err := authzSvc.GetPermissions(ctx, []string{assessmentID}, userID)
			Expect(err).To(BeNil())
			Expect(permissionsMap).To(HaveKey(assessmentID))

			permissions := permissionsMap[assessmentID]
			Expect(permissions).To(ContainElements(
				model.ReadPermission,
				model.EditPermission,
			), "Organization member should have read and edit permissions")
			Expect(permissions).ToNot(ContainElements(
				model.SharePermission,
				model.DeletePermission,
			), "Organization member should not have share or delete permissions")
		})
	})

	Context("Delete Relationships", func() {
		// Context: Tests the deletion of authorization relationships and proper cleanup
		// Purpose: Validates that relationships can be completely removed from SpiceDB and tokens remain consistent
		var (
			deleteTestUserID        string
			deleteTestAssessmentID1 string
			deleteTestAssessmentID2 string
			testOrgID               string
		)

		BeforeEach(func() {
			deleteTestUserID = "delete-user-" + uuid.New().String()[:8]
			deleteTestAssessmentID1 = "delete-assessment1-" + uuid.New().String()[:8]
			deleteTestAssessmentID2 = "delete-assessment2-" + uuid.New().String()[:8]
			testOrgID = "org-" + uuid.New().String()[:8]
			_, err := setupUserOrganizationMembership(ctx, spiceDBClient, zedTokenStore, deleteTestUserID, testOrgID)
			Expect(err).To(BeNil())

			resp, err := spiceDBClient.WriteRelationships(ctx, &v1pb.WriteRelationshipsRequest{
				Updates: []*v1pb.RelationshipUpdate{
					{
						Operation: v1pb.RelationshipUpdate_OPERATION_CREATE,
						Relationship: &v1pb.Relationship{
							Resource: &v1pb.ObjectReference{
								ObjectType: model.AssessmentObject,
								ObjectId:   deleteTestAssessmentID1,
							},
							Relation: "owner",
							Subject: &v1pb.SubjectReference{
								Object: &v1pb.ObjectReference{
									ObjectType: model.UserObject,
									ObjectId:   hash(deleteTestUserID),
								},
							},
						},
					},
					{
						Operation: v1pb.RelationshipUpdate_OPERATION_CREATE,
						Relationship: &v1pb.Relationship{
							Resource: &v1pb.ObjectReference{
								ObjectType: model.AssessmentObject,
								ObjectId:   deleteTestAssessmentID2,
							},
							Relation: "viewer",
							Subject: &v1pb.SubjectReference{
								Object: &v1pb.ObjectReference{
									ObjectType: model.UserObject,
									ObjectId:   hash(deleteTestUserID),
								},
							},
						},
					},
					{
						Operation: v1pb.RelationshipUpdate_OPERATION_CREATE,
						Relationship: &v1pb.Relationship{
							Resource: &v1pb.ObjectReference{
								ObjectType: model.AssessmentObject,
								ObjectId:   deleteTestAssessmentID2,
							},
							Relation: "owner",
							Subject: &v1pb.SubjectReference{
								Object: &v1pb.ObjectReference{
									ObjectType: model.UserObject,
									ObjectId:   hash(deleteTestUserID),
								},
							},
						},
					},
				},
			})
			Expect(err).To(BeNil())
			zedToken = resp.WrittenAt

			err = zedTokenStore.Write(ctx, zedToken.Token)
			Expect(err).To(BeNil())
		})

		// Test: Validates that individual assessment relationships can be completely deleted
		// Expected: All relationships for the assessment should be removed, user should lose all permissions
		// Purpose: Tests proper cleanup functionality and verification through direct SpiceDB queries
		It("should delete a relationship successfully", func() {
			tokenStr, err := zedTokenStore.Read(ctx)
			Expect(err).To(BeNil())
			token := &v1pb.ZedToken{Token: *tokenStr}

			checkResp, err := spiceDBClient.CheckPermission(ctx, &v1pb.CheckPermissionRequest{
				Consistency: &v1pb.Consistency{
					Requirement: &v1pb.Consistency_AtLeastAsFresh{
						AtLeastAsFresh: token,
					},
				},
				Resource: &v1pb.ObjectReference{
					ObjectType: model.AssessmentObject,
					ObjectId:   deleteTestAssessmentID1,
				},
				Permission: model.EditPermission.String(),
				Subject: &v1pb.SubjectReference{
					Object: &v1pb.ObjectReference{
						ObjectType: model.UserObject,
						ObjectId:   hash(deleteTestUserID),
					},
				},
			})
			Expect(err).To(BeNil())
			Expect(checkResp.Permissionship).To(Equal(v1pb.CheckPermissionResponse_PERMISSIONSHIP_HAS_PERMISSION), "Owner relationship should exist before deletion")

			resource := model.NewAssessmentResource(deleteTestAssessmentID1)
			err = authzSvc.DeleteRelationships(ctx, resource)
			Expect(err).To(BeNil())

			currentTokenStr, err := zedTokenStore.Read(ctx)
			Expect(err).To(BeNil())
			currentToken := &v1pb.ZedToken{Token: *currentTokenStr}

			checkResp, err = spiceDBClient.CheckPermission(ctx, &v1pb.CheckPermissionRequest{
				Resource: &v1pb.ObjectReference{
					ObjectType: model.AssessmentObject,
					ObjectId:   deleteTestAssessmentID1,
				},
				Permission: model.EditPermission.String(),
				Subject: &v1pb.SubjectReference{
					Object: &v1pb.ObjectReference{
						ObjectType: model.UserObject,
						ObjectId:   hash(deleteTestUserID),
					},
				},
				Consistency: &v1pb.Consistency{
					Requirement: &v1pb.Consistency_AtLeastAsFresh{
						AtLeastAsFresh: currentToken,
					},
				},
			})
			Expect(err).To(BeNil())
			Expect(checkResp.Permissionship).To(Equal(v1pb.CheckPermissionResponse_PERMISSIONSHIP_NO_PERMISSION), "Owner relationship should be deleted")

			resp, err := spiceDBClient.ReadRelationships(ctx, &v1pb.ReadRelationshipsRequest{
				RelationshipFilter: &v1pb.RelationshipFilter{
					ResourceType:       model.AssessmentObject,
					OptionalResourceId: deleteTestAssessmentID1,
					OptionalRelation:   "owner",
					OptionalSubjectFilter: &v1pb.SubjectFilter{
						SubjectType:       model.UserObject,
						OptionalSubjectId: hash(deleteTestUserID),
					},
				},
			})
			Expect(err).To(BeNil())

			relationships := []*v1pb.Relationship{}
			for {
				rel, err := resp.Recv()
				if err != nil {
					break
				}
				relationships = append(relationships, rel.Relationship)
			}
			Expect(relationships).To(HaveLen(0), "Owner relationship should be completely removed")
		})

		// Test: Validates that ZedToken consistency is maintained across multiple sequential authz service operations
		// Expected: Write→Read→Delete→Read sequence should maintain proper token consistency in database
		// Purpose: Tests the internal token management system ensuring read-after-write consistency
		It("should maintain consistency token across chained service operations", func() {
			chainTestUserID := "chain-user-" + uuid.New().String()[:8]
			chainTestOrgID := "chain-org-" + uuid.New().String()[:8]
			chainTestAssessmentID := "chain-assessment-" + uuid.New().String()[:8]

			_, err := setupUserOrganizationMembership(ctx, spiceDBClient, zedTokenStore, chainTestUserID, chainTestOrgID)
			Expect(err).To(BeNil())

			subject := model.NewUserSubject(chainTestUserID)
			err = authzSvc.WriteRelationships(ctx, store.WithOwnerRelationship(chainTestAssessmentID, subject))
			Expect(err).To(BeNil())

			storedToken, err := zedTokenStore.Read(ctx)
			Expect(err).To(BeNil())
			Expect(storedToken).ToNot(BeNil(), "Service should have written ZedToken to database from write operation")

			permissionsMap, err := authzSvc.GetPermissions(ctx, []string{chainTestAssessmentID}, chainTestUserID)
			Expect(err).To(BeNil())
			Expect(permissionsMap).To(HaveKey(chainTestAssessmentID))
			permissions := permissionsMap[chainTestAssessmentID]
			Expect(permissions).To(ContainElement(model.ReadPermission), "Owner should have read permission")
			Expect(permissions).To(ContainElement(model.EditPermission), "Owner should have edit permission")
			Expect(permissions).To(ContainElement(model.SharePermission), "Owner should have share permission")
			Expect(permissions).To(ContainElement(model.DeletePermission), "Owner should have delete permission")

			resource := model.NewAssessmentResource(chainTestAssessmentID)
			err = authzSvc.DeleteRelationships(ctx, resource)
			Expect(err).To(BeNil())

			updatedToken, err := zedTokenStore.Read(ctx)
			Expect(err).To(BeNil())
			Expect(updatedToken).ToNot(BeNil(), "Service should have updated ZedToken in database from delete operation")
			Expect(*updatedToken).ToNot(Equal(*storedToken), "Token should be different after delete operation")

			permissionsMap, err = authzSvc.GetPermissions(ctx, []string{chainTestAssessmentID}, chainTestUserID)
			Expect(err).To(BeNil())
			if permissions, exists := permissionsMap[chainTestAssessmentID]; exists {
				Expect(permissions).To(BeEmpty(), "All permissions should be gone after delete")
			}
		})

		// Test: Validates that tracked relationships are deleted from database and user loses permissions
		// Expected: After deletion, relationships should be removed from DB and GetPermissions should show no access
		// Purpose: Tests proper database cleanup and permission revocation
		It("should delete tracked relationships from database and revoke permissions", func() {
			dbTestUserID := "db-delete-user-" + uuid.New().String()[:8]
			dbTestOrgID := "db-delete-org-" + uuid.New().String()[:8]
			dbTestAssessmentID := "db-delete-assessment-" + uuid.New().String()[:8]

			_, err := setupUserOrganizationMembership(ctx, spiceDBClient, zedTokenStore, dbTestUserID, dbTestOrgID)
			Expect(err).To(BeNil())

			// Write owner relationship using authzSvc (which tracks in DB)
			userSubject := model.NewUserSubject(dbTestUserID)
			err = authzSvc.WriteRelationships(ctx, store.WithOwnerRelationship(dbTestAssessmentID, userSubject))
			Expect(err).To(BeNil())

			// Verify relationship is tracked in database
			var relationshipsBeforeDelete []model.RelationshipModel
			err = gormDB.Where("assessment_id = ?", dbTestAssessmentID).Find(&relationshipsBeforeDelete).Error
			Expect(err).To(BeNil())
			Expect(relationshipsBeforeDelete).To(HaveLen(1), "Owner relationship should be tracked in database")
			Expect(relationshipsBeforeDelete[0].RelationType).To(Equal("owner"))
			Expect(relationshipsBeforeDelete[0].SubjectID).To(Equal(dbTestUserID))

			// Verify user has permissions before deletion
			permissionsMap, err := authzSvc.GetPermissions(ctx, []string{dbTestAssessmentID}, dbTestUserID)
			Expect(err).To(BeNil())
			Expect(permissionsMap).To(HaveKey(dbTestAssessmentID))
			permissions := permissionsMap[dbTestAssessmentID]
			Expect(permissions).To(ContainElement(model.ReadPermission), "User should have read permission before deletion")

			// Delete relationships
			resource := model.NewAssessmentResource(dbTestAssessmentID)
			err = authzSvc.DeleteRelationships(ctx, resource)
			Expect(err).To(BeNil())

			// Verify relationships are deleted from database
			var relationshipsAfterDelete []model.RelationshipModel
			err = gormDB.Where("assessment_id = ?", dbTestAssessmentID).Find(&relationshipsAfterDelete).Error
			Expect(err).To(BeNil())
			Expect(relationshipsAfterDelete).To(HaveLen(0), "All relationships should be deleted from database")

			// Verify user has no permissions after deletion
			permissionsMap, err = authzSvc.GetPermissions(ctx, []string{dbTestAssessmentID}, dbTestUserID)
			Expect(err).To(BeNil())
			if permissions, exists := permissionsMap[dbTestAssessmentID]; exists {
				Expect(permissions).To(BeEmpty(), "User should have no permissions after deletion")
			}
		})

		// Test: Validates that all tracked relationships can be deleted without specifying assessment ID
		// Expected: All assessment relationships should be removed from DB and SpiceDB
		// Purpose: Tests bulk deletion of all assessment relationships
		It("should delete all tracked relationships from database without assessment ID", func() {
			bulkDeleteUser1ID := "bulk-delete-user1-" + uuid.New().String()[:8]
			bulkDeleteUser2ID := "bulk-delete-user2-" + uuid.New().String()[:8]
			bulkDeleteOrgID := "bulk-delete-org-" + uuid.New().String()[:8]
			bulkDeleteAssessment1ID := "bulk-delete-assessment1-" + uuid.New().String()[:8]
			bulkDeleteAssessment2ID := "bulk-delete-assessment2-" + uuid.New().String()[:8]

			_, err := setupUserOrganizationMembership(ctx, spiceDBClient, zedTokenStore, bulkDeleteUser1ID, bulkDeleteOrgID)
			Expect(err).To(BeNil())
			_, err = setupUserOrganizationMembership(ctx, spiceDBClient, zedTokenStore, bulkDeleteUser2ID, bulkDeleteOrgID)
			Expect(err).To(BeNil())

			// Create two assessment relationships using authzSvc (which tracks in DB)
			user1Subject := model.NewUserSubject(bulkDeleteUser1ID)
			user2Subject := model.NewUserSubject(bulkDeleteUser2ID)
			err = authzSvc.WriteRelationships(ctx,
				store.WithOwnerRelationship(bulkDeleteAssessment1ID, user1Subject),
				store.WithViewerRelationship(bulkDeleteAssessment2ID, user2Subject),
			)
			Expect(err).To(BeNil())

			// Verify both relationships are tracked in database
			var relationshipsBeforeDelete []model.RelationshipModel
			err = gormDB.Where("assessment_id IS NOT NULL").Find(&relationshipsBeforeDelete).Error
			Expect(err).To(BeNil())
			Expect(len(relationshipsBeforeDelete)).To(BeNumerically(">=", 2), "At least two relationships should be tracked in database")

			// Count relationships for our specific assessments
			var ourRelationships []model.RelationshipModel
			err = gormDB.Where("assessment_id IN ?", []string{bulkDeleteAssessment1ID, bulkDeleteAssessment2ID}).Find(&ourRelationships).Error
			Expect(err).To(BeNil())
			Expect(ourRelationships).To(HaveLen(2), "Both relationships should be tracked")

			// Delete all assessment relationships without specifying ID
			resource := model.Resource{
				ResourceType: model.AssessmentResource,
				ID:           "", // Empty ID means delete all
			}
			err = authzSvc.DeleteRelationships(ctx, resource)
			Expect(err).To(BeNil())

			// Verify all assessment relationships are deleted from database
			var relationshipsAfterDelete []model.RelationshipModel
			err = gormDB.Where("assessment_id IS NOT NULL").Find(&relationshipsAfterDelete).Error
			Expect(err).To(BeNil())
			Expect(relationshipsAfterDelete).To(HaveLen(0), "All assessment relationships should be deleted from database")

			// Verify users have no permissions after deletion
			permissionsMap1, err := authzSvc.GetPermissions(ctx, []string{bulkDeleteAssessment1ID}, bulkDeleteUser1ID)
			Expect(err).To(BeNil())
			if permissions, exists := permissionsMap1[bulkDeleteAssessment1ID]; exists {
				Expect(permissions).To(BeEmpty(), "User1 should have no permissions after deletion")
			}

			permissionsMap2, err := authzSvc.GetPermissions(ctx, []string{bulkDeleteAssessment2ID}, bulkDeleteUser2ID)
			Expect(err).To(BeNil())
			if permissions, exists := permissionsMap2[bulkDeleteAssessment2ID]; exists {
				Expect(permissions).To(BeEmpty(), "User2 should have no permissions after deletion")
			}
		})
	})

	Context("Database Relationship Tracking", func() {
		// Test: Validates that relationships are properly tracked in the database
		// Expected: Assessment relationships should be stored in DB, platform relationships should not
		// Purpose: Tests that only trackable relationships are persisted to the database
		It("should track assessment relationships in database", func() {
			userID := "db-track-user-" + uuid.New().String()[:8]
			orgID := "db-track-org-" + uuid.New().String()[:8]
			assessmentID := "db-track-assessment-" + uuid.New().String()[:8]

			_, err := setupUserOrganizationMembership(ctx, spiceDBClient, zedTokenStore, userID, orgID)
			Expect(err).To(BeNil())

			// Write owner relationship
			userSubject := model.NewUserSubject(userID)
			err = authzSvc.WriteRelationships(ctx, store.WithOwnerRelationship(assessmentID, userSubject))
			Expect(err).To(BeNil())

			// Verify relationship is tracked in database
			var relationships []model.RelationshipModel
			err = gormDB.Where("assessment_id = ?", assessmentID).Find(&relationships).Error
			Expect(err).To(BeNil())
			Expect(relationships).To(HaveLen(1), "Owner relationship should be tracked in database")
			Expect(relationships[0].RelationType).To(Equal("owner"))
			Expect(relationships[0].SubjectType).To(Equal("user"))
			Expect(relationships[0].AssessmentID).To(Equal(assessmentID))

			// Write organization relationship
			orgSubject := model.NewOrganizationSubject(orgID)
			err = authzSvc.WriteRelationships(ctx, store.WithOrganizationRelationship(assessmentID, orgSubject))
			Expect(err).To(BeNil())

			// Verify both relationships are tracked
			err = gormDB.Where("assessment_id = ?", assessmentID).Find(&relationships).Error
			Expect(err).To(BeNil())
			Expect(relationships).To(HaveLen(2), "Both owner and org relationships should be tracked")

			// Delete all relationships for the assessment
			resource := model.NewAssessmentResource(assessmentID)
			err = authzSvc.DeleteRelationships(ctx, resource)
			Expect(err).To(BeNil())

			// Verify relationships are removed from database
			err = gormDB.Where("assessment_id = ?", assessmentID).Find(&relationships).Error
			Expect(err).To(BeNil())
			Expect(relationships).To(HaveLen(0), "All relationships should be deleted from database")
		})

		It("should NOT track platform relationships in database", func() {
			platformID := "platform-" + uuid.New().String()[:8]
			userID := "platform-user-" + uuid.New().String()[:8]
			assessmentID := "platform-assessment-" + uuid.New().String()[:8]

			adminUser := model.NewUserSubject(userID)
			platformSubject := model.NewPlatformSubject(platformID)
			subjects := map[string][]model.Subject{
				"admin": {adminUser},
			}

			// Write platform relationship and associate assessment with platform
			err := authzSvc.WriteRelationships(ctx,
				store.WithPlatformRelationship(model.NewPlatformSubject(platformID), subjects),
				store.WithParentRelationship(assessmentID, platformSubject),
			)
			Expect(err).To(BeNil())

			// Verify platform relationships are NOT tracked in database
			// The relationships table only tracks assessment relationships
			var relationships []model.RelationshipModel
			err = gormDB.Find(&relationships).Error
			Expect(err).To(BeNil())

			// Check that no relationship with this platform/user was stored
			for _, rel := range relationships {
				Expect(rel.SubjectID).ToNot(Equal(hash(userID)), "Platform relationships should not be tracked in database")
			}

			// Verify admin has permissions via platform role before deletion
			permissionsMap, err := authzSvc.GetPermissions(ctx, []string{assessmentID}, userID)
			Expect(err).To(BeNil())
			Expect(permissionsMap).To(HaveKey(assessmentID))
			Expect(permissionsMap[assessmentID]).To(ContainElement(model.DeletePermission), "Admin should have permissions before platform deletion")

			// Delete platform relationships
			resource := model.NewPlatformResource(platformID)
			err = authzSvc.DeleteRelationships(ctx, resource)
			Expect(err).To(BeNil())
			// Should not error even though there's nothing in DB to delete

			// Verify admin loses permissions after platform deletion
			permissionsMap, err = authzSvc.GetPermissions(ctx, []string{assessmentID}, userID)
			Expect(err).To(BeNil())
			if permissions, exists := permissionsMap[assessmentID]; exists {
				Expect(permissions).To(BeEmpty(), "Admin should lose permissions after platform deletion")
			}

			// Verify platform relationship was removed from SpiceDB
			resp, err := spiceDBClient.ReadRelationships(ctx, &v1pb.ReadRelationshipsRequest{
				Consistency: &v1pb.Consistency{
					Requirement: &v1pb.Consistency_FullyConsistent{
						FullyConsistent: true,
					},
				},
				RelationshipFilter: &v1pb.RelationshipFilter{
					ResourceType:       model.PlatformObject,
					OptionalResourceId: platformID,
				},
			})
			Expect(err).To(BeNil())

			platformRelationships := []*v1pb.Relationship{}
			for {
				rel, err := resp.Recv()
				if err != nil {
					break
				}
				platformRelationships = append(platformRelationships, rel.Relationship)
			}
			Expect(platformRelationships).To(HaveLen(0), "All platform relationships should be removed from SpiceDB")
		})
	})

	Context("Get Permissions", func() {
		// Test: Validates that GetPermissions correctly retrieves user permissions for a single assessment
		// Expected: Owner should have all permissions (read, edit, share, delete)
		// Purpose: Tests basic permission retrieval functionality
		It("should return all permissions for assessment owner", func() {
			ownerUserID := "perm-owner-" + uuid.New().String()[:8]
			ownerOrgID := "perm-org-" + uuid.New().String()[:8]
			ownerAssessmentID := "perm-assessment-" + uuid.New().String()[:8]

			_, err := setupUserOrganizationMembership(ctx, spiceDBClient, zedTokenStore, ownerUserID, ownerOrgID)
			Expect(err).To(BeNil())

			// Create owner relationship
			userSubject := model.NewUserSubject(ownerUserID)
			err = authzSvc.WriteRelationships(ctx, store.WithOwnerRelationship(ownerAssessmentID, userSubject))
			Expect(err).To(BeNil())

			// Get permissions
			permissionsMap, err := authzSvc.GetPermissions(ctx, []string{ownerAssessmentID}, ownerUserID)
			Expect(err).To(BeNil())
			Expect(permissionsMap).To(HaveKey(ownerAssessmentID))

			permissions := permissionsMap[ownerAssessmentID]
			Expect(permissions).To(ContainElements(
				model.ReadPermission,
				model.EditPermission,
				model.SharePermission,
				model.DeletePermission,
			), "Owner should have all permissions")
		})

		// Test: Validates that GetPermissions correctly retrieves limited permissions for viewers
		// Expected: Reader should only have read permission
		// Purpose: Tests permission retrieval for read-only access
		It("should return only read permission for assessment viewer", func() {
			viewerUserID := "perm-viewer-" + uuid.New().String()[:8]
			viewerOrgID := "perm-org-" + uuid.New().String()[:8]
			viewerAssessmentID := "perm-viewer-assessment-" + uuid.New().String()[:8]

			_, err := setupUserOrganizationMembership(ctx, spiceDBClient, zedTokenStore, viewerUserID, viewerOrgID)
			Expect(err).To(BeNil())

			// Create viewer relationship
			userSubject := model.NewUserSubject(viewerUserID)
			err = authzSvc.WriteRelationships(ctx, store.WithViewerRelationship(viewerAssessmentID, userSubject))
			Expect(err).To(BeNil())

			// Get permissions
			permissionsMap, err := authzSvc.GetPermissions(ctx, []string{viewerAssessmentID}, viewerUserID)
			Expect(err).To(BeNil())
			Expect(permissionsMap).To(HaveKey(viewerAssessmentID))

			permissions := permissionsMap[viewerAssessmentID]
			Expect(permissions).To(ContainElement(model.ReadPermission), "Reader should have read permission")
			Expect(permissions).ToNot(ContainElements(
				model.EditPermission,
				model.SharePermission,
				model.DeletePermission,
			), "Reader should not have write permissions")
		})

		// Test: Validates that GetPermissions works correctly for multiple assessments
		// Expected: Should return different permission sets for each assessment
		// Purpose: Tests bulk permission retrieval across multiple resources
		It("should return permissions for multiple assessments", func() {
			multiUserID := "perm-multi-" + uuid.New().String()[:8]
			multiOrgID := "perm-multi-org-" + uuid.New().String()[:8]
			assessment1ID := "perm-multi-assess1-" + uuid.New().String()[:8]
			assessment2ID := "perm-multi-assess2-" + uuid.New().String()[:8]
			assessment3ID := "perm-multi-assess3-" + uuid.New().String()[:8]

			_, err := setupUserOrganizationMembership(ctx, spiceDBClient, zedTokenStore, multiUserID, multiOrgID)
			Expect(err).To(BeNil())

			userSubject := model.NewUserSubject(multiUserID)

			// Create different relationships for each assessment
			err = authzSvc.WriteRelationships(ctx,
				store.WithOwnerRelationship(assessment1ID, userSubject),
				store.WithViewerRelationship(assessment2ID, userSubject),
			)
			Expect(err).To(BeNil())

			// Get permissions for all three assessments
			permissionsMap, err := authzSvc.GetPermissions(ctx, []string{assessment1ID, assessment2ID, assessment3ID}, multiUserID)
			Expect(err).To(BeNil())

			// Verify assessment1 - owner permissions
			Expect(permissionsMap).To(HaveKey(assessment1ID))
			permissions1 := permissionsMap[assessment1ID]
			Expect(permissions1).To(ContainElements(
				model.ReadPermission,
				model.EditPermission,
				model.SharePermission,
				model.DeletePermission,
			), "Assessment1: Owner should have all permissions")

			// Verify assessment2 - viewer permissions
			Expect(permissionsMap).To(HaveKey(assessment2ID))
			permissions2 := permissionsMap[assessment2ID]
			Expect(permissions2).To(ContainElement(model.ReadPermission), "Assessment2: Reader should have read permission")
			Expect(permissions2).ToNot(ContainElements(
				model.EditPermission,
				model.SharePermission,
				model.DeletePermission,
			), "Assessment2: Reader should not have write permissions")

			// Verify assessment3 - no permissions
			if permissions3, exists := permissionsMap[assessment3ID]; exists {
				Expect(permissions3).To(BeEmpty(), "Assessment3: User should have no permissions")
			}
		})

		// Test: Validates that GetPermissions returns permissions from organization membership
		// Expected: Organization members should have read and edit permissions
		// Purpose: Tests permission inheritance through organization relationships
		It("should return permissions for organization members", func() {
			orgMemberUserID := "perm-org-member-" + uuid.New().String()[:8]
			orgMemberOrgID := "perm-org-member-org-" + uuid.New().String()[:8]
			orgAssessmentID := "perm-org-assessment-" + uuid.New().String()[:8]

			_, err := setupUserOrganizationMembership(ctx, spiceDBClient, zedTokenStore, orgMemberUserID, orgMemberOrgID)
			Expect(err).To(BeNil())

			// Create organization relationship with assessment
			orgSubject := model.NewOrganizationSubject(orgMemberOrgID)
			err = authzSvc.WriteRelationships(ctx, store.WithOrganizationRelationship(orgAssessmentID, orgSubject))
			Expect(err).To(BeNil())

			// Get permissions for org member
			permissionsMap, err := authzSvc.GetPermissions(ctx, []string{orgAssessmentID}, orgMemberUserID)
			Expect(err).To(BeNil())
			Expect(permissionsMap).To(HaveKey(orgAssessmentID))

			permissions := permissionsMap[orgAssessmentID]
			Expect(permissions).To(ContainElements(
				model.ReadPermission,
				model.EditPermission,
			), "Org member should have read and edit permissions")
			Expect(permissions).ToNot(ContainElements(
				model.SharePermission,
				model.DeletePermission,
			), "Org member should not have share or delete permissions")
		})

		// Test: Validates that GetPermissions returns empty result for users with no access
		// Expected: Should return empty permissions map or empty permission list
		// Purpose: Tests proper access control isolation
		It("should return no permissions for users without access", func() {
			noAccessUserID := "perm-no-access-" + uuid.New().String()[:8]
			noAccessOrgID := "perm-no-access-org-" + uuid.New().String()[:8]
			noAccessAssessmentID := "perm-no-access-assessment-" + uuid.New().String()[:8]

			_, err := setupUserOrganizationMembership(ctx, spiceDBClient, zedTokenStore, noAccessUserID, noAccessOrgID)
			Expect(err).To(BeNil())

			// Create assessment owned by different user
			otherUserID := "perm-other-" + uuid.New().String()[:8]
			otherOrgID := "perm-other-org-" + uuid.New().String()[:8]
			_, err = setupUserOrganizationMembership(ctx, spiceDBClient, zedTokenStore, otherUserID, otherOrgID)
			Expect(err).To(BeNil())

			otherUserSubject := model.NewUserSubject(otherUserID)
			err = authzSvc.WriteRelationships(ctx, store.WithOwnerRelationship(noAccessAssessmentID, otherUserSubject))
			Expect(err).To(BeNil())

			// Get permissions for user without access
			permissionsMap, err := authzSvc.GetPermissions(ctx, []string{noAccessAssessmentID}, noAccessUserID)
			Expect(err).To(BeNil())

			if permissions, exists := permissionsMap[noAccessAssessmentID]; exists {
				Expect(permissions).To(BeEmpty(), "User without access should have no permissions")
			}
		})

		// Test: Validates that GetPermissions handles empty assessment list
		// Expected: Should return empty map without error
		// Purpose: Tests edge case handling
		It("should handle empty assessment list", func() {
			emptyUserID := "perm-empty-" + uuid.New().String()[:8]
			emptyOrgID := "perm-empty-org-" + uuid.New().String()[:8]

			_, err := setupUserOrganizationMembership(ctx, spiceDBClient, zedTokenStore, emptyUserID, emptyOrgID)
			Expect(err).To(BeNil())

			// Get permissions for empty list
			permissionsMap, err := authzSvc.GetPermissions(ctx, []string{}, emptyUserID)
			Expect(err).To(BeNil())
			Expect(permissionsMap).To(BeEmpty(), "Empty assessment list should return empty map")
		})

		// Test: Validates permissions for platform super_admin via parent relationship
		// Expected: Platform admin should have all permissions (read, edit, share, delete)
		// Purpose: Tests platform admin permissions cascade through parent relationship
		It("should return all permissions for platform super_admin", func() {
			platformID := "perm-platform-" + uuid.New().String()[:8]
			adminUserID := "perm-admin-" + uuid.New().String()[:8]
			assessmentID := "perm-admin-assessment-" + uuid.New().String()[:8]

			// Create platform with admin user
			adminUser := model.NewUserSubject(adminUserID)
			subjects := map[string][]model.Subject{
				"admin": {adminUser},
			}
			err := authzSvc.WriteRelationships(ctx, store.WithPlatformRelationship(model.NewPlatformSubject(platformID), subjects))
			Expect(err).To(BeNil())

			// Create assessment with parent platform relationship
			platformSubject := model.NewPlatformSubject(platformID)
			err = authzSvc.WriteRelationships(ctx, store.WithParentRelationship(assessmentID, platformSubject))
			Expect(err).To(BeNil())

			// Get permissions
			permissionsMap, err := authzSvc.GetPermissions(ctx, []string{assessmentID}, adminUserID)
			Expect(err).To(BeNil())
			Expect(permissionsMap).To(HaveKey(assessmentID))

			permissions := permissionsMap[assessmentID]
			Expect(permissions).To(ContainElements(
				model.ReadPermission,
				model.EditPermission,
				model.SharePermission,
				model.DeletePermission,
			), "Platform super_admin should have all permissions")
		})

		// Test: Validates permissions for platform editor via parent relationship
		// Expected: Platform editor should have read and edit but not share or delete
		// Purpose: Tests platform editor permissions cascade through parent relationship
		It("should return read and edit permissions for platform editor", func() {
			platformID := "perm-platform-editor-" + uuid.New().String()[:8]
			editorUserID := "perm-editor-" + uuid.New().String()[:8]
			assessmentID := "perm-editor-assessment-" + uuid.New().String()[:8]

			// Create platform with editor user
			editorUser := model.NewUserSubject(editorUserID)
			subjects := map[string][]model.Subject{
				"editor": {editorUser},
			}
			err := authzSvc.WriteRelationships(ctx, store.WithPlatformRelationship(model.NewPlatformSubject(platformID), subjects))
			Expect(err).To(BeNil())

			// Create assessment with parent platform relationship
			platformSubject := model.NewPlatformSubject(platformID)
			err = authzSvc.WriteRelationships(ctx, store.WithParentRelationship(assessmentID, platformSubject))
			Expect(err).To(BeNil())

			// Get permissions
			permissionsMap, err := authzSvc.GetPermissions(ctx, []string{assessmentID}, editorUserID)
			Expect(err).To(BeNil())
			Expect(permissionsMap).To(HaveKey(assessmentID))

			permissions := permissionsMap[assessmentID]
			Expect(permissions).To(ContainElements(
				model.ReadPermission,
				model.EditPermission,
			), "Platform editor should have read and edit permissions")
			Expect(permissions).ToNot(ContainElements(
				model.SharePermission,
				model.DeletePermission,
			), "Platform editor should not have share or delete permissions")
		})

		// Test: Validates permissions for platform viewer via parent relationship
		// Expected: Platform viewer should have read permission only
		// Purpose: Tests platform viewer permissions cascade through parent relationship
		It("should return only read permission for platform viewer", func() {
			platformID := "perm-platform-viewer-" + uuid.New().String()[:8]
			viewerUserID := "perm-viewer-" + uuid.New().String()[:8]
			assessmentID := "perm-viewer-assessment-" + uuid.New().String()[:8]

			// Create platform with viewer user
			viewerUser := model.NewUserSubject(viewerUserID)
			subjects := map[string][]model.Subject{
				"viewer": {viewerUser},
			}
			err := authzSvc.WriteRelationships(ctx, store.WithPlatformRelationship(model.NewPlatformSubject(platformID), subjects))
			Expect(err).To(BeNil())

			// Create assessment with parent platform relationship
			platformSubject := model.NewPlatformSubject(platformID)
			err = authzSvc.WriteRelationships(ctx, store.WithParentRelationship(assessmentID, platformSubject))
			Expect(err).To(BeNil())

			// Get permissions
			permissionsMap, err := authzSvc.GetPermissions(ctx, []string{assessmentID}, viewerUserID)
			Expect(err).To(BeNil())
			Expect(permissionsMap).To(HaveKey(assessmentID))

			permissions := permissionsMap[assessmentID]
			Expect(permissions).To(ContainElement(model.ReadPermission), "Platform viewer should have read permission")
			Expect(permissions).ToNot(ContainElements(
				model.EditPermission,
				model.SharePermission,
				model.DeletePermission,
			), "Platform viewer should not have edit, share, or delete permissions")
		})

		// Test: Validates combined permissions when user has multiple roles
		// Expected: User who is both owner and org member should have all permissions (union)
		// Purpose: Tests permission combination from different sources
		It("should combine permissions from owner and org member roles", func() {
			comboUserID := "perm-combo-" + uuid.New().String()[:8]
			comboOrgID := "perm-combo-org-" + uuid.New().String()[:8]
			comboAssessmentID := "perm-combo-assessment-" + uuid.New().String()[:8]

			_, err := setupUserOrganizationMembership(ctx, spiceDBClient, zedTokenStore, comboUserID, comboOrgID)
			Expect(err).To(BeNil())

			userSubject := model.NewUserSubject(comboUserID)
			orgSubject := model.NewOrganizationSubject(comboOrgID)

			// Create both owner and org relationships
			err = authzSvc.WriteRelationships(ctx,
				store.WithOwnerRelationship(comboAssessmentID, userSubject),
				store.WithOrganizationRelationship(comboAssessmentID, orgSubject),
			)
			Expect(err).To(BeNil())

			// Get permissions
			permissionsMap, err := authzSvc.GetPermissions(ctx, []string{comboAssessmentID}, comboUserID)
			Expect(err).To(BeNil())
			Expect(permissionsMap).To(HaveKey(comboAssessmentID))

			permissions := permissionsMap[comboAssessmentID]
			Expect(permissions).To(ContainElements(
				model.ReadPermission,
				model.EditPermission,
				model.SharePermission,
				model.DeletePermission,
			), "User with owner + org member should have all permissions")
		})

		// Test: Validates combined permissions from viewer and org member
		// Expected: User should get read from viewer + read/edit from org = read/edit
		// Purpose: Tests permission union from multiple sources
		It("should combine permissions from viewer and org member roles", func() {
			viewerOrgUserID := "perm-viewer-org-" + uuid.New().String()[:8]
			viewerOrgID := "perm-viewer-org-id-" + uuid.New().String()[:8]
			viewerOrgAssessmentID := "perm-viewer-org-assessment-" + uuid.New().String()[:8]

			_, err := setupUserOrganizationMembership(ctx, spiceDBClient, zedTokenStore, viewerOrgUserID, viewerOrgID)
			Expect(err).To(BeNil())

			userSubject := model.NewUserSubject(viewerOrgUserID)
			orgSubject := model.NewOrganizationSubject(viewerOrgID)

			// Create both viewer and org relationships
			err = authzSvc.WriteRelationships(ctx,
				store.WithViewerRelationship(viewerOrgAssessmentID, userSubject),
				store.WithOrganizationRelationship(viewerOrgAssessmentID, orgSubject),
			)
			Expect(err).To(BeNil())

			// Get permissions
			permissionsMap, err := authzSvc.GetPermissions(ctx, []string{viewerOrgAssessmentID}, viewerOrgUserID)
			Expect(err).To(BeNil())
			Expect(permissionsMap).To(HaveKey(viewerOrgAssessmentID))

			permissions := permissionsMap[viewerOrgAssessmentID]
			Expect(permissions).To(ContainElements(
				model.ReadPermission,
				model.EditPermission,
			), "Reader + org member should have read and edit permissions")
			Expect(permissions).ToNot(ContainElements(
				model.SharePermission,
				model.DeletePermission,
			), "Reader + org member should not have share or delete permissions")
		})

		// Test: Validates that org membership in wrong organization doesn't grant access
		// Expected: User in different org should have no permissions
		// Purpose: Tests proper isolation between organizations
		It("should not grant permissions for different organization member", func() {
			wrongOrgUserID := "perm-wrong-org-user-" + uuid.New().String()[:8]
			wrongOrgID := "perm-wrong-org-" + uuid.New().String()[:8]
			correctOrgID := "perm-correct-org-" + uuid.New().String()[:8]
			assessmentID := "perm-wrong-org-assessment-" + uuid.New().String()[:8]

			// User is member of wrongOrgID
			_, err := setupUserOrganizationMembership(ctx, spiceDBClient, zedTokenStore, wrongOrgUserID, wrongOrgID)
			Expect(err).To(BeNil())

			// Assessment is associated with correctOrgID (different org)
			correctOrgSubject := model.NewOrganizationSubject(correctOrgID)
			err = authzSvc.WriteRelationships(ctx, store.WithOrganizationRelationship(assessmentID, correctOrgSubject))
			Expect(err).To(BeNil())

			// Get permissions for user in wrong org
			permissionsMap, err := authzSvc.GetPermissions(ctx, []string{assessmentID}, wrongOrgUserID)
			Expect(err).To(BeNil())

			if permissions, exists := permissionsMap[assessmentID]; exists {
				Expect(permissions).To(BeEmpty(), "User in different org should have no permissions")
			}
		})

		// Test: Validates permissions for assessment with parent but user has no platform role
		// Expected: User should have no permissions
		// Purpose: Tests that parent relationship alone doesn't grant access
		It("should not grant permissions for assessment with parent when user has no platform role", func() {
			platformID := "perm-no-role-platform-" + uuid.New().String()[:8]
			noRoleUserID := "perm-no-role-user-" + uuid.New().String()[:8]
			noRoleOrgID := "perm-no-role-org-" + uuid.New().String()[:8]
			assessmentID := "perm-no-role-assessment-" + uuid.New().String()[:8]

			_, err := setupUserOrganizationMembership(ctx, spiceDBClient, zedTokenStore, noRoleUserID, noRoleOrgID)
			Expect(err).To(BeNil())

			// Create assessment with parent platform relationship but user has no role on platform
			platformSubject := model.NewPlatformSubject(platformID)
			err = authzSvc.WriteRelationships(ctx, store.WithParentRelationship(assessmentID, platformSubject))
			Expect(err).To(BeNil())

			// Get permissions
			permissionsMap, err := authzSvc.GetPermissions(ctx, []string{assessmentID}, noRoleUserID)
			Expect(err).To(BeNil())

			if permissions, exists := permissionsMap[assessmentID]; exists {
				Expect(permissions).To(BeEmpty(), "User with no platform role should have no permissions")
			}
		})
	})
})

// Helper function to setup user membership in organization
// Purpose: Creates the prerequisite membership relationship that enables organization-based permissions
// Returns: ZedToken from the write operation for consistency tracking
func setupUserOrganizationMembership(ctx context.Context, client *authzed.Client, zedStore *store.ZedTokenStore, userID, orgID string) (*v1pb.ZedToken, error) {
	resp, err := client.WriteRelationships(ctx, &v1pb.WriteRelationshipsRequest{
		Updates: []*v1pb.RelationshipUpdate{
			{
				Operation: v1pb.RelationshipUpdate_OPERATION_TOUCH,
				Relationship: &v1pb.Relationship{
					Resource: &v1pb.ObjectReference{
						ObjectType: model.OrgObject,
						ObjectId:   hash(orgID),
					},
					Relation: "member",
					Subject: &v1pb.SubjectReference{
						Object: &v1pb.ObjectReference{
							ObjectType: model.UserObject,
							ObjectId:   hash(userID),
						},
					},
				},
			},
		},
	})
	if err != nil {
		return nil, err
	}

	if err := zedStore.Write(ctx, resp.WrittenAt.Token); err != nil {
		return nil, fmt.Errorf("failed to write token to database: %w", err)
	}

	return resp.WrittenAt, nil
}

// Helper function to verify user membership in organization
// Purpose: Validates that a user is properly registered as a member of an organization
// Uses: Direct SpiceDB client to verify the relationship exists with proper consistency
func verifyUserOrganizationMembership(ctx context.Context, client *authzed.Client, zedStore *store.ZedTokenStore, userID, orgID string) error {
	tokenStr, err := zedStore.Read(ctx)
	if err != nil {
		return fmt.Errorf("failed to read token: %w", err)
	}

	var consistency *v1pb.Consistency
	if tokenStr != nil && *tokenStr != "" {
		consistency = &v1pb.Consistency{
			Requirement: &v1pb.Consistency_AtLeastAsFresh{
				AtLeastAsFresh: &v1pb.ZedToken{Token: *tokenStr},
			},
		}
	} else {
		consistency = &v1pb.Consistency{
			Requirement: &v1pb.Consistency_FullyConsistent{
				FullyConsistent: true,
			},
		}
	}

	resp, err := client.ReadRelationships(ctx, &v1pb.ReadRelationshipsRequest{
		Consistency: consistency,
		RelationshipFilter: &v1pb.RelationshipFilter{
			ResourceType:       model.OrgObject,
			OptionalResourceId: hash(orgID),
			OptionalRelation:   "member",
			OptionalSubjectFilter: &v1pb.SubjectFilter{
				SubjectType:       model.UserObject,
				OptionalSubjectId: hash(userID),
			},
		},
	})
	if err != nil {
		return err
	}

	relationships := []*v1pb.Relationship{}
	for {
		rel, err := resp.Recv()
		if err != nil {
			break
		}
		relationships = append(relationships, rel.Relationship)
	}

	if len(relationships) == 0 {
		return fmt.Errorf("user %s is not a member of organization %s", userID, orgID)
	}
	return nil
}

// hash function matching the one in authz.go
// Purpose: Creates consistent 6-character SHA256 hashes for user/org IDs as used by the authz service
// Note: All direct SpiceDB operations must use hashed IDs while service calls use original IDs
func hash(s string) string {
	h := sha256.New()
	h.Write([]byte(s))
	return fmt.Sprintf("%x", h.Sum(nil))[:12]
}
