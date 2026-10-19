package helpers

import (
	"encoding/json"

	"github.com/google/uuid"
	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/store/model"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("inventory aggregation", func() {
	It("combines VM totals and utilization across inventories", func() {
		inventories := []api.InventoryData{
			{
				Vcenter: &api.VCenter{Id: "vc-1"},
				Vms: api.VMs{
					Total:    2,
					CpuCores: api.VMResourceBreakdown{Total: 2},
					RamGB:    api.VMResourceBreakdown{Total: 4},
					DiskGB:   api.VMResourceBreakdown{Total: 10},
				},
			},
			{
				Vcenter: &api.VCenter{Id: "vc-1"},
				Vms: api.VMs{
					Total:    6,
					CpuCores: api.VMResourceBreakdown{Total: 6},
					RamGB:    api.VMResourceBreakdown{Total: 12},
					DiskGB:   api.VMResourceBreakdown{Total: 15},
				},
			},
		}

		got := AggregateInventoryData(inventories)

		Expect(got.Vcenter).To(Equal(&api.VCenter{Id: "vc-1"}))
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

		hosts1 := []api.Host{{Id: &host1ID, CpuCores: &host1CPU, MemoryMB: &host1Memory}}
		hosts2 := []api.Host{
			{Id: &host2ID, CpuCores: &host2CPU, MemoryMB: &host2Memory},
			{Id: &host3ID, CpuCores: &host3CPU, MemoryMB: &host3Memory},
		}
		inventories := []api.InventoryData{
			{
				Vms: api.VMs{
					Total:    2,
					CpuCores: api.VMResourceBreakdown{Total: 2},
					RamGB:    api.VMResourceBreakdown{Total: 4},
				},
				Infra: api.Infra{
					TotalHosts:              1,
					Hosts:                   &hosts1,
					Datastores:              []api.Datastore{{DiskId: "datastore-1"}},
					Networks:                []api.Network{{Name: "network-1", Type: "standard"}},
					HostPowerStates:         map[string]int{"poweredOn": 1, "poweredOff": 2},
					TotalDatacenters:        &totalDatacenters1,
					ClustersPerDatacenter:   &clustersPerDatacenter1,
					StandaloneHostsDetected: &standalone1,
				},
			},
			{
				Vms: api.VMs{
					Total:    6,
					CpuCores: api.VMResourceBreakdown{Total: 6},
					RamGB:    api.VMResourceBreakdown{Total: 12},
				},
				Infra: api.Infra{
					TotalHosts:              2,
					Hosts:                   &hosts2,
					Datastores:              []api.Datastore{{DiskId: "datastore-2"}},
					Networks:                []api.Network{{Name: "network-2", Type: "distributed"}},
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
		Expect(infra.Datastores).To(Equal([]api.Datastore{{DiskId: "datastore-1"}, {DiskId: "datastore-2"}}))
		Expect(infra.Networks).To(Equal([]api.Network{
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
		inventories := []api.InventoryData{
			{
				Vms: api.VMs{Total: 1, CpuCores: api.VMResourceBreakdown{Total: 1}, RamGB: api.VMResourceBreakdown{Total: 1}, DiskGB: api.VMResourceBreakdown{Total: 10}},
			},
			{
				Vms: api.VMs{Total: 2, CpuCores: api.VMResourceBreakdown{Total: 2}, RamGB: api.VMResourceBreakdown{Total: 2}, DiskGB: api.VMResourceBreakdown{Total: 20}},
			},
			{
				Vms: api.VMs{Total: 3, CpuCores: api.VMResourceBreakdown{Total: 3}, RamGB: api.VMResourceBreakdown{Total: 3}, DiskGB: api.VMResourceBreakdown{Total: 30}},
			},
			{
				Vms: api.VMs{Total: 4, CpuCores: api.VMResourceBreakdown{Total: 4}, RamGB: api.VMResourceBreakdown{Total: 4}, DiskGB: api.VMResourceBreakdown{Total: 40}},
			},
		}

		got := AggregateInventoryData(inventories)

		Expect(got.Vms.Total).To(Equal(10))
		Expect(got.Vms.CpuCores.Total).To(Equal(10))
		Expect(got.Vms.RamGB.Total).To(Equal(10))
		Expect(got.Vms.DiskGB.Total).To(Equal(100))
	})

	It("omits the vCenter ID when inventory IDs disagree", func() {
		inventories := []api.InventoryData{
			{Vcenter: &api.VCenter{Id: "vc-1"}},
			{Vcenter: &api.VCenter{Id: "vc-2"}},
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
		inv1 := mustMarshal(api.Inventory{
			VcenterId: "vc-1",
			Clusters:  map[string]api.InventoryData{},
			Vcenter: &api.InventoryData{
				Vcenter: &api.VCenter{Id: "vc-1"},
				Vms:     api.VMs{Total: 3, CpuCores: api.VMResourceBreakdown{Total: 4}},
				Infra:   api.Infra{TotalHosts: 1, Datastores: []api.Datastore{{DiskId: "ds-1"}}, Networks: []api.Network{{Name: "net-1"}}},
			},
		})
		inv2 := mustMarshal(api.Inventory{
			VcenterId: "vc-1",
			Clusters:  map[string]api.InventoryData{},
			Vcenter: &api.InventoryData{
				Vcenter: &api.VCenter{Id: "vc-1"},
				Vms:     api.VMs{Total: 5, CpuCores: api.VMResourceBreakdown{Total: 8}},
				Infra:   api.Infra{TotalHosts: 2, Datastores: []api.Datastore{{DiskId: "ds-2"}}, Networks: []api.Network{{Name: "net-2"}}},
			},
		})

		models := []model.AssessmentInventory{
			{ID: uuid.New(), Version: model.InventoryVersionV2, Inventory: inv1},
			{ID: uuid.New(), Version: model.InventoryVersionV2, Inventory: inv2},
		}

		got, err := Aggregate(models)
		Expect(err).ToNot(HaveOccurred())
		Expect(got.Vcenter).ToNot(BeNil())
		Expect(got.Vcenter.Vms.Total).To(Equal(8))
		Expect(got.Vcenter.Vms.CpuCores.Total).To(Equal(12))
		Expect(got.Vcenter.Infra.TotalHosts).To(Equal(3))
		Expect(got.Vcenter.Infra.Datastores).To(HaveLen(2))
		Expect(got.Vcenter.Infra.Networks).To(HaveLen(2))
	})

	It("aggregates V1 assessment inventories", func() {
		inv1 := mustMarshal(api.InventoryData{
			Vcenter: &api.VCenter{Id: "vc-1"},
			Vms:     api.VMs{Total: 2},
			Infra:   api.Infra{TotalHosts: 1, Datastores: []api.Datastore{{DiskId: "ds-1"}}, Networks: []api.Network{{Name: "n-1"}}},
		})
		inv2 := mustMarshal(api.InventoryData{
			Vcenter: &api.VCenter{Id: "vc-1"},
			Vms:     api.VMs{Total: 3},
			Infra:   api.Infra{TotalHosts: 2, Datastores: []api.Datastore{{DiskId: "ds-2"}}, Networks: []api.Network{{Name: "n-2"}}},
		})

		models := []model.AssessmentInventory{
			{ID: uuid.New(), Version: model.InventoryVersionV1, Inventory: inv1},
			{ID: uuid.New(), Version: model.InventoryVersionV1, Inventory: inv2},
		}

		got, err := Aggregate(models)
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

var _ = Describe("UnmarshalInventoryData", func() {
	It("unmarshals V2 inventory", func() {
		raw := mustMarshal(api.Inventory{
			VcenterId: "vc-1",
			Clusters:  map[string]api.InventoryData{},
			Vcenter: &api.InventoryData{
				Vcenter: &api.VCenter{Id: "vc-1"},
				Vms:     api.VMs{Total: 10},
				Infra:   api.Infra{TotalHosts: 3},
			},
		})

		inv := model.AssessmentInventory{Version: model.InventoryVersionV2, Inventory: raw}
		got, err := UnmarshalInventoryData(inv)
		Expect(err).ToNot(HaveOccurred())
		Expect(got.Vms.Total).To(Equal(10))
		Expect(got.Infra.TotalHosts).To(Equal(3))
	})

	It("unmarshals V1 inventory", func() {
		raw := mustMarshal(api.InventoryData{
			Vcenter: &api.VCenter{Id: "vc-1"},
			Vms:     api.VMs{Total: 5},
			Infra:   api.Infra{TotalHosts: 2},
		})

		inv := model.AssessmentInventory{Version: model.InventoryVersionV1, Inventory: raw}
		got, err := UnmarshalInventoryData(inv)
		Expect(err).ToNot(HaveOccurred())
		Expect(got.Vms.Total).To(Equal(5))
		Expect(got.Infra.TotalHosts).To(Equal(2))
	})

	It("returns empty data when V2 inventory has nil vcenter", func() {
		raw := mustMarshal(api.Inventory{
			VcenterId: "vc-1",
			Clusters:  map[string]api.InventoryData{},
		})

		inv := model.AssessmentInventory{Version: model.InventoryVersionV2, Inventory: raw}
		got, err := UnmarshalInventoryData(inv)
		Expect(err).ToNot(HaveOccurred())
		Expect(got.Vms.Total).To(Equal(0))
	})
})

func mustMarshal(v any) []byte {
	b, err := json.Marshal(v)
	Expect(err).ToNot(HaveOccurred())
	return b
}
