package duckdb_parser

import (
	"context"
	"fmt"

	"github.com/kubev2v/migration-planner/pkg/store"
)

// StorageTopologyQuery returns CTEs that preserve available storage metadata in
// older collections without changing their schema, including read-only files.
func StorageTopologyQuery(ctx context.Context, db store.QueryInterceptor) (string, error) {
	return storageTopology(ctx, db, false)
}

// VMStorageTopologyQuery returns topology CTEs with one VM ID parameter before caller parameters.
func VMStorageTopologyQuery(ctx context.Context, db store.QueryInterceptor) (string, error) {
	return storageTopology(ctx, db, true)
}

// DatastoreResolutionQuery returns row-scoped datastore and disk resolution CTEs.
func DatastoreResolutionQuery() string {
	return datastoreResolutionQuery(false)
}

func storageTopology(ctx context.Context, db store.QueryInterceptor, vmOnly bool) (string, error) {
	var hasHBA, hasSCSI bool
	if err := db.QueryRowContext(ctx, `SELECT
    EXISTS (SELECT 1 FROM information_schema.tables
        WHERE table_catalog = current_database() AND table_schema = 'main' AND table_name = 'vhba'),
    EXISTS (SELECT 1 FROM information_schema.tables
        WHERE table_catalog = current_database() AND table_schema = 'main' AND table_name = 'vhostscsi')
`).Scan(&hasHBA, &hasSCSI); err != nil {
		return "", fmt.Errorf("checking storage topology tables: %w", err)
	}
	hba, scsi := "vhba", "vhostscsi"
	const emptyTable = "(SELECT NULL::VARCHAR AS placeholder WHERE false)"
	if !hasHBA {
		hba = emptyTable
	}
	if !hasSCSI {
		scsi = emptyTable
	}
	return storageTopologyQuery(hba, scsi, vmOnly), nil
}

func storageTopologyQuery(hba, scsi string, vmOnly bool) string {
	// Row JSON yields NULL for a missing optional column instead of a binder error.
	return fmt.Sprintf(`storage_hbas AS (
    SELECT
        to_json(h)->>'Host ID' AS "Host ID",
        to_json(h)->>'Device' AS "Device",
        to_json(h)->>'Type' AS "Type",
        to_json(h)->>'Protocol' AS "Protocol"
    FROM %s h
),
storage_scsi AS (
    SELECT
        to_json(s)->>'Host ID' AS "Host ID",
        to_json(s)->>'HBA Device' AS "HBA Device",
        to_json(s)->>'Canonical Name' AS "Canonical Name",
        to_json(s)->>'LUN Vendor' AS "LUN Vendor",
        to_json(s)->>'LUN Model' AS "LUN Model"
    FROM %s s
)`, hba, scsi) + ",\n" + datastoreResolutionQuery(vmOnly)
}

func datastoreResolutionQuery(vmOnly bool) string {
	filter := ""
	if vmOnly {
		filter = `
    WHERE dk."VM ID" = ?`
	}
	return fmt.Sprintf(`storage_datastores AS (
    SELECT d.rowid AS datastore_row_id, d.*
    FROM vdatastore d
),
storage_hosts AS (
    SELECT
        MIN(h.rowid) AS host_row_id,
        h."Object ID" AS host_id,
        LOWER(TRIM(h."Host")) AS host_name,
        h."Cluster" AS host_cluster
    FROM vhost h
    GROUP BY h."Object ID", LOWER(TRIM(h."Host")), h."Cluster"
),
storage_host_candidates AS (
    SELECT DISTINCT
        refs.reference,
        h.host_row_id,
        h.host_id,
        h.host_cluster
    FROM (
        SELECT TRIM(UNNEST(string_split(COALESCE("Hosts", ''), ','))) AS reference
        FROM storage_datastores
        UNION
        SELECT TRIM("Host") AS reference FROM vinfo
    ) refs
    JOIN storage_hosts h
        ON refs.reference = TRIM(h.host_id)
        OR LOWER(refs.reference) = h.host_name
    WHERE refs.reference != ''
),
storage_host_references AS (
    SELECT
        reference,
        MIN(host_row_id) AS host_row_id,
        MIN(host_id) AS host_id,
        MIN(host_cluster) AS host_cluster
    FROM storage_host_candidates
    GROUP BY reference
    HAVING COUNT(*) = 1
),
storage_datastore_hosts AS (
    SELECT DISTINCT
        d.datastore_row_id,
        h.host_row_id,
        h.host_id,
        h.host_cluster
    FROM storage_datastores d
    CROSS JOIN UNNEST(string_split(COALESCE(d."Hosts", ''), ',')) AS member(reference)
    JOIN storage_host_references h ON h.reference = TRIM(member.reference)
),
storage_disk_candidates AS (
    SELECT
        dk."VM ID",
        dk.rowid AS disk_row_id,
        ds.datastore_row_id,
        member.host_row_id IS NOT NULL AS host_match
    FROM vdisk dk
    JOIN storage_datastores ds
        ON ds."Name" = regexp_extract(COALESCE(dk."Path", dk."Disk Path"), '\[([^\]]+)\]', 1)
    LEFT JOIN vinfo i ON i."VM ID" = dk."VM ID"
    LEFT JOIN storage_host_references vm_host ON vm_host.reference = TRIM(i."Host")
    LEFT JOIN storage_datastore_hosts member
        ON member.datastore_row_id = ds.datastore_row_id
        AND member.host_row_id = vm_host.host_row_id%s
),
storage_disk_datastores AS (
    SELECT "VM ID", disk_row_id, datastore_row_id
    FROM storage_disk_candidates
    QUALIFY COUNT(*) OVER (PARTITION BY "VM ID", disk_row_id) = 1
        OR (host_match AND COUNT(*) FILTER (WHERE host_match)
            OVER (PARTITION BY "VM ID", disk_row_id) = 1)
)`, filter)
}
