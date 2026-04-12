package model

import (
	"time"

	"github.com/google/uuid"
)

type AssessmentClusterSizingInput struct {
	AssessmentID            uuid.UUID  `gorm:"primaryKey;column:assessment_id;type:VARCHAR(255);"`
	ExternalClusterID       string     `gorm:"primaryKey;column:external_cluster_id;type:TEXT;"`
	CpuOverCommitRatio      *string    `gorm:"column:cpu_over_commit_ratio;type:TEXT"`
	MemoryOverCommitRatio   *string    `gorm:"column:memory_over_commit_ratio;type:TEXT"`
	WorkerNodeCPU           *int       `gorm:"column:worker_node_cpu"`
	WorkerNodeThreads       *int       `gorm:"column:worker_node_threads"`
	WorkerNodeMemory        *int       `gorm:"column:worker_node_memory"`
	ControlPlaneSchedulable *bool      `gorm:"column:control_plane_schedulable"`
	ControlPlaneNodeCount   *int       `gorm:"column:control_plane_node_count"`
	ControlPlaneCPU         *int       `gorm:"column:control_plane_cpu"`
	ControlPlaneMemory      *int       `gorm:"column:control_plane_memory"`
	HostedControlPlane      *bool      `gorm:"column:hosted_control_plane"`
	UpdatedAt               *time.Time `gorm:"column:updated_at;type:timestamptz;not null;default:now()"`
}
