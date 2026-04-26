package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"

	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/internal/client"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/internal/store/model"
	"github.com/kubev2v/migration-planner/pkg/log"
)

// CostEstimationService handles TCO cost estimation calculations
type CostEstimationService struct {
	client      *client.CostEstimationClient
	store       store.Store
	accountsSvc *AccountsService
	logger      *log.StructuredLogger
}

func NewCostEstimationService(costEstimationClient *client.CostEstimationClient, store store.Store, accountsSvc *AccountsService) *CostEstimationService {
	return &CostEstimationService{
		client:      costEstimationClient,
		store:       store,
		accountsSvc: accountsSvc,
		logger:      log.NewDebugLogger("cost_estimation_service"),
	}
}

// CostEstimationRequestForm is the input form for cost estimation
type CostEstimationRequestForm struct {
	ClusterID string
	Discounts CostEstimationDiscountsForm
}

// CostEstimationDiscountsForm holds discount percentages
type CostEstimationDiscountsForm struct {
	VcfDiscountPct    float64
	VvfDiscountPct    float64
	RedhatDiscountPct float64
	AapDiscountPct    float64
}

// CalculateCostEstimation calculates TCO cost estimation for an assessment cluster
func (s *CostEstimationService) CalculateCostEstimation(
	ctx context.Context,
	assessmentID uuid.UUID,
	req *CostEstimationRequestForm,
) (*client.CostEstimationResponse, error) {
	logger := s.logger.WithContext(ctx)

	if s.client == nil {
		return nil, fmt.Errorf("cost estimation client is not configured")
	}

	user := auth.MustHaveUser(ctx)

	// 1. Partner-only check
	identity, err := s.accountsSvc.GetIdentity(ctx, user)
	if err != nil {
		return nil, fmt.Errorf("failed to get identity: %w", err)
	}
	if identity.Kind != KindPartner || identity.GroupID == nil {
		return nil, NewErrForbidden("cost estimation", assessmentID.String())
	}

	// 2. Assessment read permission check
	resource, err := s.store.Authz().GetPermissions(ctx, user.Username, model.NewAssessmentResource(assessmentID.String()))
	if err != nil {
		return nil, fmt.Errorf("authz: failed to get permissions: %w", err)
	}
	if !model.ReadPermission.In(resource.Permissions) {
		return nil, NewErrForbidden("assessment", assessmentID.String())
	}

	// 3. Get assessment
	assessment, err := s.store.Assessment().Get(ctx, assessmentID)
	if err != nil {
		if errors.Is(err, store.ErrRecordNotFound) {
			return nil, NewErrAssessmentNotFound(assessmentID)
		}
		return nil, fmt.Errorf("failed to get assessment: %w", err)
	}

	if len(assessment.Snapshots) == 0 {
		return nil, NewErrInvalidRequest("assessment has no snapshots")
	}

	// Get latest snapshot
	latestSnapshot := assessment.Snapshots[0]
	if len(latestSnapshot.Inventory) == 0 {
		return nil, NewErrInvalidRequest("latest snapshot has empty inventory")
	}

	var inventory api.Inventory
	if err := json.Unmarshal(latestSnapshot.Inventory, &inventory); err != nil {
		return nil, fmt.Errorf("failed to parse inventory: %w", err)
	}

	if len(inventory.Clusters) == 0 {
		return nil, NewErrInvalidRequest("inventory has no clusters")
	}

	clusterInventory, exists := inventory.Clusters[req.ClusterID]
	if !exists {
		return nil, NewErrResourceNotFound(uuid.Nil, fmt.Sprintf("cluster %s in assessment %s", req.ClusterID, assessmentID))
	}

	// 4. Build customerEnvironment from inventory
	customerEnv := buildCustomerEnvironment(&clusterInventory)

	tracer := logger.Operation("calculate_cost_estimation").
		WithUUID("assessment_id", assessmentID).
		WithString("cluster_id", req.ClusterID).
		WithInt("total_esxi_hosts", customerEnv.TotalEsxiHosts).
		WithInt("sockets_per_host", customerEnv.SocketsPerHost).
		WithInt("cores_per_socket", customerEnv.CoresPerSocket).
		WithInt("total_vms", customerEnv.TotalVirtualMachines).
		Build()

	// 5. Call external cost-estimation service
	costReq := &client.CostEstimationRequest{
		AssessmentID:        assessmentID.String(),
		ClusterID:           req.ClusterID,
		CustomerEnvironment: customerEnv,
		Discounts: client.Discounts{
			VcfDiscountPct:    req.Discounts.VcfDiscountPct,
			VvfDiscountPct:    req.Discounts.VvfDiscountPct,
			RedhatDiscountPct: req.Discounts.RedhatDiscountPct,
			AapDiscountPct:    req.Discounts.AapDiscountPct,
		},
	}

	resp, err := s.client.CalculateCostEstimation(ctx, costReq)
	if err != nil {
		tracer.Error(err).Log()
		return nil, fmt.Errorf("failed to call cost estimation service: %w", err)
	}

	tracer.Success().Log()
	return resp, nil
}

// buildCustomerEnvironment extracts customer environment data from inventory
func buildCustomerEnvironment(clusterInventory *api.InventoryData) client.CustomerEnvironment {
	totalHosts := clusterInventory.Infra.TotalHosts
	totalVMs := clusterInventory.Vms.Total

	// Default values
	socketsPerHost := 2
	coresPerSocket := 16

	// Try to get actual values from hosts
	if clusterInventory.Infra.Hosts != nil && len(*clusterInventory.Infra.Hosts) > 0 {
		hosts := *clusterInventory.Infra.Hosts
		firstHost := hosts[0]

		if firstHost.CpuSockets != nil && *firstHost.CpuSockets > 0 {
			socketsPerHost = *firstHost.CpuSockets
		}

		if firstHost.CpuCores != nil && firstHost.CpuSockets != nil && *firstHost.CpuSockets > 0 {
			coresPerSocket = *firstHost.CpuCores / *firstHost.CpuSockets
			if coresPerSocket <= 0 {
				coresPerSocket = 16
			}
		}
	}

	return client.CustomerEnvironment{
		TotalEsxiHosts:       totalHosts,
		SocketsPerHost:       socketsPerHost,
		CoresPerSocket:       coresPerSocket,
		TotalVirtualMachines: totalVMs,
	}
}
