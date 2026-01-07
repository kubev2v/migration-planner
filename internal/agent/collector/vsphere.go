package collector

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/kubev2v/migration-planner/pkg/opa"

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
	dataDir           string
	persistentDataDir string
	opaPoliciesDir    string
	once              sync.Once
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

func NewCollector(dataDir, persistentDataDir, opaPoliciesDir string) *Collector {
	return &Collector{
		dataDir:           dataDir,
		persistentDataDir: persistentDataDir,
		opaPoliciesDir:    opaPoliciesDir,
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
					c.run(ctx)
					return
				}
			}
		}()
	})
}

func (c *Collector) run(ctx context.Context) {
	credentialsFilePath := filepath.Join(c.persistentDataDir, config.CredentialsFile)
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

	zap.S().Named("collector").Infof("List Datastores")
	datastores, err := listDatastoresFromCollector(collector)
	if err != nil {
		zap.S().Named("collector").Errorf("failed to list datastores: %v", err)
		return
	}

	zap.S().Named("collector").Infof("List Networks")
	networks, err := listNetworksFromCollector(collector)
	if err != nil {
		zap.S().Named("collector").Errorf("failed to list networks: %v", err)
		return
	}

	zap.S().Named("collector").Infof("Create vCenter-level inventory")

	apiDatastores, datastoreIndexToName, datastoreMapping := getDatastores(hosts, datastores)
	apiNetworks, networkMapping := getNetworks(networks, collector, CountVmsByNetwork(*vms))

	infraData := service.InfrastructureData{
		Datastores:            apiDatastores,
		Networks:              apiNetworks,
		HostPowerStates:       getHostPowerStates(*hosts),
		Hosts:                 getHosts(hosts),
		ClustersPerDatacenter: *clustersPerDatacenter(datacenters, collector),
		TotalHosts:            len(*hosts),
		TotalDatacenters:      len(*datacenters),
	}
	vcenterInv := service.CreateBasicInventory(vms, infraData)

	zap.S().Named("collector").Infof("initialize OPA validator from %s.", c.opaPoliciesDir)
	opaValidator, err := opa.NewValidatorFromDir(c.opaPoliciesDir)
	if err != nil {
		zap.S().Named("collector").Errorf("failed to initialize OPA validator from %s: %v", c.opaPoliciesDir, err)
	}

	zap.S().Named("collector").Infof("Run the validation of VMs for vCenter-level")
	if err := ValidateVMs(ctx, opaValidator, *vms); err != nil {
		zap.S().Named("collector").Warnf("At least one error during VMs validation: %v", err)
	}

	zap.S().Named("collector").Infof("Fill the vCenter-level inventory object with more data")
	datastoreIDToType := buildDatastoreIDToTypeMap(datastores)
	FillInventoryObjectWithMoreData(vms, vcenterInv, datastoreIDToType)

	zap.S().Named("collector").Infof("Extract cluster mapping and build helper maps")
	clusterMapping, hostIDToPowerState, vmsByCluster := ExtractVSphereClusterIDMapping(*vms, *hosts, *clusters)

	zap.S().Named("collector").Infof("Create per-cluster inventories")
	perClusterInventories := make(map[string]*apiplanner.InventoryData)
	for _, clusterID := range clusterMapping.ClusterIDs {
		zap.S().Named("collector").Debugf("Processing cluster: %s", clusterID)

		clusterVMs := vmsByCluster[clusterID]

		// Filter infrastructure data for this cluster
		clusterInfraData := service.FilterInfraDataByClusterID(
			infraData,
			clusterID,
			clusterMapping.HostToClusterID,
			clusterVMs,
			datastoreMapping,
			datastoreIndexToName,
			networkMapping,
			hostIDToPowerState,
		)

		// Create cluster inventory
		clusterInv := service.CreateBasicInventory(&clusterVMs, clusterInfraData)

		if len(clusterVMs) > 0 {
			FillInventoryObjectWithMoreData(&clusterVMs, clusterInv, datastoreIDToType) // Fill cluster inventory with more data
		}

		perClusterInventories[clusterID] = clusterInv
	}

	// Create the full V2 Inventory structure
	zap.S().Named("collector").Infof("Create clustered inventory response")
	inventoryV2 := &apiplanner.Inventory{
		VcenterId: about.InstanceUuid,
		Clusters:  make(map[string]apiplanner.InventoryData),
		Vcenter:   vcenterInv,
	}

	// Convert per-cluster inventories to the map format
	for clusterID, clusterInv := range perClusterInventories {
		inventoryV2.Clusters[clusterID] = *clusterInv
	}

	// Write the V2 inventory to output file
	zap.S().Named("collector").Infof("Write the inventory to output file")
	if err := createOuput(filepath.Join(c.dataDir, config.InventoryFile), inventoryV2); err != nil {
		zap.S().Named("collector").Errorf("Failed to write inventory: %v", err)
		return
	}

	zap.S().Named("collector").Infof("Successfully created inventory with %d clusters", len(perClusterInventories))
}

func ValidateVMs(ctx context.Context, opaValidator *opa.Validator, vms []vspheremodel.VM) error {
	if opaValidator == nil {
		return fmt.Errorf("received opa validator is nil")
	}

	var validationErrors []error

	for i := range vms {
		concerns, err := opaValidator.ValidateVM(ctx, vms[i])
		if err != nil {
			validationErrors = append(validationErrors, err)
		}
		vms[i].Concerns = concerns
	}

	if len(validationErrors) > 0 {
		return fmt.Errorf("validation completed with %d error(s): %w", len(validationErrors), errors.Join(validationErrors...))
	}

	return nil
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

func buildDatastoreIDToTypeMap(datastores *[]vspheremodel.Datastore) map[string]string {
	datastoreIDToType := make(map[string]string)

	for _, ds := range *datastores {
		datastoreIDToType[ds.ID] = ds.Type
	}

	return datastoreIDToType
}

func FillInventoryObjectWithMoreData(vms *[]vspheremodel.VM, inv *apiplanner.InventoryData, datastoreIDToType map[string]string) {
	diskGBSet := []int{}

	totalAllocatedVCpus := 0  // For poweredOn VMs
	totalAllocatedMemory := 0 // For poweredOn VMs

	for _, vm := range *vms {
		diskGBSet = append(diskGBSet, totalCapacity(vm.Disks))

		// Update the VM count per CPU, Memory & NIC tier
		(*inv.Vms.DistributionByCpuTier)[cpuTierKey(int(vm.CpuCount))]++
		(*inv.Vms.DistributionByMemoryTier)[memoryTierKey(util.MBToGB(vm.MemoryMB))]++
		(*inv.Vms.DistributionByNicCount)[nicCountKey(len(vm.NICs))]++

		// allocated VCpu for powered On VMs
		if vm.PowerState == "poweredOn" {
			totalAllocatedVCpus += int(vm.CpuCount)
			totalAllocatedMemory += int(vm.MemoryMB)
		}

		// inventory
		migratable, hasWarning := migrationReport(vm.Concerns, inv)

		inv.Vms.OsInfo = updateOsInfo(&vm, *inv.Vms.OsInfo)

		inv.Vms.PowerStates[vm.PowerState]++

		// Update total values
		inv.Vms.CpuCores.Total += int(vm.CpuCount)
		inv.Vms.RamGB.Total += util.MBToGB(vm.MemoryMB)
		inv.Vms.DiskCount.Total += len(vm.Disks)
		inv.Vms.DiskGB.Total += totalCapacity(vm.Disks)

		// Not Migratable
		if !migratable {
			inv.Vms.CpuCores.TotalForNotMigratable += int(vm.CpuCount)
			inv.Vms.RamGB.TotalForNotMigratable += util.MBToGB(vm.MemoryMB)
			inv.Vms.DiskCount.TotalForNotMigratable += len(vm.Disks)
			inv.Vms.DiskGB.TotalForNotMigratable += totalCapacity(vm.Disks)
		} else {
			// Migratable with warning(s)
			if hasWarning {
				inv.Vms.CpuCores.TotalForMigratableWithWarnings += int(vm.CpuCount)
				inv.Vms.RamGB.TotalForMigratableWithWarnings += util.MBToGB(vm.MemoryMB)
				inv.Vms.DiskCount.TotalForMigratableWithWarnings += len(vm.Disks)
				inv.Vms.DiskGB.TotalForMigratableWithWarnings += totalCapacity(vm.Disks) // Migratable
			} else {
				// Migratable without any warnings
				inv.Vms.CpuCores.TotalForMigratable += int(vm.CpuCount)
				inv.Vms.RamGB.TotalForMigratable += util.MBToGB(vm.MemoryMB)
				inv.Vms.DiskCount.TotalForMigratable += len(vm.Disks)
				inv.Vms.DiskGB.TotalForMigratable += totalCapacity(vm.Disks)
			}
		}

		inv.Vms.DiskTypes = updateDiskTypeSummary(&vm, *inv.Vms.DiskTypes, datastoreIDToType)
	}

	// Update the disk size tier
	inv.Vms.DiskSizeTier = diskSizeTier(diskGBSet)

	// calculate the cpu and memory overcommitment ratio
	inv.Infra.CpuOverCommitment = util.FloatPtr(calcOverCommitmentRatio(totalAllocatedVCpus, sumHostsCpu(inv.Infra.Hosts)))
	inv.Infra.MemoryOverCommitment = util.FloatPtr(calcOverCommitmentRatio(totalAllocatedMemory, sumHostsMemory(inv.Infra.Hosts)))
}

func calcOverCommitmentRatio(totalAllocated, totalAvailable int) float64 {
	if totalAvailable == 0 {
		return 0.0
	}
	return util.Round(float64(totalAllocated) / float64(totalAvailable))
}

func sumHostsCpu(hosts *[]apiplanner.Host) int {
	total := 0
	for _, h := range *hosts {
		if h.CpuCores == nil {
			continue
		}
		total += *h.CpuCores
	}

	return total
}

func sumHostsMemory(hosts *[]apiplanner.Host) int {
	total := 0
	for _, h := range *hosts {
		if h.MemoryMB == nil {
			continue
		}
		total += int(*h.MemoryMB)
	}

	return total
}

// updateDiskTypeSummary calculates and updates a summary of disk usage by type for a given VM.
// It takes:
//   - vm: the VM whose disks are being analyzed,
//   - summary: a map of existing DiskTypeSummary values keyed by disk type,
//   - datastoreIDToType: a mapping from datastore IDs to their corresponding disk type names.
//
// For each disk in the VM, it:
//   - identifies the disk type,
//   - increments the VM count for that type (counting each VM only once per type),
//   - adds the disk's size to the total size for that type (in TB),
//   - rounds the total size to two decimal places.
//
// Returns a pointer to the updated map of DiskTypeSummary.
func updateDiskTypeSummary(vm *vspheremodel.VM, summary map[string]apiplanner.DiskTypeSummary, datastoreIDToType map[string]string) *map[string]apiplanner.DiskTypeSummary {
	seenTypes := make(map[string]bool)

	for _, disk := range vm.Disks {
		diskTypeName := datastoreIDToType[disk.Datastore.ID]

		if diskTypeName == "" {
			continue
		}

		diskTypeSummary := summary[diskTypeName]
		if !seenTypes[diskTypeName] {
			diskTypeSummary.VmCount++
			seenTypes[diskTypeName] = true
		}
		diskTypeSummary.TotalSizeTB += util.BytesToTB(disk.Capacity)
		summary[diskTypeName] = diskTypeSummary
	}

	for k, v := range summary {
		v.TotalSizeTB = util.Round(v.TotalSizeTB)
		summary[k] = v
	}

	return &summary
}

func diskSizeTier(diskGBSet []int) *map[string]apiplanner.DiskSizeTierSummary {
	result := make(map[string]apiplanner.DiskSizeTierSummary)

	for _, diskGB := range diskGBSet {
		diskTB := util.GBToTB(diskGB)
		var tierKey string

		switch {
		case diskTB < 10:
			tierKey = "Easy (0-10TB)"
		case diskTB < 20:
			tierKey = "Medium (10-20TB)"
		case diskTB < 50:
			tierKey = "Hard (20-50TB)"
		default:
			tierKey = "White Glove (>50TB)"
		}

		tier := result[tierKey]
		tier.TotalSizeTB += diskTB
		tier.VmCount++
		result[tierKey] = tier
	}

	for k, v := range result {
		v.TotalSizeTB = util.Round(v.TotalSizeTB)
		result[k] = v
	}

	return &result
}

func memoryTierKey(i int) string {
	var tierKey string

	switch {
	case i <= 4:
		tierKey = "0-4"
	case i <= 16:
		tierKey = "5-16"
	case i <= 32:
		tierKey = "17-32"
	case i <= 64:
		tierKey = "33-64"
	case i <= 128:
		tierKey = "65-128"
	case i <= 256:
		tierKey = "129-256"
	default:
		tierKey = "256+"
	}

	return tierKey
}

func cpuTierKey(i int) string {
	var tierKey string

	switch {
	case i <= 4:
		tierKey = "0-4"
	case i <= 8:
		tierKey = "5-8"
	case i <= 16:
		tierKey = "9-16"
	case i <= 32:
		tierKey = "17-32"
	default:
		tierKey = "32+"
	}

	return tierKey
}

func nicCountKey(i int) string {
	switch {
	case i <= 0:
		return "0"
	case i == 1:
		return "1"
	case i == 2:
		return "2"
	case i == 3:
		return "3"
	default:
		return "4+"
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

func CountVmsByNetwork(vms []vspheremodel.VM) map[string]int {
	vmsPerNetwork := make(map[string]int)
	for _, vm := range vms {
		for _, network := range vm.Networks {
			vmsPerNetwork[network.ID]++
		}
	}
	return vmsPerNetwork
}

func listNetworksFromCollector(collector *vsphere.Collector) (*[]vspheremodel.Network, error) {
	networks := &[]vspheremodel.Network{}
	err := collector.DB().List(networks, libmodel.FilterOptions{Detail: 1})
	if err != nil {
		return nil, err
	}
	return networks, nil
}

func getNetworks(networks *[]vspheremodel.Network, collector *vsphere.Collector, vmsPerNetwork map[string]int) (
	apiNetworks []apiplanner.Network,
	networkMapping map[string]string,
) {
	apiNetworks = []apiplanner.Network{}
	networkMapping = make(map[string]string, len(*networks))

	// Single iteration to build API networks and mapping
	for _, n := range *networks {
		// Build mapping
		if n.ID != "" && n.Name != "" {
			networkMapping[n.ID] = n.Name
		}

		// Build API network
		vlanId := n.VlanId
		dvNet := &vspheremodel.Network{}
		if n.Variant == vspheremodel.NetDvPortGroup {
			dvNet.WithRef(n.DVSwitch)
			_ = collector.DB().Get(dvNet)
		}
		apiNetworks = append(apiNetworks, apiplanner.Network{
			Name:     n.Name,
			Type:     apiplanner.NetworkType(getNetworkType(&n)),
			VlanId:   &vlanId,
			Dvswitch: &dvNet.Name,
			VmsCount: util.IntPtr(vmsPerNetwork[n.ID]),
		})
	}

	return apiNetworks, networkMapping
}

func getNetworkType(n *vspheremodel.Network) string {
	switch n.Variant {
	case vspheremodel.NetDvPortGroup:
		return string(apiplanner.Distributed)
	case vspheremodel.NetStandard:
		return string(apiplanner.Standard)
	case vspheremodel.NetDvSwitch:
		return string(apiplanner.Dvswitch)
	default:
		return string(apiplanner.Unsupported)
	}
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

func TransformVendorName(vendor string) string {
	raw := strings.TrimSpace(vendor) // Preserve original case
	key := strings.ToUpper(raw)      // Use uppercase for lookup only

	if transformed, exists := vendorMap[key]; exists {
		return transformed
	}

	return raw // Return original case for unmapped
}

func listDatastoresFromCollector(collector *vsphere.Collector) (*[]vspheremodel.Datastore, error) {
	datastores := &[]vspheremodel.Datastore{}
	err := collector.DB().List(datastores, libmodel.FilterOptions{Detail: 1})
	if err != nil {
		return nil, err
	}
	return datastores, nil
}

func getDatastores(hosts *[]vspheremodel.Host, datastores *[]vspheremodel.Datastore) (
	apiDatastores []apiplanner.Datastore,
	indexToName map[int]string,
	nameToID map[string]string,
) {
	// Create datastore-to-hostIDs mapping once for better performance
	datastoreToHostIDs := make(map[string][]string)
	for _, host := range *hosts {
		for _, dsRef := range host.Datastores {
			datastoreToHostIDs[dsRef.ID] = append(datastoreToHostIDs[dsRef.ID], host.ID)
		}
	}

	// Initialize return values
	apiDatastores = []apiplanner.Datastore{}
	indexToName = make(map[int]string, len(*datastores))
	nameToID = make(map[string]string, len(*datastores))

	// Single iteration to build API datastores and mappings
	for i, ds := range *datastores {
		// Build mappings
		indexToName[i] = ds.Name
		if ds.Name != "" && ds.ID != "" {
			nameToID[ds.Name] = ds.ID
		}

		// Build API datastore
		dsVendor, dsModel, dsProtocol := findDataStoreInfo(*hosts, ds.BackingDevicesNames)
		hostIDs := datastoreToHostIDs[ds.ID]
		var hostIDPtr *string
		if len(hostIDs) > 0 {
			joined := strings.Join(hostIDs, ", ")
			hostIDPtr = &joined
		}

		apiDatastores = append(apiDatastores, apiplanner.Datastore{
			TotalCapacityGB:         util.BytesToGB(ds.Capacity),
			FreeCapacityGB:          util.BytesToGB(ds.Free),
			HardwareAcceleratedMove: isHardwareAcceleratedMove(*hosts, ds.BackingDevicesNames),
			Type:                    ds.Type,
			Vendor:                  TransformVendorName(dsVendor),
			Model:                   dsModel,
			ProtocolType:            dsProtocol,
			DiskId:                  getNaa(&ds),
			HostId:                  hostIDPtr,
		})
	}

	return apiDatastores, indexToName, nameToID
}

func getHosts(hosts *[]vspheremodel.Host) *[]apiplanner.Host {
	var l []apiplanner.Host

	for _, host := range *hosts {
		cpuCores := int(host.CpuCores)
		cpuSockets := int(host.CpuSockets)
		var memoryMB *int64
		if host.MemoryBytes > 0 {
			mb := util.ConvertBytesToMB(host.MemoryBytes)
			memoryMB = &mb
		}

		l = append(l, apiplanner.Host{
			Id:         &host.ID,
			Model:      host.Model,
			Vendor:     host.Vendor,
			CpuCores:   &cpuCores,
			CpuSockets: &cpuSockets,
			MemoryMB:   memoryMB,
		})
	}

	return &l
}

func totalCapacity(disks []vspheremodel.Disk) int {
	total := 0
	for _, d := range disks {
		total += int(d.Capacity)
	}
	return util.BytesToGB(total)
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

	// Write the inventory to the file  // Note: service.InventoryData wraps the Inventory with error handling
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

func migrationReport(concern []vspheremodel.Concern, inv *apiplanner.InventoryData) (bool, bool) {
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
