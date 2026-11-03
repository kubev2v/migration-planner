package duckdb_parser

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// Storage columns remain pre-metadata; unrelated network columns match the current VM query.
func newPreStorageDatabase(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("duckdb", path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	schema, err := os.ReadFile("testdata/pre_storage_schema.sql")
	require.NoError(t, err)
	_, err = db.Exec(string(schema))
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO vinfo ("VM ID", "VM", "Template", "Cluster") VALUES ('vm-1', 'legacy-vm', false, 'cluster1');
INSERT INTO vhost ("Object ID", "Host", "Cluster") VALUES ('host-1', 'esxi.example.com', 'cluster1');
INSERT INTO vhba ("Device", "Type") VALUES ('vmhba64', 'ISCSI');
INSERT INTO vdatastore ("Object ID", "Name", "Hosts", "Type", "Address", "Capacity MiB", "Free MiB", "Backing Devices")
VALUES ('ds-1', 'legacy-ds', 'esxi.example.com', 'VMFS', 'vmhba64', 2048, 1024, '["naa.extent"]');
INSERT INTO vdisk ("VM ID", "Disk Key", "Path", "Raw", "Capacity MiB", "Disk UUID")
VALUES ('vm-1', '2000', '[legacy-ds] vm/disk.vmdk', true, 100, 'not-a-lun-uuid');`)
	require.NoError(t, err)
	return db
}

func assertPreStorageQueries(t *testing.T, db *sql.DB, vendor, model string) {
	t.Helper()
	p := New(db, nil)
	vms, err := p.VMs(context.Background(), Filters{}, Options{})
	require.NoError(t, err)
	require.Len(t, vms, 1)
	require.Equal(t, "legacy-vm", vms[0].Name)
	require.Len(t, vms[0].Disks, 1)
	disk := vms[0].Disks[0]
	require.True(t, disk.RDM)
	require.Nil(t, disk.RDMBacking)
	require.Equal(t, int64(100), disk.Capacity)
	require.Equal(t, "legacy-ds", disk.DatastoreName)
	require.Equal(t, "VMFS", disk.DatastoreType)
	require.Equal(t, []string{"naa.extent"}, disk.DatastoreBackingDeviceIDs)
	require.Empty(t, disk.DatastoreFilesystemType)
	require.Zero(t, disk.DatastoreFilesystemMajorVersion)
	require.Empty(t, disk.DatastoreStorageProtocol)
	require.Equal(t, vendor, disk.DatastoreVendor)
	require.Equal(t, model, disk.DatastoreModel)
	for _, filters := range []Filters{{}, {Cluster: "cluster1"}} {
		datastores, err := p.Datastores(context.Background(), filters, Options{})
		require.NoError(t, err)
		require.Len(t, datastores, 1)
		ds := datastores[0]
		require.Equal(t, "legacy-ds", ds.DiskId)
		require.Equal(t, "host-1", ds.HostId)
		require.Equal(t, float64(2), ds.TotalCapacityGB)
		require.Equal(t, float64(1), ds.FreeCapacityGB)
		require.Empty(t, ds.FilesystemType)
		require.Zero(t, ds.FilesystemMajorVersion)
		require.Equal(t, "N/A", ds.ProtocolType)
		if vendor == "" {
			require.Equal(t, "N/A", ds.Vendor)
			require.Equal(t, "N/A", ds.Model)
		} else {
			require.Equal(t, vendor, ds.Vendor)
			require.Equal(t, model, ds.Model)
		}
	}
}

func TestPreStorageCollectionReadOnly(t *testing.T) {
	for _, tc := range []struct {
		name       string
		members    string
		withoutHBA bool
		withSCSI   bool
	}{
		{name: "legacy HBA only"},
		{name: "legacy host IDs", members: "host-1"},
		{name: "no adapter tables", withoutHBA: true},
		{name: "LUN metadata with legacy HBA", withSCSI: true},
		{name: "LUN metadata without HBA", withoutHBA: true, withSCSI: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "legacy.duckdb")
			db := newPreStorageDatabase(t, path)
			if tc.members != "" {
				_, err := db.Exec(`UPDATE vdatastore SET "Hosts"=?`, tc.members)
				require.NoError(t, err)
			}
			if tc.withoutHBA {
				_, err := db.Exec(`DROP TABLE vhba`)
				require.NoError(t, err)
			}
			vendor, model := "", ""
			if tc.withSCSI {
				_, err := db.Exec(`CREATE TABLE vhostscsi ("Host ID" VARCHAR, "HBA Device" VARCHAR, "Canonical Name" VARCHAR, "LUN Vendor" VARCHAR, "LUN Model" VARCHAR);
INSERT INTO vhostscsi VALUES ('host-1', 'vmhba64', 'naa.extent', 'NETAPP', 'legacy-lun');`)
				require.NoError(t, err)
				vendor, model = "NETAPP", "legacy-lun"
			}
			require.NoError(t, db.Close())
			readonly, err := sql.Open("duckdb", path+"?access_mode=READ_ONLY")
			require.NoError(t, err)
			t.Cleanup(func() { _ = readonly.Close() })
			assertPreStorageQueries(t, readonly, vendor, model)
			var columns int
			require.NoError(t, readonly.QueryRow(`SELECT count(*) FROM information_schema.columns
WHERE table_name IN ('vdatastore', 'vdisk', 'vhba') AND column_name IN
('Filesystem Type', 'Filesystem Major Version', 'Storage Protocol', 'RDM LUN UUID', 'Host ID', 'Protocol')`).Scan(&columns))
			require.Zero(t, columns, "read-only queries must not upgrade the collection")
		})
	}
}

func TestPreStorageCollectionInitUpgrade(t *testing.T) {
	db := newPreStorageDatabase(t, "")
	p := New(db, nil)
	require.NoError(t, p.Init())
	require.NoError(t, p.Init())
	assertPreStorageQueries(t, db, "", "")
	var fsType, protocol, hbaType sql.NullString
	var version sql.NullInt32
	require.NoError(t, db.QueryRow(`SELECT "Filesystem Type", "Filesystem Major Version", "Storage Protocol" FROM vdatastore`).Scan(&fsType, &version, &protocol))
	require.False(t, fsType.Valid)
	require.False(t, version.Valid)
	require.False(t, protocol.Valid)
	require.NoError(t, db.QueryRow(`SELECT "Type" FROM vhba WHERE "Device"='vmhba64'`).Scan(&hbaType))
	require.Equal(t, "ISCSI", hbaType.String)
	_, err := db.Exec(`UPDATE vdatastore SET "Filesystem Type"='VMFS', "Filesystem Major Version"=6, "Storage Protocol"='iSCSI';
UPDATE vhba SET "Host ID"='host-1', "HBA Key"='hba-key', "Protocol"='iSCSI', "Model"='adapter', "Driver"='driver', "Node WWN"='node', "Port WWN"='port';`)
	require.NoError(t, err)
	require.NoError(t, p.Init())
	require.NoError(t, db.QueryRow(`SELECT "Filesystem Type", "Filesystem Major Version", "Storage Protocol" FROM vdatastore`).Scan(&fsType, &version, &protocol))
	require.Equal(t, "VMFS", fsType.String)
	require.Equal(t, int32(6), version.Int32)
	require.Equal(t, "iSCSI", protocol.String)
}

func TestPreStorageCollectionInitUpgradesMigrationHBA(t *testing.T) {
	db := newPreStorageDatabase(t, "")
	// The Agent storage migration creates this smaller table when vhba is absent.
	_, err := db.Exec(`DROP TABLE vhba;
CREATE TABLE vhba ("Host ID" VARCHAR, "Device" VARCHAR, "Protocol" VARCHAR);
INSERT INTO vhba VALUES ('host-1', 'vmhba64', 'iSCSI');`)
	require.NoError(t, err)
	p := New(db, nil)
	require.NoError(t, p.Init())
	_, err = db.Exec(`INSERT INTO vhba ("Host ID", "Device", "Type", "HBA Key", "Protocol", "Model", "Driver", "Node WWN", "Port WWN")
VALUES ('host-2', 'vmhba65', 'ISCSI', 'adapter-key', 'iSCSI', 'adapter', 'driver', 'node', 'port');`)
	require.NoError(t, err, "the upgraded schema must accept all columns written by ingestion")
	require.NoError(t, p.Init())
	var protocol, adapterType string
	require.NoError(t, db.QueryRow(`SELECT "Protocol" FROM vhba WHERE "Host ID"='host-1'`).Scan(&protocol))
	require.Equal(t, "iSCSI", protocol)
	require.NoError(t, db.QueryRow(`SELECT "Type" FROM vhba WHERE "Host ID"='host-2'`).Scan(&adapterType))
	require.Equal(t, "ISCSI", adapterType)
}
