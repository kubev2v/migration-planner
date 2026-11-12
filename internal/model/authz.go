package model

import "slices"

// Permission identifies an action available on an assessment.
type Permission string

const (
	ReadPermission   Permission = "read"
	EditPermission   Permission = "edit"
	SharePermission  Permission = "share"
	DeletePermission Permission = "delete"
)

func (p Permission) String() string {
	return string(p)
}

// In reports whether the permission is present in the given slice.
func (p Permission) In(permissions []Permission) bool {
	return slices.Contains(permissions, p)
}

// SharingSubject identifies who an assessment is shared with or by.
type SharingSubject struct {
	Type string
	ID   string
	Name string
}

// Sharing captures the sharing state of an assessment for a given viewer.
type Sharing struct {
	IsShared   bool
	SharedWith []SharingSubject
	SharedBy   *SharingSubject
}
