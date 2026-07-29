package duckdb_parser

import (
	"context"
	"crypto/md5"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"

	_ "github.com/marcboeker/go-duckdb/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sqliteCluster describes a cluster for createTestSQLite.
type sqliteCluster struct {
	id         string
	name       string
	datacenter string // datacenter name (matched to a Datacenter row)
}

// sqliteVM describes a VM for createTestSQLite.
type sqliteVM struct {
	id            string
	name          string
	clusterName   string // must match a sqliteCluster.name
	ipAddress     string // primary IP; defaults to "" when empty
	nics          string // JSON array; defaults to "[]" when empty
	guestNetworks string // JSON array; defaults to "[]" when empty
}

// createTestSQLite builds a minimal forklift SQLite database at a temp path and returns
// that path. Tables match what ingest_sqlite.go.tmpl expects.
func createTestSQLite(t *testing.T, instanceUUID string, clusters []sqliteCluster, vms []sqliteVM) string {
	t.Helper()

	sqlitePath := filepath.Join(t.TempDir(), "test-forklift.db")

	// Use an in-memory DuckDB to create and populate the SQLite file via ATTACH.
	db, err := sql.Open("duckdb", "")
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	stmts := []string{
		fmt.Sprintf("ATTACH '%s' AS dst (TYPE sqlite)", sqlitePath),

		// About
		`CREATE TABLE dst.About (InstanceUuid VARCHAR)`,
		fmt.Sprintf(`INSERT INTO dst.About VALUES ('%s')`, escapeSQLString(instanceUUID)),

		// Datacenter
		`CREATE TABLE dst.Datacenter (ID VARCHAR PRIMARY KEY, Name VARCHAR)`,

		// Folder (needed for Cluster→Datacenter join via Cluster.Parent->>'id')
		`CREATE TABLE dst.Folder (ID VARCHAR PRIMARY KEY, Datacenter VARCHAR)`,

		// Cluster
		`CREATE TABLE dst.Cluster (ID VARCHAR PRIMARY KEY, Name VARCHAR, Parent VARCHAR)`,

		// Host
		`CREATE TABLE dst.Host (ID VARCHAR PRIMARY KEY, Cluster VARCHAR, CpuCores INTEGER, CpuSockets INTEGER, MemoryBytes BIGINT, Model VARCHAR, Vendor VARCHAR, Datastores VARCHAR)`,

		// VM
		`CREATE TABLE dst.VM (
			ID VARCHAR PRIMARY KEY, Name VARCHAR, Folder VARCHAR, Host VARCHAR,
			UUID VARCHAR, Firmware VARCHAR, PowerState VARCHAR, ConnectionState VARCHAR,
			FaultToleranceEnabled INTEGER, CpuCount INTEGER, MemoryMB INTEGER,
			GuestName VARCHAR, GuestNameFromVmwareTools VARCHAR, HostName VARCHAR,
			IpAddress VARCHAR, StorageUsed BIGINT, IsTemplate INTEGER,
			ChangeTrackingEnabled INTEGER, DiskEnableUuid INTEGER,
			Disks VARCHAR, NICs VARCHAR,
			CpuHotAddEnabled INTEGER, CpuHotRemoveEnabled INTEGER, CoresPerSocket INTEGER,
			MemoryHotAddEnabled INTEGER, BalloonedMemory INTEGER,
			GuestApps VARCHAR DEFAULT '[]',
			GuestNetworks VARCHAR DEFAULT '[]'
		)`,

		// Network (referenced by ingest template; empty is fine for these tests)
		`CREATE TABLE dst.Network (ID VARCHAR PRIMARY KEY, Name VARCHAR, DVSwitch VARCHAR, VlanId VARCHAR)`,

		// Datastore
		`CREATE TABLE dst.Datastore (ID VARCHAR PRIMARY KEY, Name VARCHAR, Free BIGINT, Capacity BIGINT, MaintenanceMode VARCHAR, Type VARCHAR, BackingDevicesNames VARCHAR)`,
	}

	for _, stmt := range stmts {
		_, err := db.Exec(stmt)
		require.NoError(t, err, "setup stmt: %s", stmt)
	}

	// Insert clusters, folders, and datacenters.
	dcSeen := make(map[string]int)
	for i, c := range clusters {
		// Datacenter (one per unique name)
		if _, exists := dcSeen[c.datacenter]; !exists {
			dcID := fmt.Sprintf("datacenter-%d", len(dcSeen)+1)
			dcSeen[c.datacenter] = len(dcSeen) + 1
			_, err := db.Exec(fmt.Sprintf(
				`INSERT INTO dst.Datacenter VALUES ('%s', '%s')`,
				escapeSQLString(dcID), escapeSQLString(c.datacenter),
			))
			require.NoError(t, err)

			// Folder bridging cluster→datacenter
			folderID := fmt.Sprintf("folder-%d", dcSeen[c.datacenter])
			_, err = db.Exec(fmt.Sprintf(
				`INSERT INTO dst.Folder VALUES ('%s', '%s')`,
				escapeSQLString(folderID), escapeSQLString(dcID),
			))
			require.NoError(t, err)
		}

		dcIdx := dcSeen[c.datacenter]
		folderID := fmt.Sprintf("folder-%d", dcIdx)
		parentJSON := fmt.Sprintf(`{"id":"%s"}`, folderID)
		_, err := db.Exec(fmt.Sprintf(
			`INSERT INTO dst.Cluster VALUES ('%s', '%s', '%s')`,
			escapeSQLString(c.id), escapeSQLString(c.name), escapeSQLString(parentJSON),
		))
		require.NoError(t, err)

		// Insert one host per cluster so Host→Cluster join resolves.
		hostID := fmt.Sprintf("host-%d", i+1)
		_, err = db.Exec(fmt.Sprintf(
			`INSERT INTO dst.Host VALUES ('%s', '%s', 8, 2, 34359738368, 'ESXi', 'VMware', '[]')`,
			escapeSQLString(hostID), escapeSQLString(c.id),
		))
		require.NoError(t, err)

		// Insert VMs that belong to this cluster.
		for _, vm := range vms {
			if vm.clusterName != c.name {
				continue
			}
			nics := vm.nics
			if nics == "" {
				nics = "[]"
			}
			guestNetworks := vm.guestNetworks
			if guestNetworks == "" {
				guestNetworks = "[]"
			}
			_, err = db.Exec(fmt.Sprintf(
				`INSERT INTO dst.VM (
					ID, Name, Folder, Host, UUID, Firmware, PowerState, ConnectionState,
					FaultToleranceEnabled, CpuCount, MemoryMB, GuestName, GuestNameFromVmwareTools,
					HostName, IpAddress, StorageUsed, IsTemplate, ChangeTrackingEnabled, DiskEnableUuid,
					Disks, NICs, CpuHotAddEnabled, CpuHotRemoveEnabled, CoresPerSocket,
					MemoryHotAddEnabled, BalloonedMemory, GuestApps, GuestNetworks
				) VALUES (
					'%s', '%s', 'folder-1', '%s',
					'%s', 'bios', 'poweredOn', 'connected',
					0, 4, 8192, 'rhel', 'rhel', '', '%s',
					10737418240, 0, 0, 0, '[]', '%s',
					0, 0, 2, 0, 0, '[]', '%s'
				)`,
				escapeSQLString(vm.id),
				escapeSQLString(vm.name),
				escapeSQLString(hostID),
				escapeSQLString(vm.id+"-uuid"),
				escapeSQLString(vm.ipAddress),
				escapeSQLString(nics),
				escapeSQLString(guestNetworks),
			))
			require.NoError(t, err)
		}
	}

	_, err = db.Exec("DETACH dst")
	require.NoError(t, err)

	return sqlitePath
}

func TestIngestSqlite_PopulatesVCluster(t *testing.T) {
	ctx := context.Background()
	parser, db, cleanup := setupTestParser(t, &testValidator{})
	defer cleanup()

	clusters := []sqliteCluster{
		{id: "domain-c1", name: "cluster1", datacenter: "dc1"},
		{id: "domain-c2", name: "cluster2", datacenter: "dc1"},
	}
	vms := []sqliteVM{
		{id: "vm-001", name: "vm-1", clusterName: "cluster1"},
		{id: "vm-002", name: "vm-2", clusterName: "cluster2"},
	}
	sqlitePath := createTestSQLite(t, "vcenter-uuid-001", clusters, vms)

	result, err := parser.IngestSqlite(ctx, sqlitePath)
	require.NoError(t, err)
	require.True(t, result.IsValid())

	// vcluster must have one row per cluster.
	rows, err := db.QueryContext(ctx, `SELECT "Name", "Object ID" FROM vcluster ORDER BY "Name"`)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()

	vclusterMap := make(map[string]string)
	for rows.Next() {
		var name, objectID string
		require.NoError(t, rows.Scan(&name, &objectID))
		vclusterMap[name] = objectID
	}
	require.NoError(t, rows.Err())

	assert.Len(t, vclusterMap, 2, "vcluster should have one row per cluster")

	// Each Object ID must equal what generateClusterID would produce.
	vcenterID, err := parser.VCenterID(ctx)
	require.NoError(t, err)
	clusterDatacenters, err := parser.ClusterDatacenters(ctx)
	require.NoError(t, err)

	for _, c := range clusters {
		expectedID := generateClusterID(c.name, clusterDatacenters[c.name], vcenterID)
		assert.Equal(t, expectedID, vclusterMap[c.name],
			"Object ID for cluster %q should match generateClusterID output", c.name)
	}
}

func TestIngestSqlite_VClusterMatchesInventoryClusterKeys(t *testing.T) {
	ctx := context.Background()
	parser, db, cleanup := setupTestParser(t, &testValidator{})
	defer cleanup()

	clusters := []sqliteCluster{
		{id: "domain-c10", name: "prod-cluster", datacenter: "main-dc"},
		{id: "domain-c20", name: "dev-cluster", datacenter: "main-dc"},
	}
	vms := []sqliteVM{
		{id: "vm-101", name: "prod-vm-1", clusterName: "prod-cluster"},
		{id: "vm-201", name: "dev-vm-1", clusterName: "dev-cluster"},
	}
	sqlitePath := createTestSQLite(t, "vcenter-uuid-xyz", clusters, vms)

	result, err := parser.IngestSqlite(ctx, sqlitePath)
	require.NoError(t, err)
	require.True(t, result.IsValid())

	inv, err := parser.BuildInventory(ctx, nil)
	require.NoError(t, err)

	// Collect vcluster Object IDs.
	rows, err := db.QueryContext(ctx, `SELECT "Object ID" FROM vcluster`)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()

	vclusterIDs := make(map[string]bool)
	for rows.Next() {
		var id string
		require.NoError(t, rows.Scan(&id))
		vclusterIDs[id] = true
	}
	require.NoError(t, rows.Err())

	// Every key in Inventory.Clusters must appear in vcluster.
	for clusterID := range inv.Clusters {
		assert.True(t, vclusterIDs[clusterID],
			"Inventory cluster key %q should be present in vcluster", clusterID)
	}

	// vcluster must not contain IDs absent from Inventory.Clusters.
	assert.Len(t, vclusterIDs, len(inv.Clusters),
		"vcluster row count should match Inventory.Clusters count")
}

func TestIngestSqlite_NICsGetPerNICIPsFromGuestNetworks(t *testing.T) {
	ctx := context.Background()
	parser, db, cleanup := setupTestParser(t, &testValidator{})
	defer cleanup()

	clusters := []sqliteCluster{
		{id: "domain-c1", name: "cluster1", datacenter: "dc1"},
	}
	vms := []sqliteVM{
		{
			id:          "vm-001",
			name:        "vm-1",
			clusterName: "cluster1",
			ipAddress:   "10.0.0.1",
			nics: `[` +
				`{"network":{"kind":"Network","id":"net-1"},"mac":"aa:bb:cc:dd:ee:01","order":0,"deviceKey":100},` +
				`{"network":{"kind":"Network","id":"net-1"},"mac":"aa:bb:cc:dd:ee:02","order":1,"deviceKey":101}` +
				`]`,
			guestNetworks: `[` +
				`{"mac":"aa:bb:cc:dd:ee:01","ip":"10.0.0.1","prefix":24,"device":"eth0","network":"VM Network","origin":"","deviceConfigId":100},` +
				`{"mac":"aa:bb:cc:dd:ee:02","ip":"10.0.0.2","prefix":24,"device":"eth1","network":"VM Network","origin":"","deviceConfigId":101}` +
				`]`,
		},
	}
	sqlitePath := createTestSQLite(t, "vcenter-uuid-001", clusters, vms)

	result, err := parser.IngestSqlite(ctx, sqlitePath)
	require.NoError(t, err)
	require.True(t, result.IsValid())

	rows, err := db.QueryContext(ctx,
		`SELECT "Mac Address", "IPv4 Address" FROM vnetwork WHERE "VM ID" = 'vm-001' ORDER BY "Mac Address"`)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()

	type nicRow struct{ mac, ipv4 string }
	var nics []nicRow
	for rows.Next() {
		var r nicRow
		require.NoError(t, rows.Scan(&r.mac, &r.ipv4))
		nics = append(nics, r)
	}
	require.NoError(t, rows.Err())

	require.Len(t, nics, 2)
	assert.Equal(t, "10.0.0.1", nics[0].ipv4, "first NIC should have its own IP from GuestNetworks")
	assert.Equal(t, "10.0.0.2", nics[1].ipv4, "second NIC should have its own IP, not the VM primary IP")
}

func TestIngestSqlite_NICsFallBackToPrimaryIPWhenGuestNetworksEmpty(t *testing.T) {
	ctx := context.Background()
	parser, db, cleanup := setupTestParser(t, &testValidator{})
	defer cleanup()

	clusters := []sqliteCluster{
		{id: "domain-c1", name: "cluster1", datacenter: "dc1"},
	}
	vms := []sqliteVM{
		{
			id:          "vm-002",
			name:        "vm-2",
			clusterName: "cluster1",
			ipAddress:   "192.168.1.50",
			nics: `[` +
				`{"network":{"kind":"Network","id":"net-1"},"mac":"bb:cc:dd:ee:ff:01","order":0,"deviceKey":200},` +
				`{"network":{"kind":"Network","id":"net-1"},"mac":"bb:cc:dd:ee:ff:02","order":1,"deviceKey":201}` +
				`]`,
			// guestNetworks intentionally empty — should fall back to v.IpAddress
		},
	}
	sqlitePath := createTestSQLite(t, "vcenter-uuid-002", clusters, vms)

	result, err := parser.IngestSqlite(ctx, sqlitePath)
	require.NoError(t, err)
	require.True(t, result.IsValid())

	rows, err := db.QueryContext(ctx,
		`SELECT "Mac Address", "IPv4 Address" FROM vnetwork WHERE "VM ID" = 'vm-002' ORDER BY "Mac Address"`)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()

	type nicRow struct{ mac, ipv4 string }
	var nics []nicRow
	for rows.Next() {
		var r nicRow
		require.NoError(t, rows.Scan(&r.mac, &r.ipv4))
		nics = append(nics, r)
	}
	require.NoError(t, rows.Err())

	require.Len(t, nics, 2)
	assert.Equal(t, "192.168.1.50", nics[0].ipv4, "should fall back to VM primary IP when GuestNetworks is empty")
	assert.Equal(t, "192.168.1.50", nics[1].ipv4, "should fall back to VM primary IP when GuestNetworks is empty")
}

func TestIngestSqlite_PopulateVCluster_SkipsWhenAlreadyPopulated(t *testing.T) {
	ctx := context.Background()
	parser, db, cleanup := setupTestParser(t, &testValidator{})
	defer cleanup()

	// Pre-populate vcluster with a sentinel row.
	_, err := db.ExecContext(ctx, `INSERT INTO vcluster ("Name", "Object ID") VALUES ('existing-cluster', 'sentinel-id')`)
	require.NoError(t, err)

	clusters := []sqliteCluster{
		{id: "domain-c1", name: "cluster1", datacenter: "dc1"},
	}
	vms := []sqliteVM{
		{id: "vm-001", name: "vm-1", clusterName: "cluster1"},
	}
	sqlitePath := createTestSQLite(t, "vcenter-uuid-001", clusters, vms)

	result, err := parser.IngestSqlite(ctx, sqlitePath)
	require.NoError(t, err)
	require.True(t, result.IsValid())

	// vcluster must still contain only the pre-existing row.
	var count int
	require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM vcluster`).Scan(&count))
	assert.Equal(t, 1, count, "populateVCluster should not insert when vcluster is already populated")

	var objectID string
	require.NoError(t, db.QueryRowContext(ctx, `SELECT "Object ID" FROM vcluster WHERE "Name" = 'existing-cluster'`).Scan(&objectID))
	assert.Equal(t, "sentinel-id", objectID, "pre-existing row must not be overwritten")
}

// addCollectionColumns adds vmmoid and collection_id columns to vinfo so that
// IngestSqliteWithCollection can write to them. In production these columns are
// added by agent migration 026; in tests we add them manually.
func addCollectionColumns(t *testing.T, db *sql.DB) {
	t.Helper()
	ctx := context.Background()
	_, err := db.ExecContext(ctx, `ALTER TABLE vinfo ADD COLUMN IF NOT EXISTS vmmoid VARCHAR`)
	require.NoError(t, err, "adding vmmoid column to vinfo")
	_, err = db.ExecContext(ctx, `ALTER TABLE vinfo ADD COLUMN IF NOT EXISTS collection_id BIGINT`)
	require.NoError(t, err, "adding collection_id column to vinfo")
}

func TestIngestSqliteWithCollection_HashesVMIDsAndSetsVmmoid(t *testing.T) {
	const collectionID = int64(7)
	ctx := context.Background()

	parser, db, cleanup := setupTestParser(t, &testValidator{})
	defer cleanup()

	addCollectionColumns(t, db)

	clusters := []sqliteCluster{
		{id: "domain-c1", name: "cluster1", datacenter: "dc1"},
	}
	vms := []sqliteVM{
		{id: "vm-001", name: "vm-1", clusterName: "cluster1"},
		{id: "vm-002", name: "vm-2", clusterName: "cluster1"},
	}
	sqlitePath := createTestSQLite(t, "vcenter-uuid-001", clusters, vms)

	result, err := parser.IngestSqliteWithCollection(ctx, sqlitePath, collectionID)
	require.NoError(t, err)
	require.True(t, result.IsValid())

	rows, err := db.QueryContext(ctx, `SELECT "VM ID", vmmoid, collection_id FROM vinfo`)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()

	rowCount := 0
	for rows.Next() {
		rowCount++
		var vmID, vmmoid string
		var colID int64
		require.NoError(t, rows.Scan(&vmID, &vmmoid, &colID))

		// "VM ID" must be a 32-char lowercase hex md5 string.
		assert.Len(t, vmID, 32, "VM ID should be a 32-char md5 hex string")
		assert.Regexp(t, `^[0-9a-f]{32}$`, vmID, "VM ID should be a lowercase hex string")

		// vmmoid is the original MOID (non-empty, not itself a 32-char hash).
		assert.NotEmpty(t, vmmoid, "vmmoid should be the original MOID")
		assert.NotEqual(t, 32, len(vmmoid), "vmmoid should not look like an md5 hash (original MOIDs are shorter)")

		// collection_id must be set correctly.
		assert.Equal(t, collectionID, colID, "collection_id should match the supplied collectionID")

		// Hash must be deterministic: md5("{collectionID}_{vmmoid}") == vmID
		expectedHash := fmt.Sprintf("%x", md5.Sum(fmt.Appendf(nil, "%d_%s", collectionID, vmmoid)))
		assert.Equal(t, expectedHash, vmID, "VM ID should equal md5('%d_%s', collectionID, vmmoid)")
	}
	require.NoError(t, rows.Err())
	assert.Equal(t, len(vms), rowCount, "vinfo should have one row per non-template VM")
}

func TestIngestSqliteWithCollection_RelationalTablesMatchVinfoIDs(t *testing.T) {
	const collectionID = int64(7)
	ctx := context.Background()

	parser, db, cleanup := setupTestParser(t, &testValidator{})
	defer cleanup()

	addCollectionColumns(t, db)

	clusters := []sqliteCluster{
		{id: "domain-c1", name: "cluster1", datacenter: "dc1"},
	}
	vms := []sqliteVM{
		{id: "vm-001", name: "vm-1", clusterName: "cluster1"},
		{id: "vm-002", name: "vm-2", clusterName: "cluster1"},
	}
	sqlitePath := createTestSQLite(t, "vcenter-uuid-001", clusters, vms)

	_, err := parser.IngestSqliteWithCollection(ctx, sqlitePath, collectionID)
	require.NoError(t, err)

	// Every vcpu "VM ID" must exist in vinfo "VM ID" — no orphaned rows.
	var orphans int
	err = db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM vcpu
		WHERE "VM ID" NOT IN (SELECT "VM ID" FROM vinfo)
	`).Scan(&orphans)
	require.NoError(t, err)
	assert.Equal(t, 0, orphans, "all vcpu rows should reference a valid vinfo VM ID")
}

func TestIngestSqliteWithCollection_ZeroCollectionIDMatchesIngestSqlite(t *testing.T) {
	ctx := context.Background()

	clusters := []sqliteCluster{
		{id: "domain-c1", name: "cluster1", datacenter: "dc1"},
	}
	vms := []sqliteVM{
		{id: "vm-001", name: "vm-1", clusterName: "cluster1"},
		{id: "vm-002", name: "vm-2", clusterName: "cluster1"},
	}

	// Parser A uses IngestSqlite (the zero-collection-ID path).
	parserA, dbA, cleanupA := setupTestParser(t, &testValidator{})
	defer cleanupA()

	// Parser B uses IngestSqliteWithCollection with collectionID=0.
	parserB, dbB, cleanupB := setupTestParser(t, &testValidator{})
	defer cleanupB()

	sqlitePathA := createTestSQLite(t, "vcenter-uuid-001", clusters, vms)
	sqlitePathB := createTestSQLite(t, "vcenter-uuid-001", clusters, vms)

	resultA, err := parserA.IngestSqlite(ctx, sqlitePathA)
	require.NoError(t, err)

	resultB, err := parserB.IngestSqliteWithCollection(ctx, sqlitePathB, 0)
	require.NoError(t, err)

	assert.Equal(t, resultA.IsValid(), resultB.IsValid(), "validity should match between IngestSqlite and IngestSqliteWithCollection(0)")

	var countA, countB int
	require.NoError(t, dbA.QueryRowContext(ctx, `SELECT COUNT(*) FROM vinfo`).Scan(&countA))
	require.NoError(t, dbB.QueryRowContext(ctx, `SELECT COUNT(*) FROM vinfo`).Scan(&countB))
	assert.Equal(t, countA, countB, "VM count should match between IngestSqlite and IngestSqliteWithCollection(0)")
}
