package helpers

import (
	"math"
	"time"

	api "github.com/kubev2v/migration-planner/api/v1alpha1"
)

func aggregate(inventories []api.InventoryData) api.Inventory {
	now := time.Now().UTC()
	result := api.Inventory{Clusters: map[string]api.InventoryData{}, CreatedAt: &now}
	if len(inventories) == 0 {
		return result
	}

	aggregated := aggregateInventoryData(inventories)
	result.Vcenter = &aggregated
	if aggregated.Vcenter != nil {
		result.VcenterId = aggregated.Vcenter.Id
	}
	return result
}

func aggregateInventoryData(inventories []api.InventoryData) api.InventoryData {
	vms := aggregateVMs(inventories)

	var vcenter *api.VCenter
	if vID, ok := vcenterID(inventories); ok {
		vcenter = &api.VCenter{Id: vID}
	}

	return api.InventoryData{
		Infra:   aggregateInfra(inventories, vms),
		Vcenter: vcenter,
		Vms:     vms,
	}
}

func aggregateVMs(inventories []api.InventoryData) api.VMs {
	result := api.VMs{PowerStates: map[string]int{}}

	var complexityDistribution, diskComplexityTier, diskSizeTier map[string]api.DiskSizeTierSummary
	var diskTypes map[string]api.DiskTypeSummary
	var osInfo map[string]api.OsInfo
	var distributionByCpuTier, distributionByMemoryTier, distributionByNicCount map[string]int

	var nicCount api.VMResourceBreakdown
	var nicCountSeen bool

	for _, inventory := range inventories {
		vms := inventory.Vms

		result.Total += vms.Total
		result.TotalMigratable += vms.TotalMigratable

		result.TotalMigratableWithWarnings = addOptionalInt(result.TotalMigratableWithWarnings, vms.TotalMigratableWithWarnings)
		result.TotalWithFaultTolerance = addOptionalInt(result.TotalWithFaultTolerance, vms.TotalWithFaultTolerance)
		result.TotalWithRDM = addOptionalInt(result.TotalWithRDM, vms.TotalWithRDM)
		result.TotalWithSharedDisks = addOptionalInt(result.TotalWithSharedDisks, vms.TotalWithSharedDisks)

		result.CpuCores = addVMResourceBreakdown(result.CpuCores, vms.CpuCores)
		result.DiskCount = addVMResourceBreakdown(result.DiskCount, vms.DiskCount)
		result.DiskGB = addVMResourceBreakdown(result.DiskGB, vms.DiskGB)
		result.RamGB = addVMResourceBreakdown(result.RamGB, vms.RamGB)
		if vms.NicCount != nil {
			nicCount = addVMResourceBreakdown(nicCount, *vms.NicCount)
			nicCountSeen = true
		}

		for state, count := range vms.PowerStates {
			result.PowerStates[state] += count
		}

		complexityDistribution = mergeMap(complexityDistribution, vms.ComplexityDistribution, addDiskSizeTierSummary)
		diskComplexityTier = mergeMap(diskComplexityTier, vms.DiskComplexityTier, addDiskSizeTierSummary)
		diskSizeTier = mergeMap(diskSizeTier, vms.DiskSizeTier, addDiskSizeTierSummary)
		diskTypes = mergeMap(diskTypes, vms.DiskTypes, addDiskTypeSummary)
		osInfo = mergeMap(osInfo, vms.OsInfo, mergeOsInfo)
		distributionByCpuTier = mergeMap(distributionByCpuTier, vms.DistributionByCpuTier, addInt)
		distributionByMemoryTier = mergeMap(distributionByMemoryTier, vms.DistributionByMemoryTier, addInt)
		distributionByNicCount = mergeMap(distributionByNicCount, vms.DistributionByNicCount, addInt)
	}

	if nicCountSeen {
		result.NicCount = &nicCount
	}
	result.ComplexityDistribution = optionalMap(complexityDistribution)
	result.DiskComplexityTier = optionalMap(diskComplexityTier)
	result.DiskSizeTier = optionalMap(diskSizeTier)
	result.DiskTypes = optionalMap(diskTypes)
	result.OsInfo = optionalMap(osInfo)
	result.DistributionByCpuTier = optionalMap(distributionByCpuTier)
	result.DistributionByMemoryTier = optionalMap(distributionByMemoryTier)
	result.DistributionByNicCount = optionalMap(distributionByNicCount)

	result.IssuesBreakdown = aggregateIssuesBreakdown(inventories)
	result.MigrationWarnings = aggregateMigrationWarnings(inventories)
	result.NotMigratableReasons = aggregateNotMigratableReasons(inventories)

	return result
}

func aggregateInfra(inventories []api.InventoryData, vms api.VMs) api.Infra {
	result := api.Infra{
		Datastores:      make([]api.Datastore, 0),
		HostPowerStates: map[string]int{},
		Networks:        make([]api.Network, 0),
	}

	var hosts []api.Host
	var hostsPresent bool
	var totalDatacenters *int
	for _, inventory := range inventories {
		infra := inventory.Infra
		result.TotalHosts += infra.TotalHosts
		result.Datastores = append(result.Datastores, infra.Datastores...)
		result.Networks = append(result.Networks, infra.Networks...)
		for state, count := range infra.HostPowerStates {
			result.HostPowerStates[state] += count
		}
		totalDatacenters = addOptionalInt(totalDatacenters, infra.TotalDatacenters)
		if infra.Hosts != nil {
			hostsPresent = true
			hosts = append(hosts, *infra.Hosts...)
		}
	}
	if hostsPresent {
		result.Hosts = &hosts
	}

	clustersPerDatacenter := aggregateClustersPerDatacenter(inventories)

	result.ClustersPerDatacenter = &clustersPerDatacenter
	result.TotalDatacenters = totalDatacenters
	result.StandaloneHostsDetected = aggregateStandaloneHostFlag(inventories)
	result.VmsPerHostAverage = averageVMsPerHost(vms.Total, result.TotalHosts)
	result.CpuOverCommitment = aggregateCPUOverCommitment(inventories, vms.CpuCores.Total)
	result.MemoryOverCommitment = aggregateMemoryOverCommitment(inventories, vms.RamGB.Total)
	return result
}

func addDiskSizeTierSummary(a, b api.DiskSizeTierSummary) api.DiskSizeTierSummary {
	a.TotalSizeTB += b.TotalSizeTB
	a.VmCount += b.VmCount
	return a
}

func addDiskTypeSummary(a, b api.DiskTypeSummary) api.DiskTypeSummary {
	a.TotalSizeTB += b.TotalSizeTB
	a.VmCount += b.VmCount
	return a
}

func addInt(a, b int) int { return a + b }

// addOptionalInt adds an optional value into an optional accumulator. The result
// stays nil until at least one non-nil value is seen.
func addOptionalInt(acc, value *int) *int {
	if value == nil {
		return acc
	}
	sum := *value
	if acc != nil {
		sum += *acc
	}
	return &sum
}

func addVMResourceBreakdown(a, b api.VMResourceBreakdown) api.VMResourceBreakdown {
	a.Total += b.Total
	a.TotalForMigratable += b.TotalForMigratable
	a.TotalForMigratableWithWarnings += b.TotalForMigratableWithWarnings
	a.TotalForNotMigratable += b.TotalForNotMigratable
	return a
}

func aggregateClustersPerDatacenter(inventories []api.InventoryData) []int {
	var result []int
	seen := false
	for _, inventory := range inventories {
		values := inventory.Infra.ClustersPerDatacenter
		if values == nil {
			continue
		}
		seen = true
		result = append(result, *values...)
	}
	if !seen {
		return nil
	}
	return result
}

func aggregateCPUOverCommitment(inventories []api.InventoryData, totalAllocated int) *float64 {
	var physicalCores float64
	complete := true
	for _, inventory := range inventories {
		hosts := inventory.Infra.Hosts
		if hosts == nil {
			if inventory.Infra.TotalHosts > 0 {
				complete = false
			}
			continue
		}
		if inventory.Infra.TotalHosts > len(*hosts) {
			complete = false
		}
		for _, host := range *hosts {
			if host.CpuCores == nil {
				complete = false
				continue
			}
			physicalCores += float64(*host.CpuCores)
		}
	}
	if complete && physicalCores > 0 {
		ratio := float64(totalAllocated) / physicalCores
		return &ratio
	}
	return nil
}

func aggregateIssuesBreakdown(inventories []api.InventoryData) *api.IssuesBreakdown {
	result := &api.IssuesBreakdown{}
	seen := false
	for _, inventory := range inventories {
		issues := inventory.Vms.IssuesBreakdown
		if issues == nil {
			continue
		}
		seen = true
		result.Advisory += issues.Advisory
		result.Critical += issues.Critical
		result.Error += issues.Error
		result.Information += issues.Information
		result.Warning += issues.Warning
	}
	if !seen {
		return nil
	}
	return result
}

func aggregateMemoryOverCommitment(inventories []api.InventoryData, totalAllocatedGB int) *float64 {
	var physicalMemoryMB float64
	complete := true
	for _, inventory := range inventories {
		hosts := inventory.Infra.Hosts
		if hosts == nil {
			if inventory.Infra.TotalHosts > 0 {
				complete = false
			}
			continue
		}
		if inventory.Infra.TotalHosts > len(*hosts) {
			complete = false
		}
		for _, host := range *hosts {
			if host.MemoryMB == nil {
				complete = false
				continue
			}
			physicalMemoryMB += float64(*host.MemoryMB)
		}
	}
	if complete && physicalMemoryMB > 0 {
		ratio := float64(totalAllocatedGB) * 1024 / physicalMemoryMB
		return &ratio
	}
	return nil
}

func aggregateMigrationWarnings(inventories []api.InventoryData) []api.MigrationIssue {
	result := make(map[string]api.MigrationIssue)
	for _, inventory := range inventories {
		mergeMigrationIssues(result, inventory.Vms.MigrationWarnings)
	}
	return migrationIssuesSlice(result)
}

func aggregateNotMigratableReasons(inventories []api.InventoryData) []api.MigrationIssue {
	result := make(map[string]api.MigrationIssue)
	for _, inventory := range inventories {
		mergeMigrationIssues(result, inventory.Vms.NotMigratableReasons)
	}
	return migrationIssuesSlice(result)
}

func aggregateStandaloneHostFlag(inventories []api.InventoryData) *bool {
	var known, anyTrue bool
	for _, inventory := range inventories {
		value := inventory.Infra.StandaloneHostsDetected
		if value == nil {
			continue
		}
		known = true
		if *value {
			anyTrue = true
		}
	}
	if anyTrue {
		value := true
		return &value
	}
	if !known {
		return nil
	}
	value := false
	return &value
}

func averageVMsPerHost(totalVMs, totalHosts int) *float64 {
	if totalHosts <= 0 {
		return nil
	}
	average := math.Round(float64(totalVMs)*100/float64(totalHosts)) / 100
	return &average
}

// mergeMap folds src into dst, combining colliding keys with combine. The first
// value seen for a key is stored as-is; later values are passed as the second
// argument to combine. dst is allocated on the first non-nil src and stays nil
// otherwise, so the caller can distinguish "no data" from "empty".
func mergeMap[T any](dst map[string]T, src *map[string]T, combine func(existing, incoming T) T) map[string]T {
	if src == nil {
		return dst
	}
	if dst == nil {
		dst = make(map[string]T)
	}
	for key, value := range *src {
		if existing, ok := dst[key]; ok {
			dst[key] = combine(existing, value)
		} else {
			dst[key] = value
		}
	}
	return dst
}

// mergeMigrationIssues folds incoming issues with an ID into result, summing
// Count for issues that share the same ID. Issues without an ID are discarded.
func mergeMigrationIssues(result map[string]api.MigrationIssue, incoming []api.MigrationIssue) {
	for _, issue := range incoming {
		if issue.Id == nil || *issue.Id == "" {
			continue
		}
		id := *issue.Id
		if existing, exists := result[id]; exists {
			existing.Count += issue.Count
			result[id] = existing
		} else {
			result[id] = issue
		}
	}
}

func migrationIssuesSlice(issues map[string]api.MigrationIssue) []api.MigrationIssue {
	result := make([]api.MigrationIssue, 0, len(issues))
	for _, issue := range issues {
		result = append(result, issue)
	}
	return result
}

// mergeOsInfo sums Count and ANDs Supported, keeping the first-seen entry's
// support tier and upgrade recommendation (inventories are ordered newest first).
func mergeOsInfo(existing, incoming api.OsInfo) api.OsInfo {
	existing.Count += incoming.Count
	existing.Supported = existing.Supported && incoming.Supported
	return existing
}

// optionalMap wraps an accumulated map, returning nil when nothing was reported
// so the output distinguishes "no data" from "empty".
func optionalMap[V any](m map[string]V) *map[string]V {
	if m == nil {
		return nil
	}
	return &m
}

func vcenterID(inventories []api.InventoryData) (string, bool) {
	if len(inventories) == 0 || inventories[0].Vcenter == nil {
		return "", false
	}

	id := inventories[0].Vcenter.Id
	for _, inventory := range inventories[1:] {
		if inventory.Vcenter == nil || inventory.Vcenter.Id != id {
			return "", false
		}
	}
	return id, true
}
