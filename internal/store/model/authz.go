package model

import (
	"crypto/sha256"
	"fmt"
	"time"

	v1pb "github.com/authzed/authzed-go/proto/authzed/api/v1"
)

type SubjectType int

func (s SubjectType) String() string {
	switch s {
	case User:
		return UserObject
	case Organization:
		return OrgObject
	case Platform:
		return PlatformObject
	default:
		return UserObject // default fallback
	}
}

const (
	User SubjectType = iota
	Organization
	Platform

	OrgObject        string = "org"
	UserObject       string = "user"
	AssessmentObject string = "assessment"
	PlatformObject   string = "platform"
)

type ResourceType int

func (r ResourceType) String() string {
	switch r {
	case AssessmentResource:
		return "assessment"
	case PlatformResource:
		return "platform"
	case OrgResource:
		return "org"
	default:
		return "unknown"
	}
}

const (
	AssessmentResource ResourceType = iota
	PlatformResource
	OrgResource
)

type Subject struct {
	Kind        SubjectType
	ID          string
	GeneratedID string
}

func hash(s string) string {
	h := sha256.New()
	h.Write([]byte(s))
	return fmt.Sprintf("%x", h.Sum(nil))[:12]
}

// NewUserSubject creates a new Subject with User type.
//
// Parameters:
//   - userID: The ID of the user
//
// Returns:
//   - Subject: A subject representing a user
//
// Example:
//
//	userSubject := NewUserSubject("user123")
//	ownershipFn := WithOwnerRelationship("assessment789", userSubject)
func NewUserSubject(userID string) Subject {
	return Subject{
		Kind:        User,
		ID:          userID,
		GeneratedID: hash(userID),
	}
}

// NewOrganizationSubject creates a new Subject with Organization type.
//
// Parameters:
//   - organizationID: The ID of the organization
//
// Returns:
//   - Subject: A subject representing an organization
//
// Example:
//
//	orgSubject := NewOrganizationSubject("org456")
//	viewerFn := WithViewerRelationship("assessment789", orgSubject)
func NewOrganizationSubject(organizationID string) Subject {
	return Subject{
		Kind:        Organization,
		ID:          organizationID,
		GeneratedID: hash(organizationID),
	}
}

// NewPlatformSubject creates a new Subject with Platform type.
//
// Parameters:
//   - platformID: The ID of the platform
//
// Returns:
//   - Subject: A subject representing a platform
//
// Example:
//
//	platformSubject := NewPlatformSubject("platform123")
func NewPlatformSubject(platformID string) Subject {
	return Subject{
		Kind:        Platform,
		ID:          platformID,
		GeneratedID: hash(platformID),
	}
}

type RelationshipKind int

func (r RelationshipKind) String() string {
	switch r {
	case ViewerRelationshipKind:
		return "viewer"
	case EditorRelationshipKind:
		return "editor"
	case OwnerRelationshipKind:
		return "owner"
	case OrganizationRelationshipKind:
		return "org"
	case MemberRelationshipKind:
		return "member"
	case ParentRelationshipKind:
		return "parent"
	case AdminPlatformRelationshipKind:
		return "admin"
	case EditorPlatformRelationshipKind:
		return "editor"
	case ViewerPlatformRelationshipKind:
		return "viewer"
	default:
		return "unknown"
	}
}

const (
	ViewerRelationshipKind RelationshipKind = iota
	EditorRelationshipKind
	OwnerRelationshipKind
	OrganizationRelationshipKind
	MemberRelationshipKind
	ParentRelationshipKind
	AdminPlatformRelationshipKind
	ViewerPlatformRelationshipKind
	EditorPlatformRelationshipKind
)

type Permission int

func (p Permission) String() string {
	switch p {
	case ReadPermission:
		return "read"
	case EditPermission:
		return "edit"
	case SharePermission:
		return "share"
	case DeletePermission:
		return "delete"
	default:
		return "unknown"
	}
}

const (
	ReadPermission Permission = iota
	EditPermission
	SharePermission
	DeletePermission
)

type Resource struct {
	ID           string
	GeneratedID  string
	ResourceType ResourceType
	Permissions  []Permission
}

func NewAssessmentResource(id string) Resource {
	r := Resource{
		ID:           id,
		ResourceType: AssessmentResource,
	}
	// Assessment's ID is not hashed because it is matching the spicedb regex(i.e. it's UUID)
	if id != "" {
		r.GeneratedID = id
	}
	return r
}

func NewPlatformResource(id string) Resource {
	r := Resource{
		ID:           id,
		ResourceType: PlatformResource,
	}
	if id != "" {
		r.GeneratedID = hash(id)
	}
	return r
}

type RelationshipModel struct {
	RelationID   string    `gorm:"column:relation_id;type:varchar(100);not null"`
	AssessmentID string    `gorm:"column:assessment_id;type:varchar(255);not null"`
	CreatedAt    time.Time `gorm:"column:created_at;default:now()"`
	RelationType string    `gorm:"column:relation_type;type:varchar(50);not null"`
	SubjectID    string    `gorm:"column:subject_id;type:varchar(255);not null"`
	SubjectType  string    `gorm:"column:subject_type;type:varchar(50);not null"`
}

func (r *RelationshipModel) TableName() string {
	return "relationships"
}

// ToRelationship converts a RelationshipModel from the database to a Relationship
func (rm *RelationshipModel) ToRelationship() Relationship {
	var subjectType SubjectType
	switch rm.SubjectType {
	case UserObject:
		subjectType = User
	case OrgObject:
		subjectType = Organization
	case PlatformObject:
		subjectType = Platform
	default:
		subjectType = User
	}

	subject := Subject{
		Kind: subjectType,
		ID:   rm.SubjectID,
	}

	var relationshipKind RelationshipKind
	switch rm.RelationType {
	case "viewer":
		relationshipKind = ViewerRelationshipKind
	case "editor":
		relationshipKind = EditorRelationshipKind
	case "owner":
		relationshipKind = OwnerRelationshipKind
	case "org":
		relationshipKind = OrganizationRelationshipKind
	default:
		relationshipKind = ViewerRelationshipKind
	}

	return Relationship{
		ID:           rm.RelationID,
		CreatedAt:    rm.CreatedAt,
		AssessmentID: rm.AssessmentID,
		Kind:         relationshipKind,
		Subject:      subject,
	}
}

type RelationshipOp int

const (
	RelationshipOpUpdate RelationshipOp = iota
	RelationshipOpDelete
	RelationshipOpTouch
	RelationshipOpIgnore
)

type Relationship struct {
	ID           string
	CreatedAt    time.Time
	AssessmentID string
	Kind         RelationshipKind
	Subject      Subject
}

func NewRelationship(assessmentID string, subject Subject, kind RelationshipKind) Relationship {
	r := Relationship{
		AssessmentID: assessmentID,
		Subject:      subject,
		Kind:         kind,
		CreatedAt:    time.Now(),
	}
	r.ID = hash(r.String())
	return r
}

func (r *Relationship) ToModel() RelationshipModel {
	return RelationshipModel{
		RelationID:   r.ID,
		AssessmentID: r.AssessmentID,
		CreatedAt:    r.CreatedAt,
		RelationType: r.Kind.String(),
		SubjectID:    r.Subject.ID,
		SubjectType:  r.Subject.Kind.String(),
	}
}

// String generates the relationship's tuple in spicedb format: assessment:123#owner@user:1234
func (r Relationship) String() string {
	subj := fmt.Sprintf("%s:%s", r.Subject.Kind.String(), hash(r.Subject.ID))
	if r.Subject.Kind == Organization {
		subj = fmt.Sprintf("%s:%s#%s", OrgObject, hash(r.Subject.ID), MemberRelationshipKind.String())
	}

	resource := AssessmentObject
	switch r.Kind {
	case MemberRelationshipKind:
		resource = OrgObject
	case AdminPlatformRelationshipKind, EditorPlatformRelationshipKind, ViewerPlatformRelationshipKind:
		resource = PlatformObject
	}

	return fmt.Sprintf("%s:%s#%s@%s", resource, r.AssessmentID, r.Kind.String(), subj)
}

type Relationships []Relationship

type RelationshipFn func(updates []*v1pb.RelationshipUpdate) ([]*v1pb.RelationshipUpdate, Relationship, RelationshipOp)
