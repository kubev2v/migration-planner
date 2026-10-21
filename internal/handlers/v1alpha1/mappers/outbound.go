package mappers

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"

	model2 "github.com/kubev2v/migration-planner/internal/model"

	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	agentAPI "github.com/kubev2v/migration-planner/api/v1alpha1/agent"
	"github.com/kubev2v/migration-planner/internal/service"
	"github.com/kubev2v/migration-planner/internal/service/mappers"
	"github.com/kubev2v/migration-planner/internal/store/model"
	"github.com/kubev2v/migration-planner/internal/util"
	"github.com/kubev2v/migration-planner/pkg/estimations/complexity"
	"github.com/kubev2v/migration-planner/pkg/estimations/engines"
	"github.com/kubev2v/migration-planner/pkg/estimations/estimation"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

// normalizeDomainInventory converts a structured domain inventory to the API
// model and normalizes nil collections so the response is free of null values.
func normalizeDomainInventory(inv model2.Inventory) api.Inventory {
	inventory := InventoryToAPI(inv)
	if inventory.Clusters == nil {
		inventory.Clusters = map[string]api.InventoryData{}
	}
	normalizeInventoryData(inventory.Vcenter)
	for clusterID, data := range inventory.Clusters {
		normalizeInventoryData(&data)
		inventory.Clusters[clusterID] = data
	}
	return inventory
}

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

	if s.UpdateType != "" {
		ut := api.SourceUpdateType(s.UpdateType)
		source.UpdateType = &ut
	}

	if len(s.Inventory) > 0 {
		v := util.GetInventoryVersion(s.Inventory)
		switch v {
		case model.InventoryVersionV1:
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

func SourceInventoryToApi(si model.SourceSubsetInventory) (agentAPI.SourceSubset, error) {
	// Convert UUIDs to openapi_types.UUID pointers
	id := openapi_types.UUID(si.ID)
	sourceId := openapi_types.UUID(si.SourceID)
	name := si.Name
	vcenterId := si.VCenterID
	vmsCount := si.VMsCount

	subset := agentAPI.SourceSubset{
		Id:        &id,
		Name:      &name,
		SourceId:  &sourceId,
		VcenterId: &vcenterId,
		VmsCount:  &vmsCount,
		CreatedAt: &si.CreatedAt,
		UpdatedAt: &si.UpdatedAt,
	}

	// Set updateType
	updateType := agentAPI.SourceSubsetUpdateType(si.UpdateType)
	subset.UpdateType = &updateType

	// Unmarshal inventory
	if len(si.Inventory) > 0 {
		var inventory api.Inventory
		if err := json.Unmarshal(si.Inventory, &inventory); err != nil {
			return agentAPI.SourceSubset{}, fmt.Errorf("failed to unmarshal inventory: %w", err)
		}
		subset.Inventory = &inventory
	}

	return subset, nil
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

func StoreAssessmentToApi(a model.Assessment) (api.Assessment, error) {
	// Todo: to be replaced by AssessmentToApi
	assessment := api.Assessment{
		Id:             a.ID,
		Name:           a.Name,
		OwnerFirstName: a.OwnerFirstName,
		OwnerLastName:  a.OwnerLastName,
		CreatedAt:      a.CreatedAt,
	}

	// Build API snapshots from assessment inventories
	var mainInventory *model.AssessmentInventory
	var subsets []api.AssessmentSubsetInventory
	for i := range a.Inventories {
		inv := &a.Inventories[i]
		if inv.IsSubset {
			parsedInv, err := unmarshalInventory(inv.Inventory, inv.Version)
			if err != nil {
				return api.Assessment{}, fmt.Errorf("failed to unmarshal subset inventory %q: %w", inv.Name, err)
			}
			subsets = append(subsets, api.AssessmentSubsetInventory{
				Id:        inv.ID,
				Name:      inv.Name,
				VcenterId: inv.VCenterID,
				VmsCount:  inv.VMsCount,
				CreatedAt: inv.CreatedAt,
				Inventory: parsedInv,
			})
		} else if mainInventory == nil {
			mainInventory = inv
		}
	}

	if mainInventory != nil {
		parsedInv, err := unmarshalInventory(mainInventory.Inventory, mainInventory.Version)
		if err != nil {
			return api.Assessment{}, err
		}
		snapshot := api.Snapshot{
			CreatedAt: mainInventory.CreatedAt,
			Inventory: parsedInv,
		}
		if len(subsets) > 0 {
			snapshot.SubsetInventories = &subsets
		}
		assessment.Snapshots = []api.Snapshot{snapshot}
	} else {
		assessment.Snapshots = []api.Snapshot{}
	}

	// Set source type based on source field
	sourceType := api.AssessmentSourceType(a.SourceType)
	assessment.SourceType = sourceType
	assessment.SourceId = a.SourceID

	if len(a.Permissions) > 0 {
		perms := make([]api.AssessmentPermissions, len(a.Permissions))
		for i, p := range a.Permissions {
			perms[i] = api.AssessmentPermissions(p)
		}
		assessment.Permissions = &perms
	}

	if a.Sharing != nil {
		sharing := api.AssessmentSharing{
			IsShared:   a.Sharing.IsShared,
			SharedWith: make([]api.SharingSubject, len(a.Sharing.SharedWith)),
		}
		for i, s := range a.Sharing.SharedWith {
			sharing.SharedWith[i] = api.SharingSubject{Type: s.Type, Id: s.ID, Name: s.Name}
		}
		if a.Sharing.SharedBy != nil {
			sharing.SharedBy = &api.SharingSubject{Type: a.Sharing.SharedBy.Type, Id: a.Sharing.SharedBy.ID, Name: a.Sharing.SharedBy.Name}
		}
		assessment.Sharing = &sharing
	}

	return assessment, nil
}

func AssessmentToApi(a model2.Assessment) (api.Assessment, error) {
	assessment := api.Assessment{
		Id:             a.ID,
		Name:           a.Name,
		OwnerFirstName: a.OwnerFirstName,
		OwnerLastName:  a.OwnerLastName,
		CreatedAt:      a.CreatedAt,
	}

	assessment.Snapshots = buildSnapshots(a.MainInventory, a.SubsetInventories)

	// Set source type based on source field
	sourceType := api.AssessmentSourceType(a.SourceType)
	assessment.SourceType = sourceType
	assessment.SourceId = a.SourceID

	if len(a.Permissions) > 0 {
		perms := make([]api.AssessmentPermissions, len(a.Permissions))
		for i, p := range a.Permissions {
			perms[i] = api.AssessmentPermissions(p)
		}
		assessment.Permissions = &perms
	}

	assessment.Sharing = sharingToApi(a.Sharing)

	return assessment, nil
}

func buildSnapshots(mainInventory model2.AssessmentInventory, subsetInventories []model2.AssessmentInventory) []api.Snapshot {
	if mainInventory.Inventory.Clusters == nil && mainInventory.Inventory.Vcenter == nil {
		return []api.Snapshot{}
	}

	var subsets []api.AssessmentSubsetInventory
	for _, inv := range subsetInventories {
		subsets = append(subsets, api.AssessmentSubsetInventory{
			Id:        inv.ID,
			Name:      inv.Name,
			VcenterId: inv.VCenterID,
			VmsCount:  inv.VMsCount,
			CreatedAt: inv.CreatedAt,
			Inventory: normalizeDomainInventory(inv.Inventory),
		})
	}

	snapshot := api.Snapshot{
		CreatedAt: mainInventory.CreatedAt,
		Inventory: normalizeDomainInventory(mainInventory.Inventory),
	}
	if len(subsets) > 0 {
		snapshot.SubsetInventories = &subsets
	}
	return []api.Snapshot{snapshot}
}

// sharingToApi converts domain sharing to the API representation.
func sharingToApi(s *model2.Sharing) *api.AssessmentSharing {
	if s == nil {
		return nil
	}
	sharing := api.AssessmentSharing{
		IsShared:   s.IsShared,
		SharedWith: make([]api.SharingSubject, len(s.SharedWith)),
	}
	for i, sub := range s.SharedWith {
		sharing.SharedWith[i] = api.SharingSubject{Type: sub.Type, Id: sub.ID, Name: sub.Name}
	}
	if s.SharedBy != nil {
		sharing.SharedBy = &api.SharingSubject{Type: s.SharedBy.Type, Id: s.SharedBy.ID, Name: s.SharedBy.Name}
	}
	return &sharing
}

func AssessmentListToApi(assessments []model.Assessment) (api.AssessmentList, error) {
	assessmentList := make([]api.Assessment, len(assessments))
	for i, assessment := range assessments {
		a, err := StoreAssessmentToApi(assessment)
		if err != nil {
			return api.AssessmentList{}, err
		}
		assessmentList[i] = a
	}
	return assessmentList, nil
}

func AssessmentV2ToApi(a model.Assessment) api.AssessmentV2Summary {
	result := api.AssessmentV2Summary{
		Id:             a.ID,
		Name:           a.Name,
		CreatedAt:      a.CreatedAt,
		OwnerFirstName: a.OwnerFirstName,
		OwnerLastName:  a.OwnerLastName,
		SourceId:       a.SourceID,
	}

	switch a.SourceType {
	case service.SourceTypeAgent:
		result.SourceType = api.AssessmentV2SummarySourceTypeSource
	case service.SourceTypeRvtools:
		result.SourceType = api.AssessmentV2SummarySourceTypeRvtools
	case service.SourceTypeInventory:
		result.SourceType = api.AssessmentV2SummarySourceTypeInventory
	}

	for _, inventory := range a.Inventories {
		if !inventory.IsSubset {
			result.VmsCount = inventory.VMsCount
			result.HostsCount = inventory.HostsCount
			result.NetworksCount = inventory.NetworksCount
			result.DatastoresCount = inventory.DatastoresCount
			break
		}
	}

	if len(a.Permissions) > 0 {
		permissions := make([]api.AssessmentV2SummaryPermissions, len(a.Permissions))
		for i, permission := range a.Permissions {
			permissions[i] = api.AssessmentV2SummaryPermissions(permission)
		}
		result.Permissions = &permissions
	}

	if a.Sharing != nil {
		sharing := api.AssessmentSharing{
			IsShared:   a.Sharing.IsShared,
			SharedWith: make([]api.SharingSubject, len(a.Sharing.SharedWith)),
		}
		for i, subject := range a.Sharing.SharedWith {
			sharing.SharedWith[i] = api.SharingSubject{Type: subject.Type, Id: subject.ID, Name: subject.Name}
		}
		if a.Sharing.SharedBy != nil {
			sharing.SharedBy = &api.SharingSubject{
				Type: a.Sharing.SharedBy.Type,
				Id:   a.Sharing.SharedBy.ID,
				Name: a.Sharing.SharedBy.Name,
			}
		}
		result.Sharing = &sharing
	}

	return result
}

func AssessmentClustersToAPI(clusters map[string]api.InventoryData) []api.SubResource {
	result := make([]api.SubResource, 0, len(clusters))
	for id := range clusters {
		result = append(result, api.SubResource{Id: id})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Id < result[j].Id })
	return result
}

func AssessmentInventoryToAPI(inv model2.AssessmentInventory) (api.InventoryV2, map[string]api.InventoryData, error) {
	result := api.InventoryV2{Id: inv.VCenterID, CreatedAt: &inv.CreatedAt}
	clusters := make(map[string]api.InventoryData)

	inventory := InventoryToAPI(inv.Inventory)

	if result.Id == "" {
		result.Id = inventory.VcenterId
	}

	if inventory.CreatedAt != nil {
		result.CreatedAt = inventory.CreatedAt
	}

	result.VcenterVersion = inventory.VcenterVersion

	if inventory.Vcenter != nil {
		result.Data = *inventory.Vcenter
	}

	normalizeInventoryData(&result.Data)

	for id, cluster := range inventory.Clusters {
		normalizeInventoryData(&cluster)
		clusters[id] = cluster
	}
	return result, clusters, nil
}

func StoreAssessmentInventoryToAPI(inv model.AssessmentInventory) (api.InventoryV2, map[string]api.InventoryData, error) {
	result := api.InventoryV2{Id: inv.VCenterID, CreatedAt: &inv.CreatedAt}
	clusters := make(map[string]api.InventoryData)
	if inv.Version == model.InventoryVersionV1 {
		if err := json.Unmarshal(inv.Inventory, &result.Data); err != nil {
			return api.InventoryV2{}, nil, fmt.Errorf("failed to unmarshal v1 inventory: %w", err)
		}
		if result.Id == "" && result.Data.Vcenter != nil {
			result.Id = result.Data.Vcenter.Id
		}
		normalizeInventoryData(&result.Data)
		return result, clusters, nil
	}

	var inventory api.Inventory
	if err := json.Unmarshal(inv.Inventory, &inventory); err != nil {
		return api.InventoryV2{}, nil, fmt.Errorf("failed to unmarshal v2 inventory: %w", err)
	}
	if result.Id == "" {
		result.Id = inventory.VcenterId
	}
	if inventory.CreatedAt != nil {
		result.CreatedAt = inventory.CreatedAt
	}
	result.VcenterVersion = inventory.VcenterVersion
	if inventory.Vcenter != nil {
		result.Data = *inventory.Vcenter
	}
	normalizeInventoryData(&result.Data)
	for id, cluster := range inventory.Clusters {
		normalizeInventoryData(&cluster)
		clusters[id] = cluster
	}
	return result, clusters, nil
}

func ClusterRequirementsResponseFormToAPI(form api.ClusterRequirementsResponse) api.ClusterRequirementsResponse {
	// The form is already the API type, just return it directly
	return form
}

func ClusterRequirementsInputFormToAPI(form mappers.ClusterRequirementsInputForm) api.ClusterRequirementsStoredInput {
	response := api.ClusterRequirementsStoredInput{
		ClusterId: form.ClusterID,
	}

	if form.CpuOverCommitRatio != nil {
		response.CpuOverCommitRatio = (*api.ClusterRequirementsStoredInputCpuOverCommitRatio)(form.CpuOverCommitRatio)
	}
	if form.MemoryOverCommitRatio != nil {
		response.MemoryOverCommitRatio = (*api.ClusterRequirementsStoredInputMemoryOverCommitRatio)(form.MemoryOverCommitRatio)
	}
	response.WorkerNodeCPU = form.WorkerNodeCPU
	response.WorkerNodeThreads = form.WorkerNodeThreads
	response.WorkerNodeMemory = form.WorkerNodeMemory
	response.ControlPlaneSchedulable = form.ControlPlaneSchedulable
	response.ControlPlaneCPU = form.ControlPlaneCPU
	response.ControlPlaneMemory = form.ControlPlaneMemory
	if form.ControlPlaneNodeCount != nil {
		count := api.ClusterRequirementsStoredInputControlPlaneNodeCount(*form.ControlPlaneNodeCount)
		response.ControlPlaneNodeCount = &count
	}
	response.HostedControlPlane = form.HostedControlPlane
	response.CompactMode = form.CompactMode

	return response
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
	estimationCtx *service.EstimationContext,
) api.MigrationEstimationResponse {
	estimation := make(map[string]api.SchemaEstimationResult, len(results))
	for schema, result := range results {
		estimation[string(schema)] = schemaResultToAPI(result)
	}

	var ctx api.EstimationContext
	if estimationCtx != nil {
		params := paramMapToAPI(estimationCtx.BaseParams)
		schemas := make([]string, len(estimationCtx.Schemas))
		for i, s := range estimationCtx.Schemas {
			schemas[i] = string(s)
		}
		ctx = api.EstimationContext{Schemas: &schemas, Params: &params}
	}

	return api.MigrationEstimationResponse{
		Estimation:        estimation,
		EstimationContext: ctx,
	}
}

// paramMapToAPI converts a slice of estimation.Param to the map[string]float32 used in EstimationContext.
func paramMapToAPI(params []estimation.Param) map[string]float32 {
	m := make(map[string]float32, len(params))
	for _, p := range params {
		if v, ok := util.AnyToFloat32(p.Value); ok {
			m[p.Key] = v
		}
	}
	return m
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
		params := paramMapToAPI(estimationCtx.BaseParams)
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

func unmarshalInventory(raw []byte, version uint) (api.Inventory, error) {
	if len(raw) == 0 {
		return api.Inventory{Clusters: make(map[string]api.InventoryData)}, nil
	}
	var inventory api.Inventory
	switch version {
	case model.InventoryVersionV1:
		var invV1 api.InventoryData
		if err := json.Unmarshal(raw, &invV1); err != nil {
			return api.Inventory{}, err
		}
		normalizeInventoryData(&invV1)
		inventory.Vcenter = &invV1
		if invV1.Vcenter != nil {
			inventory.VcenterId = invV1.Vcenter.Id
		}
		inventory.Clusters = make(map[string]api.InventoryData)
	default:
		if err := json.Unmarshal(raw, &inventory); err != nil {
			return api.Inventory{}, err
		}
		if inventory.Clusters == nil {
			inventory.Clusters = make(map[string]api.InventoryData)
		}
		if inventory.Vcenter != nil {
			normalizeInventoryData(inventory.Vcenter)
		}
		for clusterID, clusterData := range inventory.Clusters {
			normalizeInventoryData(&clusterData)
			inventory.Clusters[clusterID] = clusterData
		}
	}
	return inventory, nil
}
