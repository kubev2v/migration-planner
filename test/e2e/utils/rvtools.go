package utils

import (
	"bytes"
	"fmt"
	"os"

	"github.com/xuri/excelize/v2"
)

// Helper function to convert column index to Excel column letter
func ColumnToLetter(col int) string {
	name, _ := excelize.ColumnNumberToName(col + 1)
	return name
}

func CreateExcelOnlyWithVInfo() ([]byte, error) {
	f := excelize.NewFile()
	defer f.Close()

	// Only create vInfo sheet, missing vHost and other required sheets
	sheetIndex, err := f.NewSheet("vInfo")
	if err != nil {
		return nil, err
	}
	f.SetActiveSheet(sheetIndex)
	_ = f.DeleteSheet("Sheet1")

	// Set headers for vInfo - include Host column for cluster extraction
	headers := []string{"VM", "VM ID", "VI SDK UUID", "Host", "Cluster", "Powerstate"}
	for colIndex, header := range headers {
		cellRef := ColumnToLetter(colIndex) + "1"
		if err := f.SetCellValue("vInfo", cellRef, header); err != nil {
			return nil, err
		}
	}

	// Add at least one VM data row to pass validation
	vmRow := []string{"test-vm-1", "vm-001", "12345678-1234-1234-1234-123456789abc", "esxi-host-1", "cluster1", "poweredOn"}
	for colIndex, value := range vmRow {
		cellRef := ColumnToLetter(colIndex) + "2"
		if err := f.SetCellValue("vInfo", cellRef, value); err != nil {
			return nil, err
		}
	}

	// Add minimal vHost sheet with cluster info for cluster extraction
	_, err = f.NewSheet("vHost")
	if err != nil {
		return nil, err
	}
	vHostHeaders := []string{"Datacenter", "Cluster", "# Cores", "# CPU", "Object ID", "# Memory", "Model", "Vendor", "Host", "Config status"}
	for colIndex, header := range vHostHeaders {
		cellRef := ColumnToLetter(colIndex) + "1"
		if err := f.SetCellValue("vHost", cellRef, header); err != nil {
			return nil, err
		}
	}
	vHostRow := []string{"datacenter1", "cluster1", "4", "2", "host-001", "16384", "ESXi", "VMware", "esxi-host-1", "green"}
	for colIndex, value := range vHostRow {
		cellRef := ColumnToLetter(colIndex) + "2"
		if err := f.SetCellValue("vHost", cellRef, value); err != nil {
			return nil, err
		}
	}

	var buf bytes.Buffer
	_, err = f.WriteTo(&buf)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func CreateLargeExcel() ([]byte, error) {
	f := excelize.NewFile()
	defer f.Close()

	type SheetConfig struct {
		Headers []string
		Rows    [][]string
	}

	// Create a large number of VMs to test file size limits
	var vmRows [][]string
	for i := 0; i < 1000; i++ {
		vmRows = append(vmRows, []string{
			fmt.Sprintf("test-vm-%d", i),
			fmt.Sprintf("vm-%03d", i),
			fmt.Sprintf("12345678-1234-1234-1234-%012d", i),
			"esxi-host-1",
			"4",
			"8192",
			"poweredOn",
			"cluster1",
		})
	}

	sheetConfigs := map[string]SheetConfig{
		"vInfo": {
			Headers: []string{"VM", "VM ID", "VI SDK UUID", "Host", "CPUs", "Memory", "Powerstate", "Cluster"},
			Rows:    vmRows,
		},
		"vHost": {
			Headers: []string{"Datacenter", "Cluster", "# Cores", "# CPU", "Object ID", "# Memory", "Model", "Vendor", "Host", "Config status"},
			Rows: [][]string{
				{"datacenter1", "cluster1", "4", "2", "host-001", "16384", "ESXi", "VMware", "esxi-host-1", "green"},
			},
		},
	}

	var vInfoIndex int
	for sheetName, config := range sheetConfigs {
		sheetIndex, err := f.NewSheet(sheetName)
		if err != nil {
			return nil, err
		}
		if sheetName == "vInfo" {
			vInfoIndex = sheetIndex
		}

		// Set headers
		for colIndex, header := range config.Headers {
			cellRef := ColumnToLetter(colIndex) + "1"
			if err := f.SetCellValue(sheetName, cellRef, header); err != nil {
				return nil, err
			}
		}

		// Set data rows
		for rowIndex, row := range config.Rows {
			for colIndex, value := range row {
				cellRef := ColumnToLetter(colIndex) + fmt.Sprintf("%d", rowIndex+2)
				if err := f.SetCellValue(sheetName, cellRef, value); err != nil {
					return nil, err
				}
			}
		}
	}

	f.SetActiveSheet(vInfoIndex)
	_ = f.DeleteSheet("Sheet1")

	var buf bytes.Buffer
	_, err := f.WriteTo(&buf)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func CreateValidTestExcel() ([]byte, error) {
	f := excelize.NewFile()
	defer f.Close()

	type SheetConfig struct {
		Headers []string
		Rows    [][]string
	}

	sheetConfigs := map[string]SheetConfig{
		"vInfo": {
			Headers: []string{"VM", "VM ID", "VI SDK UUID", "Host", "CPUs", "Memory", "Powerstate", "Cluster"},
			Rows: [][]string{
				{"test-vm-1", "vm-001", "12345678-1234-1234-1234-123456789abc", "esxi-host-1", "4", "8192", "poweredOn", "cluster1"},
				{"test-vm-2", "vm-002", "87654321-4321-4321-4321-210987654321", "esxi-host-2", "2", "4096", "poweredOff", "cluster1"},
			},
		},
		"vHost": {
			Headers: []string{"Datacenter", "Cluster", "# Cores", "# CPU", "Object ID", "# Memory", "Model", "Vendor", "Host", "Config status"},
			Rows: [][]string{
				{"datacenter1", "cluster1", "4", "2", "host-001", "16384", "ESXi", "VMware", "esxi-host-1", "green"},
				{"datacenter1", "cluster1", "4", "2", "host-002", "16384", "ESXi", "VMware", "esxi-host-2", "green"},
			},
		},
		"vDatastore": {
			Headers: []string{"Hosts", "Address", "Name", "Object ID", "Free MiB", "MHA", "Capacity MiB", "Type"},
			Rows: [][]string{
				{"esxi-host-1", "10.0.0.1", "datastore1", "datastore-001", "524288", "false", "1048576", "VMFS"},
			},
		},
	}

	var vInfoIndex int
	for sheetName, config := range sheetConfigs {
		sheetIndex, err := f.NewSheet(sheetName)
		if err != nil {
			return nil, err
		}
		if sheetName == "vInfo" {
			vInfoIndex = sheetIndex
		}

		// Set headers
		for colIndex, header := range config.Headers {
			cellRef := ColumnToLetter(colIndex) + "1"
			if err := f.SetCellValue(sheetName, cellRef, header); err != nil {
				return nil, err
			}
		}

		// Set data rows
		for rowIndex, row := range config.Rows {
			for colIndex, value := range row {
				cellRef := ColumnToLetter(colIndex) + fmt.Sprintf("%d", rowIndex+2)
				if err := f.SetCellValue(sheetName, cellRef, value); err != nil {
					return nil, err
				}
			}
		}
	}

	f.SetActiveSheet(vInfoIndex)
	_ = f.DeleteSheet("Sheet1")

	var buf bytes.Buffer
	if _, err := f.WriteTo(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// CreateExample1Excel creates a valid example Excel file matching the format expected by e2e tests
// This replaces the static example1.xlsx file with a dynamically generated one that meets validation requirements
func CreateExample1Excel() ([]byte, error) {
	f := excelize.NewFile()
	defer f.Close()

	type SheetConfig struct {
		Headers []string
		Rows    [][]string
	}

	sheetConfigs := map[string]SheetConfig{
		"vInfo": {
			Headers: []string{"VM", "VM ID", "VI SDK UUID", "Host", "CPUs", "Memory", "Powerstate", "Cluster"},
			Rows: [][]string{
				{"example-vm-1", "vm-example-001", "12345678-1234-1234-1234-123456789abc", "esxi-host-1", "4", "8192", "poweredOn", "cluster1"},
				{"example-vm-2", "vm-example-002", "87654321-4321-4321-4321-210987654321", "esxi-host-2", "2", "4096", "poweredOff", "cluster1"},
			},
		},
		"vHost": {
			Headers: []string{"Datacenter", "Cluster", "# Cores", "# CPU", "Object ID", "# Memory", "Model", "Vendor", "Host", "Config status"},
			Rows: [][]string{
				{"datacenter1", "cluster1", "4", "2", "host-001", "16384", "ESXi", "VMware", "esxi-host-1", "green"},
				{"datacenter1", "cluster1", "4", "2", "host-002", "16384", "ESXi", "VMware", "esxi-host-2", "green"},
			},
		},
		"vDatastore": {
			Headers: []string{"Hosts", "Address", "Name", "Object ID", "Free MiB", "MHA", "Capacity MiB", "Type"},
			Rows: [][]string{
				{"esxi-host-1", "10.0.0.1", "datastore1", "datastore-001", "524288", "false", "1048576", "VMFS"},
			},
		},
		"dvSwitch": {
			Headers: []string{"Name"},
			Rows: [][]string{
				{"dvSwitch1"},
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
		sheetIndex, err := f.NewSheet(sheetName)
		if err != nil {
			return nil, err
		}
		if sheetName == "vInfo" {
			vInfoIndex = sheetIndex
		}

		// Set headers
		for colIndex, header := range config.Headers {
			cellRef := ColumnToLetter(colIndex) + "1"
			if err := f.SetCellValue(sheetName, cellRef, header); err != nil {
				return nil, err
			}
		}

		// Set data rows
		for rowIndex, row := range config.Rows {
			for colIndex, value := range row {
				cellRef := ColumnToLetter(colIndex) + fmt.Sprintf("%d", rowIndex+2)
				if err := f.SetCellValue(sheetName, cellRef, value); err != nil {
					return nil, err
				}
			}
		}
	}

	f.SetActiveSheet(vInfoIndex)
	_ = f.DeleteSheet("Sheet1")

	var buf bytes.Buffer
	if _, err := f.WriteTo(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Helper function to create a temporary Excel file for testing
func CreateTempExcelFile(content []byte) (string, error) {
	tmpFile, err := os.CreateTemp("", "test-rvtools-*.xlsx")
	if err != nil {
		return "", err
	}
	defer tmpFile.Close()

	_, err = tmpFile.Write(content)
	if err != nil {
		os.Remove(tmpFile.Name())
		return "", err
	}

	return tmpFile.Name(), nil
}

// CreateMultiClusterTestExcel creates a test Excel file with 4 VMs across 2 clusters.
// Used for testing multi-cluster inventory scenarios.
func CreateMultiClusterTestExcel() ([]byte, error) {
	f := excelize.NewFile()
	defer f.Close()

	type SheetConfig struct {
		Headers []string
		Rows    [][]string
	}

	sheetConfigs := map[string]SheetConfig{
		"vInfo": {
			Headers: []string{"VM", "VM ID", "VI SDK UUID", "Host", "CPUs", "Memory", "Powerstate", "Cluster", "Datacenter"},
			Rows: [][]string{
				{"vm-cluster1-1", "vm-c1-001", "11111111-1111-1111-1111-111111111111", "esxi-host-1", "4", "8192", "poweredOn", "cluster1", "datacenter1"},
				{"vm-cluster1-2", "vm-c1-002", "22222222-2222-2222-2222-222222222222", "esxi-host-1", "2", "4096", "poweredOff", "cluster1", "datacenter1"},
				{"vm-cluster2-1", "vm-c2-001", "33333333-3333-3333-3333-333333333333", "esxi-host-2", "8", "16384", "poweredOn", "cluster2", "datacenter1"},
				{"vm-cluster2-2", "vm-c2-002", "44444444-4444-4444-4444-444444444444", "esxi-host-2", "4", "8192", "poweredOn", "cluster2", "datacenter1"},
			},
		},
		"vHost": {
			Headers: []string{"Datacenter", "Cluster", "# Cores", "# CPU", "Object ID", "# Memory", "Model", "Vendor", "Host", "Config status"},
			Rows: [][]string{
				{"datacenter1", "cluster1", "8", "2", "host-001", "32768", "PowerEdge R740", "Dell", "esxi-host-1", "green"},
				{"datacenter1", "cluster2", "16", "2", "host-002", "65536", "ProLiant DL380", "HP", "esxi-host-2", "green"},
			},
		},
		"vDatastore": {
			Headers: []string{"Hosts", "Address", "Name", "Object ID", "Free MiB", "MHA", "Capacity MiB", "Type"},
			Rows: [][]string{
				{"esxi-host-1", "10.0.0.1", "datastore1", "datastore-001", "524288", "false", "1048576", "VMFS"},
				{"esxi-host-2", "10.0.0.2", "datastore2", "datastore-002", "262144", "false", "524288", "VMFS"},
			},
		},
		"vCluster": {
			Headers: []string{"Name", "Object ID"},
			Rows: [][]string{
				{"cluster1", "domain-c100"},
				{"cluster2", "domain-c200"},
			},
		},
	}

	var vInfoIndex int
	for sheetName, config := range sheetConfigs {
		sheetIndex, err := f.NewSheet(sheetName)
		if err != nil {
			return nil, err
		}
		if sheetName == "vInfo" {
			vInfoIndex = sheetIndex
		}

		for colIndex, header := range config.Headers {
			cellRef := ColumnToLetter(colIndex) + "1"
			if err := f.SetCellValue(sheetName, cellRef, header); err != nil {
				return nil, err
			}
		}

		for rowIndex, row := range config.Rows {
			for colIndex, value := range row {
				cellRef := ColumnToLetter(colIndex) + fmt.Sprintf("%d", rowIndex+2)
				if err := f.SetCellValue(sheetName, cellRef, value); err != nil {
					return nil, err
				}
			}
		}
	}

	f.SetActiveSheet(vInfoIndex)
	_ = f.DeleteSheet("Sheet1")

	var buf bytes.Buffer
	if _, err := f.WriteTo(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// CreateExcelWithConcerns creates a test Excel file with VMs that will trigger migration concerns.
// Contains VMs with CBT disabled, templates, and other concern-triggering configurations.
func CreateExcelWithConcerns() ([]byte, error) {
	f := excelize.NewFile()
	defer f.Close()

	type SheetConfig struct {
		Headers []string
		Rows    [][]string
	}

	// VMs designed to trigger concerns:
	// - vm-template: Template=true triggers "Template VM" concern
	// - vm-cbt-disabled: CBT=false triggers "CBT disabled" concern
	// - vm-normal: RDM disk triggers "RDM not supported" concern
	sheetConfigs := map[string]SheetConfig{
		"vInfo": {
			Headers: []string{
				"VM", "VM ID", "VI SDK UUID", "Host", "CPUs", "Memory", "Powerstate",
				"Cluster", "Datacenter", "Template", "CBT", "Firmware",
			},
			Rows: [][]string{
				{"vm-template", "vm-001", "11111111-1111-1111-1111-111111111111", "esxi-host-1", "4", "8192", "poweredOff", "cluster1", "datacenter1", "true", "true", "bios"},
				{"vm-cbt-disabled", "vm-002", "22222222-2222-2222-2222-222222222222", "esxi-host-1", "2", "4096", "poweredOn", "cluster1", "datacenter1", "false", "false", "bios"},
				{"vm-normal", "vm-003", "33333333-3333-3333-3333-333333333333", "esxi-host-1", "4", "8192", "poweredOn", "cluster1", "datacenter1", "false", "true", "bios"},
			},
		},
		"vHost": {
			Headers: []string{"Datacenter", "Cluster", "# Cores", "# CPU", "Object ID", "# Memory", "Model", "Vendor", "Host", "Config status"},
			Rows: [][]string{
				{"datacenter1", "cluster1", "8", "2", "host-001", "32768", "ESXi", "VMware", "esxi-host-1", "green"},
			},
		},
		"vDatastore": {
			Headers: []string{"Hosts", "Address", "Name", "Object ID", "Free MiB", "MHA", "Capacity MiB", "Type"},
			Rows: [][]string{
				{"esxi-host-1", "10.0.0.1", "datastore1", "datastore-001", "524288", "false", "1048576", "VMFS"},
			},
		},
		"vCluster": {
			Headers: []string{"Name", "Object ID"},
			Rows: [][]string{
				{"cluster1", "domain-c100"},
			},
		},
		"vDisk": {
			Headers: []string{
				"VM ID", "Disk Key", "Unit #", "Path", "Disk Path", "Capacity MiB",
				"Sharing mode", "Raw", "Shared Bus", "Disk Mode", "Disk UUID",
				"Thin", "Controller", "Label", "SCSI Unit #",
			},
			Rows: [][]string{
				{"vm-003", "2000", "0", "[rdm-ds] vm-normal/rdm.vmdk", "rdm://disk-001", "102400",
					"false", "true", "scsi0", "physical", "rdm-uuid-001",
					"false", "SCSI controller 0", "Hard disk 1", "0"},
			},
		},
	}

	var vInfoIndex int
	for sheetName, config := range sheetConfigs {
		sheetIndex, err := f.NewSheet(sheetName)
		if err != nil {
			return nil, err
		}
		if sheetName == "vInfo" {
			vInfoIndex = sheetIndex
		}

		for colIndex, header := range config.Headers {
			cellRef := ColumnToLetter(colIndex) + "1"
			if err := f.SetCellValue(sheetName, cellRef, header); err != nil {
				return nil, err
			}
		}

		for rowIndex, row := range config.Rows {
			for colIndex, value := range row {
				cellRef := ColumnToLetter(colIndex) + fmt.Sprintf("%d", rowIndex+2)
				if err := f.SetCellValue(sheetName, cellRef, value); err != nil {
					return nil, err
				}
			}
		}
	}

	f.SetActiveSheet(vInfoIndex)
	_ = f.DeleteSheet("Sheet1")

	var buf bytes.Buffer
	if _, err := f.WriteTo(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
