package mappers

import (
	"encoding/json"
	"fmt"
	"slices"

	"github.com/kubev2v/migration-planner/api/v1alpha1"
	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/service"
	"github.com/kubev2v/migration-planner/internal/service/mappers"
	"github.com/kubev2v/migration-planner/internal/store/model"
	"github.com/kubev2v/migration-planner/internal/util"
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
			i := v1alpha1.InventoryData{}
			if err := json.Unmarshal(s.Inventory, &i); err != nil {
				return api.Source{}, fmt.Errorf("failed to unmarshal v1 inventory: %w", err)
			}
			if i.Vcenter == nil {
				return api.Source{}, fmt.Errorf("v1 inventory missing vcenter data")
			}
			// Normalize to prevent null values from database
			normalizeInventoryData(&i)
			source.Inventory = &v1alpha1.Inventory{
				Vcenter:   &i,
				VcenterId: i.Vcenter.Id,
				Clusters:  map[string]api.InventoryData{},
			}
		default:
			v2 := v1alpha1.Inventory{}
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
			inventory := v1alpha1.Inventory{}
			switch snapshot.Version {
			case 1:
				invV1 := v1alpha1.InventoryData{}
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

func ClusterRequirementsResponseFormToAPI(form mappers.ClusterRequirementsResponseForm) v1alpha1.ClusterRequirementsResponse {
	resourceConsumption := v1alpha1.SizingResourceConsumption{
		Cpu:    form.ResourceConsumption.CPU,
		Memory: form.ResourceConsumption.Memory,
	}

	if form.ResourceConsumption.Limits.CPU != 0.0 || form.ResourceConsumption.Limits.Memory != 0.0 {
		resourceConsumption.Limits = &v1alpha1.SizingResourceLimits{
			Cpu:    form.ResourceConsumption.Limits.CPU,
			Memory: form.ResourceConsumption.Limits.Memory,
		}
	}

	if form.ResourceConsumption.OverCommitRatio.CPU != 0.0 || form.ResourceConsumption.OverCommitRatio.Memory != 0.0 {
		resourceConsumption.OverCommitRatio = &v1alpha1.SizingOverCommitRatio{
			Cpu:    form.ResourceConsumption.OverCommitRatio.CPU,
			Memory: form.ResourceConsumption.OverCommitRatio.Memory,
		}
	}

	return v1alpha1.ClusterRequirementsResponse{
		ClusterSizing: v1alpha1.ClusterSizing{
			TotalNodes:        form.ClusterSizing.TotalNodes,
			ControlPlaneNodes: form.ClusterSizing.ControlPlaneNodes,
			WorkerNodes:       form.ClusterSizing.WorkerNodes,
			FailoverNodes:     form.ClusterSizing.FailoverNodes,
			TotalCPU:          form.ClusterSizing.TotalCPU,
			TotalMemory:       form.ClusterSizing.TotalMemory,
		},
		ResourceConsumption: resourceConsumption,
		InventoryTotals: v1alpha1.InventoryTotals{
			TotalVMs:    form.InventoryTotals.TotalVMs,
			TotalCPU:    form.InventoryTotals.TotalCPU,
			TotalMemory: form.InventoryTotals.TotalMemory,
		},
	}
}

// MigrationEstimationResultToAPI converts service MigrationAssessmentResult to API response
func MigrationEstimationResultToAPI(result service.MigrationAssessmentResult) v1alpha1.MigrationEstimationResponse {
	breakdown := make(map[string]v1alpha1.EstimationDetail)

	for name, estimation := range result.Breakdown {
		breakdown[name] = v1alpha1.EstimationDetail{
			Duration: estimation.Duration.String(),
			Reason:   estimation.Reason,
		}
	}

	return v1alpha1.MigrationEstimationResponse{
		TotalDuration: result.TotalDuration.String(),
		Breakdown:     breakdown,
	}
}
