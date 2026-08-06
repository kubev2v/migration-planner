package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/internal/store/model"
	"github.com/kubev2v/migration-planner/pkg/log"
)

type AssessmentEnhancementDataServicer interface {
	Save(ctx context.Context, assessmentID uuid.UUID, data api.EnhancementData) (*api.EnhancementData, error)
	Get(ctx context.Context, assessmentID uuid.UUID) (*api.EnhancementData, error)
}

type AssessmentEnhancementDataService struct {
	store  store.Store
	logger *log.StructuredLogger
}

func NewAssessmentEnhancementDataService(s store.Store) *AssessmentEnhancementDataService {
	return &AssessmentEnhancementDataService{
		store:  s,
		logger: log.NewDebugLogger("assessment_enhancement_data_service"),
	}
}

func (s *AssessmentEnhancementDataService) Save(
	ctx context.Context,
	assessmentID uuid.UUID,
	data api.EnhancementData,
) (*api.EnhancementData, error) {
	input := enhancementDataAPIToModel(assessmentID, data)
	result, err := s.store.AssessmentEnhancementData().Upsert(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to save enhancement data: %w", err)
	}

	out := enhancementDataModelToAPI(result)
	return &out, nil
}

func (s *AssessmentEnhancementDataService) Get(
	ctx context.Context,
	assessmentID uuid.UUID,
) (*api.EnhancementData, error) {
	input, err := s.store.AssessmentEnhancementData().Get(ctx, assessmentID)
	if err != nil {
		if errors.Is(err, store.ErrRecordNotFound) {
			empty := api.EnhancementData{}
			return &empty, nil
		}
		return nil, fmt.Errorf("failed to get enhancement data: %w", err)
	}

	out := enhancementDataModelToAPI(input)
	return &out, nil
}

func enhancementDataAPIToModel(assessmentID uuid.UUID, data api.EnhancementData) model.AssessmentEnhancementData {
	m := model.AssessmentEnhancementData{
		AssessmentID: assessmentID,
	}

	if data.DeployedEnvironment != nil {
		m.DeployedEnvEnvironment = model.TypedStringPtr(data.DeployedEnvironment.Environment)
	}

	if data.VmwareVersionCounts != nil {
		m.VMwareVerPerpetualLicensesCount = data.VmwareVersionCounts.PerpetualLicensesCount
		m.VMwareVerCoresLicensingCount = data.VmwareVersionCounts.CoresLicensingCount
		m.VMwareVerEnvironmentCount = data.VmwareVersionCounts.EnvironmentCount
		m.VMwareVerOtherNodesCount = data.VmwareVersionCounts.OtherNodesCount
	}

	if data.ActiveEnvironments != nil {
		m.ActiveEnvEnvironments = model.TypedStringSlice(data.ActiveEnvironments.Environments)
	}

	if data.VmwareSubscription != nil {
		m.VMwareSubLevel = model.TypedStringSlice(data.VmwareSubscription.Level)
	}

	if data.VsphereCore != nil {
		m.VsphereVmEncryptionEnabled = data.VsphereCore.VmEncryptionEnabled
		m.VsphereVmEncryptionPolicy = data.VsphereCore.VmEncryptionPolicy
		m.VsphereSrmEnabled = data.VsphereCore.SrmEnabled
	}

	if data.Nsx != nil {
		m.NsxFeatures = model.TypedStringSlice(data.Nsx.Features)
	}

	if data.AriaOps != nil {
		m.AriaOpsFeatures = model.TypedStringSlice(data.AriaOps.Features)
	}

	if data.AriaAutomation != nil {
		m.AriaAutomationFeatures = model.TypedStringSlice(data.AriaAutomation.Features)
	}

	if data.AriaSecure != nil {
		m.AriaSecureFeatures = model.TypedStringSlice(data.AriaSecure.Features)
	}

	if data.CustomerDetails != nil {
		m.CustomerPhysicalLocationsCount = data.CustomerDetails.PhysicalLocationsCount
		m.CustomerTargetHardware = data.CustomerDetails.TargetHardware
	}

	return m
}

func enhancementDataModelToAPI(m *model.AssessmentEnhancementData) api.EnhancementData {
	var data api.EnhancementData

	if m.DeployedEnvEnvironment != nil {
		env := api.DeployedEnvironmentInputEnvironment(*m.DeployedEnvEnvironment)
		data.DeployedEnvironment = &api.DeployedEnvironmentInput{Environment: &env}
	}

	if m.VMwareVerPerpetualLicensesCount != nil || m.VMwareVerCoresLicensingCount != nil ||
		m.VMwareVerEnvironmentCount != nil || m.VMwareVerOtherNodesCount != nil {
		data.VmwareVersionCounts = &api.VMwareVersionCountsInput{
			PerpetualLicensesCount: m.VMwareVerPerpetualLicensesCount,
			CoresLicensingCount:    m.VMwareVerCoresLicensingCount,
			EnvironmentCount:       m.VMwareVerEnvironmentCount,
			OtherNodesCount:        m.VMwareVerOtherNodesCount,
		}
	}

	if m.ActiveEnvEnvironments != nil {
		envs := model.ToTypedSlice[api.ActiveEnvironmentsInputEnvironments](m.ActiveEnvEnvironments)
		data.ActiveEnvironments = &api.ActiveEnvironmentsInput{Environments: &envs}
	}

	if m.VMwareSubLevel != nil {
		levels := model.ToTypedSlice[api.VMwareSubscriptionInputLevel](m.VMwareSubLevel)
		data.VmwareSubscription = &api.VMwareSubscriptionInput{Level: &levels}
	}

	if m.VsphereVmEncryptionEnabled != nil || m.VsphereVmEncryptionPolicy != nil ||
		m.VsphereSrmEnabled != nil {
		data.VsphereCore = &api.VsphereCoreInput{
			VmEncryptionEnabled: m.VsphereVmEncryptionEnabled,
			VmEncryptionPolicy:  m.VsphereVmEncryptionPolicy,
			SrmEnabled:          m.VsphereSrmEnabled,
		}
	}

	if m.NsxFeatures != nil {
		features := model.ToTypedSlice[api.NsxInputFeatures](m.NsxFeatures)
		data.Nsx = &api.NsxInput{Features: &features}
	}

	if m.AriaOpsFeatures != nil {
		features := model.ToTypedSlice[api.AriaOpsInputFeatures](m.AriaOpsFeatures)
		data.AriaOps = &api.AriaOpsInput{Features: &features}
	}

	if m.AriaAutomationFeatures != nil {
		features := model.ToTypedSlice[api.AriaAutomationInputFeatures](m.AriaAutomationFeatures)
		data.AriaAutomation = &api.AriaAutomationInput{Features: &features}
	}

	if m.AriaSecureFeatures != nil {
		features := model.ToTypedSlice[api.AriaSecureInputFeatures](m.AriaSecureFeatures)
		data.AriaSecure = &api.AriaSecureInput{Features: &features}
	}

	if m.CustomerPhysicalLocationsCount != nil || m.CustomerTargetHardware != nil {
		data.CustomerDetails = &api.CustomerDetailsInput{
			PhysicalLocationsCount: m.CustomerPhysicalLocationsCount,
			TargetHardware:         m.CustomerTargetHardware,
		}
	}

	return data
}
