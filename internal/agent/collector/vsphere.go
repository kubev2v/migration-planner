package collector

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
	"strings"
	"sync"
	"time"

	api "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1"
	"github.com/kubev2v/forklift/pkg/controller/provider/container/vsphere"
	"github.com/kubev2v/forklift/pkg/controller/provider/model"
	vspheremodel "github.com/kubev2v/forklift/pkg/controller/provider/model/vsphere"
	webprovider "github.com/kubev2v/forklift/pkg/controller/provider/web"
	"github.com/kubev2v/forklift/pkg/controller/provider/web/base"
	web "github.com/kubev2v/forklift/pkg/controller/provider/web/vsphere"
	libcontainer "github.com/kubev2v/forklift/pkg/lib/inventory/container"
	libmodel "github.com/kubev2v/forklift/pkg/lib/inventory/model"
	libweb "github.com/kubev2v/forklift/pkg/lib/inventory/web"
	apiplanner "github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/agent/config"
	"github.com/kubev2v/migration-planner/internal/agent/service"
	"github.com/kubev2v/migration-planner/internal/util"
	"go.uber.org/zap"
	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Collector struct {
	dataDir        string
	credentialsDir string
	once           sync.Once
}

type VMValidation struct {
	Result []vspheremodel.Concern `json:"result"`
}

var vendorMap = map[string]string{
	"NETAPP":   "NetApp",
	"EMC":      "Dell EMC",
	"PURE":     "Pure Storage",
	"3PARDATA": "HPE", // 3PAR is an HPE product line
	"ATA":      "ATA",
	"DELL EMC": "Dell EMC",
	"DELL":     "Dell",
	"HPE":      "HPE",
	"IBM":      "IBM",
	"HITACHI":  "Vantara",
	"CISCO":    "Cisco",
	"FUJITSU":  "Fujitsu",
	"LENOVO":   "Lenovo",
}

func NewCollector(dataDir string, credentialsDir string) *Collector {
	return &Collector{
		dataDir:        dataDir,
		credentialsDir: credentialsDir,
	}
}

func (c *Collector) Collect(ctx context.Context) {
	c.once.Do(func() {
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				default:
					c.run()
					if c.getOpaServerStatus() {
						return
					}
					c.waitUntilOpaIsRunning()
				}
			}
		}()
	})
}

func (c *Collector) run() {
	credentialsFilePath := filepath.Join(c.credentialsDir, config.CredentialsFile)
	zap.S().Named("collector").Infof("Waiting for credentials")
	waitForFile(credentialsFilePath)

	credsData, err := os.ReadFile(credentialsFilePath)
	if err != nil {
		zap.S().Named("collector").Errorf("failed to read credentials file: %v", err)
		return
	}

	var creds config.Credentials
	if err := json.Unmarshal(credsData, &creds); err != nil {
		zap.S().Named("collector").Errorf("failed to parse credentials JSON: %v", err)
		return
	}
	zap.S().Named("collector").Infof("Create Provider")
	provider := getProvider(creds)

	zap.S().Named("collector").Infof("Create Secret")
	secret := getSecret(creds)
	zap.S().Named("collector").Infof("Create DB")
	db, err := createDB(provider)
	if err != nil {
		zap.S().Named("collector").Errorf("failed to create DB: %v", err)
		return
	}

	zap.S().Named("collector").Infof("vSphere collector")
	collector := vsphere.New(db, provider, secret)
	container, err := startWeb(collector)
	if err != nil {
		zap.S().Named("collector").Errorf("failed to create forklift API: %v", err)
		return
	}
	defer container.Delete(collector.Owner())
	defer collector.DB().Close(true)

	zap.S().Named("collector").Infof("List VMs")
	vms := &[]vspheremodel.VM{}
	err = collector.DB().List(vms, libmodel.FilterOptions{Detail: 1, Predicate: libmodel.Eq("IsTemplate", false)})
	if err != nil {
		zap.S().Named("collector").Errorf("failed to list database: %v", err)
		return
	}

	zap.S().Named("collector").Infof("List Hosts")
	hosts := &[]vspheremodel.Host{}
	err = collector.DB().List(hosts, libmodel.FilterOptions{Detail: 1})
	if err != nil {
		zap.S().Named("collector").Errorf("failed to list database: %v", err)
		return
	}

	zap.S().Named("collector").Infof("List Datacenters")
	datacenters := &[]vspheremodel.Datacenter{}
	if err := collector.DB().List(datacenters, libmodel.FilterOptions{Detail: 1}); err != nil {
		zap.S().Named("collector").Errorf("failed to list database: %v", err)
		return
	}

	zap.S().Named("collector").Infof("List Clusters")
	clusters := &[]vspheremodel.Cluster{}
	err = collector.DB().List(clusters, libmodel.FilterOptions{Detail: 1})
	if err != nil {
		zap.S().Named("collector").Errorf("failed to list database: %v", err)
		return
	}

	zap.S().Named("collector").Infof("Get About")
	about := &vspheremodel.About{}
	err = collector.DB().Get(about)
	if err != nil {
		zap.S().Named("collector").Errorf("failed to list database about table: %v", err)
		return
	}

	zap.S().Named("collector").Infof("Create inventory")

	inv := createBasicInventoryObj(about.InstanceUuid, vms, collector, hosts, clusters, datacenters)

	opaServerAlive := c.getOpaServerStatus()
	opaServer := util.GetEnv("OPA_SERVER", "127.0.0.1:8181")
	if opaServerAlive {
		zap.S().Named("collector").Infof("Run the validation of VMs")
		vms, err = Validation(vms, opaServer)
		if err != nil {
			zap.S().Named("collector").Errorf("failed to run validation: %v", err)
			return
		}
	}

	zap.S().Named("collector").Infof("Fill the inventory object with more data")
	FillInventoryObjectWithMoreData(vms, inv)

	zap.S().Named("collector").Infof("Write the inventory to output file")
	if err := createOuput(filepath.Join(c.dataDir, config.InventoryFile), inv); err != nil {
		zap.S().Named("collector").Errorf("Fill the inventory object with more data: %v", err)
		return
	}
}

func startWeb(collector *vsphere.Collector) (*libcontainer.Container, error) {
	container := libcontainer.New()
	if err := container.Add(collector); err != nil {
		return container, err
	}

	all := []libweb.RequestHandler{
		&libweb.SchemaHandler{},
		&webprovider.ProviderHandler{
			Handler: base.Handler{
				Container: container,
			},
		},
	}
	all = append(
		all,
		web.Handlers(container)...)

	web := libweb.New(container, all...)

	web.Start()

	const maxRetries = 300
	var i int
	for i = 0; i < maxRetries; i++ {
		time.Sleep(1 * time.Second)
		if collector.HasParity() {
			break
		}
	}
	if i == maxRetries {
		return container, fmt.Errorf("timed out waiting for collector parity")
	}

	return container, nil
}

func FillInventoryObjectWithMoreData(vms *[]vspheremodel.VM, inv *apiplanner.Inventory) {
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

		guestName := vmGuestName(vm)

		osInfoMap := *inv.Vms.OsInfo
		osInfo, found := osInfoMap[guestName]
		if !found {
			osInfo.Supported = isOsSupported(vm.Concerns)
		}
		osInfo.Count++
		osInfoMap[guestName] = osInfo

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
	inv.Vms.CpuCores.Histogram = Histogram(cpuSet)
	inv.Vms.RamGB.Histogram = Histogram(memorySet)
	inv.Vms.DiskCount.Histogram = Histogram(diskCountSet)
	inv.Vms.DiskGB.Histogram = Histogram(diskGBSet)
}

func isOsSupported(concerns []vspheremodel.Concern) bool {
	for _, concern := range concerns {
		if concern.Id == "vmware.os.unsupported" {
			return false
		}
	}
	return true
}

func vmGuestName(vm vspheremodel.VM) string {
	if vm.GuestNameFromVmwareTools != "" {
		return vm.GuestNameFromVmwareTools
	}
	return vm.GuestName
}

func createBasicInventoryObj(vCenterID string, vms *[]vspheremodel.VM, collector *vsphere.Collector, hosts *[]vspheremodel.Host, clusters *[]vspheremodel.Cluster, datacenters *[]vspheremodel.Datacenter) *apiplanner.Inventory {
	return &apiplanner.Inventory{
		Vcenter: apiplanner.VCenter{
			Id: vCenterID,
		},
		Vms: apiplanner.VMs{
			Total:                len(*vms),
			PowerStates:          map[string]int{},
			Os:                   map[string]int{},
			OsInfo:               &map[string]apiplanner.OsInfo{},
			MigrationWarnings:    apiplanner.MigrationIssues{},
			NotMigratableReasons: apiplanner.MigrationIssues{},
		},
		Infra: apiplanner.Infra{
			ClustersPerDatacenter: clustersPerDatacenter(datacenters, collector),
			Datastores:            getDatastores(hosts, collector),
			HostPowerStates:       getHostPowerStates(*hosts),
			Hosts:                 getHosts(hosts),
			TotalHosts:            len(*hosts),
			TotalClusters:         len(*clusters),
			TotalDatacenters:      util.IntPtr(len(*datacenters)),
			HostsPerCluster:       getHostsPerCluster(*clusters),
			Networks:              getNetworks(collector),
		},
	}
}

func clustersPerDatacenter(datacenters *[]vspheremodel.Datacenter, collector *vsphere.Collector) *[]int {
	var h []int

	folders := &[]vspheremodel.Folder{}
	if err := collector.DB().List(folders, libmodel.FilterOptions{Detail: 1}); err != nil {
		return nil
	}

	folderByID := make(map[string]vspheremodel.Folder, len(*folders))
	for _, f := range *folders {
		folderByID[f.ID] = f
	}

	for _, dc := range *datacenters {
		hostFolderId := dc.Clusters.ID
		for _, folder := range *folders {
			if folder.ID == hostFolderId {
				h = append(h, countClustersRecursively(folder, folderByID))
				break
			}
		}
	}

	return &h
}

func countClustersRecursively(folder vspheremodel.Folder, folderByID map[string]vspheremodel.Folder) int {
	count := 0

	for _, child := range folder.Children {

		if child.Kind == vspheremodel.ClusterKind && strings.HasPrefix(child.ID, "domain-c") {
			count++
		}

		if child.Kind == vspheremodel.FolderKind {
			count += countClustersRecursively(folderByID[child.ID], folderByID) // recursive count
		}
	}

	return count
}

func getProvider(creds config.Credentials) *api.Provider {
	vsphereType := api.VSphere
	return &api.Provider{
		ObjectMeta: meta.ObjectMeta{
			UID: "1",
		},
		Spec: api.ProviderSpec{
			URL:  creds.URL,
			Type: &vsphereType,
		},
	}
}

func getSecret(creds config.Credentials) *core.Secret {
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

func Histogram(d []int) apiplanner.Histogram {
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
		} else if binIndex < 0 {
			binIndex = 0
		}
		bins[binIndex]++
	}

	return apiplanner.Histogram{
		Data:     bins,
		Step:     int(math.Round(binSize)),
		MinValue: minVal,
	}
}

func getNetworks(collector *vsphere.Collector) []apiplanner.Network {
	r := []apiplanner.Network{}
	networks := &[]vspheremodel.Network{}
	err := collector.DB().List(networks, libmodel.FilterOptions{Detail: 1})
	if err != nil {
		return nil
	}

	for _, n := range *networks {
		vlanId := n.VlanId
		dvNet := &vspheremodel.Network{}
		if n.Variant == vspheremodel.NetDvPortGroup {
			dvNet.WithRef(n.DVSwitch)
			_ = collector.DB().Get(dvNet)
		}
		r = append(r, apiplanner.Network{
			Name:     n.Name,
			Type:     apiplanner.NetworkType(getNetworkType(&n)),
			VlanId:   &vlanId,
			Dvswitch: &dvNet.Name,
		})
	}

	return r
}

func getNetworkType(n *vspheremodel.Network) string {
	if n.Variant == vspheremodel.NetDvPortGroup {
		return string(apiplanner.Distributed)
	} else if n.Variant == vspheremodel.NetStandard {
		return string(apiplanner.Standard)
	} else if n.Variant == vspheremodel.NetDvSwitch {
		return string(apiplanner.Dvswitch)
	}

	return string(apiplanner.Unsupported)
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

func findDataStoreInfo(hosts []vspheremodel.Host, names []string) (vendor, model, protocol string) {
	vendor, model, protocol = "N/A", "N/A", "N/A"
	if len(names) == 0 {
		return
	}

	for _, host := range hosts {
		for _, disk := range host.HostScsiDisks {
			if disk.CanonicalName != names[0] {
				continue
			}

			vendor = disk.Vendor

			for _, topology := range host.HostScsiTopology {
				if !util.Contains(topology.ScsiDiskKeys, disk.Key) {
					continue
				}

				hbaKey := topology.HbaKey
				for _, hba := range host.HbaDiskInfo {
					if hba.Key == hbaKey {
						model = hba.Model
						protocol = hba.Protocol
						return
					}
				}
			}
		}
	}
	return
}

func getNaa(ds *vspheremodel.Datastore) string {
	if len(ds.BackingDevicesNames) > 0 {
		return ds.BackingDevicesNames[0]
	}

	return "N/A"
}

func isHardwareAcceleratedMove(hosts []vspheremodel.Host, names []string) bool {
	supported := false
	if len(names) == 0 {
		return supported
	}

	for _, host := range hosts {
		for _, disk := range host.HostScsiDisks {
			if disk.CanonicalName != names[0] {
				continue
			}
		}

		resp, err := http.Get("http://localhost:8080/providers/vsphere/1/hosts/" + host.ID + "?advancedOption=DataMover.HardwareAcceleratedMove")
		if err != nil {
			return supported
		}
		defer resp.Body.Close()

		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return supported
		}

		var hostData web.Host
		err = json.Unmarshal(bodyBytes, &hostData)
		if err != nil {
			return supported
		}

		for _, option := range hostData.AdvancedOptions {
			if option.Key == "DataMover.HardwareAcceleratedMove" {
				supported = option.Value == "1"
				return supported
			}
		}
	}

	return supported
}

func transformVendorName(vendor string) string {
	vendor = strings.ToUpper(strings.TrimSpace(vendor))

	if transformed, exists := vendorMap[vendor]; exists {
		return transformed
	}

	return strings.TrimSpace(vendor)
}

func getDatastores(hosts *[]vspheremodel.Host, collector *vsphere.Collector) []apiplanner.Datastore {
	datastores := &[]vspheremodel.Datastore{}
	err := collector.DB().List(datastores, libmodel.FilterOptions{Detail: 1})
	if err != nil {
		return nil
	}
	res := []apiplanner.Datastore{}
	for _, ds := range *datastores {
		dsVendor, dsModel, dsProtocol := findDataStoreInfo(*hosts, ds.BackingDevicesNames)
		res = append(res, apiplanner.Datastore{
			TotalCapacityGB:         int(ds.Capacity / 1024 / 1024 / 1024),
			FreeCapacityGB:          int(ds.Free / 1024 / 1024 / 1024),
			HardwareAcceleratedMove: isHardwareAcceleratedMove(*hosts, ds.BackingDevicesNames),
			Type:                    ds.Type,
			Vendor:                  transformVendorName(dsVendor),
			Model:                   dsModel,
			ProtocolType:            dsProtocol,
			DiskId:                  getNaa(&ds),
		})
	}

	return res
}

func getHosts(hosts *[]vspheremodel.Host) *[]apiplanner.Host {
	var l []apiplanner.Host

	for _, host := range *hosts {
		l = append(l, apiplanner.Host{
			Model:  host.Model,
			Vendor: host.Vendor,
		})
	}

	return &l
}

func totalCapacity(disks []vspheremodel.Disk) int {
	total := 0
	for _, d := range disks {
		total += int(d.Capacity)
	}
	return total / 1024 / 1024 / 1024
}

func hasID(reasons apiplanner.MigrationIssues, id string) int {
	for i, reason := range reasons {
		if id == *reason.Id {
			return i
		}
	}
	return -1
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
	jsonData, err := json.Marshal(&service.InventoryData{Inventory: *inv})
	if err != nil {
		return err
	}
	_, err = file.Write(jsonData)
	if err != nil {
		return err
	}

	return nil
}

func Validation(vms *[]vspheremodel.VM, opaServer string) (*[]vspheremodel.VM, error) {
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
			vm.Concerns = append(vm.Concerns, vspheremodel.Concern{Id: c.Id, Label: c.Label, Assessment: c.Assessment, Category: c.Category})
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
			if i := hasID(inv.Vms.NotMigratableReasons, result.Id); i >= 0 {
				inv.Vms.NotMigratableReasons[i].Count++
			} else {
				inv.Vms.NotMigratableReasons = append(inv.Vms.NotMigratableReasons, apiplanner.MigrationIssue{
					Id:         &result.Id,
					Label:      result.Label,
					Count:      1,
					Assessment: result.Assessment,
				})
			}
		}
		if result.Category == "Warning" {
			hasWarning = true
			if i := hasID(inv.Vms.MigrationWarnings, result.Id); i >= 0 {
				inv.Vms.MigrationWarnings[i].Count++
			} else {
				inv.Vms.MigrationWarnings = append(inv.Vms.MigrationWarnings, apiplanner.MigrationIssue{
					Id:         &result.Id,
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

func (c *Collector) getOpaServerStatus() bool {
	opaServer := util.GetEnv("OPA_SERVER", "127.0.0.1:8181")
	zap.S().Named("collector").Infof("Check if opaServer is responding")
	resp, err := http.Get("http://" + opaServer + "/health")
	if err != nil || resp.StatusCode != http.StatusOK {
		zap.S().Named("collector").Errorf("OPA server %s is not responding", opaServer)
		return false
	}
	defer resp.Body.Close()
	zap.S().Named("collector").Infof("OPA server %s is alive", opaServer)
	return true
}

func (c *Collector) waitUntilOpaIsRunning() {
	for {
		if c.getOpaServerStatus() {
			break
		}
		time.Sleep(2 * time.Second)
	}
}
