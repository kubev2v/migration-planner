package service

import (
	"testing"

	"github.com/google/uuid"
	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_AggregateInventoryData_VCenterAggregate(t *testing.T) {
	// Setup: inventory with vCenter aggregate
	inventory := &api.Inventory{
		Vcenter: &api.InventoryData{
			Vms: api.VMs{
				Total: 100,
			},
		},
		Clusters: map[string]api.InventoryData{
			"cluster1": {Vms: api.VMs{Total: 50}},
			"cluster2": {Vms: api.VMs{Total: 50}},
		},
	}

	// Execute: single empty string means vCenter aggregate
	result, err := aggregateInventoryData(inventory, []string{""}, uuid.New())

	// Verify
	require.NoError(t, err)
	assert.Equal(t, 100, result.Vms.Total, "should return vCenter aggregate")
}

func Test_AggregateInventoryData_SingleCluster(t *testing.T) {
	inventory := &api.Inventory{
		Clusters: map[string]api.InventoryData{
			"cluster1": {
				Vms: api.VMs{
					Total: 50,
					OsInfo: &map[string]api.OsInfo{
						"RHEL 8":       {Count: 30},
						"Windows 2019": {Count: 20},
					},
				},
			},
		},
	}

	result, err := aggregateInventoryData(inventory, []string{"cluster1"}, uuid.New())

	require.NoError(t, err)
	assert.Equal(t, 50, result.Vms.Total)
	assert.Equal(t, 30, (*result.Vms.OsInfo)["RHEL 8"].Count)
}

func Test_AggregateInventoryData_MultiCluster_OsInfo(t *testing.T) {
	inventory := &api.Inventory{
		Clusters: map[string]api.InventoryData{
			"cluster1": {
				Vms: api.VMs{
					Total: 50,
					OsInfo: &map[string]api.OsInfo{
						"RHEL 8":       {Count: 30},
						"Windows 2019": {Count: 20},
					},
				},
			},
			"cluster2": {
				Vms: api.VMs{
					Total: 40,
					OsInfo: &map[string]api.OsInfo{
						"RHEL 8":       {Count: 25}, // Same OS as cluster1
						"Ubuntu 20.04": {Count: 15},
					},
				},
			},
		},
	}

	result, err := aggregateInventoryData(inventory, []string{"cluster1", "cluster2"}, uuid.New())

	require.NoError(t, err)
	assert.Equal(t, 90, result.Vms.Total, "total VMs should be sum")
	require.NotNil(t, result.Vms.OsInfo)
	assert.Equal(t, 55, (*result.Vms.OsInfo)["RHEL 8"].Count, "RHEL counts should be summed")
	assert.Equal(t, 20, (*result.Vms.OsInfo)["Windows 2019"].Count)
	assert.Equal(t, 15, (*result.Vms.OsInfo)["Ubuntu 20.04"].Count)
}

func Test_AggregateInventoryData_MultiCluster_DiskTiers(t *testing.T) {
	inventory := &api.Inventory{
		Clusters: map[string]api.InventoryData{
			"cluster1": {
				Vms: api.VMs{
					DiskComplexityTier: &map[string]api.DiskSizeTierSummary{
						"0-10TiB":  {VmCount: 20, TotalSizeTB: 5.5},
						"10-20TiB": {VmCount: 10, TotalSizeTB: 12.3},
					},
				},
			},
			"cluster2": {
				Vms: api.VMs{
					DiskComplexityTier: &map[string]api.DiskSizeTierSummary{
						"0-10TiB":  {VmCount: 15, TotalSizeTB: 3.2}, // Same tier
						"20-50TiB": {VmCount: 5, TotalSizeTB: 30.1},
					},
				},
			},
		},
	}

	result, err := aggregateInventoryData(inventory, []string{"cluster1", "cluster2"}, uuid.New())

	require.NoError(t, err)
	require.NotNil(t, result.Vms.DiskComplexityTier)

	tier0to10 := (*result.Vms.DiskComplexityTier)["0-10TiB"]
	assert.Equal(t, 35, tier0to10.VmCount, "should sum VM counts")
	assert.InDelta(t, 8.7, tier0to10.TotalSizeTB, 0.01, "should sum total sizes")

	tier10to20 := (*result.Vms.DiskComplexityTier)["10-20TiB"]
	assert.Equal(t, 10, tier10to20.VmCount)
	assert.InDelta(t, 12.3, tier10to20.TotalSizeTB, 0.01)
}

func Test_AggregateInventoryData_MultiCluster_ResourceBreakdown(t *testing.T) {
	inventory := &api.Inventory{
		Clusters: map[string]api.InventoryData{
			"cluster1": {
				Vms: api.VMs{
					CpuCores: api.VMResourceBreakdown{
						Total:                          100,
						TotalForMigratable:             80,
						TotalForMigratableWithWarnings: 15,
						TotalForNotMigratable:          5,
					},
					RamGB: api.VMResourceBreakdown{
						Total:              500,
						TotalForMigratable: 400,
					},
				},
			},
			"cluster2": {
				Vms: api.VMs{
					CpuCores: api.VMResourceBreakdown{
						Total:                          150,
						TotalForMigratable:             120,
						TotalForMigratableWithWarnings: 20,
						TotalForNotMigratable:          10,
					},
					RamGB: api.VMResourceBreakdown{
						Total:              750,
						TotalForMigratable: 600,
					},
				},
			},
		},
	}

	result, err := aggregateInventoryData(inventory, []string{"cluster1", "cluster2"}, uuid.New())

	require.NoError(t, err)
	assert.Equal(t, 250, result.Vms.CpuCores.Total)
	assert.Equal(t, 200, result.Vms.CpuCores.TotalForMigratable)
	assert.Equal(t, 35, result.Vms.CpuCores.TotalForMigratableWithWarnings)
	assert.Equal(t, 15, result.Vms.CpuCores.TotalForNotMigratable)
	assert.Equal(t, 1250, result.Vms.RamGB.Total)
	assert.Equal(t, 1000, result.Vms.RamGB.TotalForMigratable)
}

func Test_AggregateInventoryData_MultiCluster_ComplexityDistribution(t *testing.T) {
	inventory := &api.Inventory{
		Clusters: map[string]api.InventoryData{
			"cluster1": {
				Vms: api.VMs{
					ComplexityDistribution: &map[string]api.DiskSizeTierSummary{
						"0": {VmCount: 10, TotalSizeTB: 2.5},
						"1": {VmCount: 20, TotalSizeTB: 5.5},
						"2": {VmCount: 15, TotalSizeTB: 8.3},
					},
				},
			},
			"cluster2": {
				Vms: api.VMs{
					ComplexityDistribution: &map[string]api.DiskSizeTierSummary{
						"1": {VmCount: 12, TotalSizeTB: 3.2}, // Same score as cluster1
						"2": {VmCount: 18, TotalSizeTB: 7.1}, // Same score as cluster1
						"3": {VmCount: 5, TotalSizeTB: 10.5}, // New score
					},
				},
			},
		},
	}

	result, err := aggregateInventoryData(inventory, []string{"cluster1", "cluster2"}, uuid.New())

	require.NoError(t, err)
	require.NotNil(t, result.Vms.ComplexityDistribution)

	score0 := (*result.Vms.ComplexityDistribution)["0"]
	assert.Equal(t, 10, score0.VmCount, "score 0 should only come from cluster1")
	assert.InDelta(t, 2.5, score0.TotalSizeTB, 0.01)

	score1 := (*result.Vms.ComplexityDistribution)["1"]
	assert.Equal(t, 32, score1.VmCount, "score 1 should sum both clusters")
	assert.InDelta(t, 8.7, score1.TotalSizeTB, 0.01)

	score2 := (*result.Vms.ComplexityDistribution)["2"]
	assert.Equal(t, 33, score2.VmCount, "score 2 should sum both clusters")
	assert.InDelta(t, 15.4, score2.TotalSizeTB, 0.01)

	score3 := (*result.Vms.ComplexityDistribution)["3"]
	assert.Equal(t, 5, score3.VmCount, "score 3 should only come from cluster2")
	assert.InDelta(t, 10.5, score3.TotalSizeTB, 0.01)
}

func Test_AggregateInventoryData_ErrorCases(t *testing.T) {
	inventory := &api.Inventory{
		Clusters: map[string]api.InventoryData{
			"cluster1": {Vms: api.VMs{Total: 50}},
		},
	}

	t.Run("empty cluster IDs", func(t *testing.T) {
		_, err := aggregateInventoryData(inventory, []string{}, uuid.New())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "at least one cluster ID required")
	})

	t.Run("non-existent cluster", func(t *testing.T) {
		_, err := aggregateInventoryData(inventory, []string{"nonexistent"}, uuid.New())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cluster nonexistent not found")
	})

	t.Run("vCenter aggregate with no vCenter data", func(t *testing.T) {
		_, err := aggregateInventoryData(inventory, []string{""}, uuid.New())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "inventory has no vcenter-level data")
	})

	t.Run("multi-cluster with one non-existent cluster", func(t *testing.T) {
		_, err := aggregateInventoryData(inventory, []string{"cluster1", "nonexistent"}, uuid.New())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cluster nonexistent not found")
	})
}
