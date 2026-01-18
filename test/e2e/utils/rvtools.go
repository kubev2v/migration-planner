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
	headers := []string{"VM", "VM ID", "VI SDK UUID", "Host", "Cluster"}
	for colIndex, header := range headers {
		cellRef := ColumnToLetter(colIndex) + "1"
		if err := f.SetCellValue("vInfo", cellRef, header); err != nil {
			return nil, err
		}
	}

	// Add at least one VM data row to pass validation
	vmRow := []string{"test-vm-1", "vm-001", "12345678-1234-1234-1234-123456789abc", "esxi-host-1", "cluster1"}
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
	vHostHeaders := []string{"Host", "Vendor", "Model", "Object ID", "Datacenter", "Cluster"}
	for colIndex, header := range vHostHeaders {
		cellRef := ColumnToLetter(colIndex) + "1"
		if err := f.SetCellValue("vHost", cellRef, header); err != nil {
			return nil, err
		}
	}
	vHostRow := []string{"esxi-host-1", "VMware", "ESXi", "host-001", "datacenter1", "cluster1"}
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
			Headers: []string{"VM", "VM ID", "VI SDK UUID", "Host", "CPUs", "Memory", "PowerState", "Cluster"},
			Rows:    vmRows,
		},
		"vHost": {
			Headers: []string{"Host", "Vendor", "Model", "Object ID", "Datacenter", "Cluster"},
			Rows: [][]string{
				{"esxi-host-1", "VMware", "ESXi", "host-001", "datacenter1", "cluster1"},
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
			Headers: []string{"VM", "VM ID", "VI SDK UUID", "Host", "CPUs", "Memory", "PowerState", "Cluster"},
			Rows: [][]string{
				{"test-vm-1", "vm-001", "12345678-1234-1234-1234-123456789abc", "esxi-host-1", "4", "8192", "poweredOn", "cluster1"},
				{"test-vm-2", "vm-002", "87654321-4321-4321-4321-210987654321", "esxi-host-2", "2", "4096", "poweredOff", "cluster1"},
			},
		},
		"vHost": {
			Headers: []string{"Host", "Vendor", "Model", "Object ID", "Datacenter", "Cluster"},
			Rows: [][]string{
				{"esxi-host-1", "VMware", "ESXi", "host-001", "datacenter1", "cluster1"},
				{"esxi-host-2", "VMware", "ESXi", "host-002", "datacenter1", "cluster1"},
			},
		},
		"vDatastore": {
			Headers: []string{"Name", "Type", "Capacity MiB", "Free MiB", "Object ID"},
			Rows: [][]string{
				{"datastore1", "VMFS", "1048576", "524288", "datastore-001"},
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
			Headers: []string{"VM", "VM ID", "VI SDK UUID", "Host", "CPUs", "Memory", "PowerState", "Cluster"},
			Rows: [][]string{
				{"example-vm-1", "vm-example-001", "12345678-1234-1234-1234-123456789abc", "esxi-host-1", "4", "8192", "poweredOn", "cluster1"},
				{"example-vm-2", "vm-example-002", "87654321-4321-4321-4321-210987654321", "esxi-host-2", "2", "4096", "poweredOff", "cluster1"},
			},
		},
		"vHost": {
			Headers: []string{"Host", "Vendor", "Model", "Object ID", "Datacenter", "Cluster"},
			Rows: [][]string{
				{"esxi-host-1", "VMware", "ESXi", "host-001", "datacenter1", "cluster1"},
				{"esxi-host-2", "VMware", "ESXi", "host-002", "datacenter1", "cluster1"},
			},
		},
		"vDatastore": {
			Headers: []string{"Name", "Type", "Capacity MiB", "Free MiB", "Object ID"},
			Rows: [][]string{
				{"datastore1", "VMFS", "1048576", "524288", "datastore-001"},
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
