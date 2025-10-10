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
	default:
		return "assessment" // default fallback
	}
}

const (
	AssessmentResource ResourceType = iota
	PlatformResource
)

type Subject struct {
	Kind        SubjectType
	ID          string
	GeneratedID string
}

func hash(s string) string {
	h := sha256.New()
	h.Write([]byte(s))

	sum := h.Sum(nil)
	if len(sum) > 12 {
		return fmt.Sprintf("%x", sum)[:12]
	}
	ss := fmt.Sprintf("%x", sum)
	for i := len(sum); i <= 12; i++ {
		ss = fmt.Sprintf("%s0", ss)
	}
	return ss
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
//	readerFn := WithReaderRelationship("assessment789", orgSubject)
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
	case ReaderRelationshipKind:
		return "reader"
	case OwnerRelationshipKind:
		return "owner"
	case OrganizationRelationshipKind:
		return "org"
	case MemberRelationshipKind:
		return "member"
	case ParentRelationshipKind:
		return "parent"
	default:
		return "unknown"
	}
}

const (
	ReaderRelationshipKind RelationshipKind = iota
	OwnerRelationshipKind
	OrganizationRelationshipKind
	MemberRelationshipKind
	ParentRelationshipKind
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
	ResourceType ResourceType
	Permissions  []Permission
}

func NewAssessmentResource(id string) Resource {
	return Resource{
		ID:           id,
		ResourceType: AssessmentResource,
	}
}

func NewPlatformResource(id string) Resource {
	return Resource{
		ID:           id,
		ResourceType: PlatformResource,
	}
}

type RelationshipModel struct {
	RelationID   string    `gorm:"column:relation_id;type:varchar(100);not null"`
	AssessmentID string    `gorm:"column:assessment_id;type:varchar(255);not null"`
	Timestamp    time.Time `gorm:"column:timestamp;default:now()"`
	RelationType string    `gorm:"column:relation_type;type:varchar(50);not null"`
	SubjectID    string    `gorm:"column:subject_id;type:varchar(255);not null"`
	SubjectType  string    `gorm:"column:subject_type;type:varchar(50);not null"`
}

func (r *RelationshipModel) TableName() string {
	return "relationships"
}

type RelationshipOp int

const (
	RelationshipOpUpdate RelationshipOp = iota
	RelationshipOpDelete
	RelationshipOpTouch
	RelationshipOpIgnore
)

type Relationship struct {
	ID               string
	Timestamp        time.Time
	AssessmentID     string
	RelationshipKind RelationshipKind
	Subject          Subject
}

func NewRelationship(assessmentID string, subject Subject, kind RelationshipKind) Relationship {
	r := Relationship{
		AssessmentID:     assessmentID,
		Subject:          subject,
		RelationshipKind: kind,
	}
	r.ID = hash(r.String())
	return r
}

func (r *Relationship) ToModel() RelationshipModel {
	return RelationshipModel{
		RelationID:   r.ID,
		AssessmentID: r.AssessmentID,
		Timestamp:    r.Timestamp,
		RelationType: r.RelationshipKind.String(),
		SubjectID:    r.Subject.ID,
		SubjectType:  r.Subject.Kind.String(),
	}
}

// String generates the relationship's tuple in spicedb format: assessment:123#owner@user:1234
func (r Relationship) String() string {
	user := fmt.Sprintf("user:%s", hash(r.Subject.ID))
	if r.Subject.Kind == Organization {
		user = fmt.Sprintf("org:%s#member", hash(r.Subject.ID))
	}
	return fmt.Sprintf("assessment:%s#%s@%s", r.AssessmentID, r.RelationshipKind.String(), user)
}

type Relationships []Relationship

type RelationshipFn func(updates []*v1pb.RelationshipUpdate) ([]*v1pb.RelationshipUpdate, Relationship, RelationshipOp)
