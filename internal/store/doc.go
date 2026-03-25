// # Authorization Store — Design & Rationale
//
// ## Why
//
// The initial authorization model filtered resources by user ID to return
// only the caller's own resources. The partner feature introduces the need
// for customers to share resources (assessments) with partner organizations.
// This requires evolving from simple ownership filtering to a relationship-
// based authorization model where access is expressed as tuples and
// permissions are derived from those relationships.
//
// The first implementation uses a PostgreSQL relations table with Go-side
// permission resolution. The store and service interfaces follow the same
// mental model as SpiceDB/Kessel (resources, relations, subjects, permission
// resolution) so that when we migrate to Kessel/SpiceDB the service layer
// should not change significantly — only the store implementation swaps out.
//
// ## Model
//
// All authorization data lives in the relations table:
//
//	relations (
//	    resource           — resource type (e.g. "assessment", "org")
//	    resource_id        — resource instance ID
//	    relation           — relationship name: owner, viewer, member
//	    subject_namespace  — subject type: user, org
//	    subject_id         — subject instance ID
//	)
//
// A row represents a single tuple:
//
//	resource_type:resource_id#relation@subject_type:subject_id
//
// ### Relation types
//
//   - owner  — full control over a resource (read, edit, share, delete)
//   - viewer — read-only access to a resource
//   - member — org membership (resource=org, subject_namespace=user)
//
// ### Subject namespaces
//
//   - user — a direct user reference
//   - org  — an org reference; access is expanded to all org members
//
// When a resource is shared with an org (subject_namespace=org), every user
// who is a member of that org automatically has access, resolved at query time
// via a JOIN on the same table.
//
// ## Permission Resolution
//
// Permissions are derived from relations in Go code (not stored):
//
//   - owner  → read, edit, share, delete
//   - viewer → read
//
// Both Relation.Permissions() and Permission.Relations() encode these rules
// as methods on the model types.
//
// ListResources and GetPermissions resolve access through two paths:
//
//  1. Direct: the user is an owner or viewer of the resource
//  2. Indirect: the resource is shared with an org the user is a member of
//
// Both paths are combined in a single SQL UNION query. The returned Resource
// carries the resolved Permissions alongside Type and ID.
//
// ## Extensibility
//
// The builder API (With/Without/Build) and the Authz interface are
// backend-agnostic. They operate on model.Resource, model.Relation, and
// model.Subject — no SQL or storage details leak into callers. To add a new
// storage backend (e.g. SpiceDB), implement the Authz interface and swap the
// constructor in NewStoreWithAuthz. New relation types and permissions can be
// added by extending the model constants and the resolution methods.
//
// # Usage
//
// ## Writing relationships
//
// Use the RelationshipBuilder to construct a batch of relationship updates,
// then pass them to WriteRelationships:
//
//	updates := store.NewRelationshipBuilder().
//	    With(model.NewAssessmentResource(assessmentID), model.OwnerRelation, model.NewUserSubject(userID)).
//	    Build()
//
//	ctx, _ = s.NewTransactionContext(ctx)
//	err := s.Authz().WriteRelationships(ctx, updates)
//	ctx, _ = store.Commit(ctx)
//
// ## Sharing with another user
//
//	updates := store.NewRelationshipBuilder().
//	    With(model.NewAssessmentResource(assessmentID), model.ViewerRelation, model.NewUserSubject(targetUserID)).
//	    Build()
//
//	ctx, _ = s.NewTransactionContext(ctx)
//	err := s.Authz().WriteRelationships(ctx, updates)
//	ctx, _ = store.Commit(ctx)
//
// ## Sharing with a partner org
//
//	updates := store.NewRelationshipBuilder().
//	    With(model.NewAssessmentResource(assessmentID), model.ViewerRelation, model.NewOrgSubject(partnerOrgID)).
//	    Build()
//
// ## Adding org members
//
//	updates := store.NewRelationshipBuilder().
//	    With(model.NewOrgResource(orgID), model.MemberRelation, model.NewUserSubject(userID)).
//	    Build()
//
// ## Revoking access
//
//	updates := store.NewRelationshipBuilder().
//	    Without(model.NewAssessmentResource(assessmentID), model.ViewerRelation, model.NewUserSubject(targetUserID)).
//	    Build()
//
//	ctx, _ = s.NewTransactionContext(ctx)
//	err := s.Authz().WriteRelationships(ctx, updates)
//	ctx, _ = store.Commit(ctx)
//
// ## Listing all assessments a user can access (with permissions)
//
//	ctx, _ = s.NewTransactionContext(ctx)
//	resources, err := s.Authz().ListResources(ctx, userID, model.AssessmentResource)
//	ctx, _ = store.Commit(ctx)
//	// resources[0].ID          => "assess1"
//	// resources[0].Permissions => [ReadPermission, EditPermission, SharePermission, DeletePermission]
//
// ## Checking permissions on a single resource
//
//	ctx, _ = s.NewTransactionContext(ctx)
//	resource, err := s.Authz().GetPermissions(ctx, userID, model.NewAssessmentResource("assess1"))
//	ctx, _ = store.Commit(ctx)
//	// resource.Permissions => [ReadPermission]
//
// ## Deleting all relationships for a resource
//
//	ctx, _ = s.NewTransactionContext(ctx)
//	err := s.Authz().DeleteRelationships(ctx, model.NewAssessmentResource(assessmentID))
//	ctx, _ = store.Commit(ctx)
//
// ## Listing relationships on a resource
//
//	ctx, _ = s.NewTransactionContext(ctx)
//	rels, err := s.Authz().ListRelationships(ctx, model.NewAssessmentResource(assessmentID))
//	ctx, _ = store.Commit(ctx)
//	// rels[0].String() => "assessment:assess1#owner@user:jane"
package store
