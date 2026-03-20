package duckdb_parser

import (
	"bytes"
	"embed"
	"fmt"
	"sort"
	"strings"
	"text/template"

	"github.com/kubev2v/migration-planner/pkg/estimations/complexity"
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
	VmIDList         string // IN ('id1','id2') with escaped quotes, or empty
	Category         string
	OSCaseClauses    string
	Limit            int
	Offset           int
}

// buildVmIDList returns a SQL fragment for IN (id1, id2, ...) with single quotes escaped, or "" if ids is empty.
func buildVmIDList(ids []string) string {
	if len(ids) == 0 {
		return ""
	}
	escaped := make([]string, len(ids))
	for i, id := range ids {
		escaped[i] = "'" + strings.ReplaceAll(id, "'", "''") + "'"
	}
	return "(" + strings.Join(escaped, ",") + ")"
}

// VMQuery builds the VM query with filters and pagination.
func (b *QueryBuilder) VMQuery(filters Filters, options Options) (string, error) {
	const maxNetworkNumbers = 8 // RVTools vInfo has Network #1 through Network #8
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
		VmIDList:         buildVmIDList(filters.VmIDs),
		Limit:            options.Limit,
		Offset:           options.Offset,
	}
	return b.buildQuery("vm_query", mustGetTemplate("vm_query"), params)
}

// DatastoreQuery builds the datastore query with filters and pagination.
func (b *QueryBuilder) DatastoreQuery(filters Filters, options Options) (string, error) {
	params := queryParams{
		ClusterFilter: filters.Cluster,
		VmIDList:      buildVmIDList(filters.VmIDs),
		Limit:         options.Limit,
		Offset:        options.Offset,
	}
	return b.buildQuery("datastore_query", mustGetTemplate("datastore_query"), params)
}

// NetworkQuery builds the network query with filters and pagination.
func (b *QueryBuilder) NetworkQuery(filters Filters, options Options) (string, error) {
	params := queryParams{
		ClusterFilter: filters.Cluster,
		VmIDList:      buildVmIDList(filters.VmIDs),
		Limit:         options.Limit,
		Offset:        options.Offset,
	}
	return b.buildQuery("network_query", mustGetTemplate("network_query"), params)
}

// HostQuery builds the host query with filters and pagination.
func (b *QueryBuilder) HostQuery(filters Filters, options Options) (string, error) {
	params := queryParams{
		ClusterFilter: filters.Cluster,
		VmIDList:      buildVmIDList(filters.VmIDs),
		Limit:         options.Limit,
		Offset:        options.Offset,
	}
	return b.buildQuery("host_query", mustGetTemplate("host_query"), params)
}

// OsQuery builds the OS summary query with filters.
func (b *QueryBuilder) OsQuery(filters Filters) (string, error) {
	params := queryParams{
		ClusterFilter: filters.Cluster,
		VmIDList:      buildVmIDList(filters.VmIDs),
	}
	return b.buildQuery("os_query", mustGetTemplate("os_query"), params)
}

// ClustersQuery builds the clusters query.
func (b *QueryBuilder) ClustersQuery() (string, error) {
	return b.buildQuery("clusters_query", mustGetTemplate("clusters_query"), nil)
}

// ClusterDatacentersQuery builds the cluster to datacenter mapping query.
func (b *QueryBuilder) ClusterDatacentersQuery() (string, error) {
	return b.buildQuery("cluster_datacenters_query", mustGetTemplate("cluster_datacenters_query"), nil)
}

// ClusterObjectIDsQuery builds the cluster name to Object ID mapping query.
func (b *QueryBuilder) ClusterObjectIDsQuery() (string, error) {
	return b.buildQuery("cluster_object_ids_query", mustGetTemplate("cluster_object_ids_query"), nil)
}

// VMCountQuery builds the VM count query with filters.
func (b *QueryBuilder) VMCountQuery(filters Filters) (string, error) {
	params := queryParams{
		ClusterFilter:    filters.Cluster,
		PowerStateFilter: filters.PowerState,
		VmIDList:         buildVmIDList(filters.VmIDs),
	}
	return b.buildQuery("vm_count_query", mustGetTemplate("vm_count_query"), params)
}

// PowerStateCountsQuery builds the power state counts query.
func (b *QueryBuilder) PowerStateCountsQuery(filters Filters) (string, error) {
	params := queryParams{
		ClusterFilter: filters.Cluster,
		VmIDList:      buildVmIDList(filters.VmIDs),
	}
	return b.buildQuery("power_state_counts_query", mustGetTemplate("power_state_counts_query"), params)
}

// HostPowerStateCountsQuery builds the host power state counts query.
func (b *QueryBuilder) HostPowerStateCountsQuery(filters Filters) (string, error) {
	params := queryParams{
		ClusterFilter: filters.Cluster,
		VmIDList:      buildVmIDList(filters.VmIDs),
	}
	return b.buildQuery("host_power_state_counts_query", mustGetTemplate("host_power_state_counts_query"), params)
}

// CPUTierQuery builds the CPU tier distribution query.
func (b *QueryBuilder) CPUTierQuery(filters Filters) (string, error) {
	params := queryParams{
		ClusterFilter: filters.Cluster,
		VmIDList:      buildVmIDList(filters.VmIDs),
	}
	return b.buildQuery("cpu_tier_query", mustGetTemplate("cpu_tier_query"), params)
}

// MemoryTierQuery builds the memory tier distribution query.
func (b *QueryBuilder) MemoryTierQuery(filters Filters) (string, error) {
	params := queryParams{
		ClusterFilter: filters.Cluster,
		VmIDList:      buildVmIDList(filters.VmIDs),
	}
	return b.buildQuery("memory_tier_query", mustGetTemplate("memory_tier_query"), params)
}

// NicTierQuery builds the NIC count tier distribution query.
func (b *QueryBuilder) NicTierQuery(filters Filters) (string, error) {
	params := queryParams{
		ClusterFilter: filters.Cluster,
		VmIDList:      buildVmIDList(filters.VmIDs),
	}
	return b.buildQuery("nic_tier_query", mustGetTemplate("nic_tier_query"), params)
}

// ComplexityDistributionQuery builds the complexity distribution query.
func (b *QueryBuilder) ComplexityDistributionQuery(filters Filters) (string, error) {
	params := queryParams{
		ClusterFilter: filters.Cluster,
	}
	return b.buildQuery("complexity_distribution_query", mustGetTemplate("complexity_distribution_query"), params)
}

// DiskSizeTierQuery builds the disk size tier distribution query.
func (b *QueryBuilder) DiskSizeTierQuery(filters Filters) (string, error) {
	params := queryParams{
		ClusterFilter: filters.Cluster,
		VmIDList:      buildVmIDList(filters.VmIDs),
	}
	return b.buildQuery("disk_size_tier_query", mustGetTemplate("disk_size_tier_query"), params)
}

// DiskTypeSummaryQuery builds the disk type summary query.
func (b *QueryBuilder) DiskTypeSummaryQuery(filters Filters) (string, error) {
	params := queryParams{
		ClusterFilter: filters.Cluster,
		VmIDList:      buildVmIDList(filters.VmIDs),
	}
	return b.buildQuery("disk_type_summary_query", mustGetTemplate("disk_type_summary_query"), params)
}

// ResourceTotalsQuery builds the resource totals query.
func (b *QueryBuilder) ResourceTotalsQuery(filters Filters) (string, error) {
	params := queryParams{
		ClusterFilter: filters.Cluster,
		VmIDList:      buildVmIDList(filters.VmIDs),
	}
	return b.buildQuery("resource_totals_query", mustGetTemplate("resource_totals_query"), params)
}

// AllocatedVCPUsQuery builds the allocated vCPUs query.
func (b *QueryBuilder) AllocatedVCPUsQuery(filters Filters) (string, error) {
	params := queryParams{
		ClusterFilter: filters.Cluster,
		VmIDList:      buildVmIDList(filters.VmIDs),
	}
	return b.buildQuery("allocated_vcpus_query", mustGetTemplate("allocated_vcpus_query"), params)
}

// AllocatedMemoryQuery builds the allocated memory query.
func (b *QueryBuilder) AllocatedMemoryQuery(filters Filters) (string, error) {
	params := queryParams{
		ClusterFilter: filters.Cluster,
		VmIDList:      buildVmIDList(filters.VmIDs),
	}
	return b.buildQuery("allocated_memory_query", mustGetTemplate("allocated_memory_query"), params)
}

// TotalHostCPUsQuery builds the total host CPUs query.
func (b *QueryBuilder) TotalHostCPUsQuery(filters Filters) (string, error) {
	params := queryParams{
		ClusterFilter: filters.Cluster,
		VmIDList:      buildVmIDList(filters.VmIDs),
	}
	return b.buildQuery("total_host_cpus_query", mustGetTemplate("total_host_cpus_query"), params)
}

// TotalHostMemoryQuery builds the total host memory query.
func (b *QueryBuilder) TotalHostMemoryQuery(filters Filters) (string, error) {
	params := queryParams{
		ClusterFilter: filters.Cluster,
		VmIDList:      buildVmIDList(filters.VmIDs),
	}
	return b.buildQuery("total_host_memory_query", mustGetTemplate("total_host_memory_query"), params)
}

// VMCountByNetworkQuery builds the VM count by network query.
func (b *QueryBuilder) VMCountByNetworkQuery(filters Filters) (string, error) {
	params := queryParams{
		ClusterFilter: filters.Cluster,
		VmIDList:      buildVmIDList(filters.VmIDs),
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
		VmIDList:      buildVmIDList(filters.VmIDs),
	}
	return b.buildQuery("migratable_count_query", mustGetTemplate("migratable_count_query"), params)
}

// MigratableWithWarningsCountQuery builds the migratable with warnings count query.
func (b *QueryBuilder) MigratableWithWarningsCountQuery(filters Filters) (string, error) {
	params := queryParams{
		ClusterFilter: filters.Cluster,
		VmIDList:      buildVmIDList(filters.VmIDs),
	}
	return b.buildQuery("migratable_with_warnings_count_query", mustGetTemplate("migratable_with_warnings_count_query"), params)
}

// NotMigratableCountQuery builds the not migratable VMs count query.
func (b *QueryBuilder) NotMigratableCountQuery(filters Filters) (string, error) {
	params := queryParams{
		ClusterFilter: filters.Cluster,
		VmIDList:      buildVmIDList(filters.VmIDs),
	}
	return b.buildQuery("not_migratable_count_query", mustGetTemplate("not_migratable_count_query"), params)
}

// MigrationIssuesQuery builds the migration issues query.
func (b *QueryBuilder) MigrationIssuesQuery(filters Filters, category string) (string, error) {
	params := queryParams{
		ClusterFilter: filters.Cluster,
		VmIDList:      buildVmIDList(filters.VmIDs),
		Category:      category,
	}
	return b.buildQuery("migration_issues_query", mustGetTemplate("migration_issues_query"), params)
}

// ResourceBreakdownsQuery builds the resource breakdowns query.
func (b *QueryBuilder) ResourceBreakdownsQuery(filters Filters) (string, error) {
	params := queryParams{
		ClusterFilter: filters.Cluster,
		VmIDList:      buildVmIDList(filters.VmIDs),
	}
	return b.buildQuery("resource_breakdowns_query", mustGetTemplate("resource_breakdowns_query"), params)
}

// VCenterQuery builds the vCenter ID query.
func (b *QueryBuilder) VCenterQuery() (string, error) {
	return b.buildQuery("vcenter_query", mustGetTemplate("vcenter_query"), nil)
}

// VMsWithSharedDisksCountQuery builds the VMs with shared disks count query.
func (b *QueryBuilder) VMsWithSharedDisksCountQuery(filters Filters) (string, error) {
	params := queryParams{
		ClusterFilter: filters.Cluster,
		VmIDList:      buildVmIDList(filters.VmIDs),
	}
	return b.buildQuery("vms_with_shared_disks_count_query", mustGetTemplate("vms_with_shared_disks_count_query"), params)
}

// generateOSCaseClauses reads complexity.OSDifficultyScores and generates SQL WHEN clauses.
func generateOSCaseClauses() string {
	scoreToLevel := map[int]string{
		1: "easy",
		2: "medium",
		3: "hard",
		4: "database",
	}
	var clauses []string
	for keyword, score := range complexity.OSDifficultyScores {
		level := scoreToLevel[score]
		clauses = append(clauses, fmt.Sprintf(
			"            WHEN LOWER(effective_os) LIKE '%%%s%%' THEN '%s'", strings.ToLower(keyword), level))
	}
	sort.Strings(clauses) // deterministic output
	return strings.Join(clauses, "\n")
}

// PopulateComplexityQuery returns a query that computes and stores per-VM migration complexity.
func (b *QueryBuilder) PopulateComplexityQuery() (string, error) {
	params := queryParams{
		OSCaseClauses: generateOSCaseClauses(),
	}
	return b.buildQuery("populate_complexity", mustGetTemplate("populate_complexity"), params)
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
