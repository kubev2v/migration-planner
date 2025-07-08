package types

import (
	"github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/store/model"
)

type ReportRenderer interface {
	Render(data *ReportData) (string, error)
	SupportedFormat() ReportFormat
}

type InventoryProcessor interface {
	ProcessInventory(source *model.Source) (*ReportData, error)
}

type ReportFormat string

const (
	ReportFormatCSV  ReportFormat = "csv"
	ReportFormatHTML ReportFormat = "html"
)

type ReportType string

const (
	ReportTypeSummary ReportType = "summary"
)

type ReportOptions struct {
	Format          ReportFormat
	Type            ReportType
	IncludeWarnings bool
}

type ReportData struct {
	Source     *model.Source
	Inventory  *v1alpha1.Inventory
	Metrics    *ProcessedMetrics
	Options    ReportOptions
	Timestamps ReportTimestamps
}

type ProcessedMetrics struct {
	Executive      ExecutiveMetrics
	Resources      ResourceMetrics
	OSDetails      []OSDetail
	Warnings       []WarningDetail
	Storage        []StorageDetail
	Network        []NetworkDetail
	Hosts          []HostDetail
	Infrastructure InfrastructureMetrics
}

type ExecutiveMetrics struct {
	TotalVMs        int
	PoweredOn       int
	PoweredOff      int
	MigratableVMs   int
	TotalHosts      int
	TotalClusters   int
	TotalDatastores int
	TotalNetworks   int
}

type ResourceMetrics struct {
	CPU    ResourceDetail
	Memory ResourceDetail
	Disk   ResourceDetail
}

type ResourceDetail struct {
	Total        int
	Average      float64
	Recommended  int
	Distribution []DistributionBucket
}

type DistributionBucket struct {
	Range string
	Count int
}

type OSDetail struct {
	Name       string
	Count      int
	Percentage float64
	Priority   string
}

type WarningDetail struct {
	Label       string
	Count       int
	Percentage  float64
	Impact      string
	Description string
}

type StorageDetail struct {
	Vendor      string
	Type        string
	Protocol    string
	TotalGB     int
	FreeGB      int
	Utilization float64
	HWAccel     bool
}

type NetworkDetail struct {
	Name     string
	Type     string
	VlanID   *string
	DVSwitch *string
}

type HostDetail struct {
	Vendor string
	Model  string
	Count  int
}

type InfrastructureMetrics struct {
	TotalDatacenters      int
	TotalClusters         int
	TotalHosts            int
	ClustersPerDatacenter []int
	HostsPerCluster       []int
	HostPowerStates       map[string]int
}

type ReportTimestamps struct {
	Generated     string
	GeneratedTime string
}

type ReportTemplateData struct {
	CSS                  string
	GeneratedDate        string
	GeneratedTime        string
	TotalVMs             int
	TotalHosts           int
	TotalDatastores      int
	TotalNetworks        int
	OSTable              string
	DiskSizeTable        string
	StorageTable         string
	WarningsTableSection string
	WarningsChartSection string
	CPUTotal             int
	CPUAverage           string
	CPURecommended       int
	MemoryTotal          int
	MemoryAverage        string
	MemoryRecommended    int
	StorageTotal         int
	StorageAverage       string
	StorageRecommended   int
	JavaScript           string
}

type EmptyReportTemplateData struct {
	CSS           string
	GeneratedDate string
	GeneratedTime string
	SourceName    string
	SourceID      string
	CreatedAt     string
	OnPremises    bool
}
