package helpers

import (
	api "github.com/kubev2v/migration-planner/api/v1alpha1"
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

		got := aggregate(inventories)

		Expect(got.Clusters).To(BeEmpty())
		Expect(got.VcenterId).To(Equal("vc-1"))
		Expect(got.Vcenter).ToNot(BeNil())
		Expect(got.Vcenter.Vcenter).To(Equal(&api.VCenter{Id: "vc-1"}))
		Expect(got.Vcenter.Vms.Total).To(Equal(8))
		Expect(got.Vcenter.Vms.CpuCores.Total).To(Equal(8))
		Expect(got.Vcenter.Vms.RamGB.Total).To(Equal(16))
		Expect(got.Vcenter.Vms.DiskGB.Total).To(Equal(25))
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

		got := aggregate(inventories)

		infra := got.Vcenter.Infra
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

		got := aggregate(inventories)

		Expect(got.Vcenter).ToNot(BeNil())
		Expect(got.Vcenter.Vms.Total).To(Equal(10))
		Expect(got.Vcenter.Vms.CpuCores.Total).To(Equal(10))
		Expect(got.Vcenter.Vms.RamGB.Total).To(Equal(10))
		Expect(got.Vcenter.Vms.DiskGB.Total).To(Equal(100))
	})

	It("omits the vCenter ID when inventory IDs disagree", func() {
		inventories := []api.InventoryData{
			{Vcenter: &api.VCenter{Id: "vc-1"}},
			{Vcenter: &api.VCenter{Id: "vc-2"}},
		}

		got := aggregate(inventories)

		Expect(got.VcenterId).To(BeEmpty())
		Expect(got.Vcenter).ToNot(BeNil())
		Expect(got.Vcenter.Vcenter).To(BeNil())
	})

	It("returns an empty aggregate when there are no inventories", func() {
		got := aggregate(nil)

		Expect(got.Clusters).To(BeEmpty())
		Expect(got.Clusters).ToNot(BeNil())
		Expect(got.Vcenter).To(BeNil())
		Expect(got.VcenterId).To(BeEmpty())
	})
})
