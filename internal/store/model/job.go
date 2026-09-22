package model

import (
	"github.com/google/uuid"
	"github.com/riverqueue/river"
)

// RVToolsJobMetadata is stored in river_job.metadata to track progress and results.
type RVToolsJobMetadata struct {
	Status       string     `json:"status,omitempty"`        // validating, parsing
	Error        string     `json:"error,omitempty"`         // error message if failed
	AssessmentID *uuid.UUID `json:"assessment_id,omitempty"` // set when completed
}

// Job status constants
const (
	JobStatusPending    = "pending"
	JobStatusParsing    = "parsing"
	JobStatusValidating = "validating"
	JobStatusCompleted  = "completed"
	JobStatusFailed     = "failed"
	JobStatusCancelled  = "cancelled"
)

type JobOperationType int

const (
	CreateOperationType JobOperationType = iota
	UpdateOperationType
)

type RVToolsJobArgs struct {
	Name          string           `json:"name"`
	OperationType JobOperationType `json:"op_type"`
	AssessmentID  uuid.UUID        `json:"assessment_id"`
	File          string           `json:"file"`
	OrgID         string           `json:"org_id"`
	Username      string           `json:"username"`
	FirstName     string           `json:"first_name"`
	LastName      string           `json:"last_name"`
}

func (RVToolsJobArgs) Kind() string {
	return "rvtools_assessment"
}

func (RVToolsJobArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue:       "default",
		MaxAttempts: 1,
	}
}
