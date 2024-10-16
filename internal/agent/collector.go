package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	api "github.com/konveyor/forklift-controller/pkg/apis/forklift/v1beta1"
	"github.com/konveyor/forklift-controller/pkg/controller/provider/container/vsphere"
	"github.com/konveyor/forklift-controller/pkg/controller/provider/model"
	vspheremodel "github.com/konveyor/forklift-controller/pkg/controller/provider/model/vsphere"
	web "github.com/konveyor/forklift-controller/pkg/controller/provider/web/vsphere"
	libmodel "github.com/konveyor/forklift-controller/pkg/lib/inventory/model"
	apiplanner "github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/util"
	"github.com/kubev2v/migration-planner/pkg/log"
	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Collector struct {
	log     *log.PrefixLogger
	dataDir string
	once    sync.Once
}

func NewCollector(log *log.PrefixLogger, dataDir string) *Collector {
	return &Collector{
		log:     log,
		dataDir: dataDir,
	}
}

func (c *Collector) collect(ctx context.Context) {
	c.once.Do(func() {
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				default:
					c.run()
					return
				}
			}
		}()
	})
}

func (c *Collector) run() {
	credentialsFilePath := filepath.Join(c.dataDir, CredentialsFile)
	c.log.Infof("Waiting for credentials")
	waitForFile(credentialsFilePath)

	credsData, err := os.ReadFile(credentialsFilePath)
	if err != nil {
		c.log.Errorf("Error reading credentials file: %v\n", err)
		return
	}

	var creds Credentials
	if err := json.Unmarshal(credsData, &creds); err != nil {
		c.log.Errorf("Error parsing credentials JSON: %v\n", err)
		return
	}

	opaServer := util.GetEnv("OPA_SERVER", "127.0.0.1:8181")
	c.log.Infof("Create Provider")
	provider := getProvider(creds)

	c.log.Infof("Create Secret")
	secret := getSecret(creds)

	c.log.Infof("Check if opaServer is responding")
	resp, err := http.Get("http://" + opaServer + "/health")
	if err != nil || resp.StatusCode != http.StatusOK {
		c.log.Errorf("OPA server %s is not responding", opaServer)
		return
	}
	defer resp.Body.Close()

	c.log.Infof("Create DB")
	db, err := createDB(provider)
	if err != nil {
		c.log.Errorf("Error creating DB: %s", err)
		return
	}

	c.log.Infof("vSphere collector")
	collector, err := createCollector(db, provider, secret)
	if err != nil {
		c.log.Errorf("Error running collector: %s", err)
		return
	}
	defer collector.DB().Close(true)
	defer collector.Shutdown()

	c.log.Infof("List VMs")
	vms := &[]vspheremodel.VM{}
	err = collector.DB().List(vms, libmodel.FilterOptions{Detail: 1})
	if err != nil {
		c.log.Errorf("Error list database: %s", err)
		return
	}

	c.log.Infof("List Hosts")
	hosts := &[]vspheremodel.Host{}
	err = collector.DB().List(hosts, libmodel.FilterOptions{Detail: 1})
	if err != nil {
		c.log.Errorf("Error list database: %s", err)
		return
	}

	c.log.Infof("List Clusters")
	clusters := &[]vspheremodel.Cluster{}
	err = collector.DB().List(clusters, libmodel.FilterOptions{Detail: 1})
	if err != nil {
		c.log.Errorf("Error list database: %s", err)
		return
	}

	c.log.Infof("Create inventory")
	inv := createBasicInventoryObj(vms, collector, hosts, clusters)

	c.log.Infof("Run the validation of VMs")
	vms, err = validation(vms, opaServer)
	if err != nil {
		c.log.Errorf("Error running validation: %s", err)
		return
	}

	c.log.Infof("Fill the inventory object with more data")
	fillInventoryObjectWithMoreData(vms, inv)

	c.log.Infof("Write the inventory to output file")
	if err := createOuput(filepath.Join(c.dataDir, InventoryFile), inv); err != nil {
		c.log.Errorf("Fill the inventory object with more data: %s", err)
		return
	}
}

func fillInventoryObjectWithMoreData(vms *[]vspheremodel.VM, inv *apiplanner.Inventory) {
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
		migratable, hasWarning := migrationReport(vm.Concerns, inv)
		inv.Vms.Os[vm.GuestName]++
		inv.Vms.PowerStates[vm.PowerState]++

		// Update total values
		inv.Vms.CpuCores.Total += int(vm.CpuCount)
		inv.Vms.RamGB.Total += int(vm.MemoryMB / 1024)
		inv.Vms.DiskCount.Total += len(vm.Disks)
		inv.Vms.DiskGB.Total += totalCapacity(vm.Disks)

		// Not Migratable
		if !migratable {
			inv.Vms.CpuCores.TotalForNotMigratable += int(vm.CpuCount)
			inv.Vms.RamGB.TotalForNotMigratable += int(vm.MemoryMB / 1024)
			inv.Vms.DiskCount.TotalForNotMigratable += len(vm.Disks)
			inv.Vms.DiskGB.TotalForNotMigratable += totalCapacity(vm.Disks)
		} else {
			// Migratable with warning(s)
			if hasWarning {
				inv.Vms.CpuCores.TotalForMigratableWithWarnings += int(vm.CpuCount)
				inv.Vms.RamGB.TotalForMigratableWithWarnings += int(vm.MemoryMB / 1024)
				inv.Vms.DiskCount.TotalForMigratableWithWarnings += len(vm.Disks)
				inv.Vms.DiskGB.TotalForMigratableWithWarnings += totalCapacity(vm.Disks) //Migratable
			} else {
				// Migratable without any warnings
				inv.Vms.CpuCores.TotalForMigratable += int(vm.CpuCount)
				inv.Vms.RamGB.TotalForMigratable += int(vm.MemoryMB / 1024)
				inv.Vms.DiskCount.TotalForMigratable += len(vm.Disks)
				inv.Vms.DiskGB.TotalForMigratable += totalCapacity(vm.Disks)
			}
		}

	}

	// Histogram
	inv.Vms.CpuCores.Histogram = histogram(cpuSet)
	inv.Vms.RamGB.Histogram = histogram(memorySet)
	inv.Vms.DiskCount.Histogram = histogram(diskCountSet)
	inv.Vms.DiskGB.Histogram = histogram(diskGBSet)
}

func createBasicInventoryObj(vms *[]vspheremodel.VM, collector *vsphere.Collector, hosts *[]vspheremodel.Host, clusters *[]vspheremodel.Cluster) *apiplanner.Inventory {
	return &apiplanner.Inventory{
		Vms: apiplanner.VMs{
			Total:       len(*vms),
			PowerStates: map[string]int{},
			Os:          map[string]int{},
			MigrationWarnings: []struct {
				Assessment string `json:"assessment"`
				Count      int    `json:"count"`
				Label      string `json:"label"`
			}{},
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
}

func getProvider(creds Credentials) *api.Provider {
	vsphereType := api.VSphere
	return &api.Provider{
		Spec: api.ProviderSpec{
			URL:  creds.URL,
			Type: &vsphereType,
		},
	}
}

func getSecret(creds Credentials) *core.Secret {
	return &core.Secret{
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

func migrationReport(concern []vspheremodel.Concern, inv *apiplanner.Inventory) (bool, bool) {
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
					Count:      1,
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
					Count:      1,
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
	return migratable, hasWarning
}

func waitForFile(filename string) {
	for {
		// Check if the file exists
		if _, err := os.Stat(filename); err == nil {
			// File exists, exit the loop
			break
		} else if os.IsNotExist(err) {
			// File does not exist, wait and check again
			time.Sleep(2 * time.Second) // Wait for 2 seconds before checking again
		} else {
			return
		}
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
