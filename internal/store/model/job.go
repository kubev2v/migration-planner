package model

import "github.com/google/uuid"

// RVToolsJobMetadata is stored in river_job.metadata to track progress and results.
type RVToolsJobMetadata struct {
	Status       string     `json:"status,omitempty"`        // parsing, validating
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
