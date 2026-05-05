package duckdb_parser

import (
	"context"
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
	id          string
	name        string
	clusterName string // must match a sqliteCluster.name
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
			MemoryHotAddEnabled INTEGER, BalloonedMemory INTEGER
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
			_, err = db.Exec(fmt.Sprintf(
				`INSERT INTO dst.VM VALUES (
					'%s', '%s', 'folder-1', '%s',
					'%s', 'bios', 'poweredOn', 'connected',
					0, 4, 8192, 'rhel', 'rhel', '', '',
					10737418240, 0, 0, 0, '[]', '[]',
					0, 0, 2, 0, 0
				)`,
				escapeSQLString(vm.id),
				escapeSQLString(vm.name),
				escapeSQLString(hostID),
				escapeSQLString(vm.id+"-uuid"),
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

	inv, err := parser.BuildInventory(ctx)
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
