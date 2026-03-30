package mappers

import (
	"encoding/json"
	"fmt"
	"slices"

	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/service"
	"github.com/kubev2v/migration-planner/internal/service/mappers"
	"github.com/kubev2v/migration-planner/internal/store/model"
	"github.com/kubev2v/migration-planner/internal/util"
	"github.com/kubev2v/migration-planner/pkg/estimations/complexity"
	"github.com/kubev2v/migration-planner/pkg/estimations/engines"
	"github.com/kubev2v/migration-planner/pkg/estimations/estimation"
)

// normalizeInventoryData ensures all nil maps and slices are initialized to empty ones
// This prevents null values in JSON that can cause UI crashes when reading data from database
func normalizeInventoryData(data *api.InventoryData) {
	if data == nil {
		return
	}

	// Normalize Infra fields
	if data.Infra.Datastores == nil {
		data.Infra.Datastores = []api.Datastore{}
	}
	if data.Infra.Networks == nil {
		data.Infra.Networks = []api.Network{}
	}
	if data.Infra.HostPowerStates == nil {
		data.Infra.HostPowerStates = map[string]int{}
	}

	// Normalize VMs fields
	if data.Vms.PowerStates == nil {
		data.Vms.PowerStates = map[string]int{}
	}
	if data.Vms.MigrationWarnings == nil {
		data.Vms.MigrationWarnings = api.MigrationIssues{}
	}
	if data.Vms.NotMigratableReasons == nil {
		data.Vms.NotMigratableReasons = api.MigrationIssues{}
	}
	if data.Vms.OsInfo == nil {
		data.Vms.OsInfo = &map[string]api.OsInfo{}
	}
	if data.Vms.DiskSizeTier == nil {
		data.Vms.DiskSizeTier = &map[string]api.DiskSizeTierSummary{}
	}
	if data.Vms.DiskTypes == nil {
		data.Vms.DiskTypes = &map[string]api.DiskTypeSummary{}
	}
	if data.Vms.DistributionByCpuTier == nil {
		data.Vms.DistributionByCpuTier = &map[string]int{}
	}
	if data.Vms.DistributionByMemoryTier == nil {
		data.Vms.DistributionByMemoryTier = &map[string]int{}
	}
	if data.Vms.DistributionByNicCount == nil {
		data.Vms.DistributionByNicCount = &map[string]int{}
	}
	if data.Vms.DistributionByComplexity == nil {
		data.Vms.DistributionByComplexity = &map[string]int{}
	}
}

func SourceToApi(s model.Source) (api.Source, error) {
	source := api.Source{
		Id:         s.ID,
		Inventory:  nil,
		CreatedAt:  s.CreatedAt,
		UpdatedAt:  s.UpdatedAt,
		OnPremises: s.OnPremises,
		Name:       s.Name,
	}

	if len(s.Inventory) > 0 {
		v := util.GetInventoryVersion(s.Inventory)
		switch v {
		case model.SnapshotVersionV1:
			i := api.InventoryData{}
			if err := json.Unmarshal(s.Inventory, &i); err != nil {
				return api.Source{}, fmt.Errorf("failed to unmarshal v1 inventory: %w", err)
			}
			if i.Vcenter == nil {
				return api.Source{}, fmt.Errorf("v1 inventory missing vcenter data")
			}
			// Normalize to prevent null values from database
			normalizeInventoryData(&i)
			source.Inventory = &api.Inventory{
				Vcenter:   &i,
				VcenterId: i.Vcenter.Id,
				Clusters:  map[string]api.InventoryData{},
			}
		default:
			v2 := api.Inventory{}
			if err := json.Unmarshal(s.Inventory, &v2); err != nil {
				return api.Source{}, fmt.Errorf("failed to unmarshal v2 inventory: %w", err)
			}
			// Ensure clusters map is never nil (fix for null values from database)
			if v2.Clusters == nil {
				v2.Clusters = make(map[string]api.InventoryData)
			}
			// Normalize vcenter and all cluster inventories to prevent null values
			if v2.Vcenter != nil {
				normalizeInventoryData(v2.Vcenter)
			}
			for clusterID, clusterData := range v2.Clusters {
				normalizeInventoryData(&clusterData)
				v2.Clusters[clusterID] = clusterData
			}
			source.Inventory = &v2
		}
	}

	if len(s.Labels) > 0 {
		labels := make([]api.Label, 0, len(s.Labels))
		for _, label := range s.Labels {
			labels = append(labels, api.Label{Key: label.Key, Value: label.Value})
		}
		source.Labels = &labels
	}

	// Map ImageInfra fields to API infra
	source.Infra = &struct {
		Proxy        *api.AgentProxy            `json:"proxy,omitempty"`
		SshPublicKey *api.ValidatedSSHPublicKey `json:"sshPublicKey" validate:"omitnil,ssh_key"`
		VmNetwork    *api.VmNetwork             `json:"vmNetwork,omitempty"`
	}{}

	// Map proxy fields
	if s.ImageInfra.HttpProxyUrl != "" || s.ImageInfra.HttpsProxyUrl != "" || s.ImageInfra.NoProxyDomains != "" {
		source.Infra.Proxy = &api.AgentProxy{}
		if s.ImageInfra.HttpProxyUrl != "" {
			source.Infra.Proxy.HttpUrl = &s.ImageInfra.HttpProxyUrl
		}
		if s.ImageInfra.HttpsProxyUrl != "" {
			source.Infra.Proxy.HttpsUrl = &s.ImageInfra.HttpsProxyUrl
		}
		if s.ImageInfra.NoProxyDomains != "" {
			source.Infra.Proxy.NoProxy = &s.ImageInfra.NoProxyDomains
		}
	}

	// Map SSH public key
	if s.ImageInfra.SshPublicKey != "" {
		source.Infra.SshPublicKey = &s.ImageInfra.SshPublicKey
	}

	// Map VM network fields
	if s.ImageInfra.IpAddress != "" || s.ImageInfra.SubnetMask != "" || s.ImageInfra.DefaultGateway != "" || s.ImageInfra.Dns != "" {
		source.Infra.VmNetwork = &api.VmNetwork{
			Ipv4: &api.Ipv4Config{
				IpAddress:      s.ImageInfra.IpAddress,
				SubnetMask:     s.ImageInfra.SubnetMask,
				DefaultGateway: s.ImageInfra.DefaultGateway,
				Dns:            s.ImageInfra.Dns,
			},
		}
	}

	// Map agent version and warning (from ImageInfra, independent of agents)
	if s.ImageInfra.AgentVersion != nil {
		source.AgentVersion = s.ImageInfra.AgentVersion
	}
	if warning := service.CheckAgentVersionWarning(&s.ImageInfra); warning != nil {
		source.AgentVersionWarning = warning
	}

	// We are mapping only the first agent based on created_at timestamp and ignore the rest for now.
	// TODO:
	// Remark: If multiple agents are deployed, we pass only the first one based on created_at timestamp
	// while other agents in up-to-date states exists.
	// Which one should be presented in the API response?
	if len(s.Agents) == 0 {
		return source, nil
	}

	slices.SortFunc(s.Agents, func(a model.Agent, b model.Agent) int {
		if a.CreatedAt.Before(b.CreatedAt) {
			return -1
		}
		if a.CreatedAt.After(b.CreatedAt) {
			return 1
		}
		return 0
	})
	agent := AgentToApi(s.Agents[0])
	source.Agent = &agent

	return source, nil
}

func SourceListToApi(sources ...model.SourceList) api.SourceList {
	sourceList := []api.Source{}
	for _, source := range sources {
		for _, s := range source {
			apiSource, err := SourceToApi(s)
			if err != nil {
				continue
			}
			sourceList = append(sourceList, apiSource)
		}
	}
	return sourceList
}

func AgentToApi(a model.Agent) api.Agent {
	return api.Agent{
		Id:            a.ID,
		Status:        api.StringToAgentStatus(a.Status),
		StatusInfo:    a.StatusInfo,
		CreatedAt:     a.CreatedAt,
		UpdatedAt:     a.UpdatedAt,
		CredentialUrl: a.CredUrl,
		Version:       a.Version,
	}
}

func AssessmentToApi(a model.Assessment) (api.Assessment, error) {
	assessment := api.Assessment{
		Id:             a.ID,
		Name:           a.Name,
		OwnerFirstName: a.OwnerFirstName,
		OwnerLastName:  a.OwnerLastName,
		CreatedAt:      a.CreatedAt,
		Snapshots:      make([]api.Snapshot, len(a.Snapshots)),
	}

	// Convert snapshots
	for i, snapshot := range a.Snapshots {
		assessment.Snapshots[i] = api.Snapshot{
			CreatedAt: snapshot.CreatedAt,
		}
		if len(snapshot.Inventory) > 0 {
			inventory := api.Inventory{}
			switch snapshot.Version {
			case 1:
				invV1 := api.InventoryData{}
				if err := json.Unmarshal(snapshot.Inventory, &invV1); err != nil {
					return api.Assessment{}, err
				}
				// Normalize to prevent null values from database
				normalizeInventoryData(&invV1)
				inventory.Vcenter = &invV1
				inventory.VcenterId = invV1.Vcenter.Id
				// Ensure clusters is initialized
				if inventory.Clusters == nil {
					inventory.Clusters = make(map[string]api.InventoryData)
				}
			case 2:
				if err := json.Unmarshal(snapshot.Inventory, &inventory); err != nil {
					return api.Assessment{}, err
				}
				// Ensure clusters map is never nil (fix for null values from database)
				if inventory.Clusters == nil {
					inventory.Clusters = make(map[string]api.InventoryData)
				}
				// Normalize vcenter and all cluster inventories to prevent null values
				if inventory.Vcenter != nil {
					normalizeInventoryData(inventory.Vcenter)
				}
				for clusterID, clusterData := range inventory.Clusters {
					normalizeInventoryData(&clusterData)
					inventory.Clusters[clusterID] = clusterData
				}
			default:
				return api.Assessment{}, fmt.Errorf("unsupported snapshot version: %d", snapshot.Version)
			}
			assessment.Snapshots[i].Inventory = inventory
		} else {
			// Initialize empty inventory with non-nil Clusters
			assessment.Snapshots[i].Inventory = api.Inventory{
				Clusters: make(map[string]api.InventoryData),
			}
		}
	}

	// Set source type based on source field
	sourceType := api.AssessmentSourceType(a.SourceType)
	assessment.SourceType = sourceType
	assessment.SourceId = a.SourceID

	return assessment, nil
}

func AssessmentListToApi(assessments []model.Assessment) (api.AssessmentList, error) {
	assessmentList := make([]api.Assessment, len(assessments))
	for i, assessment := range assessments {
		a, err := AssessmentToApi(assessment)
		if err != nil {
			return api.AssessmentList{}, err
		}
		assessmentList[i] = a
	}
	return assessmentList, nil
}

func ClusterRequirementsResponseFormToAPI(form mappers.ClusterRequirementsResponseForm) api.ClusterRequirementsResponse {
	resourceConsumption := api.SizingResourceConsumption{
		Cpu:    form.ResourceConsumption.CPU,
		Memory: form.ResourceConsumption.Memory,
	}

	if form.ResourceConsumption.Limits.CPU != 0.0 || form.ResourceConsumption.Limits.Memory != 0.0 {
		resourceConsumption.Limits = &api.SizingResourceLimits{
			Cpu:    form.ResourceConsumption.Limits.CPU,
			Memory: form.ResourceConsumption.Limits.Memory,
		}
	}

	if form.ResourceConsumption.OverCommitRatio.CPU != 0.0 || form.ResourceConsumption.OverCommitRatio.Memory != 0.0 {
		resourceConsumption.OverCommitRatio = &api.SizingOverCommitRatio{
			Cpu:    form.ResourceConsumption.OverCommitRatio.CPU,
			Memory: form.ResourceConsumption.OverCommitRatio.Memory,
		}
	}

	return api.ClusterRequirementsResponse{
		ClusterSizing: api.ClusterSizing{
			TotalNodes:        form.ClusterSizing.TotalNodes,
			ControlPlaneNodes: form.ClusterSizing.ControlPlaneNodes,
			WorkerNodes:       form.ClusterSizing.WorkerNodes,
			FailoverNodes:     form.ClusterSizing.FailoverNodes,
			TotalCPU:          form.ClusterSizing.TotalCPU,
			TotalMemory:       form.ClusterSizing.TotalMemory,
		},
		ResourceConsumption: resourceConsumption,
		InventoryTotals: api.InventoryTotals{
			TotalVMs:    form.InventoryTotals.TotalVMs,
			TotalCPU:    form.InventoryTotals.TotalCPU,
			TotalMemory: form.InventoryTotals.TotalMemory,
		},
	}
}

// MigrationComplexityResultToAPI converts the service result to the API response type.
func MigrationComplexityResultToAPI(result service.MigrationComplexityResult) api.MigrationComplexityResponse {
	byDisk := make([]api.ComplexityDiskScoreEntry, len(result.ComplexityByDisk))
	for i, entry := range result.ComplexityByDisk {
		byDisk[i] = api.ComplexityDiskScoreEntry{
			Score:       entry.Score,
			VmCount:     entry.VMCount,
			TotalSizeTB: entry.TotalSizeTB,
		}
	}

	byOS := make([]api.ComplexityOSScoreEntry, len(result.ComplexityByOS))
	for i, entry := range result.ComplexityByOS {
		byOS[i] = api.ComplexityOSScoreEntry{
			Score:   entry.Score,
			VmCount: entry.VMCount,
		}
	}

	byOSName := make([]api.ComplexityOSNameEntry, len(result.ComplexityByOSName))
	for i, entry := range result.ComplexityByOSName {
		byOSName[i] = api.ComplexityOSNameEntry{
			OsName:  entry.Name,
			Score:   entry.Score,
			VmCount: entry.VMCount,
		}
	}

	return api.MigrationComplexityResponse{
		ComplexityByDisk:   byDisk,
		ComplexityByOS:     byOS,
		ComplexityByOSName: byOSName,
		DiskSizeRatings:    result.DiskSizeRatings,
		OsRatings:          result.OSRatings,
	}
}

// MigrationEstimationResultToAPI converts the schema-keyed service result map to the API response.
func MigrationEstimationResultToAPI(
	results map[engines.Schema]*service.MigrationAssessmentResult,
) api.MigrationEstimationResponse {
	response := make(api.MigrationEstimationResponse, len(results))
	for schema, result := range results {
		response[string(schema)] = schemaResultToAPI(result)
	}
	return response
}

func schemaResultToAPI(result *service.MigrationAssessmentResult) api.SchemaEstimationResult {
	breakdown := make(map[string]api.EstimationDetail, len(result.Breakdown))
	for name, est := range result.Breakdown {
		breakdown[name] = estimationDetailToAPI(est)
	}
	return api.SchemaEstimationResult{
		MinTotalDuration: result.MinTotalDuration.String(),
		MaxTotalDuration: result.MaxTotalDuration.String(),
		Breakdown:        breakdown,
	}
}

// OsDiskEstimationResultToAPI maps the OsDisk complexity buckets, per-bucket estimation
// results, estimation context, and complexity matrix into the API response.
func OsDiskEstimationResultToAPI(
	buckets []complexity.OSDiskEntry,
	estimations map[int]map[string]*service.MigrationAssessmentResult,
	estimationCtx *service.EstimationContext,
	complexityMatrix map[complexity.Score]map[complexity.Score]complexity.Score,
) api.MigrationEstimationByComplexityResponse {
	entries := make([]api.OsDiskEstimationEntry, len(buckets))
	for i, b := range buckets {
		entry := api.OsDiskEstimationEntry{
			Score:           b.Score,
			VmCount:         b.VMCount,
			TotalDiskSizeTB: float32(b.TotalSizeTB),
		}
		if schemaResults, ok := estimations[b.Score]; ok {
			apiEst := make(map[string]api.SchemaEstimationResult, len(schemaResults))
			for schema, r := range schemaResults {
				apiEst[schema] = schemaResultToAPI(r)
			}
			entry.Estimation = &apiEst
		}
		entries[i] = entry
	}

	matrix := make(map[string]map[string]int, len(complexityMatrix))
	for osScore, diskMap := range complexityMatrix {
		inner := make(map[string]int, len(diskMap))
		for diskScore, combined := range diskMap {
			inner[fmt.Sprintf("%d", diskScore)] = combined
		}
		matrix[fmt.Sprintf("%d", osScore)] = inner
	}

	var ctx *api.EstimationContext
	if estimationCtx != nil {
		params := make(map[string]float32, len(estimationCtx.BaseParams))
		for _, p := range estimationCtx.BaseParams {
			if v, ok := p.Value.(float64); ok {
				params[p.Key] = float32(v)
			}
		}
		schemas := make([]string, len(estimationCtx.Schemas))
		for i, s := range estimationCtx.Schemas {
			schemas[i] = string(s)
		}
		ctx = &api.EstimationContext{Schemas: &schemas, Params: &params}
	}

	return api.MigrationEstimationByComplexityResponse{
		ComplexityByOsDisk: entries,
		ComplexityMatrix:   matrix,
		EstimationContext:  ctx,
	}
}

func estimationDetailToAPI(est estimation.Estimation) api.EstimationDetail {
	detail := api.EstimationDetail{Reason: est.Reason}
	if est.IsRanged() {
		min := est.MinDuration.String()
		max := est.MaxDuration.String()
		detail.MinDuration = &min
		detail.MaxDuration = &max
	} else {
		d := est.Duration.String()
		detail.Duration = &d
	}
	return detail
}
