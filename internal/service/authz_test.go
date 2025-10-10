package service_test

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"

	v1pb "github.com/authzed/authzed-go/proto/authzed/api/v1"
	"github.com/authzed/authzed-go/v1"
	"github.com/authzed/grpcutil"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"gorm.io/gorm"

	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/internal/config"
	"github.com/kubev2v/migration-planner/internal/service"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/internal/store/model"
)

// Authz Service Test Suite
//
// This test suite provides comprehensive coverage of the authorization service implementation,
// which wraps the store-level authz operations with transaction management and business logic.
//
// Test Philosophy:
// - Use service APIs for verification when possible (GetPermissions, ListAssessments, etc.)
// - EXCEPTION: CreateUser verification requires direct SpiceDB access since there's no other way
// - Test transaction management (commit/rollback)
// - Use auth.User instead of raw userID strings
// - Follow the pattern from store/authz_test.go but at service level
//
// Test Contexts:
//
// 1. CreateUser
//    - Tests user creation and organization membership
//    - Validates transaction commit
//    - Uses direct SpiceDB access to verify org membership relationship
//
// 2. CreateAssessmentRelationship
//    - Tests initial assessment relationship setup (owner + parent platform)
//    - Validates owner permissions via GetPermissions
//    - Tests transaction management
//
// 3. InitializePlatform
//    - Tests platform role assignment (admin, editor, viewer)
//    - Validates idempotent deletion and recreation
//    - Tests permission cascade to assessments by creating test assessments
//    - Validates unknown roles are ignored
//
// 4. ListAssessments
//    - Tests assessment discovery based on permissions
//    - Coverage: owner, viewer, org member, platform roles
//    - Validates isolation (users only see accessible assessments)

var _ = Describe("Authz Service", Ordered, func() {
	var (
		authzSvc      *service.AuthzService
		s             store.Store
		gormDB        *gorm.DB
		spiceDBClient *authzed.Client
		ctx           context.Context
	)

	BeforeAll(func() {
		ctx = context.Background()

		spiceDBEndpoint := os.Getenv("SPICEDB_ENDPOINT")
		if spiceDBEndpoint == "" {
			spiceDBEndpoint = "localhost:50051"
		}

		// Create SpiceDB client for direct verification in CreateUser tests
		var err error
		spiceDBClient, err = authzed.NewClient(
			spiceDBEndpoint,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpcutil.WithInsecureBearerToken("foobar"),
		)
		if err != nil {
			Skip("SpiceDB not available: " + err.Error())
		}

		// Test connection
		_, err = spiceDBClient.ReadSchema(ctx, &v1pb.ReadSchemaRequest{})
		if err != nil {
			Skip("SpiceDB not reachable: " + err.Error())
		}

		// Initialize database and store
		cfg, err := config.New()
		Expect(err).To(BeNil())

		gormDB, err = store.InitDB(cfg)
		Expect(err).To(BeNil())

		s = store.NewStoreWithAuthz(gormDB, spiceDBClient)
		authzSvc = service.NewAuthzService(s)
	})

	AfterAll(func() {
		if s != nil {
			s.Close()
		}
		if spiceDBClient != nil {
			spiceDBClient.Close()
		}
	})

	// cleanupSpiceDB deletes all relationships for assessments, platform, and organizations
	cleanupSpiceDB := func() {
		// Delete all assessment relationships
		_, err := spiceDBClient.DeleteRelationships(ctx, &v1pb.DeleteRelationshipsRequest{
			RelationshipFilter: &v1pb.RelationshipFilter{
				ResourceType: model.AssessmentObject,
			},
		})
		if err != nil {
			GinkgoT().Logf("Failed to cleanup assessment relationships: %v", err)
		}

		// Delete all platform relationships
		_, err = spiceDBClient.DeleteRelationships(ctx, &v1pb.DeleteRelationshipsRequest{
			RelationshipFilter: &v1pb.RelationshipFilter{
				ResourceType: model.PlatformObject,
			},
		})
		if err != nil {
			GinkgoT().Logf("Failed to cleanup platform relationships: %v", err)
		}

		// Delete all organization relationships
		_, err = spiceDBClient.DeleteRelationships(ctx, &v1pb.DeleteRelationshipsRequest{
			RelationshipFilter: &v1pb.RelationshipFilter{
				ResourceType: model.OrgObject,
			},
		})
		if err != nil {
			GinkgoT().Logf("Failed to cleanup org relationships: %v", err)
		}
	}

	Context("CreateUser", func() {
		// Test: Validates user creation and organization membership
		// Expected: User should be created with org membership, verifiable via direct SpiceDB query
		// Purpose: Tests the foundation for all user-based authorization
		It("should create user with organization membership", func() {
			userID := "create-user-" + uuid.New().String()[:8]
			orgID := "create-org-" + uuid.New().String()[:8]

			user := auth.User{
				Username:     userID,
				Organization: orgID,
			}

			err := authzSvc.CreateUser(ctx, user)
			Expect(err).To(BeNil())

			// Verify user-org membership via direct SpiceDB read
			resp, err := spiceDBClient.ReadRelationships(ctx, &v1pb.ReadRelationshipsRequest{
				Consistency: &v1pb.Consistency{
					Requirement: &v1pb.Consistency_FullyConsistent{
						FullyConsistent: true,
					},
				},
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
			Expect(err).To(BeNil())

			relationships := []*v1pb.Relationship{}
			for {
				rel, err := resp.Recv()
				if err != nil {
					break
				}
				relationships = append(relationships, rel.Relationship)
			}
			Expect(relationships).To(HaveLen(1), "User should be member of organization")
			Expect(relationships[0].Resource.ObjectType).To(Equal(model.OrgObject))
			Expect(relationships[0].Resource.ObjectId).To(Equal(hash(orgID)))
			Expect(relationships[0].Relation).To(Equal("member"))
			Expect(relationships[0].Subject.Object.ObjectType).To(Equal(model.UserObject))
			Expect(relationships[0].Subject.Object.ObjectId).To(Equal(hash(userID)))
		})

		// Test: Validates user can be used in subsequent authorization operations
		// Expected: Newly created user should be able to create assessments and have relationships
		// Purpose: Tests that user creation is complete and functional
		It("should allow created user to participate in authorization operations", func() {
			userID := "participant-user-" + uuid.New().String()[:8]
			orgID := "participant-org-" + uuid.New().String()[:8]
			assessmentID := uuid.New().String()

			user := auth.User{
				Username:     userID,
				Organization: orgID,
			}

			// Create user
			err := authzSvc.CreateUser(ctx, user)
			Expect(err).To(BeNil())

			// Create assessment relationship
			err = authzSvc.CreateAssessmentRelationship(ctx, assessmentID, user)
			Expect(err).To(BeNil())

			// Verify user has all owner permissions
			permissions, err := authzSvc.GetPermissions(ctx, assessmentID, user)
			Expect(err).To(BeNil())
			Expect(permissions).To(ContainElements(
				model.ReadPermission,
				model.EditPermission,
				model.SharePermission,
				model.DeletePermission,
			), "Owner should have all permissions")
		})

		AfterEach(func() {
			cleanupSpiceDB()
		})
	})

	Context("CreateAssessmentRelationship", func() {
		// Test: Validates creating initial assessment relationships (owner + parent platform)
		// Expected: User should become owner with all permissions, platform relationship established
		// Purpose: Tests assessment creation authorization setup
		It("should create owner and parent platform relationships", func() {
			userID := "owner-user-" + uuid.New().String()[:8]
			orgID := "owner-org-" + uuid.New().String()[:8]
			assessmentID := uuid.New().String()

			user := auth.User{
				Username:     userID,
				Organization: orgID,
			}

			// Create user first
			err := authzSvc.CreateUser(ctx, user)
			Expect(err).To(BeNil())

			// Create assessment relationship
			err = authzSvc.CreateAssessmentRelationship(ctx, assessmentID, user)
			Expect(err).To(BeNil())

			// Verify owner has all permissions
			permissions, err := authzSvc.GetPermissions(ctx, assessmentID, user)
			Expect(err).To(BeNil())
			Expect(permissions).To(ContainElements(
				model.ReadPermission,
				model.EditPermission,
				model.SharePermission,
				model.DeletePermission,
			), "Owner should have all permissions")
		})

		// Test: Validates assessment appears in owner's list
		// Expected: Assessment should be discoverable via ListAssessments
		// Purpose: Tests integration between CreateAssessmentRelationship and ListAssessments
		It("should make assessment visible to owner via ListAssessments", func() {
			userID := "list-owner-" + uuid.New().String()[:8]
			orgID := "list-org-" + uuid.New().String()[:8]
			assessmentID := uuid.New().String()

			user := auth.User{
				Username:     userID,
				Organization: orgID,
			}

			// Create user
			err := authzSvc.CreateUser(ctx, user)
			Expect(err).To(BeNil())

			// Verify initially no assessments
			assessmentsBefore, err := authzSvc.ListAssessments(ctx, user)
			Expect(err).To(BeNil())
			Expect(assessmentsBefore).ToNot(ContainElement(assessmentID))

			// Create assessment relationship
			err = authzSvc.CreateAssessmentRelationship(ctx, assessmentID, user)
			Expect(err).To(BeNil())

			// Verify assessment now visible
			assessmentsAfter, err := authzSvc.ListAssessments(ctx, user)
			Expect(err).To(BeNil())
			Expect(assessmentsAfter).To(ContainElement(assessmentID))
		})

		AfterEach(func() {
			cleanupSpiceDB()
		})
	})

	Context("InitializePlatform", func() {
		// Test: Validates platform initialization with multiple roles
		// Expected: All roles (admin, editor, viewer) should grant appropriate permissions
		// Purpose: Tests platform-level permission management
		It("should initialize platform with admin, editor, and viewer roles", func() {
			adminUserID := "platform-admin-" + uuid.New().String()[:8]
			editorUserID := "platform-editor-" + uuid.New().String()[:8]
			viewerUserID := "platform-viewer-" + uuid.New().String()[:8]
			ownerUserID := "platform-owner-" + uuid.New().String()[:8]
			orgID := "platform-org-" + uuid.New().String()[:8]

			adminUser := auth.User{Username: adminUserID, Organization: orgID}
			editorUser := auth.User{Username: editorUserID, Organization: orgID}
			viewerUser := auth.User{Username: viewerUserID, Organization: orgID}
			ownerUser := auth.User{Username: ownerUserID, Organization: orgID}

			// Initialize platform with roles (platform users don't need to be created)
			platformUsers := map[string][]string{
				"admin":  {adminUserID},
				"editor": {editorUserID},
				"viewer": {viewerUserID},
			}

			err := authzSvc.InitilizePlatform(ctx, platformUsers)
			Expect(err).To(BeNil())

			// Create only the owner user who will create assessments
			err = authzSvc.CreateUser(ctx, ownerUser)
			Expect(err).To(BeNil())

			// Create assessments to test permission cascade
			assessment1ID := uuid.New().String()
			assessment2ID := uuid.New().String()
			assessment3ID := uuid.New().String()

			err = authzSvc.CreateAssessmentRelationship(ctx, assessment1ID, ownerUser)
			Expect(err).To(BeNil())
			err = authzSvc.CreateAssessmentRelationship(ctx, assessment2ID, ownerUser)
			Expect(err).To(BeNil())
			err = authzSvc.CreateAssessmentRelationship(ctx, assessment3ID, ownerUser)
			Expect(err).To(BeNil())

			// Verify admin has all permissions on all assessments
			adminPerms1, err := authzSvc.GetPermissions(ctx, assessment1ID, adminUser)
			Expect(err).To(BeNil())
			Expect(adminPerms1).To(ContainElements(
				model.ReadPermission,
				model.EditPermission,
				model.SharePermission,
				model.DeletePermission,
			), "Admin should have all permissions on assessment1")

			adminPerms2, err := authzSvc.GetPermissions(ctx, assessment2ID, adminUser)
			Expect(err).To(BeNil())
			Expect(adminPerms2).To(ContainElements(
				model.ReadPermission,
				model.EditPermission,
				model.SharePermission,
				model.DeletePermission,
			), "Admin should have all permissions on assessment2")

			// Verify editor has read and edit permissions
			editorPerms1, err := authzSvc.GetPermissions(ctx, assessment1ID, editorUser)
			Expect(err).To(BeNil())
			Expect(editorPerms1).To(ContainElements(
				model.ReadPermission,
				model.EditPermission,
			), "Editor should have read and edit permissions")
			Expect(editorPerms1).ToNot(ContainElements(
				model.SharePermission,
				model.DeletePermission,
			), "Editor should not have share or delete permissions")

			editorPerms2, err := authzSvc.GetPermissions(ctx, assessment2ID, editorUser)
			Expect(err).To(BeNil())
			Expect(editorPerms2).To(ContainElements(
				model.ReadPermission,
				model.EditPermission,
			), "Editor should have read and edit permissions on assessment2")

			// Verify viewer has only read permission
			viewerPerms1, err := authzSvc.GetPermissions(ctx, assessment1ID, viewerUser)
			Expect(err).To(BeNil())
			Expect(viewerPerms1).To(ContainElement(model.ReadPermission),
				"Viewer should have read permission")
			Expect(viewerPerms1).ToNot(ContainElements(
				model.EditPermission,
				model.SharePermission,
				model.DeletePermission,
			), "Viewer should not have edit, share, or delete permissions")

			viewerPerms3, err := authzSvc.GetPermissions(ctx, assessment3ID, viewerUser)
			Expect(err).To(BeNil())
			Expect(viewerPerms3).To(ContainElement(model.ReadPermission),
				"Viewer should have read permission on assessment3")
		})

		// Test: Validates platform can be reinitialized (idempotent)
		// Expected: Old roles should be removed, new roles should work
		// Purpose: Tests that InitializePlatform properly cleans up old relationships
		It("should be idempotent and replace existing platform roles", func() {
			oldAdminID := "old-admin-" + uuid.New().String()[:8]
			newAdminID := "new-admin-" + uuid.New().String()[:8]
			orgID := "reinit-org-" + uuid.New().String()[:8]
			assessmentID := uuid.New().String()

			oldAdmin := auth.User{Username: oldAdminID, Organization: orgID}
			newAdmin := auth.User{Username: newAdminID, Organization: orgID}

			// Create users
			err := authzSvc.CreateUser(ctx, oldAdmin)
			Expect(err).To(BeNil())
			err = authzSvc.CreateUser(ctx, newAdmin)
			Expect(err).To(BeNil())

			// Initialize platform with old admin
			err = authzSvc.InitilizePlatform(ctx, map[string][]string{
				"admin": {oldAdminID},
			})
			Expect(err).To(BeNil())

			// Create assessment
			ownerUserID := "reinit-owner-" + uuid.New().String()[:8]
			ownerUser := auth.User{Username: ownerUserID, Organization: orgID}
			err = authzSvc.CreateUser(ctx, ownerUser)
			Expect(err).To(BeNil())
			err = authzSvc.CreateAssessmentRelationship(ctx, assessmentID, ownerUser)
			Expect(err).To(BeNil())

			// Verify old admin has permissions
			oldAdminPermissions, err := authzSvc.GetPermissions(ctx, assessmentID, oldAdmin)
			Expect(err).To(BeNil())
			Expect(oldAdminPermissions).To(ContainElement(model.DeletePermission))

			// Reinitialize with new admin
			err = authzSvc.InitilizePlatform(ctx, map[string][]string{
				"admin": {newAdminID},
			})
			Expect(err).To(BeNil())

			// Verify old admin lost permissions
			oldAdminPermissionsAfter, err := authzSvc.GetPermissions(ctx, assessmentID, oldAdmin)
			Expect(err).To(BeNil())
			Expect(oldAdminPermissionsAfter).ToNot(ContainElement(model.DeletePermission),
				"Old admin should lose permissions after reinit")

			// Verify new admin has permissions
			newAdminPermissions, err := authzSvc.GetPermissions(ctx, assessmentID, newAdmin)
			Expect(err).To(BeNil())
			Expect(newAdminPermissions).To(ContainElement(model.DeletePermission),
				"New admin should have permissions after reinit")
		})

		// Test: Validates handling of empty role lists
		// Expected: Should succeed without error
		// Purpose: Tests edge case handling
		It("should handle empty role lists", func() {
			err := authzSvc.InitilizePlatform(ctx, map[string][]string{
				"admin":  {},
				"editor": {},
				"viewer": {},
			})
			Expect(err).To(BeNil())
		})

		// Test: Validates unknown roles are ignored
		// Expected: Should succeed, only known roles processed
		// Purpose: Tests robustness against invalid input
		It("should ignore unknown roles", func() {
			adminUserID := "unknown-role-admin-" + uuid.New().String()[:8]
			orgID := "unknown-role-org-" + uuid.New().String()[:8]

			adminUser := auth.User{Username: adminUserID, Organization: orgID}
			err := authzSvc.CreateUser(ctx, adminUser)
			Expect(err).To(BeNil())

			// Include unknown role
			err = authzSvc.InitilizePlatform(ctx, map[string][]string{
				"admin":        {adminUserID},
				"unknown_role": {"some-user"},
			})
			Expect(err).To(BeNil())
		})

		AfterEach(func() {
			cleanupSpiceDB()
		})
	})

	Context("ListAssessments", func() {
		// Test: Validates user can list owned assessments
		// Expected: Should return assessment where user is owner
		// Purpose: Tests basic assessment discovery
		It("should list assessments owned by user", func() {
			userID := "list-owner-user-" + uuid.New().String()[:8]
			orgID := "list-owner-org-" + uuid.New().String()[:8]
			assessment1ID := uuid.New().String()
			assessment2ID := uuid.New().String()

			user := auth.User{Username: userID, Organization: orgID}

			// Create user
			err := authzSvc.CreateUser(ctx, user)
			Expect(err).To(BeNil())

			// Create two assessments owned by user
			err = authzSvc.CreateAssessmentRelationship(ctx, assessment1ID, user)
			Expect(err).To(BeNil())
			err = authzSvc.CreateAssessmentRelationship(ctx, assessment2ID, user)
			Expect(err).To(BeNil())

			// List assessments
			assessments, err := authzSvc.ListAssessments(ctx, user)
			Expect(err).To(BeNil())
			Expect(assessments).To(ContainElements(assessment1ID, assessment2ID))
		})

		// Test: Validates user can list assessments shared as viewer
		// Expected: Should return assessments where user has viewer access
		// Purpose: Tests viewer-based discovery
		It("should list assessments where user is viewer", func() {
			ownerID := "list-viewer-owner-" + uuid.New().String()[:8]
			viewerID := "list-viewer-user-" + uuid.New().String()[:8]
			orgID := "list-viewer-org-" + uuid.New().String()[:8]
			assessmentID := uuid.New().String()

			owner := auth.User{Username: ownerID, Organization: orgID}
			viewer := auth.User{Username: viewerID, Organization: orgID}

			// Create users
			err := authzSvc.CreateUser(ctx, owner)
			Expect(err).To(BeNil())
			err = authzSvc.CreateUser(ctx, viewer)
			Expect(err).To(BeNil())

			// Owner creates assessment
			err = authzSvc.CreateAssessmentRelationship(ctx, assessmentID, owner)
			Expect(err).To(BeNil())

			// Share with viewer
			err = authzSvc.WriteRelationships(ctx,
				model.NewRelationship(assessmentID, model.NewUserSubject(viewerID), model.ViewerRelationshipKind),
			)
			Expect(err).To(BeNil())

			// Reader should see assessment
			assessments, err := authzSvc.ListAssessments(ctx, viewer)
			Expect(err).To(BeNil())
			Expect(assessments).To(ContainElement(assessmentID))
		})

		// Test: Validates org peers do NOT have automatic access to assessments
		// Expected: Organization members should not see each other's assessments without explicit sharing
		// Purpose: Tests that organization membership alone doesn't grant access
		It("should NOT grant automatic access to org peers", func() {
			user1ID := "list-org-no-access-user1-" + uuid.New().String()[:8]
			user2ID := "list-org-no-access-user2-" + uuid.New().String()[:8]
			orgID := "list-org-no-access-" + uuid.New().String()[:8]
			assessmentID := uuid.New().String()

			user1 := auth.User{Username: user1ID, Organization: orgID}
			user2 := auth.User{Username: user2ID, Organization: orgID}

			// Create users in same org
			err := authzSvc.CreateUser(ctx, user1)
			Expect(err).To(BeNil())
			err = authzSvc.CreateUser(ctx, user2)
			Expect(err).To(BeNil())

			// User1 creates assessment
			err = authzSvc.CreateAssessmentRelationship(ctx, assessmentID, user1)
			Expect(err).To(BeNil())

			// User2 should NOT see assessment (no explicit org relationship written)
			assessments, err := authzSvc.ListAssessments(ctx, user2)
			Expect(err).To(BeNil())
			Expect(assessments).ToNot(ContainElement(assessmentID))
		})

		// Test: Validates org members can list org assessments when explicitly shared
		// Expected: Should return assessments when organization relationship is explicitly written
		// Purpose: Tests organization-based discovery via explicit relationship
		It("should list assessments accessible via explicit organization relationship", func() {
			user1ID := "list-org-user1-" + uuid.New().String()[:8]
			user2ID := "list-org-user2-" + uuid.New().String()[:8]
			orgID := "list-org-shared-" + uuid.New().String()[:8]
			assessmentID := uuid.New().String()

			user1 := auth.User{Username: user1ID, Organization: orgID}
			user2 := auth.User{Username: user2ID, Organization: orgID}

			// Create users in same org
			err := authzSvc.CreateUser(ctx, user1)
			Expect(err).To(BeNil())
			err = authzSvc.CreateUser(ctx, user2)
			Expect(err).To(BeNil())

			// User1 creates assessment
			err = authzSvc.CreateAssessmentRelationship(ctx, assessmentID, user1)
			Expect(err).To(BeNil())

			// Explicitly associate assessment with org
			err = authzSvc.WriteRelationships(ctx,
				model.NewRelationship(assessmentID, model.NewOrganizationSubject(orgID), model.OrganizationRelationshipKind),
			)
			Expect(err).To(BeNil())

			// User2 should see assessment via explicit org relationship
			assessments, err := authzSvc.ListAssessments(ctx, user2)
			Expect(err).To(BeNil())
			Expect(assessments).To(ContainElement(assessmentID))
		})

		// Test: Validates platform users can list all assessments
		// Expected: Platform admin/editor/viewer should see all assessments
		// Purpose: Tests platform-based discovery
		It("should list assessments accessible via platform role", func() {
			platformAdminID := "list-platform-admin-" + uuid.New().String()[:8]
			ownerID := "list-platform-owner-" + uuid.New().String()[:8]
			orgID := "list-platform-org-" + uuid.New().String()[:8]
			assessmentID := uuid.New().String()

			platformAdmin := auth.User{Username: platformAdminID, Organization: orgID}
			owner := auth.User{Username: ownerID, Organization: orgID}

			// Create users
			err := authzSvc.CreateUser(ctx, platformAdmin)
			Expect(err).To(BeNil())
			err = authzSvc.CreateUser(ctx, owner)
			Expect(err).To(BeNil())

			// Initialize platform with admin
			err = authzSvc.InitilizePlatform(ctx, map[string][]string{
				"admin": {platformAdminID},
			})
			Expect(err).To(BeNil())

			// Owner creates assessment (which has parent platform relationship)
			err = authzSvc.CreateAssessmentRelationship(ctx, assessmentID, owner)
			Expect(err).To(BeNil())

			// Platform admin should see assessment
			assessments, err := authzSvc.ListAssessments(ctx, platformAdmin)
			Expect(err).To(BeNil())
			Expect(assessments).To(ContainElement(assessmentID))
		})

		// Test: Validates user with no permissions sees empty list
		// Expected: Should return empty list
		// Purpose: Tests isolation
		It("should return empty list for user with no permissions", func() {
			userID := "list-no-access-" + uuid.New().String()[:8]
			orgID := "list-no-access-org-" + uuid.New().String()[:8]

			user := auth.User{Username: userID, Organization: orgID}

			// Create user
			err := authzSvc.CreateUser(ctx, user)
			Expect(err).To(BeNil())

			// List assessments (should be empty)
			assessments, err := authzSvc.ListAssessments(ctx, user)
			Expect(err).To(BeNil())
			Expect(assessments).To(BeEmpty())
		})

		AfterEach(func() {
			cleanupSpiceDB()
		})
	})

	Context("WriteRelationships", func() {
		// Test: Validates writing owner relationship
		// Expected: User should gain all permissions
		// Purpose: Tests owner relationship creation
		It("should write owner relationship and grant all permissions", func() {
			ownerID := "write-owner-" + uuid.New().String()[:8]
			newOwnerID := "write-new-owner-" + uuid.New().String()[:8]
			orgID := "write-owner-org-" + uuid.New().String()[:8]
			assessmentID := uuid.New().String()

			owner := auth.User{Username: ownerID, Organization: orgID}
			newOwner := auth.User{Username: newOwnerID, Organization: orgID}

			// Create users
			err := authzSvc.CreateUser(ctx, owner)
			Expect(err).To(BeNil())
			err = authzSvc.CreateUser(ctx, newOwner)
			Expect(err).To(BeNil())

			// Create assessment
			err = authzSvc.CreateAssessmentRelationship(ctx, assessmentID, owner)
			Expect(err).To(BeNil())

			// Add new owner
			err = authzSvc.WriteRelationships(ctx,
				model.NewRelationship(assessmentID, model.NewUserSubject(newOwnerID), model.OwnerRelationshipKind),
			)
			Expect(err).To(BeNil())

			// Verify new owner has all permissions
			permissions, err := authzSvc.GetPermissions(ctx, assessmentID, newOwner)
			Expect(err).To(BeNil())
			Expect(permissions).To(ContainElements(
				model.ReadPermission,
				model.EditPermission,
				model.SharePermission,
				model.DeletePermission,
			))
		})

		// Test: Validates writing viewer relationship
		// Expected: User should gain read permission only
		// Purpose: Tests viewer relationship creation
		It("should write viewer relationship and grant read permission only", func() {
			ownerID := "write-viewer-owner-" + uuid.New().String()[:8]
			viewerID := "write-viewer-" + uuid.New().String()[:8]
			orgID := "write-viewer-org-" + uuid.New().String()[:8]
			assessmentID := uuid.New().String()

			owner := auth.User{Username: ownerID, Organization: orgID}
			viewer := auth.User{Username: viewerID, Organization: orgID}

			// Create users
			err := authzSvc.CreateUser(ctx, owner)
			Expect(err).To(BeNil())
			err = authzSvc.CreateUser(ctx, viewer)
			Expect(err).To(BeNil())

			// Create assessment
			err = authzSvc.CreateAssessmentRelationship(ctx, assessmentID, owner)
			Expect(err).To(BeNil())

			// Add viewer
			err = authzSvc.WriteRelationships(ctx,
				model.NewRelationship(assessmentID, model.NewUserSubject(viewerID), model.ViewerRelationshipKind),
			)
			Expect(err).To(BeNil())

			// Verify viewer has only read permission
			permissions, err := authzSvc.GetPermissions(ctx, assessmentID, viewer)
			Expect(err).To(BeNil())
			Expect(permissions).To(ContainElement(model.ReadPermission))
			Expect(permissions).ToNot(ContainElements(
				model.EditPermission,
				model.SharePermission,
				model.DeletePermission,
			))
		})

		// Test: Validates writing organization relationship
		// Expected: Org members should gain read and edit permissions
		// Purpose: Tests organization relationship creation
		It("should write organization relationship and grant org members read+edit permissions", func() {
			ownerID := "write-org-owner-" + uuid.New().String()[:8]
			memberID := "write-org-member-" + uuid.New().String()[:8]
			orgID := "write-org-shared-" + uuid.New().String()[:8]
			assessmentID := uuid.New().String()

			owner := auth.User{Username: ownerID, Organization: orgID}
			member := auth.User{Username: memberID, Organization: orgID}

			// Create users
			err := authzSvc.CreateUser(ctx, owner)
			Expect(err).To(BeNil())
			err = authzSvc.CreateUser(ctx, member)
			Expect(err).To(BeNil())

			// Create assessment
			err = authzSvc.CreateAssessmentRelationship(ctx, assessmentID, owner)
			Expect(err).To(BeNil())

			// Associate with org
			err = authzSvc.WriteRelationships(ctx,
				model.NewRelationship(assessmentID, model.NewOrganizationSubject(orgID), model.OrganizationRelationshipKind),
			)
			Expect(err).To(BeNil())

			// Verify member has read and edit permissions
			permissions, err := authzSvc.GetPermissions(ctx, assessmentID, member)
			Expect(err).To(BeNil())
			Expect(permissions).To(ContainElements(
				model.ReadPermission,
				model.EditPermission,
			))
			Expect(permissions).ToNot(ContainElements(
				model.SharePermission,
				model.DeletePermission,
			))
		})

		// Test: Validates error when writing organization relationship with user subject
		// Expected: Should return error
		// Purpose: Tests validation
		It("should return error when writing organization relationship with non-organization subject", func() {
			ownerID := "write-invalid-owner-" + uuid.New().String()[:8]
			orgID := "write-invalid-org-" + uuid.New().String()[:8]
			assessmentID := uuid.New().String()

			owner := auth.User{Username: ownerID, Organization: orgID}

			// Create user
			err := authzSvc.CreateUser(ctx, owner)
			Expect(err).To(BeNil())

			// Create assessment
			err = authzSvc.CreateAssessmentRelationship(ctx, assessmentID, owner)
			Expect(err).To(BeNil())

			// Try to write org relationship with user subject (should fail)
			err = authzSvc.WriteRelationships(ctx,
				model.NewRelationship(assessmentID, model.NewUserSubject(ownerID), model.OrganizationRelationshipKind),
			)
			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(ContainSubstring("subject must be an organization"))
		})

		AfterEach(func() {
			cleanupSpiceDB()
		})
	})

	Context("DeleteRelationships", func() {
		// Test: Validates deleting specific relationship (unshare)
		// Expected: Specific relationship removed, permissions revoked
		// Purpose: Tests granular relationship deletion
		It("should delete specific owner relationship and revoke permissions", func() {
			owner1ID := "delete-owner1-" + uuid.New().String()[:8]
			owner2ID := "delete-owner2-" + uuid.New().String()[:8]
			orgID := "delete-owner-org-" + uuid.New().String()[:8]
			assessmentID := uuid.New().String()

			owner1 := auth.User{Username: owner1ID, Organization: orgID}
			owner2 := auth.User{Username: owner2ID, Organization: orgID}

			// Create users
			err := authzSvc.CreateUser(ctx, owner1)
			Expect(err).To(BeNil())
			err = authzSvc.CreateUser(ctx, owner2)
			Expect(err).To(BeNil())

			// Create assessment
			err = authzSvc.CreateAssessmentRelationship(ctx, assessmentID, owner1)
			Expect(err).To(BeNil())

			// Add second owner
			err = authzSvc.WriteRelationships(ctx,
				model.NewRelationship(assessmentID, model.NewUserSubject(owner2ID), model.OwnerRelationshipKind),
			)
			Expect(err).To(BeNil())

			// Verify both have permissions
			permissions1, err := authzSvc.GetPermissions(ctx, assessmentID, owner1)
			Expect(err).To(BeNil())
			Expect(permissions1).To(ContainElement(model.DeletePermission))

			permissions2, err := authzSvc.GetPermissions(ctx, assessmentID, owner2)
			Expect(err).To(BeNil())
			Expect(permissions2).To(ContainElement(model.DeletePermission))

			// Delete owner2 relationship
			err = authzSvc.DeleteRelationships(ctx,
				model.NewRelationship(assessmentID, model.NewUserSubject(owner2ID), model.OwnerRelationshipKind),
			)
			Expect(err).To(BeNil())

			// Verify owner2 lost permissions
			permissions2After, err := authzSvc.GetPermissions(ctx, assessmentID, owner2)
			Expect(err).To(BeNil())
			Expect(permissions2After).ToNot(ContainElement(model.DeletePermission))

			// Verify owner1 still has permissions
			permissions1After, err := authzSvc.GetPermissions(ctx, assessmentID, owner1)
			Expect(err).To(BeNil())
			Expect(permissions1After).To(ContainElement(model.DeletePermission))
		})

		// Test: Validates deleting viewer relationship
		// Expected: Reader loses read permission
		// Purpose: Tests viewer relationship deletion
		It("should delete viewer relationship and revoke read permission", func() {
			ownerID := "delete-viewer-owner-" + uuid.New().String()[:8]
			viewerID := "delete-viewer-" + uuid.New().String()[:8]
			orgID := "delete-viewer-org-" + uuid.New().String()[:8]
			assessmentID := uuid.New().String()

			owner := auth.User{Username: ownerID, Organization: orgID}
			viewer := auth.User{Username: viewerID, Organization: orgID}

			// Create users
			err := authzSvc.CreateUser(ctx, owner)
			Expect(err).To(BeNil())
			err = authzSvc.CreateUser(ctx, viewer)
			Expect(err).To(BeNil())

			// Create assessment and share
			err = authzSvc.CreateAssessmentRelationship(ctx, assessmentID, owner)
			Expect(err).To(BeNil())
			err = authzSvc.WriteRelationships(ctx,
				model.NewRelationship(assessmentID, model.NewUserSubject(viewerID), model.ViewerRelationshipKind),
			)
			Expect(err).To(BeNil())

			// Verify viewer has permission
			permissionsBefore, err := authzSvc.GetPermissions(ctx, assessmentID, viewer)
			Expect(err).To(BeNil())
			Expect(permissionsBefore).To(ContainElement(model.ReadPermission))

			// Delete viewer relationship
			err = authzSvc.DeleteRelationships(ctx,
				model.NewRelationship(assessmentID, model.NewUserSubject(viewerID), model.ViewerRelationshipKind),
			)
			Expect(err).To(BeNil())

			// Verify viewer lost permission
			permissionsAfter, err := authzSvc.GetPermissions(ctx, assessmentID, viewer)
			Expect(err).To(BeNil())
			Expect(permissionsAfter).To(BeEmpty())
		})

		// Test: Validates deleting organization relationship
		// Expected: Org members lose read+edit permissions
		// Purpose: Tests organization relationship deletion
		It("should delete organization relationship and revoke org member permissions", func() {
			ownerID := "delete-org-owner-" + uuid.New().String()[:8]
			memberID := "delete-org-member-" + uuid.New().String()[:8]
			orgID := "delete-org-shared-" + uuid.New().String()[:8]
			assessmentID := uuid.New().String()

			owner := auth.User{Username: ownerID, Organization: orgID}
			member := auth.User{Username: memberID, Organization: orgID}

			// Create users
			err := authzSvc.CreateUser(ctx, owner)
			Expect(err).To(BeNil())
			err = authzSvc.CreateUser(ctx, member)
			Expect(err).To(BeNil())

			// Create assessment and associate with org
			err = authzSvc.CreateAssessmentRelationship(ctx, assessmentID, owner)
			Expect(err).To(BeNil())
			err = authzSvc.WriteRelationships(ctx,
				model.NewRelationship(assessmentID, model.NewOrganizationSubject(orgID), model.OrganizationRelationshipKind),
			)
			Expect(err).To(BeNil())

			// Verify member has permissions
			permissionsBefore, err := authzSvc.GetPermissions(ctx, assessmentID, member)
			Expect(err).To(BeNil())
			Expect(permissionsBefore).To(ContainElements(model.ReadPermission, model.EditPermission))

			// Delete org relationship
			err = authzSvc.DeleteRelationships(ctx,
				model.NewRelationship(assessmentID, model.NewOrganizationSubject(orgID), model.OrganizationRelationshipKind),
			)
			Expect(err).To(BeNil())

			// Verify member lost permissions (but owner still has them)
			permissionsAfter, err := authzSvc.GetPermissions(ctx, assessmentID, member)
			Expect(err).To(BeNil())
			Expect(permissionsAfter).To(BeEmpty())

			// Owner should still have all permissions
			ownerPermissions, err := authzSvc.GetPermissions(ctx, assessmentID, owner)
			Expect(err).To(BeNil())
			Expect(ownerPermissions).To(ContainElement(model.DeletePermission))
		})

		// Test: Validates DeleteAllRelationships removes all relationships
		// Expected: All users lose permissions
		// Purpose: Tests bulk deletion
		It("should delete all assessment relationships via DeleteAllRelationships", func() {
			owner1ID := "delete-all-owner1-" + uuid.New().String()[:8]
			owner2ID := "delete-all-owner2-" + uuid.New().String()[:8]
			viewerID := "delete-all-viewer-" + uuid.New().String()[:8]
			orgID := "delete-all-org-" + uuid.New().String()[:8]
			assessmentID := uuid.New().String()

			owner1 := auth.User{Username: owner1ID, Organization: orgID}
			owner2 := auth.User{Username: owner2ID, Organization: orgID}
			viewer := auth.User{Username: viewerID, Organization: orgID}

			// Create users
			err := authzSvc.CreateUser(ctx, owner1)
			Expect(err).To(BeNil())
			err = authzSvc.CreateUser(ctx, owner2)
			Expect(err).To(BeNil())
			err = authzSvc.CreateUser(ctx, viewer)
			Expect(err).To(BeNil())

			// Create assessment with multiple relationships
			err = authzSvc.CreateAssessmentRelationship(ctx, assessmentID, owner1)
			Expect(err).To(BeNil())
			err = authzSvc.WriteRelationships(ctx,
				model.NewRelationship(assessmentID, model.NewUserSubject(owner2ID), model.OwnerRelationshipKind),
			)
			Expect(err).To(BeNil())
			err = authzSvc.WriteRelationships(ctx,
				model.NewRelationship(assessmentID, model.NewUserSubject(viewerID), model.ViewerRelationshipKind),
			)
			Expect(err).To(BeNil())

			// Verify all have permissions
			permissions1, err := authzSvc.GetPermissions(ctx, assessmentID, owner1)
			Expect(err).To(BeNil())
			Expect(permissions1).ToNot(BeEmpty())

			permissions2, err := authzSvc.GetPermissions(ctx, assessmentID, owner2)
			Expect(err).To(BeNil())
			Expect(permissions2).ToNot(BeEmpty())

			permissionsReader, err := authzSvc.GetPermissions(ctx, assessmentID, viewer)
			Expect(err).To(BeNil())
			Expect(permissionsReader).ToNot(BeEmpty())

			// Delete all relationships
			err = authzSvc.DeleteAllRelationships(ctx, assessmentID)
			Expect(err).To(BeNil())

			// Verify all lost permissions
			permissions1After, err := authzSvc.GetPermissions(ctx, assessmentID, owner1)
			Expect(err).To(BeNil())
			Expect(permissions1After).To(BeEmpty())

			permissions2After, err := authzSvc.GetPermissions(ctx, assessmentID, owner2)
			Expect(err).To(BeNil())
			Expect(permissions2After).To(BeEmpty())

			permissionsReaderAfter, err := authzSvc.GetPermissions(ctx, assessmentID, viewer)
			Expect(err).To(BeNil())
			Expect(permissionsReaderAfter).To(BeEmpty())
		})

		AfterEach(func() {
			cleanupSpiceDB()
		})
	})

	Context("GetPermissions", func() {
		// Test: Validates owner has all permissions
		// Expected: Read, edit, share, delete
		// Purpose: Tests owner permission retrieval
		It("should return all permissions for owner", func() {
			ownerID := "get-perm-owner-" + uuid.New().String()[:8]
			orgID := "get-perm-org-" + uuid.New().String()[:8]
			assessmentID := uuid.New().String()

			owner := auth.User{Username: ownerID, Organization: orgID}

			err := authzSvc.CreateUser(ctx, owner)
			Expect(err).To(BeNil())

			err = authzSvc.CreateAssessmentRelationship(ctx, assessmentID, owner)
			Expect(err).To(BeNil())

			permissions, err := authzSvc.GetPermissions(ctx, assessmentID, owner)
			Expect(err).To(BeNil())
			Expect(permissions).To(ContainElements(
				model.ReadPermission,
				model.EditPermission,
				model.SharePermission,
				model.DeletePermission,
			))
		})

		// Test: Validates viewer has only read permission
		// Expected: Read only
		// Purpose: Tests viewer permission retrieval
		It("should return only read permission for viewer", func() {
			ownerID := "get-perm-viewer-owner-" + uuid.New().String()[:8]
			viewerID := "get-perm-viewer-" + uuid.New().String()[:8]
			orgID := "get-perm-viewer-org-" + uuid.New().String()[:8]
			assessmentID := uuid.New().String()

			owner := auth.User{Username: ownerID, Organization: orgID}
			viewer := auth.User{Username: viewerID, Organization: orgID}

			err := authzSvc.CreateUser(ctx, owner)
			Expect(err).To(BeNil())
			err = authzSvc.CreateUser(ctx, viewer)
			Expect(err).To(BeNil())

			err = authzSvc.CreateAssessmentRelationship(ctx, assessmentID, owner)
			Expect(err).To(BeNil())
			err = authzSvc.WriteRelationships(ctx,
				model.NewRelationship(assessmentID, model.NewUserSubject(viewerID), model.ViewerRelationshipKind),
			)
			Expect(err).To(BeNil())

			permissions, err := authzSvc.GetPermissions(ctx, assessmentID, viewer)
			Expect(err).To(BeNil())
			Expect(permissions).To(ContainElement(model.ReadPermission))
			Expect(permissions).ToNot(ContainElements(
				model.EditPermission,
				model.SharePermission,
				model.DeletePermission,
			))
		})

		// Test: Validates org member has read and edit permissions
		// Expected: Read, edit (no share, delete)
		// Purpose: Tests organization permission retrieval
		It("should return read and edit permissions for org member", func() {
			ownerID := "get-perm-org-owner-" + uuid.New().String()[:8]
			memberID := "get-perm-org-member-" + uuid.New().String()[:8]
			orgID := "get-perm-org-shared-" + uuid.New().String()[:8]
			assessmentID := uuid.New().String()

			owner := auth.User{Username: ownerID, Organization: orgID}
			member := auth.User{Username: memberID, Organization: orgID}

			err := authzSvc.CreateUser(ctx, owner)
			Expect(err).To(BeNil())
			err = authzSvc.CreateUser(ctx, member)
			Expect(err).To(BeNil())

			err = authzSvc.CreateAssessmentRelationship(ctx, assessmentID, owner)
			Expect(err).To(BeNil())
			err = authzSvc.WriteRelationships(ctx,
				model.NewRelationship(assessmentID, model.NewOrganizationSubject(orgID), model.OrganizationRelationshipKind),
			)
			Expect(err).To(BeNil())

			permissions, err := authzSvc.GetPermissions(ctx, assessmentID, member)
			Expect(err).To(BeNil())
			Expect(permissions).To(ContainElements(
				model.ReadPermission,
				model.EditPermission,
			))
			Expect(permissions).ToNot(ContainElements(
				model.SharePermission,
				model.DeletePermission,
			))
		})

		// Test: Validates platform admin has all permissions
		// Expected: All permissions via platform relationship
		// Purpose: Tests platform admin permission retrieval
		It("should return all permissions for platform admin", func() {
			adminID := "get-perm-admin-" + uuid.New().String()[:8]
			ownerID := "get-perm-admin-owner-" + uuid.New().String()[:8]
			orgID := "get-perm-admin-org-" + uuid.New().String()[:8]
			assessmentID := uuid.New().String()

			admin := auth.User{Username: adminID, Organization: orgID}
			owner := auth.User{Username: ownerID, Organization: orgID}

			err := authzSvc.CreateUser(ctx, admin)
			Expect(err).To(BeNil())
			err = authzSvc.CreateUser(ctx, owner)
			Expect(err).To(BeNil())

			err = authzSvc.InitilizePlatform(ctx, map[string][]string{
				"admin": {adminID},
			})
			Expect(err).To(BeNil())

			err = authzSvc.CreateAssessmentRelationship(ctx, assessmentID, owner)
			Expect(err).To(BeNil())

			permissions, err := authzSvc.GetPermissions(ctx, assessmentID, admin)
			Expect(err).To(BeNil())
			Expect(permissions).To(ContainElements(
				model.ReadPermission,
				model.EditPermission,
				model.SharePermission,
				model.DeletePermission,
			))
		})

		// Test: Validates user with no access has no permissions
		// Expected: Empty permissions
		// Purpose: Tests access control isolation
		It("should return empty permissions for user with no access", func() {
			ownerID := "get-perm-no-access-owner-" + uuid.New().String()[:8]
			noAccessID := "get-perm-no-access-" + uuid.New().String()[:8]
			orgID := "get-perm-no-access-org-" + uuid.New().String()[:8]
			assessmentID := uuid.New().String()

			owner := auth.User{Username: ownerID, Organization: orgID}
			noAccess := auth.User{Username: noAccessID, Organization: orgID}

			err := authzSvc.CreateUser(ctx, owner)
			Expect(err).To(BeNil())
			err = authzSvc.CreateUser(ctx, noAccess)
			Expect(err).To(BeNil())

			err = authzSvc.CreateAssessmentRelationship(ctx, assessmentID, owner)
			Expect(err).To(BeNil())

			permissions, err := authzSvc.GetPermissions(ctx, assessmentID, noAccess)
			Expect(err).To(BeNil())
			Expect(permissions).To(BeEmpty())
		})

		AfterEach(func() {
			cleanupSpiceDB()
		})
	})

	Context("GetBulkPermissions", func() {
		// Test: Validates bulk permission retrieval for multiple assessments
		// Expected: Returns map with different permissions for each assessment
		// Purpose: Tests bulk operation
		It("should return permissions for multiple assessments", func() {
			ownerID := "bulk-owner-" + uuid.New().String()[:8]
			orgID := "bulk-org-" + uuid.New().String()[:8]
			assessment1ID := uuid.New().String()
			assessment2ID := uuid.New().String()
			assessment3ID := uuid.New().String()

			owner := auth.User{Username: ownerID, Organization: orgID}

			err := authzSvc.CreateUser(ctx, owner)
			Expect(err).To(BeNil())

			// Assessment 1: owner
			err = authzSvc.CreateAssessmentRelationship(ctx, assessment1ID, owner)
			Expect(err).To(BeNil())

			// Assessment 2: viewer
			otherOwnerID := "bulk-other-owner-" + uuid.New().String()[:8]
			otherOwner := auth.User{Username: otherOwnerID, Organization: orgID}
			err = authzSvc.CreateUser(ctx, otherOwner)
			Expect(err).To(BeNil())
			err = authzSvc.CreateAssessmentRelationship(ctx, assessment2ID, otherOwner)
			Expect(err).To(BeNil())
			err = authzSvc.WriteRelationships(ctx,
				model.NewRelationship(assessment2ID, model.NewUserSubject(ownerID), model.ViewerRelationshipKind),
			)
			Expect(err).To(BeNil())

			// Assessment 3: no access
			err = authzSvc.CreateAssessmentRelationship(ctx, assessment3ID, otherOwner)
			Expect(err).To(BeNil())

			// Get bulk permissions
			permissionsMap, err := authzSvc.GetBulkPermissions(ctx, []string{assessment1ID, assessment2ID, assessment3ID}, owner)
			Expect(err).To(BeNil())

			// Verify assessment1 permissions (owner)
			Expect(permissionsMap[assessment1ID]).To(ContainElement(model.DeletePermission))

			// Verify assessment2 permissions (viewer)
			Expect(permissionsMap[assessment2ID]).To(ContainElement(model.ReadPermission))
			Expect(permissionsMap[assessment2ID]).ToNot(ContainElement(model.DeletePermission))

			// Verify assessment3 permissions (none)
			Expect(permissionsMap[assessment3ID]).To(BeEmpty())
		})

		// Test: Validates empty assessment list
		// Expected: Returns empty map
		// Purpose: Tests edge case
		It("should return empty map for empty assessment list", func() {
			userID := "bulk-empty-" + uuid.New().String()[:8]
			orgID := "bulk-empty-org-" + uuid.New().String()[:8]

			user := auth.User{Username: userID, Organization: orgID}

			err := authzSvc.CreateUser(ctx, user)
			Expect(err).To(BeNil())

			permissionsMap, err := authzSvc.GetBulkPermissions(ctx, []string{}, user)
			Expect(err).To(BeNil())
			Expect(permissionsMap).To(BeEmpty())
		})

		AfterEach(func() {
			cleanupSpiceDB()
		})
	})

	Context("HasPermission", func() {
		// Test: Validates HasPermission returns true for granted permission
		// Expected: True for owner with delete permission
		// Purpose: Tests positive case
		It("should return true when user has the specified permission", func() {
			ownerID := "has-perm-owner-" + uuid.New().String()[:8]
			orgID := "has-perm-org-" + uuid.New().String()[:8]
			assessmentID := uuid.New().String()

			owner := auth.User{Username: ownerID, Organization: orgID}

			err := authzSvc.CreateUser(ctx, owner)
			Expect(err).To(BeNil())

			err = authzSvc.CreateAssessmentRelationship(ctx, assessmentID, owner)
			Expect(err).To(BeNil())

			// Owner should have delete permission
			hasPermission, err := authzSvc.HasPermission(ctx, assessmentID, owner, model.DeletePermission)
			Expect(err).To(BeNil())
			Expect(hasPermission).To(BeTrue())
		})

		// Test: Validates HasPermission returns false for non-granted permission
		// Expected: False for viewer with delete permission
		// Purpose: Tests negative case
		It("should return false when user does not have the specified permission", func() {
			ownerID := "has-perm-no-owner-" + uuid.New().String()[:8]
			viewerID := "has-perm-no-viewer-" + uuid.New().String()[:8]
			orgID := "has-perm-no-org-" + uuid.New().String()[:8]
			assessmentID := uuid.New().String()

			owner := auth.User{Username: ownerID, Organization: orgID}
			viewer := auth.User{Username: viewerID, Organization: orgID}

			err := authzSvc.CreateUser(ctx, owner)
			Expect(err).To(BeNil())
			err = authzSvc.CreateUser(ctx, viewer)
			Expect(err).To(BeNil())

			err = authzSvc.CreateAssessmentRelationship(ctx, assessmentID, owner)
			Expect(err).To(BeNil())
			err = authzSvc.WriteRelationships(ctx,
				model.NewRelationship(assessmentID, model.NewUserSubject(viewerID), model.ViewerRelationshipKind),
			)
			Expect(err).To(BeNil())

			// Reader should NOT have delete permission
			hasPermission, err := authzSvc.HasPermission(ctx, assessmentID, viewer, model.DeletePermission)
			Expect(err).To(BeNil())
			Expect(hasPermission).To(BeFalse())
		})

		// Test: Validates all permission types
		// Expected: Correct true/false for each permission type
		// Purpose: Tests comprehensive permission checking
		It("should correctly check all permission types", func() {
			ownerID := "has-perm-all-owner-" + uuid.New().String()[:8]
			viewerID := "has-perm-all-viewer-" + uuid.New().String()[:8]
			orgID := "has-perm-all-org-" + uuid.New().String()[:8]
			assessmentID := uuid.New().String()

			owner := auth.User{Username: ownerID, Organization: orgID}
			viewer := auth.User{Username: viewerID, Organization: orgID}

			err := authzSvc.CreateUser(ctx, owner)
			Expect(err).To(BeNil())
			err = authzSvc.CreateUser(ctx, viewer)
			Expect(err).To(BeNil())

			err = authzSvc.CreateAssessmentRelationship(ctx, assessmentID, owner)
			Expect(err).To(BeNil())
			err = authzSvc.WriteRelationships(ctx,
				model.NewRelationship(assessmentID, model.NewUserSubject(viewerID), model.ViewerRelationshipKind),
			)
			Expect(err).To(BeNil())

			// Owner should have all permissions
			hasRead, _ := authzSvc.HasPermission(ctx, assessmentID, owner, model.ReadPermission)
			hasEdit, _ := authzSvc.HasPermission(ctx, assessmentID, owner, model.EditPermission)
			hasShare, _ := authzSvc.HasPermission(ctx, assessmentID, owner, model.SharePermission)
			hasDelete, _ := authzSvc.HasPermission(ctx, assessmentID, owner, model.DeletePermission)

			Expect(hasRead).To(BeTrue())
			Expect(hasEdit).To(BeTrue())
			Expect(hasShare).To(BeTrue())
			Expect(hasDelete).To(BeTrue())

			// Reader should only have read
			viewerHasRead, _ := authzSvc.HasPermission(ctx, assessmentID, viewer, model.ReadPermission)
			viewerHasEdit, _ := authzSvc.HasPermission(ctx, assessmentID, viewer, model.EditPermission)
			viewerHasShare, _ := authzSvc.HasPermission(ctx, assessmentID, viewer, model.SharePermission)
			viewerHasDelete, _ := authzSvc.HasPermission(ctx, assessmentID, viewer, model.DeletePermission)

			Expect(viewerHasRead).To(BeTrue())
			Expect(viewerHasEdit).To(BeFalse())
			Expect(viewerHasShare).To(BeFalse())
			Expect(viewerHasDelete).To(BeFalse())
		})

		AfterEach(func() {
			cleanupSpiceDB()
		})
	})
})

// hash function matching the one in authz.go
// Purpose: Creates consistent 12-character SHA256 hashes for user/org IDs as used by the authz service
func hash(s string) string {
	h := sha256.New()
	h.Write([]byte(s))
	return fmt.Sprintf("%x", h.Sum(nil))[:12]
}
