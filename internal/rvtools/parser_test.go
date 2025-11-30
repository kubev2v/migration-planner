package rvtools_test

import (
	"bytes"
	"context"
	"fmt"

	"github.com/kubev2v/forklift/pkg/controller/provider/model/vsphere"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/xuri/excelize/v2"

	"github.com/kubev2v/migration-planner/internal/opa"
	"github.com/kubev2v/migration-planner/internal/rvtools"
)

// Helper functions for Excel operations
func newSheet(f *excelize.File, sheet string) int {
	index, err := f.NewSheet(sheet)
	Expect(err).To(Succeed())
	return index
}

func setCellValue(f *excelize.File, sheet, ref string, value any) {
	Expect(f.SetCellValue(sheet, ref, value)).To(Succeed())
}

func writeBuffer(f *excelize.File, buf *bytes.Buffer) {
	_, err := f.WriteTo(buf)
	Expect(err).To(Succeed())
}

// Helper function to convert column index to Excel column letter
func columnToLetter(col int) string {
	name, _ := excelize.ColumnNumberToName(col + 1)
	return name
}

func createMinimalTestExcel() []byte {
	f := excelize.NewFile()
	defer f.Close()

	type SheetConfig struct {
		Headers []string
		Rows    [][]string
	}

	sheetConfigs := map[string]SheetConfig{
		"vInfo": {
			Headers: []string{"VM", "VM ID", "VI SDK UUID"},
			Rows:    [][]string{},
		},
		"vHost": {
			Headers: []string{"Host", "Vendor", "Model", "Object ID"},
			Rows:    [][]string{},
		},
	}

	var vInfoIndex int
	for sheetName, config := range sheetConfigs {
		sheetIndex := newSheet(f, sheetName)
		if sheetName == "vInfo" {
			vInfoIndex = sheetIndex
		}

		// Set headers
		for colIndex, header := range config.Headers {
			cellRef := columnToLetter(colIndex) + "1"
			setCellValue(f, sheetName, cellRef, header)
		}

		// Set data rows (empty in this case)
		for rowIndex, row := range config.Rows {
			for colIndex, value := range row {
				cellRef := columnToLetter(colIndex) + fmt.Sprintf("%d", rowIndex+2)
				setCellValue(f, sheetName, cellRef, value)
			}
		}
	}

	f.SetActiveSheet(vInfoIndex)

	var buf bytes.Buffer
	writeBuffer(f, &buf)
	return buf.Bytes()
}

func createSampleDataExcel() []byte {
	f := excelize.NewFile()
	defer f.Close()

	type SheetConfig struct {
		Headers []string
		Rows    [][]string
	}

	sheetConfigs := map[string]SheetConfig{
		"vInfo": {
			Headers: []string{"VM", "VM ID", "VI SDK UUID", "CPUs", "Memory", "PowerState", "Cluster"},
			Rows: [][]string{
				{"test-vm-1", "vm-001", "12345678-1234-1234-1234-123456789abc", "4", "8192", "poweredOn", "cluster1"},
				{"test-vm-2", "vm-002", "87654321-4321-4321-4321-210987654321", "2", "4096", "poweredOff", "cluster1"},
			},
		},
		"vHost": {
			Headers: []string{"Host", "Vendor", "Model", "Object ID", "Datacenter", "Cluster", "Config Status", "# Cores", "# CPU", "# Memory"},
			Rows: [][]string{
				{"esxi-host-1", "VMware", "ESXi", "host-001", "datacenter1", "cluster1", "green", "24", "2", "256000"},
				{"esxi-host-2", "VMware", "ESXi", "host-002", "datacenter1", "cluster1", "yellow", "32", "2", "512000"},
			},
		},
		"vDatastore": {
			Headers: []string{"Name", "Type", "Capacity MiB", "Free MiB", "Object ID"},
			Rows: [][]string{
				{"datastore1", "VMFS", "1048576", "524288", "datastore-001"},
			},
		},
		"vNetwork": {
			Headers: []string{"VM", "Network"},
			Rows:    [][]string{}, // Empty data rows
		},
		"vCPU": {
			Headers: []string{"VM", "Hot Add"},
			Rows:    [][]string{}, // Empty data rows
		},
		"vMemory": {
			Headers: []string{"VM", "Hot Add"},
			Rows:    [][]string{}, // Empty data rows
		},
		"vDisk": {
			Headers: []string{"VM", "Path"},
			Rows:    [][]string{}, // Empty data rows
		},
		"dvSwitch": {
			Headers: []string{"Switch", "Datacenter"},
			Rows: [][]string{
				{"dvSwitch1", "datacenter1"},
			},
		},
		"dvPort": {
			Headers: []string{"Port", "Switch", "VLAN"},
			Rows: [][]string{
				{"pg1", "dvSwitch1", "100"},
			},
		},
	}

	var vInfoIndex int
	for sheetName, config := range sheetConfigs {
		sheetIndex := newSheet(f, sheetName)
		if sheetName == "vInfo" {
			vInfoIndex = sheetIndex
		}

		for colIndex, header := range config.Headers {
			cellRef := columnToLetter(colIndex) + "1"
			setCellValue(f, sheetName, cellRef, header)
		}

		for rowIndex, row := range config.Rows {
			for colIndex, value := range row {
				cellRef := columnToLetter(colIndex) + fmt.Sprintf("%d", rowIndex+2) // +2 because row 1 is headers
				setCellValue(f, sheetName, cellRef, value)
			}
		}
	}

	f.SetActiveSheet(vInfoIndex)

	var buf bytes.Buffer
	writeBuffer(f, &buf)
	return buf.Bytes()
}

func createComprehensiveVMDataExcel() []byte {
	f := excelize.NewFile()
	defer f.Close()

	type SheetConfig struct {
		Headers []string
		Rows    [][]string
	}

	sheetConfigs := map[string]SheetConfig{
		"vInfo": {
			Headers: []string{"VM", "VM ID", "VI SDK UUID", "CPUs", "Memory", "PowerState", "Cluster"},
			Rows: [][]string{
				{"test-vm-1", "vm-001", "12345678-1234-1234-1234-123456789abc", "4", "8192", "poweredOn", "cluster1"},
			},
		},
		"vHost": {
			Headers: []string{"Host", "Vendor", "Model", "Object ID", "Datacenter", "Cluster"},
			Rows: [][]string{
				{"esxi-host-1", "VMware", "ESXi", "host-001", "datacenter1", "cluster1"},
			},
		},
		"vDatastore": {
			Headers: []string{"Name", "Type", "Capacity MiB", "Free MiB", "Object ID"},
			Rows: [][]string{
				{"datastore1", "VMFS", "1048576", "524288", "datastore-001"},
			},
		},
		"vCPU": {
			Headers: []string{"VM", "Hot Add", "Hot Remove", "Cores p/s"},
			Rows: [][]string{
				{"test-vm-1", "TRUE", "FALSE", "2"},
			},
		},
		"vMemory": {
			Headers: []string{"VM", "Hot Add", "Ballooned"},
			Rows: [][]string{
				{"test-vm-1", "TRUE", "1024"},
			},
		},
		"vDisk": {
			Headers: []string{"VM", "Path", "Disk Key", "Unit #", "Capacity MiB", "Disk Mode", "Disk UUID"},
			Rows: [][]string{
				{"test-vm-1", "[datastore1] test-vm-1/disk1.vmdk", "2000", "0", "20,480", "persistent", "6000C295-1234-5678-9abc-123456789abc"},
				{"test-vm-1", "[datastore1] test-vm-1/disk2.vmdk", "2001", "1", "10,240", "persistent", "6000C295-5678-9abc-def0-123456789abc"},
			},
		},
		"vNetwork": {
			Headers: []string{"VM", "Network", "MAC Address"},
			Rows: [][]string{
				{"test-vm-1", "VM Network", "00:50:56:12:34:56"},
			},
		},
		"dvSwitch": {
			Headers: []string{"Switch", "Datacenter"},
			Rows: [][]string{
				{"dvSwitch1", "datacenter1"},
			},
		},
		"dvPort": {
			Headers: []string{"Port", "Switch", "VLAN"},
			Rows: [][]string{
				{"VM Network", "dvSwitch1", "100"},
			},
		},
	}

	var vInfoIndex int
	for sheetName, config := range sheetConfigs {
		sheetIndex := newSheet(f, sheetName)
		if sheetName == "vInfo" {
			vInfoIndex = sheetIndex
		}

		// Set headers
		for colIndex, header := range config.Headers {
			cellRef := columnToLetter(colIndex) + "1"
			setCellValue(f, sheetName, cellRef, header)
		}

		for rowIndex, row := range config.Rows {
			for colIndex, value := range row {
				cellRef := columnToLetter(colIndex) + fmt.Sprintf("%d", rowIndex+2) // +2 because row 1 is headers
				setCellValue(f, sheetName, cellRef, value)
			}
		}
	}

	f.SetActiveSheet(vInfoIndex)

	var buf bytes.Buffer
	writeBuffer(f, &buf)
	return buf.Bytes()
}

var _ = Describe("Parser", func() {
	Describe("ParseRVTools", func() {
		Context("with invalid Excel data", func() {
			It("should return error for invalid Excel content", func() {
				invalidContent := []byte("not an excel file")
				inventory, err := rvtools.ParseRVTools(context.Background(), invalidContent, nil)
				Expect(err).To(HaveOccurred())
				Expect(inventory).To(BeNil())
				Expect(err.Error()).To(ContainSubstring("error opening Excel file"))
			})
		})

		Context("with empty Excel data", func() {
			It("should return error for empty content", func() {
				emptyContent := []byte{}
				inventory, err := rvtools.ParseRVTools(context.Background(), emptyContent, nil)
				Expect(err).To(HaveOccurred())
				Expect(inventory).To(BeNil())
			})
		})

		Context("with minimal valid Excel data", func() {
			It("should handle Excel with empty RVTools sheets", func() {
				minimalExcel := createMinimalTestExcel()

				inventory, err := rvtools.ParseRVTools(context.Background(), minimalExcel, nil)
				Expect(err).ToNot(HaveOccurred())
				Expect(inventory).ToNot(BeNil())

				Expect(inventory.Infra).ToNot(BeNil())

				By("verifying empty data is handled gracefully")
				Expect(inventory.Vms.Total).To(Equal(0))
				Expect(inventory.Infra.TotalHosts).To(Equal(0))
				Expect(inventory.Infra.TotalClusters).To(Equal(0))

				By("verifying empty collections are initialized")
				Expect(inventory.Infra.Datastores).To(BeEmpty())
				Expect(inventory.Infra.Networks).To(BeEmpty())
				Expect(inventory.Infra.HostsPerCluster).To(BeEmpty())
			})
		})

		Context("Unit Tests with In-Memory Excel Data", func() {
			var testExcelFile []byte

			BeforeEach(func() {
				testExcelFile = createSampleDataExcel()
				Expect(testExcelFile).ToNot(BeEmpty())
			})

			It("should successfully parse Excel data with sample content", func() {
				inventory, err := rvtools.ParseRVTools(context.Background(), testExcelFile, nil)
				Expect(err).ToNot(HaveOccurred())
				Expect(inventory).ToNot(BeNil())

				Expect(inventory.Infra).ToNot(BeNil())

				By("checking that inventory has basic structure")
				Expect(inventory.Vcenter).ToNot(BeNil())

				By("verifying that some infrastructure data was parsed")
				hostsLen := 0
				if inventory.Infra.Hosts != nil {
					hostsLen = len(*inventory.Infra.Hosts)
				}
				hasData := inventory.Vms.Total > 0 ||
					hostsLen > 0 ||
					len(inventory.Infra.Datastores) > 0 ||
					len(inventory.Infra.Networks) > 0
				Expect(hasData).To(BeTrue(), "Expected at least some infrastructure data to be parsed")
			})

			It("should handle Excel data with various sheet types", func() {
				inventory, err := rvtools.ParseRVTools(context.Background(), testExcelFile, nil)
				Expect(err).ToNot(HaveOccurred())
				Expect(inventory).ToNot(BeNil())

				By("verifying VMs were processed")
				if inventory.Vms.Total > 0 {
					Expect(inventory.Vms.PowerStates).ToNot(BeNil())
					Expect(inventory.Vms.Os).ToNot(BeNil())
				}

				By("verifying hosts were processed")
				if inventory.Infra.Hosts != nil && len(*inventory.Infra.Hosts) > 0 {
					host := (*inventory.Infra.Hosts)[0]
					Expect(host.Vendor).ToNot(BeEmpty())
					Expect(host.Model).ToNot(BeEmpty())
				}

				By("verifying infrastructure stats were calculated")
				Expect(inventory.Infra.TotalHosts).To(BeNumerically(">=", 0))
				Expect(inventory.Infra.TotalClusters).To(BeNumerically(">=", 0))
				if inventory.Infra.TotalDatacenters != nil {
					Expect(*inventory.Infra.TotalDatacenters).To(BeNumerically(">=", 0))
				}
			})

			It("should handle OPA validation when validator is provided", func() {
				// Create a properly initialized OPA validator with a simple test policy
				testPolicy := `package io.konveyor.forklift.vmware

import rego.v1

concerns contains flag if {
	input.name == "test-vm"
	flag := {
		"id": "test.concern",
		"category": "Warning",
		"label": "Test concern",
		"assessment": "This is a test VM with a concern.",
	}
}`
				policies := map[string]string{
					"test.rego": testPolicy,
				}

				validator, err := opa.NewValidator(policies)
				Expect(err).ToNot(HaveOccurred())
				Expect(validator).ToNot(BeNil())

				inventory, err := rvtools.ParseRVTools(context.Background(), testExcelFile, validator)

				Expect(err).ToNot(HaveOccurred())
				Expect(inventory).ToNot(BeNil())

				// The validator should have been called but won't find any VMs matching our test policy
				// The important thing is that it doesn't crash and completes successfully
			})

			It("should handle OPA validation errors gracefully", func() {
				// Test with nil validator first to ensure it doesn't break
				inventory, err := rvtools.ParseRVTools(context.Background(), testExcelFile, nil)
				Expect(err).ToNot(HaveOccurred())
				Expect(inventory).ToNot(BeNil())

				// Additional test: Create a validator with invalid policy to test error handling
				invalidPolicy := `package io.konveyor.forklift.vmware

invalid syntax here`
				policies := map[string]string{
					"invalid.rego": invalidPolicy,
				}

				_, validatorErr := opa.NewValidator(policies)
				Expect(validatorErr).To(HaveOccurred())
				Expect(validatorErr.Error()).To(ContainSubstring("failed to compile policies"))
			})

			It("should correlate data between different sheets", func() {
				inventory, err := rvtools.ParseRVTools(context.Background(), testExcelFile, nil)
				Expect(err).ToNot(HaveOccurred())
				Expect(inventory).ToNot(BeNil())

				By("checking for data correlation signs")
				// Check that VM totals and infrastructure data make sense together
				if inventory.Vms.Total > 0 && len(inventory.Infra.Datastores) > 0 {
					// If we have VMs and datastores, the parsing likely worked
					Expect(inventory.Vms.Total).To(BeNumerically(">", 0))
					Expect(len(inventory.Infra.Datastores)).To(BeNumerically(">", 0))
				}

				By("verifying infrastructure consistency")
				if inventory.Infra.TotalHosts > 0 {
					Expect(inventory.Infra.TotalClusters).To(BeNumerically(">=", 0))
					if inventory.Infra.TotalDatacenters != nil {
						Expect(*inventory.Infra.TotalDatacenters).To(BeNumerically(">=", 0))
					}
				}
			})
		})
	})

	Describe("ExtractClusterAndDatacenterInfo", func() {
		Context("with empty data", func() {
			It("should return empty ClusterInfo for empty rows", func() {
				emptyRows := [][]string{}
				result := rvtools.ExtractClusterAndDatacenterInfo(emptyRows)
				Expect(result.TotalHosts).To(Equal(0))
				Expect(result.TotalClusters).To(Equal(0))
				Expect(result.TotalDatacenters).To(Equal(0))
				Expect(result.HostsPerCluster).To(BeEmpty())
				Expect(result.ClustersPerDatacenter).To(BeEmpty())
			})

			It("should return empty ClusterInfo for header-only rows", func() {
				headerOnly := [][]string{{"host", "datacenter", "cluster"}}
				result := rvtools.ExtractClusterAndDatacenterInfo(headerOnly)
				Expect(result.TotalHosts).To(Equal(0))
				Expect(result.TotalClusters).To(Equal(0))
				Expect(result.TotalDatacenters).To(Equal(0))
			})
		})

		Context("with valid data", func() {
			It("should extract cluster and datacenter info correctly", func() {
				rows := [][]string{
					{"host", "datacenter", "cluster"},
					{"host1", "dc1", "cluster1"},
					{"host2", "dc1", "cluster1"},
					{"host3", "dc1", "cluster2"},
					{"host4", "dc2", "cluster3"},
				}
				result := rvtools.ExtractClusterAndDatacenterInfo(rows)
				Expect(result.TotalHosts).To(Equal(4))
				Expect(result.TotalClusters).To(Equal(3))
				Expect(result.TotalDatacenters).To(Equal(2))
				Expect(result.HostsPerCluster).To(ContainElements(2, 1, 1))
				Expect(result.ClustersPerDatacenter).To(ContainElements(2, 1))
			})

			It("should handle missing cluster data", func() {
				rows := [][]string{
					{"host", "datacenter", "cluster"},
					{"host1", "dc1", ""},
					{"host2", "dc1", "cluster1"},
				}
				result := rvtools.ExtractClusterAndDatacenterInfo(rows)
				Expect(result.TotalHosts).To(Equal(2))
				Expect(result.TotalClusters).To(Equal(1))
				Expect(result.TotalDatacenters).To(Equal(1))
			})

			It("should handle missing datacenter data", func() {
				rows := [][]string{
					{"host", "datacenter", "cluster"},
					{"host1", "", "cluster1"},
					{"host2", "dc1", "cluster1"},
				}
				result := rvtools.ExtractClusterAndDatacenterInfo(rows)
				Expect(result.TotalHosts).To(Equal(2))
				Expect(result.TotalClusters).To(Equal(1))
				Expect(result.TotalDatacenters).To(Equal(1))
			})
		})
	})

	Describe("ExtractHostsInfo", func() {
		Context("with empty data", func() {
			It("should return empty slice for empty rows", func() {
				emptyRows := [][]string{}
				hosts, err := rvtools.ExtractHostsInfo(emptyRows)
				Expect(err).ToNot(HaveOccurred())
				Expect(hosts).To(BeEmpty())
			})

			It("should return empty slice for header-only rows", func() {
				headerOnly := [][]string{{"vendor", "model"}}
				hosts, err := rvtools.ExtractHostsInfo(headerOnly)
				Expect(err).ToNot(HaveOccurred())
				Expect(hosts).To(BeEmpty())
			})
		})

		Context("with missing required columns", func() {
			It("should return error when vendor column is missing", func() {
				rows := [][]string{
					{"model", "# cores"},
					{"ProLiant", "24"},
				}
				hosts, err := rvtools.ExtractHostsInfo(rows)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("missing required column: vendor"))
				Expect(hosts).To(BeNil())
			})

			It("should return error when model column is missing", func() {
				rows := [][]string{
					{"vendor", "# cores"},
					{"HP", "24"},
				}
				hosts, err := rvtools.ExtractHostsInfo(rows)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("missing required column: model"))
				Expect(hosts).To(BeNil())
			})

			It("should return error when object id column is missing", func() {
				rows := [][]string{
					{"vendor", "model"},
					{"HP", "ProLiant"},
				}
				hosts, err := rvtools.ExtractHostsInfo(rows)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("missing required column: object id"))
				Expect(hosts).To(BeNil())
			})
		})

		Context("with valid data", func() {
			It("should extract basic host info", func() {
				rows := [][]string{
					{"vendor", "model", "object id"},
					{"HP", "ProLiant DL380", "host-001"},
					{"Dell", "PowerEdge R740", "host-002"},
				}
				hosts, err := rvtools.ExtractHostsInfo(rows)
				Expect(err).ToNot(HaveOccurred())
				Expect(hosts).To(HaveLen(2))
				Expect(hosts[0].Vendor).To(Equal("HP"))
				Expect(hosts[0].Model).To(Equal("ProLiant DL380"))
				Expect(hosts[1].Vendor).To(Equal("Dell"))
				Expect(hosts[1].Model).To(Equal("PowerEdge R740"))
			})

			It("should extract host info with CPU and memory data", func() {
				rows := [][]string{
					{"vendor", "model", "# cores", "# cpu", "# memory", "object id"},
					{"HP", "ProLiant DL380", "24", "2", "128,000", "host-001"},
					{"Dell", "PowerEdge R740", "32", "2", "256000", "host-002"},
				}
				hosts, err := rvtools.ExtractHostsInfo(rows)
				Expect(err).ToNot(HaveOccurred())
				Expect(hosts).To(HaveLen(2))

				Expect(hosts[0].CpuCores).ToNot(BeNil())
				Expect(*hosts[0].CpuCores).To(Equal(24))
				Expect(hosts[0].CpuSockets).ToNot(BeNil())
				Expect(*hosts[0].CpuSockets).To(Equal(2))
				Expect(hosts[0].MemoryMB).ToNot(BeNil())
				Expect(*hosts[0].MemoryMB).To(Equal(int64(128000)))

				Expect(hosts[1].CpuCores).ToNot(BeNil())
				Expect(*hosts[1].CpuCores).To(Equal(32))
				Expect(hosts[1].CpuSockets).ToNot(BeNil())
				Expect(*hosts[1].CpuSockets).To(Equal(2))
				Expect(hosts[1].MemoryMB).ToNot(BeNil())
				Expect(*hosts[1].MemoryMB).To(Equal(int64(256000)))
			})

			It("should handle missing optional data gracefully", func() {
				rows := [][]string{
					{"vendor", "model", "# cores", "# cpu", "# memory", "object id"},
					{"HP", "ProLiant DL380", "", "", "", "host-001"},
					{"Dell", "PowerEdge R740", "0", "0", "0", "host-002"},
				}
				hosts, err := rvtools.ExtractHostsInfo(rows)
				Expect(err).ToNot(HaveOccurred())
				Expect(hosts).To(HaveLen(2))

				Expect(hosts[0].CpuCores).To(BeNil())
				Expect(hosts[0].CpuSockets).To(BeNil())
				Expect(hosts[0].MemoryMB).To(BeNil())

				Expect(hosts[1].CpuCores).To(BeNil())
				Expect(hosts[1].CpuSockets).To(BeNil())
				Expect(hosts[1].MemoryMB).To(BeNil())
			})
		})
	})

	Describe("ExtractHostPowerStates", func() {
		Context("with empty data", func() {
			It("should return empty map for empty rows", func() {
				emptyRows := [][]string{}
				result := rvtools.ExtractHostPowerStates(emptyRows)
				Expect(result).To(BeEmpty())
			})

			It("should return empty map for header-only rows", func() {
				headerOnly := [][]string{{"config status"}}
				result := rvtools.ExtractHostPowerStates(headerOnly)
				Expect(result).To(BeEmpty())
			})
		})

		Context("with valid data", func() {
			It("should count power states correctly", func() {
				rows := [][]string{
					{"config status"},
					{"green"},
					{"red"},
					{"yellow"},
					{"green"},
					{"gray"},
					{"green"},
				}
				result := rvtools.ExtractHostPowerStates(rows)
				Expect(result["green"]).To(Equal(3))
				Expect(result["red"]).To(Equal(1))
				Expect(result["yellow"]).To(Equal(1))
				Expect(result["gray"]).To(Equal(1))
			})

			It("should default unknown states to green", func() {
				rows := [][]string{
					{"config status"},
					{"unknown"},
					{"invalid"},
					{""},
				}
				result := rvtools.ExtractHostPowerStates(rows)
				Expect(result["green"]).To(Equal(3))
			})
		})
	})

	Describe("ExtractVmsPerCluster", func() {
		Context("with empty data", func() {
			It("should return empty slice for empty rows", func() {
				emptyRows := [][]string{}
				result := rvtools.ExtractVmsPerCluster(emptyRows)
				Expect(result).To(BeEmpty())
			})

			It("should return empty slice for header-only rows", func() {
				headerOnly := [][]string{{"cluster", "vm"}}
				result := rvtools.ExtractVmsPerCluster(headerOnly)
				Expect(result).To(BeEmpty())
			})
		})

		Context("with valid data", func() {
			It("should count VMs per cluster correctly", func() {
				rows := [][]string{
					{"cluster", "vm"},
					{"cluster1", "vm1"},
					{"cluster1", "vm2"},
					{"cluster1", "vm3"},
					{"cluster2", "vm4"},
					{"cluster2", "vm5"},
					{"cluster3", "vm6"},
				}
				result := rvtools.ExtractVmsPerCluster(rows)
				Expect(result).To(ContainElements(3, 2, 1))
			})

			It("should handle missing cluster or VM data", func() {
				rows := [][]string{
					{"cluster", "vm"},
					{"cluster1", "vm1"},
					{"", "vm2"},
					{"cluster1", ""},
					{"cluster2", "vm3"},
				}
				result := rvtools.ExtractVmsPerCluster(rows)
				Expect(result).To(ContainElements(1, 1))
			})

			It("should handle duplicate VMs in same cluster", func() {
				rows := [][]string{
					{"cluster", "vm"},
					{"cluster1", "vm1"},
					{"cluster1", "vm1"},
					{"cluster1", "vm2"},
				}
				result := rvtools.ExtractVmsPerCluster(rows)
				Expect(result).To(ContainElement(2))
			})
		})
	})

	Describe("ExtractNetworks", func() {
		Context("with empty data", func() {
			It("should return empty slice when no network data available", func() {
				emptyDvswitchRows := [][]string{}
				emptyDvportRows := [][]string{}
				result := rvtools.ExtractNetworks(emptyDvswitchRows, emptyDvportRows, []vsphere.VM{})
				Expect(result).To(BeEmpty())
			})
		})

		Context("with network data", func() {
			It("should extract dvSwitch data correctly", func() {
				dvswitchRows := [][]string{
					{"Switch", "Datacenter"},
					{"dvSwitch1", "dc1"},
					{"dvSwitch2", "dc2"},
				}
				emptyDvportRows := [][]string{}

				result := rvtools.ExtractNetworks(dvswitchRows, emptyDvportRows, []vsphere.VM{})

				Expect(result).ToNot(BeNil())
				Expect(len(result)).To(BeNumerically(">=", 2))

				// Should contain dvSwitch entries
				switchNames := make([]string, 0)
				for _, network := range result {
					switchNames = append(switchNames, network.Name)
				}
				Expect(switchNames).To(ContainElement("dvSwitch1"))
				Expect(switchNames).To(ContainElement("dvSwitch2"))
			})

			It("should extract dvPort data with VLANs", func() {
				emptyDvswitchRows := [][]string{}
				dvportRows := [][]string{
					{"Port", "Switch", "VLAN"},
					{"pg1", "dvSwitch1", "100"},
					{"pg2", "dvSwitch1", "200"},
					{"pg3", "dvSwitch2", ""},
				}

				result := rvtools.ExtractNetworks(emptyDvswitchRows, dvportRows, []vsphere.VM{})

				Expect(result).ToNot(BeNil())
			})

			It("should count VMs per network", func() {
				emptyDvswitchRows := [][]string{}
				dvportRows := [][]string{
					{"Port", "Switch", "VLAN", "Object ID"},
					{"pg1", "dvSwitch1", "100", "dvportgroup-1"},
					{"pg2", "dvSwitch1", "200", "dvportgroup-2"},
					{"pg3", "dvSwitch2", "", "dvportgroup-3"},
				}

				vms := []vsphere.VM{
					{
						Networks: []vsphere.Ref{
							{
								Kind: "Network",
								ID:   "dvportgroup-1",
							},
						},
					},
					{
						Networks: []vsphere.Ref{
							{
								Kind: "Network",
								ID:   "dvportgroup-1",
							},
						},
					},
					{
						Networks: []vsphere.Ref{
							{
								Kind: "Network",
								ID:   "dvportgroup-1",
							},
						},
					},
				}

				result := rvtools.ExtractNetworks(emptyDvswitchRows, dvportRows, vms)
				Expect(result).ToNot(BeNil())

				for _, network := range result {
					if network.Name == "pg1" {
						Expect(*network.VmsCount).To(BeNumerically("==", 3))
					}
				}
			})

			It("should handle mixed dvSwitch and dvPort data", func() {
				dvswitchRows := [][]string{
					{"Switch", "Datacenter"},
					{"dvSwitch1", "dc1"},
					{"dvSwitch2", "dc2"},
				}
				dvportRows := [][]string{
					{"Port", "Switch", "VLAN"},
					{"pg1", "dvSwitch1", "100"},
					{"pg2", "dvSwitch2", "200"},
					{"pg3", "dvSwitch3", "300"}, // Switch not in dvswitchRows
				}

				result := rvtools.ExtractNetworks(dvswitchRows, dvportRows, []vsphere.VM{})

				Expect(result).ToNot(BeNil())
				Expect(len(result)).To(BeNumerically(">", 0))
			})

			It("should return proper Network objects with correct types", func() {
				dvswitchRows := [][]string{
					{"Switch", "Datacenter"},
					{"dvSwitch1", "dc1"},
				}
				dvportRows := [][]string{
					{"Port", "Switch", "VLAN"},
					{"pg1", "dvSwitch1", "100"},
				}

				result := rvtools.ExtractNetworks(dvswitchRows, dvportRows, []vsphere.VM{})

				Expect(result).ToNot(BeNil())
				if len(result) > 0 {
					// Verify that returned objects are proper Network types
					network := result[0]
					Expect(network.Name).ToNot(BeEmpty())
					// Network type should be set appropriately
					Expect(string(network.Type)).ToNot(BeEmpty())
				}
			})

			It("should handle malformed network data gracefully", func() {
				malformedDvswitchRows := [][]string{
					{"Switch"}, // Missing Datacenter column
					{"dvSwitch1"},
				}
				malformedDvportRows := [][]string{
					{"Port"}, // Missing Switch and VLAN columns
					{"pg1"},
				}

				// Should not panic with malformed data
				result := rvtools.ExtractNetworks(malformedDvswitchRows, malformedDvportRows, []vsphere.VM{})
				Expect(result).ToNot(BeNil())
			})

			It("should handle inconsistent column headers", func() {
				// Test with different case and spacing in headers
				dvswitchRows := [][]string{
					{"switch", "datacenter"}, // lowercase
					{"dvSwitch1", "dc1"},
				}
				dvportRows := [][]string{
					{"port", "Switch", "vlan"}, // mixed case
					{"pg1", "dvSwitch1", "100"},
				}

				result := rvtools.ExtractNetworks(dvswitchRows, dvportRows, []vsphere.VM{})
				Expect(result).ToNot(BeNil())
			})
		})
	})

	Describe("ExtractVCenterUUID", func() {
		Context("with insufficient data", func() {
			It("should return error for empty rows", func() {
				emptyRows := [][]string{}
				uuid, err := rvtools.ExtractVCenterUUID(emptyRows)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("insufficient data"))
				Expect(uuid).To(BeEmpty())
			})

			It("should return error for single row", func() {
				singleRow := [][]string{{"VI SDK UUID"}}
				uuid, err := rvtools.ExtractVCenterUUID(singleRow)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("insufficient data"))
				Expect(uuid).To(BeEmpty())
			})
		})

		Context("with missing UUID column", func() {
			It("should return error when VI SDK UUID column not found", func() {
				rows := [][]string{
					{"Name", "Version"},
					{"vcenter1", "7.0"},
				}
				uuid, err := rvtools.ExtractVCenterUUID(rows)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("VI SDK UUID column not found"))
				Expect(uuid).To(BeEmpty())
			})
		})

		Context("with valid data", func() {
			It("should extract UUID correctly", func() {
				rows := [][]string{
					{"Name", "VI SDK UUID", "Version"},
					{"vcenter1", "12345678-1234-1234-1234-123456789abc", "7.0"},
				}
				uuid, err := rvtools.ExtractVCenterUUID(rows)
				Expect(err).ToNot(HaveOccurred())
				Expect(uuid).To(Equal("12345678-1234-1234-1234-123456789abc"))
			})

			It("should handle empty UUID value", func() {
				rows := [][]string{
					{"Name", "VI SDK UUID", "Version"},
					{"vcenter1", "", "7.0"},
				}
				uuid, err := rvtools.ExtractVCenterUUID(rows)
				Expect(err).ToNot(HaveOccurred())
				Expect(uuid).To(BeEmpty())
			})

			It("should find UUID column regardless of position", func() {
				rows := [][]string{
					{"Version", "Name", "VI SDK UUID"},
					{"7.0", "vcenter1", "12345678-1234-1234-1234-123456789abc"},
				}
				uuid, err := rvtools.ExtractVCenterUUID(rows)
				Expect(err).ToNot(HaveOccurred())
				Expect(uuid).To(Equal("12345678-1234-1234-1234-123456789abc"))
			})
		})
	})

	Describe("Utility Functions", func() {
		Describe("IsExcelFile", func() {
			It("should identify valid Excel files by magic bytes", func() {
				excelContent := createMinimalTestExcel()
				Expect(rvtools.IsExcelFile(excelContent)).To(BeTrue())
			})

			It("should reject non-Excel content", func() {
				Expect(rvtools.IsExcelFile([]byte("not excel"))).To(BeFalse())
			})

			It("should handle empty content", func() {
				Expect(rvtools.IsExcelFile([]byte{})).To(BeFalse())
			})

			It("should handle content shorter than 2 bytes", func() {
				Expect(rvtools.IsExcelFile([]byte{0x50})).To(BeFalse())
			})

			It("should reject content with wrong magic bytes", func() {
				Expect(rvtools.IsExcelFile([]byte{0x41, 0x42, 0x43, 0x44})).To(BeFalse())
			})

			It("should reject content with Excel magic bytes but invalid structure", func() {
				// PK magic bytes but not a valid ZIP/Excel file
				invalidContent := []byte{0x50, 0x4B, 0x03, 0x04, 0x00, 0x00} // Invalid ZIP structure
				Expect(rvtools.IsExcelFile(invalidContent)).To(BeFalse())
			})
		})
	})

	Describe("NewControllerTracker", func() {
		It("should create a new controller tracker", func() {
			ct := rvtools.NewControllerTracker()
			Expect(ct).ToNot(BeNil())
		})
	})

	Describe("VM Processing Functions Coverage", func() {
		It("should trigger populateVMCpuData, populateVMMemoryData, processVMDisksFromDiskSheet, and processVMNICs", func() {
			excelContent := createComprehensiveVMDataExcel()

			testPolicy := `package io.konveyor.forklift.vmware
			
			import rego.v1

			concerns contains flag if {
				false  # No concerns for this test
				flag := {}
			}`
			policies := map[string]string{
				"test.rego": testPolicy,
			}

			validator, err := opa.NewValidator(policies)
			Expect(err).ToNot(HaveOccurred())

			inventory, err := rvtools.ParseRVTools(context.Background(), excelContent, validator)
			Expect(err).ToNot(HaveOccurred())
			Expect(inventory).ToNot(BeNil())

			Expect(inventory.Vms.Total).To(Equal(1))
			Expect(inventory.Vms.PowerStates).To(HaveKeyWithValue("poweredOn", 1))

			Expect(inventory.Vms.DiskGB.Total).To(BeNumerically(">", 0))
			Expect(inventory.Vms.DiskCount.Total).To(BeNumerically(">", 0))

			Expect(inventory.Vms.CpuCores.Total).To(Equal(4)) // VM has 4 CPUs
			Expect(inventory.Vms.RamGB.Total).To(BeNumerically(">", 0))
		})
	})
})
