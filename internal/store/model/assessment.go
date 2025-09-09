package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"

	api "github.com/kubev2v/migration-planner/api/v1alpha1"
)

type Assessment struct {
	ID         uuid.UUID `gorm:"primaryKey;column:id;type:VARCHAR(255);"`
	CreatedAt  time.Time `gorm:"not null;default:now()"`
	UpdatedAt  *time.Time
	Name       string     `gorm:"not null;uniqueIndex:org_id_name"`
	OrgID      string     `gorm:"not null;uniqueIndex:org_id_name;index:assessments_org_id_idx"`
	Username   string     `gorm:"type:VARCHAR(255)"`
	SourceType string     `gorm:"not null;type:VARCHAR(100)"`
	SourceID   *uuid.UUID `gorm:"type:TEXT"`
	Snapshots  []Snapshot `gorm:"foreignKey:AssessmentID;references:ID;constraint:OnDelete:CASCADE;"`
}

type Snapshot struct {
	ID           uint                      `gorm:"primaryKey;autoIncrement"`
	CreatedAt    time.Time                 `gorm:"not null;default:now()"`
	Inventory    *JSONField[api.Inventory] `gorm:"type:jsonb;not null"`
	AssessmentID uuid.UUID                 `gorm:"not null;type:VARCHAR(255);"`
}

type AssessmentList []Assessment

func (a Assessment) String() string {
	val, _ := json.Marshal(a)
	return string(val)
}

func (s Snapshot) String() string {
	val, _ := json.Marshal(s)
	return string(val)
}
