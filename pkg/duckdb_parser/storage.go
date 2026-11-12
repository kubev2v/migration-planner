package duckdb_parser

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

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
	tables, err := storageTables(ctx, db)
	if err != nil {
		return "", err
	}
	return storageTopologyQuery(tables.hba, tables.scsi, vmOnly), nil
}

type storageTableSources struct {
	hostSource string
	hba        string
	scsi       string
}

func storageTables(ctx context.Context, db store.QueryInterceptor) (storageTableSources, error) {
	rows, err := db.QueryContext(ctx, `SELECT table_name, column_name FROM information_schema.columns
WHERE table_catalog = current_database() AND table_schema = 'main'
    AND table_name IN ('vhost', 'vhba', 'vhostscsi')`)
	if err != nil {
		return storageTableSources{}, fmt.Errorf("reading storage topology columns: %w", err)
	}
	defer func() { _ = rows.Close() }()

	columns := make(map[string]map[string]bool)
	for rows.Next() {
		var table, column string
		if err := rows.Scan(&table, &column); err != nil {
			return storageTableSources{}, fmt.Errorf("scanning storage topology column: %w", err)
		}
		if columns[table] == nil {
			columns[table] = make(map[string]bool)
		}
		columns[table][column] = true
	}
	if err := rows.Err(); err != nil {
		return storageTableSources{}, fmt.Errorf("reading storage topology columns: %w", err)
	}

	tables := storageTableSources{hostSource: "NULL::VARCHAR"}
	if columns["vhost"]["VI SDK UUID"] {
		tables.hostSource = `h."VI SDK UUID"`
	}

	// Project only storage columns; NULL fallbacks keep old read-only files usable.
	for _, table := range []struct {
		name    string
		columns []string
		query   *string
	}{
		{"vhba", []string{"Host ID", "Device", "Type", "HBA Key", "Protocol", "Model", "Driver", "Node WWN", "Port WWN"}, &tables.hba},
		{"vhostscsi", []string{"Host ID", "HBA Device", "SCSI Disk Key", "Canonical Name", "LUN Vendor", "LUN Model", "LUN UUID"}, &tables.scsi},
	} {
		projection := make([]string, 0, len(table.columns))
		for _, column := range table.columns {
			expression := "NULL::VARCHAR"
			if columns[table.name][column] {
				expression = fmt.Sprintf(`CAST("%s" AS VARCHAR)`, column)
			}
			projection = append(projection, fmt.Sprintf(`%s AS "%s"`, expression, column))
		}
		from := " WHERE false"
		if len(columns[table.name]) > 0 {
			from = " FROM " + table.name
		}
		*table.query = "(SELECT " + strings.Join(projection, ", ") + from + ")"
	}
	return tables, nil
}

func storageTopologyQuery(hba, scsi string, vmOnly bool) string {
	return datastoreResolutionQuery(vmOnly) + ",\n" + fmt.Sprintf(`storage_topology_hosts AS (
    SELECT host_id
    FROM storage_hosts
    WHERE NULLIF(TRIM(host_id), '') IS NOT NULL
    GROUP BY host_id
    HAVING COUNT(*) = 1
),
storage_hbas AS (
    SELECT
        h."Host ID" AS "Host ID",
        h."Device" AS "Device",
        h."Type" AS "Type",
        h."Protocol" AS "Protocol"
    FROM %s h
    JOIN storage_topology_hosts host ON host.host_id = (h."Host ID")
),
storage_scsi AS (
    SELECT
        s."Host ID" AS "Host ID",
        s."HBA Device" AS "HBA Device",
        s."Canonical Name" AS "Canonical Name",
        s."LUN Vendor" AS "LUN Vendor",
        s."LUN Model" AS "LUN Model"
    FROM %s s
    JOIN storage_topology_hosts host ON host.host_id = (s."Host ID")
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
func (p *Parser) hostStorage(ctx context.Context) (map[string]hostStorageFacts, error) {
	tables, err := storageTables(ctx, p.db)
	if err != nil {
		return nil, err
	}
	query := fmt.Sprintf(`WITH owners AS (
    SELECT DISTINCT TRIM("Object ID") AS host_id, LOWER(TRIM("Host")) AS host_name,
        COALESCE("Cluster", '') AS cluster,
        COALESCE(h."Datacenter", '') AS datacenter,
        COALESCE(NULLIF(LOWER(TRIM(%s)), 'vm'), '') AS source,
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
    SELECT TRIM(h."Host ID") AS host_id,
        NULLIF(TRIM(h."Device"), '') AS device,
        NULLIF(TRIM(h."HBA Key"), '') AS key,
        NULLIF(TRIM(h."Type"), '') AS type,
        NULLIF(TRIM(h."Protocol"), '') AS protocol,
        NULLIF(TRIM(h."Model"), '') AS model,
        NULLIF(TRIM(h."Driver"), '') AS driver,
        NULLIF(TRIM(h."Node WWN"), '') AS nodeWwn,
        NULLIF(TRIM(h."Port WWN"), '') AS portWwn
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
    SELECT TRIM(s."Host ID") AS host_id,
        NULLIF(TRIM(s."Canonical Name"), '') AS canonicalName,
        NULLIF(TRIM(s."SCSI Disk Key"), '') AS key,
        NULLIF(TRIM(s."LUN UUID"), '') AS lunUuid,
        NULLIF(TRIM(s."LUN Vendor"), '') AS vendor,
        NULLIF(TRIM(s."LUN Model"), '') AS model,
        NULLIF(TRIM(s."HBA Device"), '') AS adapter
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
LEFT JOIN host_luns l ON l.host_id = owner.host_id`, tables.hostSource, tables.hba, tables.scsi)
	rows, err := p.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("querying host storage: %w", err)
	}
	defer func() { _ = rows.Close() }()
	result := make(map[string]hostStorageFacts)
	for rows.Next() {
		var id, adapters, luns string
		if err := rows.Scan(&id, &adapters, &luns); err != nil {
			return nil, fmt.Errorf("scanning host storage row: %w", err)
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
