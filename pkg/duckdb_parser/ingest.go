package duckdb_parser

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"go.uber.org/zap"
)

var stmtRegex = regexp.MustCompile(`(?s)(CREATE|INSERT|UPDATE|DROP|ALTER|WITH|INSTALL|LOAD|ATTACH|DETACH|DELETE).*?;`)

// criticalStmtPatterns defines patterns for statements that must succeed.
// If any of these fail, the ingestion should fail immediately.
var criticalStmtPatterns = []string{
	"INSTALL ",           // Extension installation must succeed
	"LOAD ",              // Extension loading must succeed
	"CREATE TABLE vinfo", // Main VM data table creation must succeed
}

// isCriticalStatement checks if a statement matches any critical pattern.
func isCriticalStatement(stmt string) bool {
	upperStmt := strings.ToUpper(stmt)
	for _, pattern := range criticalStmtPatterns {
		if strings.Contains(upperStmt, strings.ToUpper(pattern)) {
			return true
		}
	}
	return false
}

// xlsxErrorMappings maps DuckDB error substrings to user-friendly messages.
var xlsxErrorMappings = map[string]string{
	"No xl/workbook.xml found":         "The file is corrupted or not a valid Excel file",
	"\"vInfo\" not found in xlsx file": "File is not a valid RVTools export (missing required 'vInfo' sheet)",
}

// translateXLSXError converts technical DuckDB xlsx errors to user-friendly messages.
func translateXLSXError(err error) error {
	errStr := err.Error()
	for pattern, message := range xlsxErrorMappings {
		if strings.Contains(errStr, pattern) {
			return fmt.Errorf("%s", message)
		}
	}
	return err
}

// IngestRvTools ingests data from an RVTools Excel file, runs VM validation if a validator
// is configured, and validates the schema for required tables/columns.
// Returns a ValidationResult with errors (fatal) and warnings (non-fatal).
// If ValidationResult.HasErrors() is true, the inventory cannot be built.
func (p *Parser) IngestRvTools(ctx context.Context, excelFile string) (ValidationResult, error) {
	query, err := p.builder.IngestRvtoolsQuery(excelFile)
	if err != nil {
		return ValidationResult{}, fmt.Errorf("building rvtools ingestion query: %w", err)
	}
	if err := p.executeStatements(ctx, query); err != nil {
		return ValidationResult{}, fmt.Errorf("ingesting rvtools data: %w", err)
	}

	// Validate schema against vinfo_raw (unfiltered RVTools data) for granular error reporting
	result := p.ValidateSchema(ctx, "vinfo_raw")

	// Drop vinfo_raw now that validation is complete
	if err := p.dropVinfoRaw(ctx); err != nil {
		return result, fmt.Errorf("dropping vinfo_raw: %w", err)
	}

	// Only run post-ingestion steps if schema is valid (we have VMs to process)
	if result.IsValid() {
		if err := p.populateComplexity(ctx); err != nil {
			return result, fmt.Errorf("populating complexity: %w", err)
		}
		if err := p.validateVMs(ctx); err != nil {
			return result, fmt.Errorf("validating VMs: %w", err)
		}
	}

	return result, nil
}

// IngestSqlite ingests data from a forklift SQLite database, runs VM validation if a validator
// is configured, and validates the schema for required tables/columns.
// Returns a ValidationResult with errors (fatal) and warnings (non-fatal).
// If ValidationResult.HasErrors() is true, the inventory cannot be built.
func (p *Parser) IngestSqlite(ctx context.Context, sqliteFile string) (ValidationResult, error) {
	query, err := p.builder.IngestSqliteQuery(sqliteFile)
	if err != nil {
		return ValidationResult{}, fmt.Errorf("building sqlite ingestion query: %w", err)
	}
	if err := p.executeStatements(ctx, query); err != nil {
		return ValidationResult{}, fmt.Errorf("ingesting sqlite data: %w", err)
	}

	// Validate schema against vinfo (SQLite inserts directly into vinfo, no vinfo_raw)
	result := p.ValidateSchema(ctx, "vinfo")

	// Only run post-ingestion steps if schema is valid (we have VMs to process)
	if result.IsValid() {
		if err := p.populateComplexity(ctx); err != nil {
			return result, fmt.Errorf("populating complexity: %w", err)
		}
		if err := p.populateVCluster(ctx); err != nil {
			return result, fmt.Errorf("populating vcluster: %w", err)
		}
		if err := p.validateVMs(ctx); err != nil {
			return result, fmt.Errorf("validating VMs: %w", err)
		}
	}

	return result, nil
}

// stagedClusterFeatures holds forklift's real cluster-level HA flag, carried across the `src`
// detach boundary during SQLite ingestion via the cluster_features_staging table (see
// ingest_sqlite.go.tmpl). Zero-valued when a cluster has no staged row (e.g. RVTools ingestion,
// which never creates the staging table at all).
type stagedClusterFeatures struct {
	DasEnabled bool
}

// populateVCluster inserts a row into vcluster for each cluster, using the same anonymous ID
// that BuildInventory assigns as the cluster's key. This lets vcluster serve as a name→ID map
// for the agent without exposing real VMware moref IDs. HA (DasEnabled) is joined in from
// cluster_features_staging when present - only the SQLite ingest path stages it, since RVTools
// ingestion populates vcluster (including features) directly and this function never runs for
// it (see the no-op guard below).
// It is a no-op when vcluster already contains data (e.g. after RVTools ingestion).
func (p *Parser) populateVCluster(ctx context.Context) error {
	var count int
	if err := p.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM vcluster`).Scan(&count); err != nil {
		return fmt.Errorf("checking vcluster: %w", err)
	}
	if count > 0 {
		return nil
	}

	vcenterID, err := p.VCenterID(ctx)
	if err != nil {
		zap.S().Named("duckdb_parser").Warnf("Failed to get vCenter ID: %v", err)
		vcenterID = ""
	}

	clusterDatacenters, err := p.ClusterDatacenters(ctx)
	if err != nil {
		return fmt.Errorf("getting cluster datacenters: %w", err)
	}

	clusters, err := p.Clusters(ctx)
	if err != nil {
		return fmt.Errorf("getting clusters: %w", err)
	}

	features, err := p.clusterFeaturesStaging(ctx)
	if err != nil {
		return fmt.Errorf("reading staged cluster features: %w", err)
	}

	values := make([]string, 0, len(clusters))
	for _, clusterName := range clusters {
		datacenter := clusterDatacenters[clusterName]
		id := generateClusterID(clusterName, datacenter, vcenterID)
		f := features[clusterName]
		values = append(values, fmt.Sprintf(
			"('%s', '%s', %t)",
			escapeSQLString(clusterName), escapeSQLString(id), f.DasEnabled,
		))
	}
	if len(values) == 0 {
		return nil
	}
	query := fmt.Sprintf(
		`INSERT INTO vcluster ("Name", "Object ID", "DasEnabled") VALUES %s`,
		strings.Join(values, ", "),
	)
	if _, err := p.db.ExecContext(ctx, query); err != nil {
		return fmt.Errorf("inserting vcluster rows: %w", err)
	}
	if err := p.dropClusterFeaturesStaging(ctx); err != nil {
		return fmt.Errorf("dropping cluster_features_staging: %w", err)
	}
	return nil
}

// clusterFeaturesStaging reads forklift's real HA flag staged by the SQLite ingest template
// before it detached the source database (see ingest_sqlite.go.tmpl). Returns an empty map when
// the staging table doesn't exist, which is expected for RVTools ingestion - only the SQLite
// template creates it.
func (p *Parser) clusterFeaturesStaging(ctx context.Context) (map[string]stagedClusterFeatures, error) {
	var exists bool
	err := p.db.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'cluster_features_staging')`,
	).Scan(&exists)
	if err != nil {
		return nil, fmt.Errorf("checking cluster_features_staging: %w", err)
	}
	if !exists {
		return map[string]stagedClusterFeatures{}, nil
	}

	rows, err := p.db.QueryContext(ctx,
		`SELECT "Name", "DasEnabled" FROM cluster_features_staging`,
	)
	if err != nil {
		return nil, fmt.Errorf("querying cluster_features_staging: %w", err)
	}
	defer func() { _ = rows.Close() }()

	result := make(map[string]stagedClusterFeatures)
	for rows.Next() {
		var name string
		var f stagedClusterFeatures
		if err := rows.Scan(&name, &f.DasEnabled); err != nil {
			return nil, fmt.Errorf("scanning cluster_features_staging row: %w", err)
		}
		result[name] = f
	}
	return result, rows.Err()
}

// dropClusterFeaturesStaging drops the temporary staging table used to carry forklift's cluster
// feature flags across the src detach boundary during SQLite ingestion.
func (p *Parser) dropClusterFeaturesStaging(ctx context.Context) error {
	_, err := p.db.ExecContext(ctx, "DROP TABLE IF EXISTS cluster_features_staging")
	return err
}

// dropVinfoRaw drops the temporary vinfo_raw table used during RVTools ingestion.
// This table holds unfiltered data from the Excel file and is only needed for validation.
func (p *Parser) dropVinfoRaw(ctx context.Context) error {
	_, err := p.db.ExecContext(ctx, "DROP TABLE IF EXISTS vinfo_raw")
	return err
}

// executeStatements executes a multi-statement SQL string.
// Critical statements (INSTALL, LOAD, CREATE TABLE vinfo) must succeed or an error is returned.
// Non-critical statements (INSERT for optional sheets, ALTER for optional columns) are logged but don't fail.
func (p *Parser) executeStatements(ctx context.Context, query string) error {
	stmts := stmtRegex.FindAllString(query, -1)
	for _, stmt := range stmts {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := p.db.ExecContext(ctx, stmt); err != nil {
			if isCriticalStatement(stmt) {
				return translateXLSXError(err)
			}
			// Non-critical failures are logged but don't stop execution
			zap.S().Debugw("non-critical statement failed", "error", err)
		}
	}
	return nil
}

// populateComplexity computes and stores per-VM migration complexity based on OS type and disk size.
func (p *Parser) populateComplexity(ctx context.Context) error {
	query, err := p.builder.PopulateComplexityQuery()
	if err != nil {
		return fmt.Errorf("building complexity query: %w", err)
	}
	if _, err := p.db.ExecContext(ctx, query); err != nil {
		return fmt.Errorf("executing complexity query: %w", err)
	}
	return nil
}

// validateVMs runs the configured VM validator (e.g., OPA) to populate the concerns table.
func (p *Parser) validateVMs(ctx context.Context) error {
	if p.validator == nil {
		return nil
	}

	vms, err := p.VMs(ctx, Filters{}, Options{})
	if err != nil {
		return fmt.Errorf("getting VMs for validation: %w", err)
	}

	builder := NewConcernValuesBuilder()
	for _, vm := range vms {
		concerns, err := p.validator.Validate(ctx, vm)
		if err != nil {
			zap.S().Warnw("validation failed for VM", "vm_id", vm.ID, "error", err)
			continue
		}
		builder.Append(vm.ID, concerns...)
	}

	return InsertConcerns(ctx, p.db, builder)
}
