package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"time"

	api "github.com/konveyor/forklift-controller/pkg/apis/forklift/v1beta1"
	"github.com/konveyor/forklift-controller/pkg/controller/provider/container/vsphere"
	"github.com/konveyor/forklift-controller/pkg/controller/provider/model"
	vspheremodel "github.com/konveyor/forklift-controller/pkg/controller/provider/model/vsphere"
	web "github.com/konveyor/forklift-controller/pkg/controller/provider/web/vsphere"
	libmodel "github.com/konveyor/forklift-controller/pkg/lib/inventory/model"
	apiplanner "github.com/kubev2v/migration-planner/api/v1alpha1"
	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func main() {
	// Parse command-line arguments
	if len(os.Args) < 3 {
		fmt.Println("Usage: collector <creds_file> <inv_file>")
		os.Exit(1)
	}
	credsFile := os.Args[1]
	outputFile := os.Args[2]

	// Load credentials from file
	credsData, err := os.ReadFile(credsFile)
	if err != nil {
		fmt.Printf("Error reading credentials file: %v\n", err)
		os.Exit(1)
	}

	// Parse JSON credentials
	var creds struct {
		Url      string `json:"url"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.Unmarshal(credsData, &creds); err != nil {
		fmt.Printf("Error parsing credentials JSON: %v\n", err)
		os.Exit(1)
	}

	opaServer := "127.0.0.1:8181"
	// Provider
	vsphereType := api.VSphere
	provider := &api.Provider{
		Spec: api.ProviderSpec{
			URL:  creds.Url,
			Type: &vsphereType,
		},
	}

	// Secret
	secret := &core.Secret{
		ObjectMeta: meta.ObjectMeta{
			Name:      "vsphere-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"user":               []byte(creds.Username),
			"password":           []byte(creds.Password),
			"insecureSkipVerify": []byte("true"),
		},
	}

	// Check if opaServer is responding
	resp, err := http.Get("http://" + opaServer + "/health")
	if err != nil || resp.StatusCode != http.StatusOK {
		fmt.Println("OPA server " + opaServer + " is not responding")
		return
	}
	defer resp.Body.Close()

	// DB
	db, err := createDB(provider)
	if err != nil {
		fmt.Println("Error creating DB.", err)
		return
	}

	// Vshere collector
	collector, err := createCollector(db, provider, secret)
	if err != nil {
		fmt.Println("Error creating collector.", err)
		return
	}
	defer collector.DB().Close(true)
	defer collector.Shutdown()

	// List VMs
	vms := &[]vspheremodel.VM{}
	err = collector.DB().List(vms, libmodel.FilterOptions{Detail: 1})
	if err != nil {
		fmt.Println(err)
		return
	}
	hosts := &[]vspheremodel.Host{}
	err = collector.DB().List(hosts, libmodel.FilterOptions{Detail: 1})
	if err != nil {
		fmt.Println(err)
		return
	}
	clusters := &[]vspheremodel.Cluster{}
	err = collector.DB().List(clusters, libmodel.FilterOptions{Detail: 1})
	if err != nil {
		fmt.Println(err)
		return
	}

	// Create inventory
	inv := &apiplanner.Inventory{
		Vms: apiplanner.VMs{
			Total:       len(*vms),
			PowerStates: map[string]int{},
			Os:          map[string]int{},
			NotMigratableReasons: []struct {
				Assessment string `json:"assessment"`
				Count      int    `json:"count"`
				Label      string `json:"label"`
			}{},
		},
		Infra: apiplanner.Infra{
			Datastores:      getDatastores(collector),
			HostPowerStates: getHostPowerStates(*hosts),
			TotalHosts:      len(*hosts),
			TotalClusters:   len(*clusters),
			HostsPerCluster: getHostsPerCluster(*clusters),
			Networks:        getNetworks(collector),
		},
	}

	// Run the validation of VMs
	vms, err = validation(vms, opaServer)
	if err != nil {
		fmt.Println(err)
		return
	}

	// Transform VM data to inventory.json
	cpuSet := []int{}
	memorySet := []int{}
	diskGBSet := []int{}
	diskCountSet := []int{}
	for _, vm := range *vms {
		// histogram collection
		cpuSet = append(cpuSet, int(vm.CpuCount))
		memorySet = append(memorySet, int(vm.MemoryMB/1024))
		diskGBSet = append(diskGBSet, totalCapacity(vm.Disks))
		diskCountSet = append(diskCountSet, len(vm.Disks))

		// inventory
		migrationReport(vm.Concerns, inv)
		inv.Vms.Os[vm.GuestName]++
		inv.Vms.PowerStates[vm.PowerState]++

		// CPU
		inv.Vms.CpuCores.Total += int(vm.CpuCount)
		if !isMigratable(vm) {
			inv.Vms.CpuCores.TotalForNotMigratable += int(vm.CpuCount)
		} else {
			if isMigratebleWithWarning(vm) {
				inv.Vms.CpuCores.TotalForMigratableWithWarnings += int(vm.CpuCount)
			} else {
				inv.Vms.CpuCores.TotalForMigratable += int(vm.CpuCount)
			}
		}

		// RAM
		inv.Vms.RamGB.Total += int(vm.MemoryMB / 1024)
		if !isMigratable(vm) {
			inv.Vms.RamGB.TotalForNotMigratable += int(vm.MemoryMB / 1024)
		} else {
			if isMigratebleWithWarning(vm) {
				inv.Vms.RamGB.TotalForMigratableWithWarnings += int(vm.MemoryMB / 1024)
			} else {
				inv.Vms.RamGB.TotalForMigratable += int(vm.MemoryMB / 1024)
			}
		}

		// DiskCount
		inv.Vms.DiskCount.Total += len(vm.Disks)
		if !isMigratable(vm) {
			inv.Vms.DiskCount.TotalForNotMigratable += len(vm.Disks)
		} else {
			if isMigratebleWithWarning(vm) {
				inv.Vms.DiskCount.TotalForMigratableWithWarnings += len(vm.Disks)
			} else {
				inv.Vms.DiskCount.TotalForMigratable += len(vm.Disks)
			}
		}

		// DiskGB
		inv.Vms.DiskGB.Total += totalCapacity(vm.Disks)
		if !isMigratable(vm) {
			inv.Vms.DiskGB.TotalForNotMigratable += totalCapacity(vm.Disks)
		} else {
			if isMigratebleWithWarning(vm) {
				inv.Vms.DiskGB.TotalForMigratableWithWarnings += totalCapacity(vm.Disks)
			} else {
				inv.Vms.DiskGB.TotalForMigratable += totalCapacity(vm.Disks)
			}
		}
	}

	// Histogram
	inv.Vms.CpuCores.Histogram = histogram(cpuSet)
	inv.Vms.RamGB.Histogram = histogram(memorySet)
	inv.Vms.DiskCount.Histogram = histogram(diskCountSet)
	inv.Vms.DiskGB.Histogram = histogram(diskGBSet)

	// Write the inventory to output file:
	if err := createOuput(outputFile, inv); err != nil {
		fmt.Println("Error writing output:", err)
		return
	}
}

func histogram(d []int) struct {
	Data     []int `json:"data"`
	MinValue int   `json:"minValue"`
	Step     int   `json:"step"`
} {
	minVal := slices.Min(d)
	maxVal := slices.Max(d)

	// Calculate the range of values, number of data points, number of bins, and bin size
	rangeValues := maxVal - minVal
	numberOfDataPoints := len(d)
	numberOfBins := int(math.Sqrt(float64(numberOfDataPoints)))
	binSize := float64(rangeValues) / float64(numberOfBins)

	// Initialize the bins with 0s
	bins := make([]int, numberOfBins)

	// Fill the bins based on data points
	for _, data := range d {
		binIndex := int(float64(data-minVal) / binSize)
		if binIndex == numberOfBins {
			binIndex--
		}
		bins[binIndex]++
	}

	return struct {
		Data     []int `json:"data"`
		MinValue int   `json:"minValue"`
		Step     int   `json:"step"`
	}{
		Data:     bins,
		Step:     int(math.Round(binSize)),
		MinValue: minVal,
	}
}

func isMigratable(vm vspheremodel.VM) bool {
	for _, c := range vm.Concerns {
		if c.Category == "Critical" {
			return false
		}
	}
	return true
}

func isMigratebleWithWarning(vm vspheremodel.VM) bool {
	for _, c := range vm.Concerns {
		if c.Category == "Warning" {
			return true
		}
	}

	return false
}

func getNetworks(collector *vsphere.Collector) []struct {
	Name string                       `json:"name"`
	Type apiplanner.InfraNetworksType `json:"type"`
} {
	r := []struct {
		Name string                       `json:"name"`
		Type apiplanner.InfraNetworksType `json:"type"`
	}{}
	networks := &[]vspheremodel.Network{}
	err := collector.DB().List(networks, libmodel.FilterOptions{Detail: 1})
	if err != nil {
		return nil
	}
	for _, n := range *networks {
		r = append(r, struct {
			Name string                       `json:"name"`
			Type apiplanner.InfraNetworksType `json:"type"`
		}{Name: n.Name, Type: apiplanner.InfraNetworksType(getNetworkType(&n))})
	}

	return r
}

// FIXME:
func getNetworkType(n *vspheremodel.Network) string {
	if n.Key == "hosted" {
		return "standard"
	} else {
		return "distributed"
	}
}

func getHostsPerCluster(clusters []vspheremodel.Cluster) []int {
	res := []int{}
	for _, c := range clusters {
		res = append(res, len(c.Hosts))
	}
	return res
}

func getHostPowerStates(hosts []vspheremodel.Host) map[string]int {
	states := map[string]int{}

	for _, host := range hosts {
		states[host.Status]++
	}

	return states
}

func getDatastores(collector *vsphere.Collector) []struct {
	FreeCapacityGB  int    `json:"freeCapacityGB"`
	TotalCapacityGB int    `json:"totalCapacityGB"`
	Type            string `json:"type"`
} {
	datastores := &[]vspheremodel.Datastore{}
	err := collector.DB().List(datastores, libmodel.FilterOptions{Detail: 1})
	if err != nil {
		return nil
	}
	res := []struct {
		FreeCapacityGB  int    `json:"freeCapacityGB"`
		TotalCapacityGB int    `json:"totalCapacityGB"`
		Type            string `json:"type"`
	}{}
	for _, ds := range *datastores {
		res = append(res, struct {
			FreeCapacityGB  int    `json:"freeCapacityGB"`
			TotalCapacityGB int    `json:"totalCapacityGB"`
			Type            string `json:"type"`
		}{TotalCapacityGB: int(ds.Capacity / 1024 / 1024 / 1024), FreeCapacityGB: int(ds.Free / 1024 / 1024 / 1024), Type: ds.Type})
	}

	return res
}

func totalCapacity(disks []vspheremodel.Disk) int {
	total := 0
	for _, d := range disks {
		total += int(d.Capacity)
	}
	return total / 1024 / 1024 / 1024
}

func hasLabel(
	reasons []struct {
		Assessment string `json:"assessment"`
		Count      int    `json:"count"`
		Label      string `json:"label"`
	},
	label string,
) int {
	for i, reason := range reasons {
		if label == reason.Label {
			return i
		}
	}
	return -1
}

func createCollector(db libmodel.DB, provider *api.Provider, secret *core.Secret) (*vsphere.Collector, error) {
	collector := vsphere.New(db, provider, secret)

	// Collect
	err := collector.Start()
	if err != nil {
		return nil, err
	}

	// Wait for collector.
	for {
		time.Sleep(1 * time.Second)
		if collector.HasParity() {
			break
		}
	}
	return collector, nil
}

func createDB(provider *api.Provider) (libmodel.DB, error) {
	path := filepath.Join("/tmp", "db.db")
	models := model.Models(provider)
	db := libmodel.New(path, models...)
	err := db.Open(true)
	if err != nil {
		return nil, err
	}
	return db, nil
}

func createOuput(outputFile string, inv *apiplanner.Inventory) error {
	// Create or open the file
	file, err := os.Create(outputFile)
	if err != nil {
		return err
	}
	// Ensure the file is closed properly
	defer file.Close()

	// Write the string to the file
	jsonData, err := json.Marshal(&InventoryData{Inventory: *inv})
	if err != nil {
		return err
	}
	_, err = file.Write(jsonData)
	if err != nil {
		return err
	}

	return nil
}

func validation(vms *[]vspheremodel.VM, opaServer string) (*[]vspheremodel.VM, error) {
	res := []vspheremodel.VM{}
	for _, vm := range *vms {
		// Prepare the JSON data to MTV OPA server format.
		r := web.Workload{}
		r.With(&vm)
		vmJson := map[string]interface{}{
			"input": r,
		}

		vmData, err := json.Marshal(vmJson)
		if err != nil {
			return nil, err
		}

		// Prepare the HTTP request to OPA server
		req, err := http.NewRequest(
			"POST",
			fmt.Sprintf("http://%s/v1/data/io/konveyor/forklift/vmware/concerns", opaServer),
			bytes.NewBuffer(vmData),
		)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")

		// Send the HTTP request to OPA server
		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			continue
		}

		// Check the response status
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("invalid status code")
		}

		// Read the response body
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}

		// Save the report to map
		var vmValidation VMValidation
		err = json.Unmarshal(body, &vmValidation)
		if err != nil {
			return nil, err
		}
		for _, c := range vmValidation.Result {
			vm.Concerns = append(vm.Concerns, vspheremodel.Concern{Label: c.Label, Assessment: c.Assessment, Category: c.Category})
		}
		resp.Body.Close()
		res = append(res, vm)
	}
	return &res, nil
}

func migrationReport(concern []vspheremodel.Concern, inv *apiplanner.Inventory) {
	migratable := true
	hasWarning := false
	for _, result := range concern {
		if result.Category == "Critical" {
			migratable = false
			if i := hasLabel(inv.Vms.NotMigratableReasons, result.Label); i >= 0 {
				inv.Vms.NotMigratableReasons[i].Count++
			} else {
				inv.Vms.NotMigratableReasons = append(inv.Vms.NotMigratableReasons, NotMigratableReason{
					Label:      result.Label,
					Count:      0,
					Assessment: result.Assessment,
				})
			}
		}
		if result.Category == "Warning" {
			hasWarning = true
			if i := hasLabel(inv.Vms.MigrationWarnings, result.Label); i >= 0 {
				inv.Vms.MigrationWarnings[i].Count++
			} else {
				inv.Vms.MigrationWarnings = append(inv.Vms.MigrationWarnings, NotMigratableReason{
					Label:      result.Label,
					Count:      0,
					Assessment: result.Assessment,
				})
			}
		}
	}
	if hasWarning {
		if inv.Vms.TotalMigratableWithWarnings == nil {
			total := 0
			inv.Vms.TotalMigratableWithWarnings = &total
		}
		*inv.Vms.TotalMigratableWithWarnings++
	}
	if migratable {
		inv.Vms.TotalMigratable++
	}
}

type NotMigratableReasons []NotMigratableReason

type NotMigratableReason struct {
	Assessment string `json:"assessment"`
	Count      int    `json:"count"`
	Label      string `json:"label"`
}

type VMResult struct {
	Assessment string `json:"assessment"`
	Category   string `json:"category"`
	Label      string `json:"label"`
}

type VMValidation struct {
	Result []VMResult `json:"result"`
}

type InventoryData struct {
	Inventory apiplanner.Inventory `json:"inventory"`
	Error     string               `json:"error"`
}
