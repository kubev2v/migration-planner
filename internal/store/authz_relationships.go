package store

import (
	v1pb "github.com/authzed/authzed-go/proto/authzed/api/v1"

	"github.com/kubev2v/migration-planner/internal/store/model"
)

// WithOwnerRelationship creates a relationship function that adds an owner relationship.
// Owner relationships grant full control over an assessment (read, edit, share, delete permissions).
//
// Parameters:
//   - assessmentID: The ID of the assessment to grant ownership on
//   - subject: The subject (user only) to grant ownership to
//
// Returns:
//   - model.RelationshipFn: A function that can be used with WriteRelationships
//
// Note:
//   - Returns RelationshipOpUpdate (tracked in database)
//   - Only accepts User subjects; Organization subjects are not valid for owner relation
//
// Example:
//
//	userSubject := model.NewUserSubject("user123")
//	err := authzService.WriteRelationships(ctx, WithOwnerRelationship("assessment789", userSubject))
//	if err != nil {
//	    log.Printf("Failed to grant ownership: %v", err)
//	}
func WithOwnerRelationship(assessmentID string, subject model.Subject) model.RelationshipFn {
	return func(updates []*v1pb.RelationshipUpdate) ([]*v1pb.RelationshipUpdate, model.Relationship, model.RelationshipOp) {
		if subject.Kind != model.User {
			return updates, model.Relationship{}, model.RelationshipOpIgnore
		}
		relationshipUpdate := &v1pb.RelationshipUpdate{
			Operation: v1pb.RelationshipUpdate_OPERATION_TOUCH,
			Relationship: &v1pb.Relationship{
				Resource: &v1pb.ObjectReference{
					ObjectType: model.AssessmentObject,
					ObjectId:   assessmentID,
				},
				Relation: model.OwnerRelationshipKind.String(),
				Subject: &v1pb.SubjectReference{
					Object: &v1pb.ObjectReference{
						ObjectType: subject.Kind.String(),
						ObjectId:   subject.GeneratedID,
					},
				},
			},
		}

		relationship := model.NewRelationship(assessmentID, subject, model.OwnerRelationshipKind)

		return append(updates, relationshipUpdate), relationship, model.RelationshipOpUpdate
	}
}

// WithViewerRelationship creates a relationship function that adds a viewer relationship.
// Viewer relationships grant read-only access to an assessment (read permission only).
//
// Parameters:
//   - assessmentID: The ID of the assessment to grant read access on
//   - subject: The subject (user only) to grant read access to
//
// Returns:
//   - model.RelationshipFn: A function that can be used with WriteRelationships
//
// Note:
//   - Returns RelationshipOpUpdate (tracked in database)
//   - Only accepts User subjects; Organization subjects are not valid for viewer relation
//
// Example:
//
//	userSubject := model.NewUserSubject("user456")
//	err := authzService.WriteRelationships(ctx, WithViewerRelationship("assessment789", userSubject))
//	if err != nil {
//	    log.Printf("Failed to grant read access: %v", err)
//	}
func WithViewerRelationship(assessmentID string, subject model.Subject) model.RelationshipFn {
	return func(updates []*v1pb.RelationshipUpdate) ([]*v1pb.RelationshipUpdate, model.Relationship, model.RelationshipOp) {
		if subject.Kind != model.User {
			return updates, model.Relationship{}, model.RelationshipOpIgnore
		}
		relationshipUpdate := &v1pb.RelationshipUpdate{
			Operation: v1pb.RelationshipUpdate_OPERATION_TOUCH,
			Relationship: &v1pb.Relationship{
				Resource: &v1pb.ObjectReference{
					ObjectType: model.AssessmentObject,
					ObjectId:   assessmentID,
				},
				Relation: model.ViewerRelationshipKind.String(),
				Subject: &v1pb.SubjectReference{
					Object: &v1pb.ObjectReference{
						ObjectType: subject.Kind.String(),
						ObjectId:   subject.GeneratedID,
					},
				},
			},
		}

		relationship := model.NewRelationship(assessmentID, subject, model.ViewerRelationshipKind)

		return append(updates, relationshipUpdate), relationship, model.RelationshipOpUpdate
	}
}

// WithEditorRelationship creates a relationship function that adds an editor relationship.
// Editor relationships grant read and edit access to an assessment (read and edit permissions).
//
// Parameters:
//   - assessmentID: The ID of the assessment to grant edit access on
//   - subject: The subject (user only) to grant edit access to
//
// Returns:
//   - model.RelationshipFn: A function that can be used with WriteRelationships
//
// Note:
//   - Returns RelationshipOpUpdate (tracked in database)
//   - Only accepts User subjects; Organization subjects are not valid for editor relation
//
// Example:
//
//	userSubject := model.NewUserSubject("user456")
//	err := authzService.WriteRelationships(ctx, WithEditorRelationship("assessment789", userSubject))
//	if err != nil {
//	    log.Printf("Failed to grant edit access: %v", err)
//	}
func WithEditorRelationship(assessmentID string, subject model.Subject) model.RelationshipFn {
	return func(updates []*v1pb.RelationshipUpdate) ([]*v1pb.RelationshipUpdate, model.Relationship, model.RelationshipOp) {
		if subject.Kind != model.User {
			return updates, model.Relationship{}, model.RelationshipOpIgnore
		}
		relationshipUpdate := &v1pb.RelationshipUpdate{
			Operation: v1pb.RelationshipUpdate_OPERATION_TOUCH,
			Relationship: &v1pb.Relationship{
				Resource: &v1pb.ObjectReference{
					ObjectType: model.AssessmentObject,
					ObjectId:   assessmentID,
				},
				Relation: model.EditorRelationshipKind.String(),
				Subject: &v1pb.SubjectReference{
					Object: &v1pb.ObjectReference{
						ObjectType: subject.Kind.String(),
						ObjectId:   subject.GeneratedID,
					},
				},
			},
		}

		relationship := model.NewRelationship(assessmentID, subject, model.EditorRelationshipKind)

		return append(updates, relationshipUpdate), relationship, model.RelationshipOpUpdate
	}
}

// WithOrganizationRelationship creates a relationship function that associates an assessment with an organization.
// Organization relationships grant all org members read and edit permissions on the assessment.
//
// Parameters:
//   - assessmentID: The ID of the assessment to associate with the organization
//   - subject: The organization subject (must be of type Organization)
//
// Returns:
//   - model.RelationshipFn: A function that can be used with WriteRelationships
//
// Note:
//   - Returns RelationshipOpUpdate (tracked in database)
//   - Only accepts Organization subjects; returns RelationshipOpIgnore for other types
//   - All members of the organization automatically get read+edit permissions
//
// Example:
//
//	orgSubject := model.NewOrganizationSubject("org123")
//	err := authzService.WriteRelationships(ctx, WithOrganizationRelationship("assessment789", orgSubject))
//	if err != nil {
//	    log.Printf("Failed to associate assessment with organization: %v", err)
//	}
func WithOrganizationRelationship(assessmentID string, subject model.Subject) model.RelationshipFn {
	return func(updates []*v1pb.RelationshipUpdate) ([]*v1pb.RelationshipUpdate, model.Relationship, model.RelationshipOp) {
		if subject.Kind != model.Organization {
			return updates, model.Relationship{}, model.RelationshipOpIgnore
		}
		relationshipUpdate := &v1pb.RelationshipUpdate{
			Operation: v1pb.RelationshipUpdate_OPERATION_TOUCH,
			Relationship: &v1pb.Relationship{
				Resource: &v1pb.ObjectReference{
					ObjectType: model.AssessmentObject,
					ObjectId:   assessmentID,
				},
				Relation: model.OrganizationRelationshipKind.String(),
				Subject: &v1pb.SubjectReference{
					Object: &v1pb.ObjectReference{
						ObjectType: model.Organization.String(),
						ObjectId:   subject.GeneratedID,
					},
				},
			},
		}

		relationship := model.NewRelationship(assessmentID, subject, model.OrganizationRelationshipKind)

		return append(updates, relationshipUpdate), relationship, model.RelationshipOpUpdate
	}
}

// WithPlatformRelationship creates relationship functions that add users to a platform with specific roles.
// This establishes role relationships (admin, viewer, editor) that cascade permissions to assessments.
//
// Parameters:
//   - platformID: The ID of the platform
//   - subjects: A map where keys are role names ("admin", "viewer", "editor") and values are slices of user subjects
//
// Returns:
//   - model.RelationshipFn: A function that can be used with WriteRelationships
//
// Note:
//   - Returns RelationshipOpIgnore (NOT tracked in database)
//   - Only accepts User subjects; non-user subjects are skipped
//   - Platform roles cascade to assessments via parent relationship:
//   - admin -> super_admin permission (read, edit, share, delete)
//   - editor -> edit permission (read, edit)
//   - viewer -> view permission (read)
//
// Example:
//
//	adminUser := model.NewUserSubject("admin123")
//	viewerUser := model.NewUserSubject("viewer456")
//	subjects := map[string][]model.Subject{
//	    "admin": {adminUser},
//	    "viewer": {viewerUser},
//	}
//	err := authzService.WriteRelationships(ctx, WithPlatformRelationship("platform789", subjects))
//	if err != nil {
//	    log.Printf("Failed to establish platform relationships: %v", err)
//	}
func WithPlatformRelationship(platform model.Subject, subjects map[string][]model.Subject) model.RelationshipFn {
	return func(updates []*v1pb.RelationshipUpdate) ([]*v1pb.RelationshipUpdate, model.Relationship, model.RelationshipOp) {
		if platform.Kind != model.Platform {
			return updates, model.Relationship{}, model.RelationshipOpIgnore
		}

		for relation, subjectList := range subjects {
			for _, subject := range subjectList {
				if subject.Kind != model.User {
					continue // Platform relations only accept users
				}
				roleUpdate := &v1pb.RelationshipUpdate{
					Operation: v1pb.RelationshipUpdate_OPERATION_TOUCH,
					Relationship: &v1pb.Relationship{
						Resource: &v1pb.ObjectReference{
							ObjectType: model.PlatformObject,
							ObjectId:   platform.GeneratedID,
						},
						Relation: relation,
						Subject: &v1pb.SubjectReference{
							Object: &v1pb.ObjectReference{
								ObjectType: model.UserObject,
								ObjectId:   subject.GeneratedID,
							},
						},
					},
				}
				updates = append(updates, roleUpdate)
			}
		}

		// We don't track this relationship because we don't want to show it to the user in UI.
		return updates, model.Relationship{}, model.RelationshipOpIgnore
	}
}

// WithParentRelationship creates a relationship function that associates an assessment with a platform.
// This establishes the parent relationship that enables platform-level permission inheritance.
//
// Parameters:
//   - assessmentID: The ID of the assessment
//   - subject: The platform subject (must be of type Platform)
//
// Returns:
//   - model.RelationshipFn: A function that can be used with WriteRelationships
//
// Note:
//   - Returns RelationshipOpIgnore (NOT tracked in database)
//   - Only accepts Platform subjects; returns RelationshipOpIgnore for other types
//   - Enables platform admin/editor/viewer permissions to cascade to the assessment
//
// Example:
//
//	platformSubject := model.NewPlatformSubject("platform789")
//	err := authzService.WriteRelationships(ctx, WithParentRelationship("assessment123", platformSubject))
//	if err != nil {
//	    log.Printf("Failed to establish parent relationship: %v", err)
//	}
func WithParentRelationship(assessmentID string, subject model.Subject) model.RelationshipFn {
	return func(updates []*v1pb.RelationshipUpdate) ([]*v1pb.RelationshipUpdate, model.Relationship, model.RelationshipOp) {
		if subject.Kind != model.Platform {
			return updates, model.Relationship{}, model.RelationshipOpIgnore
		}

		relationshipUpdate := &v1pb.RelationshipUpdate{
			Operation: v1pb.RelationshipUpdate_OPERATION_TOUCH,
			Relationship: &v1pb.Relationship{
				Resource: &v1pb.ObjectReference{
					ObjectType: model.AssessmentObject,
					ObjectId:   assessmentID,
				},
				Relation: model.ParentRelationshipKind.String(),
				Subject: &v1pb.SubjectReference{
					Object: &v1pb.ObjectReference{
						ObjectType: model.PlatformObject,
						ObjectId:   subject.GeneratedID,
					},
				},
			},
		}

		relationship := model.NewRelationship(assessmentID, subject, model.ParentRelationshipKind)

		return append(updates, relationshipUpdate), relationship, model.RelationshipOpIgnore
	}
}

// WithMemberRelationship creates a relationship function that adds a user to an organization.
// This establishes membership that grants access to all assessments associated with the organization.
//
// Parameters:
//   - user: The user subject to add to the organization
//   - org: The organization subject to add the user to
//
// Returns:
//   - model.RelationshipFn: A function that can be used with WriteRelationships
//
// Note:
//   - Returns RelationshipOpTouch (tracked differently than assessment relationships)
//   - Organization members automatically get read+edit permissions on org assessments
//
// Example:
//
//	userSubject := model.NewUserSubject("user123")
//	orgSubject := model.NewOrganizationSubject("org456")
//	err := authzService.WriteRelationships(ctx, WithMemberRelationship(userSubject, orgSubject))
//	if err != nil {
//	    log.Printf("Failed to add user to organization: %v", err)
//	}
//
//	// Can also be combined with other relationship operations:
//	err = authzService.WriteRelationships(ctx,
//	    WithMemberRelationship(userSubject, orgSubject),
//	    WithOwnerRelationship("assessment789", userSubject),
//	)
func WithMemberRelationship(user, org model.Subject) model.RelationshipFn {
	return func(updates []*v1pb.RelationshipUpdate) ([]*v1pb.RelationshipUpdate, model.Relationship, model.RelationshipOp) {
		if user.Kind != model.User || org.Kind != model.Organization {
			return updates, model.Relationship{}, model.RelationshipOpIgnore
		}
		orgObject := model.OrgObject

		relationshipUpdate := &v1pb.RelationshipUpdate{
			Operation: v1pb.RelationshipUpdate_OPERATION_TOUCH,
			Relationship: &v1pb.Relationship{
				Resource: &v1pb.ObjectReference{
					ObjectType: orgObject,
					ObjectId:   org.GeneratedID,
				},
				Relation: model.MemberRelationshipKind.String(),
				Subject: &v1pb.SubjectReference{
					Object: &v1pb.ObjectReference{
						ObjectType: model.UserObject,
						ObjectId:   user.GeneratedID,
					},
				},
			},
		}

		relationship := model.NewRelationship(org.GeneratedID, user, model.MemberRelationshipKind)

		return append(updates, relationshipUpdate), relationship, model.RelationshipOpTouch
	}
}

// WithoutOwnerRelationship creates a relationship function that removes an owner relationship.
// This revokes ownership and all associated permissions (read, edit, share, delete).
//
// Parameters:
//   - assessmentID: The ID of the assessment to remove ownership from
//   - subject: The user subject to remove ownership from
//
// Returns:
//   - model.RelationshipFn: A function that can be used with WriteRelationships
//
// Note:
//   - Returns RelationshipOpDelete (removed from database)
//   - Use this to revoke full control from a user
//
// Example:
//
//	userSubject := model.NewUserSubject("user123")
//	err := authzService.WriteRelationships(ctx, WithoutOwnerRelationship("assessment789", userSubject))
//	if err != nil {
//	    log.Printf("Failed to remove ownership: %v", err)
//	}
func WithoutOwnerRelationship(assessmentID string, subject model.Subject) model.RelationshipFn {
	return func(updates []*v1pb.RelationshipUpdate) ([]*v1pb.RelationshipUpdate, model.Relationship, model.RelationshipOp) {
		if subject.Kind != model.User {
			return updates, model.Relationship{}, model.RelationshipOpIgnore
		}

		relationshipUpdate := &v1pb.RelationshipUpdate{
			Operation: v1pb.RelationshipUpdate_OPERATION_DELETE,
			Relationship: &v1pb.Relationship{
				Resource: &v1pb.ObjectReference{
					ObjectType: model.AssessmentObject,
					ObjectId:   assessmentID,
				},
				Relation: model.OwnerRelationshipKind.String(),
				Subject: &v1pb.SubjectReference{
					Object: &v1pb.ObjectReference{
						ObjectType: subject.Kind.String(),
						ObjectId:   subject.GeneratedID,
					},
				},
			},
		}

		relationship := model.NewRelationship(assessmentID, subject, model.OwnerRelationshipKind)

		return append(updates, relationshipUpdate), relationship, model.RelationshipOpDelete
	}
}

// WithoutViewerRelationship creates a relationship function that removes a viewer relationship.
// This revokes read-only access to an assessment.
//
// Parameters:
//   - assessmentID: The ID of the assessment to remove read access from
//   - subject: The user subject to remove read access from
//
// Returns:
//   - model.RelationshipFn: A function that can be used with WriteRelationships
//
// Note:
//   - Returns RelationshipOpDelete (removed from database)
//   - Use this to revoke read-only access from a user
//
// Example:
//
//	userSubject := model.NewUserSubject("user456")
//	err := authzService.WriteRelationships(ctx, WithoutViewerRelationship("assessment789", userSubject))
//	if err != nil {
//	    log.Printf("Failed to remove read access: %v", err)
//	}
func WithoutViewerRelationship(assessmentID string, subject model.Subject) model.RelationshipFn {
	return func(updates []*v1pb.RelationshipUpdate) ([]*v1pb.RelationshipUpdate, model.Relationship, model.RelationshipOp) {
		if subject.Kind != model.User {
			return updates, model.Relationship{}, model.RelationshipOpIgnore
		}
		relationshipUpdate := &v1pb.RelationshipUpdate{
			Operation: v1pb.RelationshipUpdate_OPERATION_DELETE,
			Relationship: &v1pb.Relationship{
				Resource: &v1pb.ObjectReference{
					ObjectType: model.AssessmentObject,
					ObjectId:   assessmentID,
				},
				Relation: model.ViewerRelationshipKind.String(),
				Subject: &v1pb.SubjectReference{
					Object: &v1pb.ObjectReference{
						ObjectType: subject.Kind.String(),
						ObjectId:   subject.GeneratedID,
					},
				},
			},
		}

		relationship := model.NewRelationship(assessmentID, subject, model.ViewerRelationshipKind)

		return append(updates, relationshipUpdate), relationship, model.RelationshipOpDelete
	}
}

// WithoutEditorRelationship creates a relationship function that removes an editor relationship.
// This revokes read and edit access to an assessment.
//
// Parameters:
//   - assessmentID: The ID of the assessment to remove edit access from
//   - subject: The user subject to remove edit access from
//
// Returns:
//   - model.RelationshipFn: A function that can be used with WriteRelationships
//
// Note:
//   - Returns RelationshipOpDelete (removed from database)
//   - Use this to revoke edit access from a user
//
// Example:
//
//	userSubject := model.NewUserSubject("user456")
//	err := authzService.WriteRelationships(ctx, WithoutEditorRelationship("assessment789", userSubject))
//	if err != nil {
//	    log.Printf("Failed to remove edit access: %v", err)
//	}
func WithoutEditorRelationship(assessmentID string, subject model.Subject) model.RelationshipFn {
	return func(updates []*v1pb.RelationshipUpdate) ([]*v1pb.RelationshipUpdate, model.Relationship, model.RelationshipOp) {
		if subject.Kind != model.User {
			return updates, model.Relationship{}, model.RelationshipOpIgnore
		}
		relationshipUpdate := &v1pb.RelationshipUpdate{
			Operation: v1pb.RelationshipUpdate_OPERATION_DELETE,
			Relationship: &v1pb.Relationship{
				Resource: &v1pb.ObjectReference{
					ObjectType: model.AssessmentObject,
					ObjectId:   assessmentID,
				},
				Relation: model.EditorRelationshipKind.String(),
				Subject: &v1pb.SubjectReference{
					Object: &v1pb.ObjectReference{
						ObjectType: subject.Kind.String(),
						ObjectId:   subject.GeneratedID,
					},
				},
			},
		}

		relationship := model.NewRelationship(assessmentID, subject, model.EditorRelationshipKind)

		return append(updates, relationshipUpdate), relationship, model.RelationshipOpDelete
	}
}

// WithoutOrganizationRelationship creates a relationship function that removes an organization relationship.
// This revokes organization-level access, removing read+edit permissions from all org members.
//
// Parameters:
//   - assessmentID: The ID of the assessment to remove organization access from
//   - subject: The organization subject to disassociate from the assessment
//
// Returns:
//   - model.RelationshipFn: A function that can be used with WriteRelationships
//
// Note:
//   - Returns RelationshipOpDelete (removed from database)
//   - Only accepts Organization subjects; returns RelationshipOpIgnore for other types
//   - All organization members lose read+edit permissions on the assessment
//
// Example:
//
//	orgSubject := model.NewOrganizationSubject("org789")
//	err := authzService.WriteRelationships(ctx, WithoutOrganizationRelationship("assessment123", orgSubject))
//	if err != nil {
//	    log.Printf("Failed to remove organization access: %v", err)
//	}
func WithoutOrganizationRelationship(assessmentID string, subject model.Subject) model.RelationshipFn {
	return func(updates []*v1pb.RelationshipUpdate) ([]*v1pb.RelationshipUpdate, model.Relationship, model.RelationshipOp) {
		if subject.Kind != model.Organization {
			return updates, model.Relationship{}, model.RelationshipOpIgnore
		}

		relationshipUpdate := &v1pb.RelationshipUpdate{
			Operation: v1pb.RelationshipUpdate_OPERATION_DELETE,
			Relationship: &v1pb.Relationship{
				Resource: &v1pb.ObjectReference{
					ObjectType: model.AssessmentObject,
					ObjectId:   assessmentID,
				},
				Relation: model.OrganizationRelationshipKind.String(),
				Subject: &v1pb.SubjectReference{
					Object: &v1pb.ObjectReference{
						ObjectType: model.Organization.String(),
						ObjectId:   subject.GeneratedID,
					},
				},
			},
		}

		relationship := model.NewRelationship(assessmentID, subject, model.OrganizationRelationshipKind)

		return append(updates, relationshipUpdate), relationship, model.RelationshipOpDelete
	}
}

// WithoutMemberRelationship creates a relationship function that removes a user from an organization.
// This revokes organization membership and all associated permissions on org assessments.
//
// Parameters:
//   - user: The user subject to remove from the organization
//   - org: The organization subject to remove the user from
//
// Returns:
//   - model.RelationshipFn: A function that can be used with WriteRelationships
//
// Note:
//   - Returns RelationshipOpDelete (removed from database)
//   - User loses access to all assessments associated with the organization
//
// Example:
//
//	userSubject := model.NewUserSubject("user123")
//	orgSubject := model.NewOrganizationSubject("org456")
//	err := authzService.WriteRelationships(ctx, WithoutMemberRelationship(userSubject, orgSubject))
//	if err != nil {
//	    log.Printf("Failed to remove user from organization: %v", err)
//	}
func WithoutMemberRelationship(user, org model.Subject) model.RelationshipFn {
	return func(updates []*v1pb.RelationshipUpdate) ([]*v1pb.RelationshipUpdate, model.Relationship, model.RelationshipOp) {
		if user.Kind != model.User || org.Kind != model.Organization {
			return updates, model.Relationship{}, model.RelationshipOpIgnore
		}
		orgObject := model.OrgObject

		relationshipUpdate := &v1pb.RelationshipUpdate{
			Operation: v1pb.RelationshipUpdate_OPERATION_DELETE,
			Relationship: &v1pb.Relationship{
				Resource: &v1pb.ObjectReference{
					ObjectType: orgObject,
					ObjectId:   org.GeneratedID,
				},
				Relation: model.MemberRelationshipKind.String(),
				Subject: &v1pb.SubjectReference{
					Object: &v1pb.ObjectReference{
						ObjectType: model.UserObject,
						ObjectId:   user.GeneratedID,
					},
				},
			},
		}

		relationship := model.NewRelationship(org.GeneratedID, user, model.MemberRelationshipKind)

		return append(updates, relationshipUpdate), relationship, model.RelationshipOpDelete
	}
}
