package model

import (
	"fmt"
	"slices"
)

// ResourceType identifies the type of resource in a relationship tuple.
type ResourceType string

const (
	AssessmentResource ResourceType = "assessment"
	OrgResource        ResourceType = "org"
)

// SubjectType identifies the type of subject in a relationship tuple.
type SubjectType string

const (
	UserSubject SubjectType = "user"
	OrgSubject  SubjectType = "org"
)

// Relation represents a stored relation between a resource and a subject.
type Relation string

const (
	OwnerRelation  Relation = "owner"
	ViewerRelation Relation = "viewer"
	MemberRelation Relation = "member"
)

// Permission represents a computed permission derived from relations.
type Permission string

const (
	ReadPermission   Permission = "read"
	EditPermission   Permission = "edit"
	SharePermission  Permission = "share"
	DeletePermission Permission = "delete"
)

// In returns true if the permission is present in the given slice.
func (p Permission) In(perms []Permission) bool {
	return slices.Contains(perms, p)
}

// Permissions returns the permissions that this relation grants.
func (r Relation) Permissions() []Permission {
	switch r {
	case OwnerRelation:
		return []Permission{ReadPermission, EditPermission, SharePermission, DeletePermission}
	case ViewerRelation:
		return []Permission{ReadPermission}
	default:
		return nil
	}
}

// Relations returns the relation types that grant this permission.
func (p Permission) Relations() []string {
	switch p {
	case ReadPermission:
		return []string{string(OwnerRelation), string(ViewerRelation)}
	case EditPermission, SharePermission, DeletePermission:
		return []string{string(OwnerRelation)}
	default:
		return nil
	}
}

// Subject represents the "who" in a relationship.
type Subject struct {
	ID   string
	Kind SubjectType
}

// Relationship represents a single tuple: resource_type:resource_id#relation@subject_type:subject_id
type Relationship struct {
	ResourceType ResourceType
	ResourceID   string
	Relation     Relation
	Subject      Subject
}

func (r Relationship) String() string {
	return fmt.Sprintf("%s:%s#%s@%s:%s",
		r.ResourceType, r.ResourceID,
		r.Relation,
		r.Subject.Kind, r.Subject.ID,
	)
}

func NewRelationship(resourceType ResourceType, resourceID string, relation Relation, subject Subject) Relationship {
	return Relationship{
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Relation:     relation,
		Subject:      subject,
	}
}

// Resource identifies a resource. Permissions is populated by query methods
// (ListResources, GetPermissions) and nil on input (DeleteRelationships, builder).
type Resource struct {
	Type        ResourceType
	ID          string
	Permissions []Permission
}

// Resource constructors

func NewAssessmentResource(id string) Resource { return Resource{Type: AssessmentResource, ID: id} }
func NewOrgResource(id string) Resource        { return Resource{Type: OrgResource, ID: id} }

// Subject constructors

func NewUserSubject(id string) Subject { return Subject{Kind: UserSubject, ID: id} }
func NewOrgSubject(id string) Subject  { return Subject{Kind: OrgSubject, ID: id} }

// Operation represents a write operation type.
type Operation int

const (
	OperationTouch  Operation = iota // create or idempotent update (upsert)
	OperationDelete                  // delete
)

// RelationshipUpdate pairs an operation with a relationship for batch writes.
type RelationshipUpdate struct {
	Operation    Operation
	Relationship Relationship
}

// RelationSqlModel is the GORM model for the relations table.
type RelationSqlModel struct {
	ID               int64  `gorm:"primaryKey;autoIncrement"`
	Resource         string `gorm:"not null;uniqueIndex:uq_resource_relation"`
	ResourceID       string `gorm:"column:resource_id;not null;uniqueIndex:uq_resource_relation"`
	Relation         string `gorm:"not null;uniqueIndex:uq_resource_relation"`
	SubjectNamespace string `gorm:"column:subject_namespace;not null;uniqueIndex:uq_resource_relation"`
	SubjectID        string `gorm:"column:subject_id;not null;uniqueIndex:uq_resource_relation"`
}

func (RelationSqlModel) TableName() string { return "relations" }

func (r RelationSqlModel) ToRelationship() Relationship {
	return Relationship{
		ResourceType: ResourceType(r.Resource),
		ResourceID:   r.ResourceID,
		Relation:     Relation(r.Relation),
		Subject: Subject{
			Kind: SubjectType(r.SubjectNamespace),
			ID:   r.SubjectID,
		},
	}
}

func (rel Relationship) ToSql() RelationSqlModel {
	return RelationSqlModel{
		Resource:         string(rel.ResourceType),
		ResourceID:       rel.ResourceID,
		Relation:         string(rel.Relation),
		SubjectNamespace: string(rel.Subject.Kind),
		SubjectID:        rel.Subject.ID,
	}
}
