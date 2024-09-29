package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"time"

	api "github.com/konveyor/forklift-controller/pkg/apis/forklift/v1beta1"
	"github.com/konveyor/forklift-controller/pkg/controller/provider/container/vsphere"
	"github.com/konveyor/forklift-controller/pkg/controller/provider/model"
	vspheremodel "github.com/konveyor/forklift-controller/pkg/controller/provider/model/vsphere"
	web "github.com/konveyor/forklift-controller/pkg/controller/provider/web/vsphere"
	libmodel "github.com/konveyor/forklift-controller/pkg/lib/inventory/model"
	apiplanner "github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/view"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type VCenterCreds struct {
	Url      string `json:"url"`
	Username string `json:"username"`
	Password string `json:"password"`
}

func main() {
	logger := log.New(os.Stdout, "*********** Collector: ", log.Ldate|log.Ltime|log.Lshortfile)
	// Parse command-line arguments
	if len(os.Args) < 3 {
		fmt.Println("Usage: collector <creds_file> <inv_file>")
		os.Exit(1)
	}
	credsFile := os.Args[1]
	outputFile := os.Args[2]

	logger.Println("Wait for credentials")
	waitForFile(credsFile)

	logger.Println("Load credentials from file")
	credsData, err := os.ReadFile(credsFile)
	if err != nil {
		fmt.Printf("Error reading credentials file: %v\n", err)
		os.Exit(1)
	}

	var creds VCenterCreds
	if err := json.Unmarshal(credsData, &creds); err != nil {
		fmt.Printf("Error parsing credentials JSON: %v\n", err)
		os.Exit(1)
	}

	opaServer := "127.0.0.1:8181"
	logger.Println("Create Provider")
	provider := getProvider(creds)

	logger.Println("Create Secret")
	secret := getSecret(creds)

	logger.Println("Create govmomi client")
	govmomiClient := createGoVmomiClient(creds)

	logger.Println("Check if opaServer is responding")
	resp, err := http.Get("http://" + opaServer + "/health")
	if err != nil || resp.StatusCode != http.StatusOK {
		fmt.Println("OPA server " + opaServer + " is not responding")
		return
	}
	defer resp.Body.Close()

	logger.Println("Create DB")
	db, err := createDB(provider)
	if err != nil {
		fmt.Println("Error creating DB.", err)
		return
	}

	logger.Println("vSphere collector")
	collector, err := createCollector(db, provider, secret)
	if err != nil {
		fmt.Println("Error creating collector.", err)
		return
	}
	defer collector.DB().Close(true)
	defer collector.Shutdown()

	logger.Println("List VMs")
	vms := &[]vspheremodel.VM{}
	err = collector.DB().List(vms, libmodel.FilterOptions{Detail: 1})
	if err != nil {
		fmt.Println(err)
		return
	}

	logger.Println("List Hosts")
	hosts := &[]vspheremodel.Host{}
	err = collector.DB().List(hosts, libmodel.FilterOptions{Detail: 1})
	if err != nil {
		fmt.Println(err)
		return
	}

	logger.Println("List Clusters")
	clusters := &[]vspheremodel.Cluster{}
	err = collector.DB().List(clusters, libmodel.FilterOptions{Detail: 1})
	if err != nil {
		fmt.Println(err)
		return
	}

	vlanIDs := retrieveVlanInformation(logger, govmomiClient)

	logger.Println("Create inventory")
	inv := createBasicInventoryObj(vms, collector, hosts, clusters, vlanIDs)

	logger.Println("Run the validation of VMs")
	vms, err = validation(vms, opaServer)
	if err != nil {
		fmt.Println(err)
		return
	}

	logger.Println("Fill the inventory object with more data")
	fillInventoryObjectWithMoreData(vms, inv)

	logger.Println("Write the inventory to output file")
	if err := createOuput(outputFile, inv); err != nil {
		fmt.Println("Error writing output:", err)
		return
	}
}

func retrieveVlanInformation(logger *log.Logger, govmomiClient *govmomi.Client) []string {
	logger.Println("retrieveVlanInformation")
	ctx := context.Background()

	seenVlanIds := retrieveDistributedVLANs(logger, govmomiClient, ctx)
	seenVlanIds = retrieveStandardVLANs(logger, govmomiClient, ctx, seenVlanIds)
	for
	return vlanIDs
}

func retrieveStandardVLANs(logger *log.Logger, govmomiClient *govmomi.Client, ctx context.Context, seenVlanIds map[string]bool) map[string]bool {
	// Use ViewManager to create a view of HostSystem objects
	m := view.NewManager(govmomiClient.Client)
	v, err := m.CreateContainerView(ctx, govmomiClient.ServiceContent.RootFolder, []string{"HostSystem"}, true)
	if err != nil {
		logger.Fatal("Error creating container view: %v\n", err)
		return seenVlanIds
	}
	defer v.Destroy(ctx)

	// Retrieve all HostSystem objects
	var hostList []mo.HostSystem
	err = v.Retrieve(ctx, []string{"HostSystem"}, []string{"name", "config.network.vswitch"}, &hostList)
	if err != nil {
		logger.Fatal("Error retrieving HostSystem list: %v\n", err)
		return seenVlanIds
	}

	// Iterate over each HostSystem to retrieve standard vSwitches
	for _, host := range hostList {
		logger.Println("Host: %s\n", host.Name)

		// Retrieve standard vSwitches and their VLANs
		for _, portgroup := range host.Config.Network.Portgroup {
			// PortGroup has a VLAN ID assigned
			logger.Println("    PortGroup:", portgroup, " , VLAN ID ", portgroup.Spec.VlanId)
			strVlanID := strconv.Itoa(int((portgroup.Spec.VlanId)))
			if !seenVlanIds[strVlanID] {
				seenVlanIds[strVlanID] = true
			}
		}
	}
	return seenVlanIds
}

func retrieveDistributedVLANs(logger *log.Logger, govmomiClient *govmomi.Client, ctx context.Context) map[string]bool {
	vlans := []string{}
	seenVlanIDs := map[string]bool{}
	m := view.NewManager(govmomiClient.Client)
	v, err := m.CreateContainerView(ctx, govmomiClient.ServiceContent.RootFolder, []string{"DistributedVirtualSwitch"}, true)
	if err != nil {
		logger.Println("Error creating container view: %v\n", err)
		return nil
	}
	defer v.Destroy(ctx)

	// Retrieve all DVS objects
	var dvsList []mo.DistributedVirtualSwitch
	err = v.Retrieve(ctx, []string{"DistributedVirtualSwitch"}, []string{"name", "portgroup"}, &dvsList)
	if err != nil {
		logger.Println("Error retrieving DVS list: %v\n", err)
		return nil
	}
	for _, dvs := range dvsList {
		logger.Println("DVS: %s\n", dvs.Name)

		// Retrieve VLAN information for each port group
		for _, pgRef := range dvs.Portgroup {
			var pgMo mo.DistributedVirtualPortgroup
			err = govmomiClient.RetrieveOne(ctx, pgRef, []string{"name", "config"}, &pgMo)
			if err != nil {
				logger.Println("Error retrieving portgroup properties: %v\n", err)
				continue
			}
			if setting, ok := pgMo.Config.DefaultPortConfig.(*types.VMwareDVSPortSetting); ok {
				switch vlan := setting.Vlan.(type) {
				case *types.VmwareDistributedVirtualSwitchVlanIdSpec:
					// If it's a VLAN ID, retrieve the VLAN ID
					logger.Printf("VLAN ID: %d\n", vlan.VlanId)
					strVlanId := strconv.Itoa(int(vlan.VlanId))
					if !seenVlanIDs[strVlanId] {
						seenVlanIDs[strVlanId] = true
						vlans = append(vlans, strVlanId)
					}
				case *types.VmwareDistributedVirtualSwitchTrunkVlanSpec:
					// If it's a Trunk VLAN Spec, retrieve the range of VLAN IDs
					logger.Printf("Trunk VLAN IDs: %v\n", vlan.VlanId)
					for _, vlanIdRange := range vlan.VlanId {
						strRange := fmt.Sprintf("Range %s - %s", strconv.Itoa(int(vlanIdRange.Start)), strconv.Itoa(int(vlanIdRange.End)))
						if !seenVlanIDs[strRange] {
							seenVlanIDs[strRange] = true
							vlans = append(vlans, strRange)
						}
					}
				default:
					logger.Println("Unknown VLAN Spec type")
				}
			}
		}
	}

	return seenVlanIDs
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

func createBasicInventoryObj(vms *[]vspheremodel.VM, collector *vsphere.Collector, hosts *[]vspheremodel.Host, clusters *[]vspheremodel.Cluster, vlanIDs []string) *apiplanner.Inventory {
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
			Vlans:           vlanIDs,
		},
	}
}
func createGoVmomiClient(creds VCenterCreds) *govmomi.Client {
	vc := flag.String("vc", creds.Url, "vCenter URL")
	username := flag.String("username", creds.Username, "vCenter username")
	password := flag.String("password", creds.Password, "vCenter password")
	flag.Parse()

	u, err := url.Parse(*vc)
	if err != nil {
		fmt.Printf("Failed to parse vCenter URL: %v\n", err)
		os.Exit(1)
	}

	u.User = url.UserPassword(*username, *password)
	// Create a new vSphere client
	ctx := context.Background()
	govmomiC, err := govmomi.NewClient(ctx, u, true)
	if err != nil {
		fmt.Printf("Failed to create vSphere client: %v\n", err)
		os.Exit(1)
	}

	return govmomiC
}
func getProvider(creds VCenterCreds) *api.Provider {
	vsphereType := api.VSphere
	return &api.Provider{
		Spec: api.ProviderSpec{
			URL:  creds.Url,
			Type: &vsphereType,
		},
	}
}

func getSecret(creds VCenterCreds) *core.Secret {
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

type InventoryData struct {
	Inventory apiplanner.Inventory `json:"inventory"`
	Error     string               `json:"error"`
}
