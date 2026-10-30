package duckdb_parser

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/kubev2v/migration-planner/pkg/inventory"
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
	hba, scsi, err := storageTables(ctx, db)
	if err != nil {
		return "", err
	}
	return storageTopologyQuery(hba, scsi, vmOnly), nil
}

func storageTables(ctx context.Context, db store.QueryInterceptor) (string, string, error) {
	var hasHBA, hasSCSI bool
	if err := db.QueryRowContext(ctx, `SELECT
    EXISTS (SELECT 1 FROM information_schema.tables
        WHERE table_catalog = current_database() AND table_schema = 'main' AND table_name = 'vhba'),
    EXISTS (SELECT 1 FROM information_schema.tables
        WHERE table_catalog = current_database() AND table_schema = 'main' AND table_name = 'vhostscsi')
`).Scan(&hasHBA, &hasSCSI); err != nil {
		return "", "", fmt.Errorf("checking storage topology tables: %w", err)
	}
	hba, scsi := "vhba", "vhostscsi"
	const emptyTable = "(SELECT NULL::VARCHAR AS placeholder WHERE false)"
	if !hasHBA {
		hba = emptyTable
	}
	if !hasSCSI {
		scsi = emptyTable
	}
	return hba, scsi, nil
}

func storageTopologyQuery(hba, scsi string, vmOnly bool) string {
	// Row JSON yields NULL for a missing optional column instead of a binder error.
	return datastoreResolutionQuery(vmOnly) + ",\n" + fmt.Sprintf(`storage_topology_hosts AS (
    SELECT host_id
    FROM storage_hosts
    WHERE NULLIF(TRIM(host_id), '') IS NOT NULL
    GROUP BY host_id
    HAVING COUNT(*) = 1
),
storage_hbas AS (
    SELECT
        to_json(h)->>'Host ID' AS "Host ID",
        to_json(h)->>'Device' AS "Device",
        to_json(h)->>'Type' AS "Type",
        to_json(h)->>'Protocol' AS "Protocol"
    FROM %s h
    JOIN storage_topology_hosts host ON host.host_id = (to_json(h)->>'Host ID')
),
storage_scsi AS (
    SELECT
        to_json(s)->>'Host ID' AS "Host ID",
        to_json(s)->>'HBA Device' AS "HBA Device",
        to_json(s)->>'Canonical Name' AS "Canonical Name",
        to_json(s)->>'LUN Vendor' AS "LUN Vendor",
        to_json(s)->>'LUN Model' AS "LUN Model"
    FROM %s s
    JOIN storage_topology_hosts host ON host.host_id = (to_json(s)->>'Host ID')
)`, hba, scsi)
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

type hostStorageFacts struct {
	adapters []inventory.HostStorageAdapter
	luns     []inventory.HostStorageLun
}

// hostStorage resolves owners globally, before the caller filters or paginates
// hosts. Duplicate rows must have matching host facts and a known source to
// share children; legacy duplicates without source identity remain unassigned.
// Configuration status is transient and does not distinguish host owners.
// Independently aggregate adapters and LUNs to avoid a Cartesian join.
// Row JSON keeps absent optional columns readable without upgrading old files.
func (p *Parser) hostStorage(ctx context.Context) (map[string]hostStorageFacts, error) {
	hba, scsi, err := storageTables(ctx, p.db)
	if err != nil {
		return nil, err
	}
	query := fmt.Sprintf(`WITH owners AS (
    SELECT DISTINCT TRIM("Object ID") AS host_id, LOWER(TRIM("Host")) AS host_name,
        COALESCE("Cluster", '') AS cluster,
        COALESCE(to_json(h)->>'Datacenter', '') AS datacenter,
        COALESCE(NULLIF(LOWER(TRIM(to_json(h)->>'VI SDK UUID')), 'vm'), '') AS source,
        COUNT(*) OVER (PARTITION BY TRIM("Object ID")) AS host_rows,
        COALESCE(h."Model", 'N/A') AS model,
        COALESCE(h."Vendor", 'N/A') AS vendor,
        COALESCE(h."# Cores", 0) AS cpu_cores,
        COALESCE(h."# CPU", 0) AS cpu_sockets,
        COALESCE(h."# Memory", 0) AS memory,
        COALESCE(h."VMotion support", false) AS vmotion,
        COALESCE(h."Storage VMotion support", false) AS storage_vmotion
    FROM vhost h
), unique_owners AS (
    SELECT host_id FROM owners WHERE host_id != ''
    GROUP BY host_id HAVING COUNT(*) = 1
        AND (MIN(source) != '' OR MAX(host_rows) = 1)
), adapters AS (
    SELECT TRIM(to_json(h)->>'Host ID') AS host_id,
        NULLIF(TRIM(to_json(h)->>'Device'), '') AS device,
        NULLIF(TRIM(to_json(h)->>'HBA Key'), '') AS key,
        NULLIF(TRIM(to_json(h)->>'Type'), '') AS type,
        NULLIF(TRIM(to_json(h)->>'Protocol'), '') AS protocol,
        NULLIF(TRIM(to_json(h)->>'Model'), '') AS model,
        NULLIF(TRIM(to_json(h)->>'Driver'), '') AS driver,
        NULLIF(TRIM(to_json(h)->>'Node WWN'), '') AS nodeWwn,
        NULLIF(TRIM(to_json(h)->>'Port WWN'), '') AS portWwn
    FROM %s h
), adapter_records AS (
    SELECT host_id, COALESCE('device:' || device, 'key:' || key) AS identity,
        CASE WHEN COUNT(DISTINCT device) = 1 THEN MIN(device) END AS device,
        CASE WHEN COUNT(DISTINCT key) = 1 THEN MIN(key) END AS key,
        CASE WHEN COUNT(DISTINCT type) = 1 THEN MIN(type) END AS type,
        CASE WHEN COUNT(DISTINCT protocol) = 1 THEN MIN(protocol) END AS protocol,
        CASE WHEN COUNT(DISTINCT model) = 1 THEN MIN(model) END AS model,
        CASE WHEN COUNT(DISTINCT driver) = 1 THEN MIN(driver) END AS driver,
        CASE WHEN COUNT(DISTINCT nodeWwn) = 1 THEN MIN(nodeWwn) END AS nodeWwn,
        CASE WHEN COUNT(DISTINCT portWwn) = 1 THEN MIN(portWwn) END AS portWwn
    FROM adapters WHERE device IS NOT NULL OR key IS NOT NULL
    GROUP BY host_id, identity
), host_adapters AS (
    SELECT host_id, to_json(list(struct_pack(device := device, key := key,
        type := type, protocol := protocol, model := model, driver := driver,
        nodeWwn := nodeWwn, portWwn := portWwn)
        ORDER BY COALESCE(device, key), identity)) AS records
    FROM adapter_records GROUP BY host_id
), luns AS (
    SELECT TRIM(to_json(s)->>'Host ID') AS host_id,
        NULLIF(TRIM(to_json(s)->>'Canonical Name'), '') AS canonicalName,
        NULLIF(TRIM(to_json(s)->>'SCSI Disk Key'), '') AS key,
        NULLIF(TRIM(to_json(s)->>'LUN UUID'), '') AS lunUuid,
        NULLIF(TRIM(to_json(s)->>'LUN Vendor'), '') AS vendor,
        NULLIF(TRIM(to_json(s)->>'LUN Model'), '') AS model,
        NULLIF(TRIM(to_json(s)->>'HBA Device'), '') AS adapter
    FROM %s s
), lun_records AS (
    SELECT host_id, COALESCE('name:' || canonicalName, 'key:' || key, 'uuid:' || lunUuid) AS identity,
        CASE WHEN COUNT(DISTINCT canonicalName) = 1 THEN MIN(canonicalName) END AS canonicalName,
        CASE WHEN COUNT(DISTINCT key) = 1 THEN MIN(key) END AS key,
        CASE WHEN COUNT(DISTINCT lunUuid) = 1 THEN MIN(lunUuid) END AS lunUuid,
        CASE WHEN COUNT(DISTINCT vendor) = 1 THEN MIN(vendor) END AS vendor,
        CASE WHEN COUNT(DISTINCT model) = 1 THEN MIN(model) END AS model,
        list(DISTINCT adapter ORDER BY adapter) FILTER (WHERE adapter IS NOT NULL) AS adapterDevices
    FROM luns WHERE canonicalName IS NOT NULL OR key IS NOT NULL OR lunUuid IS NOT NULL
    GROUP BY host_id, identity
), host_luns AS (
    SELECT host_id, to_json(list(struct_pack(canonicalName := canonicalName,
        key := key, lunUuid := lunUuid, vendor := vendor, model := model,
        adapterDevices := adapterDevices)
        ORDER BY COALESCE(canonicalName, key, lunUuid), identity)) AS records
    FROM lun_records GROUP BY host_id
)
SELECT owner.host_id, COALESCE(a.records, '[]')::VARCHAR, COALESCE(l.records, '[]')::VARCHAR
FROM unique_owners owner
LEFT JOIN host_adapters a ON a.host_id = owner.host_id
LEFT JOIN host_luns l ON l.host_id = owner.host_id`, hba, scsi)
	rows, err := p.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	result := make(map[string]hostStorageFacts)
	for rows.Next() {
		var id, adapters, luns string
		if err := rows.Scan(&id, &adapters, &luns); err != nil {
			return nil, err
		}
		var facts hostStorageFacts
		if err := json.Unmarshal([]byte(adapters), &facts.adapters); err != nil {
			return nil, fmt.Errorf("decoding host adapters: %w", err)
		}
		if err := json.Unmarshal([]byte(luns), &facts.luns); err != nil {
			return nil, fmt.Errorf("decoding host LUNs: %w", err)
		}
		result[id] = facts
	}
	return result, rows.Err()
}
