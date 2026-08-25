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

	"github.com/kubev2v/migration-planner/pkg/duckdb_parser/models"
	"github.com/kubev2v/migration-planner/pkg/inventory"
)

// sqliteCluster describes a cluster for createTestSQLite.
type sqliteCluster struct {
	id          string
	name        string
	datacenter  string // datacenter name (matched to a Datacenter row)
	dasEnabled  bool
	drsEnabled  bool
	drsBehavior string
}

// sqliteVM describes a VM for createTestSQLite.
type sqliteVM struct {
	id                    string
	name                  string
	clusterName           string // must match a sqliteCluster.name
	ipAddress             string // primary IP; defaults to "" when empty
	nics                  string // JSON array; defaults to "[]" when empty
	guestNetworks         string // JSON array; defaults to "[]" when empty
	faultToleranceEnabled bool
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

		// About (all three columns needed so the template's INSERT INTO about succeeds
		// and VCenterID() returns the real UUID for cluster ID hashing)
		`CREATE TABLE dst.About ("APIVersion" VARCHAR, "Product" VARCHAR, "InstanceUuid" VARCHAR)`,
		fmt.Sprintf(`INSERT INTO dst.About VALUES ('7.0', 'VMware vCenter', '%s')`, escapeSQLString(instanceUUID)),

		// Datacenter
		`CREATE TABLE dst.Datacenter (ID VARCHAR PRIMARY KEY, Name VARCHAR)`,

		// Folder (needed for Cluster→Datacenter join via Cluster.Parent->>'id')
		`CREATE TABLE dst.Folder (ID VARCHAR PRIMARY KEY, Datacenter VARCHAR)`,

		// Cluster
		`CREATE TABLE dst.Cluster (ID VARCHAR PRIMARY KEY, Name VARCHAR, Parent VARCHAR, DasEnabled BOOLEAN DEFAULT false, DrsEnabled BOOLEAN DEFAULT false, DrsBehavior VARCHAR DEFAULT '')`,

		// Host
		`CREATE TABLE dst.Host (ID VARCHAR PRIMARY KEY, Cluster VARCHAR, CpuCores INTEGER, CpuSockets INTEGER, MemoryBytes BIGINT, Model VARCHAR, Vendor VARCHAR, Datastores VARCHAR, VMotionSupported BOOLEAN DEFAULT false, StorageVMotionSupported BOOLEAN DEFAULT false)`,

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
		`CREATE TABLE dst.Datastore (ID VARCHAR PRIMARY KEY, Name VARCHAR, Free BIGINT, Capacity BIGINT, MaintenanceMode VARCHAR, Type VARCHAR, BackingDevicesNames VARCHAR, IORMEnabled BOOLEAN, IORMCongestionThreshold INTEGER, IORMCongestionThresholdMode VARCHAR, IORMPercentOfPeakThroughput INTEGER)`,
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
			`INSERT INTO dst.Cluster VALUES ('%s', '%s', '%s', %t, %t, '%s')`,
			escapeSQLString(c.id), escapeSQLString(c.name), escapeSQLString(parentJSON),
			c.dasEnabled, c.drsEnabled, escapeSQLString(c.drsBehavior),
		))
		require.NoError(t, err)

		// Insert one host per cluster so Host→Cluster join resolves.
		hostID := fmt.Sprintf("host-%d", i+1)
		_, err = db.Exec(fmt.Sprintf(
			`INSERT INTO dst.Host VALUES ('%s', '%s', 8, 2, 34359738368, 'ESXi', 'VMware', '[]', true, true)`,
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
			ftEnabled := 0
			if vm.faultToleranceEnabled {
				ftEnabled = 1
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
					%d, 4, 8192, 'rhel', 'rhel', '', '%s',
					10737418240, 0, 0, 0, '[]', '%s',
					0, 0, 2, 0, 0, '[]', '%s'
				)`,
				escapeSQLString(vm.id),
				escapeSQLString(vm.name),
				escapeSQLString(hostID),
				escapeSQLString(vm.id+"-uuid"),
				ftEnabled,
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

	// Each Object ID must match the anonymized hash that generateClusterID produces.
	// This also proves DuckDB's sha256() matches Go's crypto/sha256.
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

func TestIngestSqlite_PopulatesVClusterFeatures(t *testing.T) {
	ctx := context.Background()
	parser, db, cleanup := setupTestParser(t, &testValidator{})
	defer cleanup()

	clusters := []sqliteCluster{
		{id: "domain-c1", name: "cluster-ha-on", datacenter: "dc1", dasEnabled: true, drsEnabled: true, drsBehavior: "fullyAutomated"},
		{id: "domain-c2", name: "cluster-ha-off", datacenter: "dc1"},
	}
	vms := []sqliteVM{
		{id: "vm-001", name: "vm-1", clusterName: "cluster-ha-on"},
		{id: "vm-002", name: "vm-2", clusterName: "cluster-ha-off"},
	}
	sqlitePath := createTestSQLite(t, "vcenter-uuid-002", clusters, vms)

	result, err := parser.IngestSqlite(ctx, sqlitePath)
	require.NoError(t, err)
	require.True(t, result.IsValid())

	rows, err := db.QueryContext(ctx, `SELECT "Name", "Object ID", "DasEnabled", "DrsEnabled", "DrsDefaultVmBehavior" FROM vcluster ORDER BY "Name"`)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()

	type gotFeatures struct {
		objectID string
		das      bool
		drs      bool
		drsMode  string
	}
	got := make(map[string]gotFeatures)
	for rows.Next() {
		var name, objectID, drsMode string
		var das, drs bool
		require.NoError(t, rows.Scan(&name, &objectID, &das, &drs, &drsMode))
		got[name] = gotFeatures{objectID: objectID, das: das, drs: drs, drsMode: drsMode}
	}
	require.NoError(t, rows.Err())

	require.Contains(t, got, "cluster-ha-on")
	assert.Regexp(t, `^cluster-[0-9a-f]{16}$`, got["cluster-ha-on"].objectID, "Object ID must be anonymized")
	assert.True(t, got["cluster-ha-on"].das, "HA should be enabled")
	assert.True(t, got["cluster-ha-on"].drs, "DRS should be enabled")
	assert.Equal(t, "fullyAutomated", got["cluster-ha-on"].drsMode, "DRS mode should be wired from forklift DrsBehavior")

	require.Contains(t, got, "cluster-ha-off")
	assert.Regexp(t, `^cluster-[0-9a-f]{16}$`, got["cluster-ha-off"].objectID, "Object ID must be anonymized")
	assert.False(t, got["cluster-ha-off"].das, "HA should be disabled")
	assert.False(t, got["cluster-ha-off"].drs, "DRS should be disabled")
	assert.Equal(t, "None", got["cluster-ha-off"].drsMode, "empty DrsBehavior should default to None")

	inv, err := parser.BuildInventory(ctx, nil)
	require.NoError(t, err)

	onID, offID := got["cluster-ha-on"].objectID, got["cluster-ha-off"].objectID
	var onCluster, offCluster *inventory.InventoryData
	for id, cluster := range inv.Clusters {
		c := cluster
		switch id {
		case onID:
			onCluster = &c
		case offID:
			offCluster = &c
		}
	}

	require.NotNil(t, onCluster)
	require.NotNil(t, onCluster.ClusterFeatures)
	require.NotNil(t, onCluster.ClusterFeatures.HaEnabled)
	assert.True(t, *onCluster.ClusterFeatures.HaEnabled)
	require.NotNil(t, onCluster.ClusterFeatures.DrsEnabled)
	assert.True(t, *onCluster.ClusterFeatures.DrsEnabled)
	require.NotNil(t, onCluster.ClusterFeatures.DrsMode)
	assert.Equal(t, "fullyAutomated", *onCluster.ClusterFeatures.DrsMode)

	require.NotNil(t, offCluster)
	require.NotNil(t, offCluster.ClusterFeatures)
	require.NotNil(t, offCluster.ClusterFeatures.HaEnabled)
	assert.False(t, *offCluster.ClusterFeatures.HaEnabled)
	require.NotNil(t, offCluster.ClusterFeatures.DrsEnabled)
	assert.False(t, *offCluster.ClusterFeatures.DrsEnabled)
	require.NotNil(t, offCluster.ClusterFeatures.DrsMode)
	assert.Equal(t, "None", *offCluster.ClusterFeatures.DrsMode)
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

func TestIngestSqlite_VHostReadsVMotionFromForklift(t *testing.T) {
	ctx := context.Background()
	parser, db, cleanup := setupTestParser(t, &testValidator{})
	defer cleanup()

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

	var vmotionSupport, storageVmotionSupport bool
	err = db.QueryRowContext(ctx,
		`SELECT "VMotion support", "Storage VMotion support" FROM vhost LIMIT 1`).
		Scan(&vmotionSupport, &storageVmotionSupport)
	require.NoError(t, err)
	assert.True(t, vmotionSupport, "VMotion support should be true, not hard-coded false")
	assert.True(t, storageVmotionSupport, "Storage VMotion support should be true, not hard-coded false")
}

// TestIngestSqlite_FaultToleranceEnabledMapping validates that the agent's
// FaultToleranceEnabled boolean (0/1) is translated into vCenter FT State
// vocabulary ('notConfigured'/'enabled') on ingest.
func TestIngestSqlite_FaultToleranceEnabledMapping(t *testing.T) {
	ctx := context.Background()
	parser, db, cleanup := setupTestParser(t, &testValidator{})
	defer cleanup()

	clusters := []sqliteCluster{
		{id: "domain-c1", name: "cluster1", datacenter: "dc1"},
	}
	vms := []sqliteVM{
		{id: "vm-ft-on", name: "vm-ft-on", clusterName: "cluster1", faultToleranceEnabled: true},
		{id: "vm-ft-off", name: "vm-ft-off", clusterName: "cluster1", faultToleranceEnabled: false},
	}
	sqlitePath := createTestSQLite(t, "vcenter-uuid-ft", clusters, vms)

	result, err := parser.IngestSqlite(ctx, sqlitePath)
	require.NoError(t, err)
	require.True(t, result.IsValid())

	var ftState string
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT "FT State" FROM vinfo WHERE "VM ID" = 'vm-ft-on'`).Scan(&ftState))
	assert.Equal(t, "enabled", ftState)

	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT "FT State" FROM vinfo WHERE "VM ID" = 'vm-ft-off'`).Scan(&ftState))
	assert.Equal(t, "notConfigured", ftState)

	// End-to-end: predicate must resolve these through the real VMs() query path.
	vmsOut, err := parser.VMs(ctx, Filters{}, Options{})
	require.NoError(t, err)
	vmMap := make(map[string]models.VM)
	for _, vm := range vmsOut {
		vmMap[vm.ID] = vm
	}
	assert.True(t, vmMap["vm-ft-on"].FaultToleranceEnabled)
	assert.False(t, vmMap["vm-ft-off"].FaultToleranceEnabled)
}

func TestIngestSqlite_DatastoreIORMValues(t *testing.T) {
	ctx := context.Background()
	parser, db, cleanup := setupTestParser(t, &testValidator{})
	defer cleanup()

	clusters := []sqliteCluster{
		{id: "domain-c1", name: "cluster1", datacenter: "dc1"},
	}
	vms := []sqliteVM{
		{id: "vm-001", name: "vm-1", clusterName: "cluster1"},
	}
	sqlitePath := createTestSQLite(t, "vcenter-uuid-001", clusters, vms)

	// Insert a datastore with non-default IORM values into the SQLite file.
	srcDB, err := sql.Open("duckdb", "")
	require.NoError(t, err)
	_, err = srcDB.Exec(fmt.Sprintf("ATTACH '%s' AS dst (TYPE sqlite)", sqlitePath))
	require.NoError(t, err)
	_, err = srcDB.Exec(`INSERT INTO dst.Datastore (ID, Name, Free, Capacity, MaintenanceMode, Type, BackingDevicesNames, IORMEnabled, IORMCongestionThreshold) VALUES ('ds-1', 'datastore-1', 1073741824, 2147483648, 'False', 'VMFS', '[]', true, 50)`)
	require.NoError(t, err)
	_, err = srcDB.Exec("DETACH dst")
	require.NoError(t, err)
	require.NoError(t, srcDB.Close())

	result, err := parser.IngestSqlite(ctx, sqlitePath)
	require.NoError(t, err)
	require.True(t, result.IsValid())

	var siocEnabled bool
	var congestionThreshold int
	err = db.QueryRowContext(ctx,
		`SELECT "SIOC Enabled", "SIOC Congestion Threshold" FROM vdatastore WHERE "Object ID" = 'ds-1'`).
		Scan(&siocEnabled, &congestionThreshold)
	require.NoError(t, err)

	assert.True(t, siocEnabled, "SIOC Enabled should be true")
	assert.Equal(t, 50, congestionThreshold, "SIOC Congestion Threshold should be 50, not the default 30")
}
