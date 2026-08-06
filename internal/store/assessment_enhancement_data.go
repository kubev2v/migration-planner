package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/internal/store/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type AssessmentEnhancementData interface {
	Upsert(ctx context.Context, input model.AssessmentEnhancementData) (*model.AssessmentEnhancementData, error)
	Get(ctx context.Context, assessmentID uuid.UUID) (*model.AssessmentEnhancementData, error)
}

type AssessmentEnhancementDataStore struct {
	db *gorm.DB
}

var _ AssessmentEnhancementData = (*AssessmentEnhancementDataStore)(nil)

func NewAssessmentEnhancementDataStore(db *gorm.DB) AssessmentEnhancementData {
	return &AssessmentEnhancementDataStore{db: db}
}

func (s *AssessmentEnhancementDataStore) Upsert(ctx context.Context, input model.AssessmentEnhancementData) (*model.AssessmentEnhancementData, error) {
	now := time.Now()
	input.UpdatedAt = &now

	result := s.getDB(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "assessment_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"deployed_env_environment",
			"vmware_ver_perpetual_licenses_count",
			"vmware_ver_cores_licensing_count",
			"vmware_ver_environment_count",
			"vmware_ver_other_nodes_count",
			"active_env_environments",
			"vmware_sub_level",
			"vsphere_vm_encryption_enabled",
			"vsphere_vm_encryption_policy",
			"vsphere_srm_enabled",
			"nsx_features",
			"aria_ops_features",
			"aria_automation_features",
			"aria_secure_features",
			"customer_physical_locations_count",
			"customer_target_hardware",
			"updated_at",
		}),
	}).Create(&input)
	if result.Error != nil {
		return nil, fmt.Errorf("upserting assessment enhancement data: %w", result.Error)
	}

	return &input, nil
}

func (s *AssessmentEnhancementDataStore) Get(ctx context.Context, assessmentID uuid.UUID) (*model.AssessmentEnhancementData, error) {
	var input model.AssessmentEnhancementData
	result := s.getDB(ctx).First(&input, "assessment_id = ?", assessmentID)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, ErrRecordNotFound
		}
		return nil, fmt.Errorf("querying assessment enhancement data: %w", result.Error)
	}

	return &input, nil
}

func (s *AssessmentEnhancementDataStore) getDB(ctx context.Context) *gorm.DB {
	tx := FromContext(ctx)
	if tx != nil {
		return tx
	}
	return s.db
}
