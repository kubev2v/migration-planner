package duckdb_parser

import (
	"bytes"
	"embed"
	"fmt"
	"strings"
	"text/template"
)

//go:embed templates/*.tmpl
var templateFS embed.FS

// getTemplate reads a template from the embedded filesystem.
func getTemplate(name string) (string, error) {
	content, err := templateFS.ReadFile("templates/" + name + ".go.tmpl")
	if err != nil {
		return "", fmt.Errorf("reading template %s: %w", name, err)
	}
	return string(content), nil
}

// mustGetTemplate reads a template and panics on error (for init-time loading).
func mustGetTemplate(name string) string {
	content, err := getTemplate(name)
	if err != nil {
		panic(err)
	}
	return content
}

// QueryBuilder builds SQL queries from templates.
type QueryBuilder struct{}

// NewBuilder creates a new Builder.
func NewBuilder() *QueryBuilder {
	return &QueryBuilder{}
}

type ingestParams struct {
	FilePath string
}

// CreateSchemaQuery returns queries to create all RVTools tables with proper schema.
func (b *QueryBuilder) CreateSchemaQuery() (string, error) {
	return b.buildQuery("create_schema", mustGetTemplate("create_schema"), nil)
}

// IngestRvtoolsQuery returns a query that inserts data from an RVTools Excel file into schema tables.
func (b *QueryBuilder) IngestRvtoolsQuery(filePath string) (string, error) {
	return b.buildQuery("ingest_rvtools", mustGetTemplate("ingest_rvtools"), ingestParams{FilePath: filePath})
}

// IngestSqliteQuery returns a query that creates RVTools-shaped tables from a forklift SQLite database.
func (b *QueryBuilder) IngestSqliteQuery(filePath string) (string, error) {
	return b.buildQuery("ingest_sqlite", mustGetTemplate("ingest_sqlite"), ingestParams{FilePath: filePath})
}

// queryParams holds all template parameters for queries.
type queryParams struct {
	NetworkColumns   string
	ClusterFilter    string
	OSFilter         string
	PowerStateFilter string
	VmIDFilter       string
	Category         string
	Limit            int
	Offset           int
}

// VMQuery builds the VM query with filters and pagination.
func (b *QueryBuilder) VMQuery(filters Filters, options Options) (string, error) {
	const maxNetworkNumbers = 25
	quoted := make([]string, 0, maxNetworkNumbers)
	for i := 1; i <= maxNetworkNumbers; i++ {
		quoted = append(quoted, fmt.Sprintf(`i."Network #%d"`, i))
	}
	networkColumns := strings.Join(quoted, ", ")

	params := queryParams{
		NetworkColumns:   networkColumns,
		ClusterFilter:    filters.Cluster,
		OSFilter:         filters.OS,
		PowerStateFilter: filters.PowerState,
		VmIDFilter:       filters.VmId,
		Limit:            options.Limit,
		Offset:           options.Offset,
	}
	return b.buildQuery("vm_query", mustGetTemplate("vm_query"), params)
}

// DatastoreQuery builds the datastore query with filters and pagination.
func (b *QueryBuilder) DatastoreQuery(filters Filters, options Options) (string, error) {
	params := queryParams{
		ClusterFilter: filters.Cluster,
		Limit:         options.Limit,
		Offset:        options.Offset,
	}
	return b.buildQuery("datastore_query", mustGetTemplate("datastore_query"), params)
}

// NetworkQuery builds the network query with filters and pagination.
func (b *QueryBuilder) NetworkQuery(filters Filters, options Options) (string, error) {
	params := queryParams{
		ClusterFilter: filters.Cluster,
		Limit:         options.Limit,
		Offset:        options.Offset,
	}
	return b.buildQuery("network_query", mustGetTemplate("network_query"), params)
}

// HostQuery builds the host query with filters and pagination.
func (b *QueryBuilder) HostQuery(filters Filters, options Options) (string, error) {
	params := queryParams{
		ClusterFilter: filters.Cluster,
		Limit:         options.Limit,
		Offset:        options.Offset,
	}
	return b.buildQuery("host_query", mustGetTemplate("host_query"), params)
}

// OsQuery builds the OS summary query with filters.
func (b *QueryBuilder) OsQuery(filters Filters) (string, error) {
	params := queryParams{
		ClusterFilter: filters.Cluster,
	}
	return b.buildQuery("os_query", mustGetTemplate("os_query"), params)
}

// ClustersQuery builds the clusters query.
func (b *QueryBuilder) ClustersQuery() (string, error) {
	return b.buildQuery("clusters_query", mustGetTemplate("clusters_query"), nil)
}

// VMCountQuery builds the VM count query with filters.
func (b *QueryBuilder) VMCountQuery(filters Filters) (string, error) {
	params := queryParams{
		ClusterFilter:    filters.Cluster,
		PowerStateFilter: filters.PowerState,
	}
	return b.buildQuery("vm_count_query", mustGetTemplate("vm_count_query"), params)
}

// PowerStateCountsQuery builds the power state counts query.
func (b *QueryBuilder) PowerStateCountsQuery(filters Filters) (string, error) {
	params := queryParams{
		ClusterFilter: filters.Cluster,
	}
	return b.buildQuery("power_state_counts_query", mustGetTemplate("power_state_counts_query"), params)
}

// HostPowerStateCountsQuery builds the host power state counts query.
func (b *QueryBuilder) HostPowerStateCountsQuery(filters Filters) (string, error) {
	params := queryParams{
		ClusterFilter: filters.Cluster,
	}
	return b.buildQuery("host_power_state_counts_query", mustGetTemplate("host_power_state_counts_query"), params)
}

// CPUTierQuery builds the CPU tier distribution query.
func (b *QueryBuilder) CPUTierQuery(filters Filters) (string, error) {
	params := queryParams{
		ClusterFilter: filters.Cluster,
	}
	return b.buildQuery("cpu_tier_query", mustGetTemplate("cpu_tier_query"), params)
}

// MemoryTierQuery builds the memory tier distribution query.
func (b *QueryBuilder) MemoryTierQuery(filters Filters) (string, error) {
	params := queryParams{
		ClusterFilter: filters.Cluster,
	}
	return b.buildQuery("memory_tier_query", mustGetTemplate("memory_tier_query"), params)
}

// DiskSizeTierQuery builds the disk size tier distribution query.
func (b *QueryBuilder) DiskSizeTierQuery(filters Filters) (string, error) {
	params := queryParams{
		ClusterFilter: filters.Cluster,
	}
	return b.buildQuery("disk_size_tier_query", mustGetTemplate("disk_size_tier_query"), params)
}

// DiskTypeSummaryQuery builds the disk type summary query.
func (b *QueryBuilder) DiskTypeSummaryQuery(filters Filters) (string, error) {
	params := queryParams{
		ClusterFilter: filters.Cluster,
	}
	return b.buildQuery("disk_type_summary_query", mustGetTemplate("disk_type_summary_query"), params)
}

// ResourceTotalsQuery builds the resource totals query.
func (b *QueryBuilder) ResourceTotalsQuery(filters Filters) (string, error) {
	params := queryParams{
		ClusterFilter: filters.Cluster,
	}
	return b.buildQuery("resource_totals_query", mustGetTemplate("resource_totals_query"), params)
}

// AllocatedVCPUsQuery builds the allocated vCPUs query.
func (b *QueryBuilder) AllocatedVCPUsQuery(filters Filters) (string, error) {
	params := queryParams{
		ClusterFilter: filters.Cluster,
	}
	return b.buildQuery("allocated_vcpus_query", mustGetTemplate("allocated_vcpus_query"), params)
}

// AllocatedMemoryQuery builds the allocated memory query.
func (b *QueryBuilder) AllocatedMemoryQuery(filters Filters) (string, error) {
	params := queryParams{
		ClusterFilter: filters.Cluster,
	}
	return b.buildQuery("allocated_memory_query", mustGetTemplate("allocated_memory_query"), params)
}

// TotalHostCPUsQuery builds the total host CPUs query.
func (b *QueryBuilder) TotalHostCPUsQuery(filters Filters) (string, error) {
	params := queryParams{
		ClusterFilter: filters.Cluster,
	}
	return b.buildQuery("total_host_cpus_query", mustGetTemplate("total_host_cpus_query"), params)
}

// TotalHostMemoryQuery builds the total host memory query.
func (b *QueryBuilder) TotalHostMemoryQuery(filters Filters) (string, error) {
	params := queryParams{
		ClusterFilter: filters.Cluster,
	}
	return b.buildQuery("total_host_memory_query", mustGetTemplate("total_host_memory_query"), params)
}

// VMCountByNetworkQuery builds the VM count by network query.
func (b *QueryBuilder) VMCountByNetworkQuery(filters Filters) (string, error) {
	params := queryParams{
		ClusterFilter: filters.Cluster,
	}
	return b.buildQuery("vm_count_by_network_query", mustGetTemplate("vm_count_by_network_query"), params)
}

// DatacenterCountQuery builds the datacenter count query.
func (b *QueryBuilder) DatacenterCountQuery() (string, error) {
	return b.buildQuery("datacenter_count_query", mustGetTemplate("datacenter_count_query"), nil)
}

// ClustersPerDatacenterQuery builds the clusters per datacenter query.
func (b *QueryBuilder) ClustersPerDatacenterQuery() (string, error) {
	return b.buildQuery("clusters_per_datacenter_query", mustGetTemplate("clusters_per_datacenter_query"), nil)
}

// MigratableCountQuery builds the migratable VMs count query.
func (b *QueryBuilder) MigratableCountQuery(filters Filters) (string, error) {
	params := queryParams{
		ClusterFilter: filters.Cluster,
	}
	return b.buildQuery("migratable_count_query", mustGetTemplate("migratable_count_query"), params)
}

// MigratableWithWarningsCountQuery builds the migratable with warnings count query.
func (b *QueryBuilder) MigratableWithWarningsCountQuery(filters Filters) (string, error) {
	params := queryParams{
		ClusterFilter: filters.Cluster,
	}
	return b.buildQuery("migratable_with_warnings_count_query", mustGetTemplate("migratable_with_warnings_count_query"), params)
}

// NotMigratableCountQuery builds the not migratable VMs count query.
func (b *QueryBuilder) NotMigratableCountQuery(filters Filters) (string, error) {
	params := queryParams{
		ClusterFilter: filters.Cluster,
	}
	return b.buildQuery("not_migratable_count_query", mustGetTemplate("not_migratable_count_query"), params)
}

// MigrationIssuesQuery builds the migration issues query.
func (b *QueryBuilder) MigrationIssuesQuery(filters Filters, category string) (string, error) {
	params := queryParams{
		ClusterFilter: filters.Cluster,
		Category:      category,
	}
	return b.buildQuery("migration_issues_query", mustGetTemplate("migration_issues_query"), params)
}

// ResourceBreakdownsQuery builds the resource breakdowns query.
func (b *QueryBuilder) ResourceBreakdownsQuery(filters Filters) (string, error) {
	params := queryParams{
		ClusterFilter: filters.Cluster,
	}
	return b.buildQuery("resource_breakdowns_query", mustGetTemplate("resource_breakdowns_query"), params)
}

// VCenterQuery builds the vCenter ID query.
func (b *QueryBuilder) VCenterQuery() (string, error) {
	return b.buildQuery("vcenter_query", mustGetTemplate("vcenter_query"), nil)
}

func (b *QueryBuilder) buildQuery(name, tmplContent string, params any) (string, error) {
	tmpl, err := template.New(name).Parse(tmplContent)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, params); err != nil {
		return "", err
	}
	return strings.TrimSpace(buf.String()), nil
}
