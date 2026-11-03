package helpers

import (
	"encoding/json"

	"github.com/google/uuid"
	svcmodel "github.com/kubev2v/migration-planner/internal/model"
	storemappers "github.com/kubev2v/migration-planner/internal/store/mappers"
	"github.com/kubev2v/migration-planner/internal/store/model"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("inventory aggregation", func() {
	It("combines VM totals and utilization across inventories", func() {
		inventories := []svcmodel.InventoryData{
			{
				Vcenter: &svcmodel.VCenter{Id: "vc-1"},
				Vms: svcmodel.VMs{
					Total:    2,
					CpuCores: svcmodel.VMResourceBreakdown{Total: 2},
					RamGB:    svcmodel.VMResourceBreakdown{Total: 4},
					DiskGB:   svcmodel.VMResourceBreakdown{Total: 10},
				},
			},
			{
				Vcenter: &svcmodel.VCenter{Id: "vc-1"},
				Vms: svcmodel.VMs{
					Total:    6,
					CpuCores: svcmodel.VMResourceBreakdown{Total: 6},
					RamGB:    svcmodel.VMResourceBreakdown{Total: 12},
					DiskGB:   svcmodel.VMResourceBreakdown{Total: 15},
				},
			},
		}

		got := AggregateInventoryData(inventories)

		Expect(got.Vcenter).To(Equal(&svcmodel.VCenter{Id: "vc-1"}))
		Expect(got.Vms.Total).To(Equal(8))
		Expect(got.Vms.CpuCores.Total).To(Equal(8))
		Expect(got.Vms.RamGB.Total).To(Equal(16))
		Expect(got.Vms.DiskGB.Total).To(Equal(25))
	})

	It("aggregates infrastructure fields across inventories", func() {
		host1CPU, host2CPU, host3CPU := 4, 4, 4
		host1Memory, host2Memory, host3Memory := int64(8192), int64(8192), int64(8192)
		host1ID, host2ID, host3ID := "host-1", "host-2", "host-3"
		totalDatacenters1, totalDatacenters2 := 1, 2
		standalone1, standalone2 := false, true
		clustersPerDatacenter1 := []int{1}
		clustersPerDatacenter2 := []int{2, 1}

		hosts1 := []svcmodel.Host{{Id: &host1ID, CpuCores: &host1CPU, MemoryMB: &host1Memory}}
		hosts2 := []svcmodel.Host{
			{Id: &host2ID, CpuCores: &host2CPU, MemoryMB: &host2Memory},
			{Id: &host3ID, CpuCores: &host3CPU, MemoryMB: &host3Memory},
		}
		inventories := []svcmodel.InventoryData{
			{
				Vms: svcmodel.VMs{
					Total:    2,
					CpuCores: svcmodel.VMResourceBreakdown{Total: 2},
					RamGB:    svcmodel.VMResourceBreakdown{Total: 4},
				},
				Infra: svcmodel.Infra{
					TotalHosts:              1,
					Hosts:                   &hosts1,
					Datastores:              []svcmodel.Datastore{{DiskId: "datastore-1"}},
					Networks:                []svcmodel.Network{{Name: "network-1", Type: "standard"}},
					HostPowerStates:         map[string]int{"poweredOn": 1, "poweredOff": 2},
					TotalDatacenters:        &totalDatacenters1,
					ClustersPerDatacenter:   &clustersPerDatacenter1,
					StandaloneHostsDetected: &standalone1,
				},
			},
			{
				Vms: svcmodel.VMs{
					Total:    6,
					CpuCores: svcmodel.VMResourceBreakdown{Total: 6},
					RamGB:    svcmodel.VMResourceBreakdown{Total: 12},
				},
				Infra: svcmodel.Infra{
					TotalHosts:              2,
					Hosts:                   &hosts2,
					Datastores:              []svcmodel.Datastore{{DiskId: "datastore-2"}},
					Networks:                []svcmodel.Network{{Name: "network-2", Type: "distributed"}},
					HostPowerStates:         map[string]int{"poweredOn": 2, "maintenance": 1},
					TotalDatacenters:        &totalDatacenters2,
					ClustersPerDatacenter:   &clustersPerDatacenter2,
					StandaloneHostsDetected: &standalone2,
				},
			},
		}

		got := AggregateInventoryData(inventories)

		infra := got.Infra
		Expect(infra.TotalHosts).To(Equal(3))
		Expect(infra.Hosts).ToNot(BeNil())
		Expect(*infra.Hosts).To(Equal(append(hosts1, hosts2...)))
		Expect(infra.Datastores).To(Equal([]svcmodel.Datastore{{DiskId: "datastore-1"}, {DiskId: "datastore-2"}}))
		Expect(infra.Networks).To(Equal([]svcmodel.Network{
			{Name: "network-1", Type: "standard"},
			{Name: "network-2", Type: "distributed"},
		}))
		Expect(infra.HostPowerStates).To(Equal(map[string]int{"poweredOn": 3, "poweredOff": 2, "maintenance": 1}))
		Expect(infra.TotalDatacenters).ToNot(BeNil())
		Expect(*infra.TotalDatacenters).To(Equal(3))
		Expect(infra.ClustersPerDatacenter).ToNot(BeNil())
		Expect(*infra.ClustersPerDatacenter).To(Equal([]int{1, 2, 1}))
		Expect(infra.StandaloneHostsDetected).ToNot(BeNil())
		Expect(*infra.StandaloneHostsDetected).To(BeTrue())
		Expect(infra.VmsPerHostAverage).ToNot(BeNil())
		Expect(*infra.VmsPerHostAverage).To(Equal(2.67))
		Expect(infra.CpuOverCommitment).ToNot(BeNil())
		Expect(*infra.CpuOverCommitment).To(Equal(8.0 / 12.0))
		Expect(infra.MemoryOverCommitment).ToNot(BeNil())
		Expect(*infra.MemoryOverCommitment).To(Equal(16.0 * 1024 / (3 * 8192)))
	})

	It("aggregates a slice with more than three inventories", func() {
		inventories := []svcmodel.InventoryData{
			{
				Vms: svcmodel.VMs{Total: 1, CpuCores: svcmodel.VMResourceBreakdown{Total: 1}, RamGB: svcmodel.VMResourceBreakdown{Total: 1}, DiskGB: svcmodel.VMResourceBreakdown{Total: 10}},
			},
			{
				Vms: svcmodel.VMs{Total: 2, CpuCores: svcmodel.VMResourceBreakdown{Total: 2}, RamGB: svcmodel.VMResourceBreakdown{Total: 2}, DiskGB: svcmodel.VMResourceBreakdown{Total: 20}},
			},
			{
				Vms: svcmodel.VMs{Total: 3, CpuCores: svcmodel.VMResourceBreakdown{Total: 3}, RamGB: svcmodel.VMResourceBreakdown{Total: 3}, DiskGB: svcmodel.VMResourceBreakdown{Total: 30}},
			},
			{
				Vms: svcmodel.VMs{Total: 4, CpuCores: svcmodel.VMResourceBreakdown{Total: 4}, RamGB: svcmodel.VMResourceBreakdown{Total: 4}, DiskGB: svcmodel.VMResourceBreakdown{Total: 40}},
			},
		}

		got := AggregateInventoryData(inventories)

		Expect(got.Vms.Total).To(Equal(10))
		Expect(got.Vms.CpuCores.Total).To(Equal(10))
		Expect(got.Vms.RamGB.Total).To(Equal(10))
		Expect(got.Vms.DiskGB.Total).To(Equal(100))
	})

	It("omits the vCenter ID when inventory IDs disagree", func() {
		inventories := []svcmodel.InventoryData{
			{Vcenter: &svcmodel.VCenter{Id: "vc-1"}},
			{Vcenter: &svcmodel.VCenter{Id: "vc-2"}},
		}

		got := AggregateInventoryData(inventories)

		Expect(got.Vcenter).To(BeNil())
	})

	It("returns an empty aggregate when there are no inventories", func() {
		got := AggregateInventoryData(nil)

		Expect(got.Vms.Total).To(Equal(0))
		Expect(got.Vcenter).To(BeNil())
	})
})

var _ = Describe("Aggregate", func() {
	It("aggregates V2 assessment inventories", func() {
		inv1 := mustMarshal(svcmodel.Inventory{
			VcenterId: "vc-1",
			Clusters:  map[string]svcmodel.InventoryData{},
			Vcenter: &svcmodel.InventoryData{
				Vcenter: &svcmodel.VCenter{Id: "vc-1"},
				Vms:     svcmodel.VMs{Total: 3, CpuCores: svcmodel.VMResourceBreakdown{Total: 4}},
				Infra:   svcmodel.Infra{TotalHosts: 1, Datastores: []svcmodel.Datastore{{DiskId: "ds-1"}}, Networks: []svcmodel.Network{{Name: "net-1"}}},
			},
		})
		inv2 := mustMarshal(svcmodel.Inventory{
			VcenterId: "vc-1",
			Clusters:  map[string]svcmodel.InventoryData{},
			Vcenter: &svcmodel.InventoryData{
				Vcenter: &svcmodel.VCenter{Id: "vc-1"},
				Vms:     svcmodel.VMs{Total: 5, CpuCores: svcmodel.VMResourceBreakdown{Total: 8}},
				Infra:   svcmodel.Infra{TotalHosts: 2, Datastores: []svcmodel.Datastore{{DiskId: "ds-2"}}, Networks: []svcmodel.Network{{Name: "net-2"}}},
			},
		})

		models := []model.AssessmentInventory{
			{ID: uuid.New(), Version: model.InventoryVersionV2, Inventory: inv1},
			{ID: uuid.New(), Version: model.InventoryVersionV2, Inventory: inv2},
		}

		inventories := make([]svcmodel.AssessmentInventory, len(models))
		for i, stored := range models {
			inventory, err := storemappers.AssessmentInventoryFromStore(stored)
			Expect(err).NotTo(HaveOccurred())
			inventories[i] = inventory
		}
		got, err := Aggregate(inventories)
		Expect(err).ToNot(HaveOccurred())
		Expect(got.Vcenter).ToNot(BeNil())
		Expect(got.Vcenter.Vms.Total).To(Equal(8))
		Expect(got.Vcenter.Vms.CpuCores.Total).To(Equal(12))
		Expect(got.Vcenter.Infra.TotalHosts).To(Equal(3))
		Expect(got.Vcenter.Infra.Datastores).To(HaveLen(2))
		Expect(got.Vcenter.Infra.Networks).To(HaveLen(2))
	})

	It("aggregates V1 assessment inventories", func() {
		inv1 := mustMarshal(svcmodel.InventoryData{
			Vcenter: &svcmodel.VCenter{Id: "vc-1"},
			Vms:     svcmodel.VMs{Total: 2},
			Infra:   svcmodel.Infra{TotalHosts: 1, Datastores: []svcmodel.Datastore{{DiskId: "ds-1"}}, Networks: []svcmodel.Network{{Name: "n-1"}}},
		})
		inv2 := mustMarshal(svcmodel.InventoryData{
			Vcenter: &svcmodel.VCenter{Id: "vc-1"},
			Vms:     svcmodel.VMs{Total: 3},
			Infra:   svcmodel.Infra{TotalHosts: 2, Datastores: []svcmodel.Datastore{{DiskId: "ds-2"}}, Networks: []svcmodel.Network{{Name: "n-2"}}},
		})

		models := []model.AssessmentInventory{
			{ID: uuid.New(), Version: model.InventoryVersionV1, Inventory: inv1},
			{ID: uuid.New(), Version: model.InventoryVersionV1, Inventory: inv2},
		}

		inventories := make([]svcmodel.AssessmentInventory, len(models))
		for i, stored := range models {
			inventory, err := storemappers.AssessmentInventoryFromStore(stored)
			Expect(err).NotTo(HaveOccurred())
			inventories[i] = inventory
		}
		got, err := Aggregate(inventories)
		Expect(err).ToNot(HaveOccurred())
		Expect(got.Vcenter).ToNot(BeNil())
		Expect(got.Vcenter.Vms.Total).To(Equal(5))
		Expect(got.Vcenter.Infra.TotalHosts).To(Equal(3))
	})

	It("returns empty inventory for nil input", func() {
		got, err := Aggregate(nil)
		Expect(err).ToNot(HaveOccurred())
		Expect(got.Vcenter).To(BeNil())
	})
})

var _ = Describe("AssessmentInventoryFromStore", func() {
	It("unmarshals V2 inventory", func() {
		raw := mustMarshal(svcmodel.Inventory{
			VcenterId: "vc-1",
			Clusters:  map[string]svcmodel.InventoryData{},
			Vcenter: &svcmodel.InventoryData{
				Vcenter: &svcmodel.VCenter{Id: "vc-1"},
				Vms:     svcmodel.VMs{Total: 10},
				Infra:   svcmodel.Infra{TotalHosts: 3},
			},
		})

		inv := model.AssessmentInventory{Version: model.InventoryVersionV2, Inventory: raw}
		inventory, err := storemappers.AssessmentInventoryFromStore(inv)
		got := svcmodel.InventoryData{}
		if inventory.Inventory.Vcenter != nil {
			got = *inventory.Inventory.Vcenter
		}
		Expect(err).ToNot(HaveOccurred())
		Expect(got.Vms.Total).To(Equal(10))
		Expect(got.Infra.TotalHosts).To(Equal(3))
	})

	It("unmarshals V1 inventory", func() {
		raw := mustMarshal(svcmodel.InventoryData{
			Vcenter: &svcmodel.VCenter{Id: "vc-1"},
			Vms:     svcmodel.VMs{Total: 5},
			Infra:   svcmodel.Infra{TotalHosts: 2},
		})

		inv := model.AssessmentInventory{Version: model.InventoryVersionV1, Inventory: raw}
		inventory, err := storemappers.AssessmentInventoryFromStore(inv)
		got := svcmodel.InventoryData{}
		if inventory.Inventory.Vcenter != nil {
			got = *inventory.Inventory.Vcenter
		}
		Expect(err).ToNot(HaveOccurred())
		Expect(got.Vms.Total).To(Equal(5))
		Expect(got.Infra.TotalHosts).To(Equal(2))
	})

	It("returns empty data when V2 inventory has nil vcenter", func() {
		raw := mustMarshal(svcmodel.Inventory{
			VcenterId: "vc-1",
			Clusters:  map[string]svcmodel.InventoryData{},
		})

		inv := model.AssessmentInventory{Version: model.InventoryVersionV2, Inventory: raw}
		inventory, err := storemappers.AssessmentInventoryFromStore(inv)
		got := svcmodel.InventoryData{}
		if inventory.Inventory.Vcenter != nil {
			got = *inventory.Inventory.Vcenter
		}
		Expect(err).ToNot(HaveOccurred())
		Expect(got.Vms.Total).To(Equal(0))
	})
})

func mustMarshal(v any) []byte {
	b, err := json.Marshal(v)
	Expect(err).ToNot(HaveOccurred())
	return b
}
