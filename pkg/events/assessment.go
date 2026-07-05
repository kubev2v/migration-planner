package events

import (
	"encoding/json"
	"time"
)

type AssessmentEventPayload struct {
	Assessment AssessmentData `json:"assessment"`
}

type AssessmentData struct {
	ID         string          `json:"id"`
	SnapshotID uint            `json:"snapshot_id,omitempty"`
	Name       string          `json:"name,omitempty"`
	OrgID      string          `json:"org_id,omitempty"`
	Username   string          `json:"username,omitempty"`
	SourceType string          `json:"source_type,omitempty"`
	PartnerID  *string         `json:"partner_id,omitempty"`
	Location   *string         `json:"location,omitempty"`
	Inventory  json.RawMessage `json:"inventory,omitempty"`
	CreatedAt  time.Time       `json:"created_at,omitempty"`
	UpdatedAt  *time.Time      `json:"updated_at,omitempty"`
	DeletedAt  *time.Time      `json:"deleted_at,omitempty"`
}

func NewAssessmentCreatedPayload(data AssessmentData) AssessmentEventPayload {
	return AssessmentEventPayload{Assessment: data}
}

func NewAssessmentDeletedPayload(assessmentID string, deletedAt time.Time) AssessmentEventPayload {
	return AssessmentEventPayload{Assessment: AssessmentData{ID: assessmentID, DeletedAt: &deletedAt}}
}
