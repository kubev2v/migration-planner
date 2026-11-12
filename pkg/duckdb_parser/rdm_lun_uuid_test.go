package duckdb_parser

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRDMLunUUIDIngestSQLite(t *testing.T) {
	p, db, cleanup := setupTestParser(t, nil)
	defer cleanup()
	path := createTestSQLite(t, "vcenter-1", []sqliteCluster{{id: "c1", name: "cluster", datacenter: "dc"}}, []sqliteVM{{id: "vm-1", name: "rdm-vm", clusterName: "cluster"}})
	source, err := sql.Open("duckdb", "")
	require.NoError(t, err)
	_, err = source.Exec(fmt.Sprintf("ATTACH '%s' AS src (TYPE sqlite)", path))
	require.NoError(t, err)
	_, err = source.Exec(`UPDATE src.VM SET Disks = ?`, `[
 {"key":2000,"rdm":true,"lunUuid":"  lun-uuid-A  ","serial":"different-backing-A","capacity":1048576,"file":"[datastore1] vm/a.vmdk"},
 {"key":2001,"rdm":true,"lunUuid":"lun-uuid-B","capacity":2097152,"file":"[datastore1] vm/b.vmdk"},
 {"key":2002,"rdm":false,"lunUuid":"must-not-leak","serial":"ordinary-serial","capacity":3145728,"file":"[datastore1] vm/c.vmdk"},
 {"key":2003,"rdm":true,"serial":"not-a-lun-uuid","capacity":4194304,"file":"[datastore1] vm/d.vmdk"},
 {"key":2004,"rdm":true,"deviceName":"/vmfs/devices/disks/naa.ver2","physicalMode":true,"capacity":5242880,"file":"[datastore1] vm/e.vmdk"},
 {"key":2005,"rdm":true,"lunUuid":"   ","capacity":6291456,"file":"[datastore1] vm/f.vmdk"}
 ]`)
	require.NoError(t, err)
	require.NoError(t, source.Close())
	result, err := p.IngestSqlite(context.Background(), path)
	require.NoError(t, err)
	require.True(t, result.IsValid(), "%v", result.Errors)
	assertRDMLunUUIDRows(t, db, true)
}

func TestRDMLunUUIDIngestRVTools(t *testing.T) {
	for _, withColumn := range []bool{true, false} {
		t.Run(fmt.Sprintf("RawLUNIDPresent=%t", withColumn), func(t *testing.T) {
			p, db, cleanup := setupTestParser(t, nil)
			defer cleanup()
			sheets := defaultStandardSheets([]map[string]string{{"VM ID": "vm-1", "VM": "rdm-vm", "VI SDK UUID": "vcenter-1", "Host": "esxi-host-1", "Cluster": "cluster", "Datacenter": "dc", "Template": "false"}}, []map[string]string{{"Host": "esxi-host-1", "Object ID": "host-1", "Cluster": "cluster", "Datacenter": "dc"}})
			headers := []string{"VM ID", "Disk Key", "Unit #", "Path", "Disk Path", "Capacity MiB", "Sharing mode", "Raw", "Shared Bus", "Disk Mode", "Disk UUID", "Thin", "Controller", "Label", "SCSI Unit #"}
			if withColumn {
				headers = append(headers, "Raw LUN ID")
			}
			rows := []map[string]string{}
			rawIDs := []string{"  lun-uuid-A  ", "lun-uuid-B", "must-not-leak", "", "", "   "}
			for i, id := range rawIDs {
				raw := "true"
				if i == 2 {
					raw = "false"
				}
				rows = append(rows, map[string]string{"VM ID": "vm-1", "Disk Key": fmt.Sprint(2000 + i), "Unit #": fmt.Sprint(i), "Path": fmt.Sprintf("[datastore1] vm/%d.vmdk", i), "Capacity MiB": fmt.Sprint(i + 1), "Raw": raw, "Raw LUN ID": id, "Disk UUID": "not-a-lun-uuid", "Controller": "scsi0", "SCSI Unit #": fmt.Sprint(i)})
			}
			sheets = append(sheets, NewExcelSheet("vDisk", headers, rows))
			result, err := p.IngestRvTools(context.Background(), createTestExcel(t, sheets...))
			require.NoError(t, err)
			require.True(t, result.IsValid(), "%v", result.Errors)
			assertRDMLunUUIDRows(t, db, withColumn)
		})
	}
}

func assertRDMLunUUIDRows(t *testing.T, db *sql.DB, known bool) {
	t.Helper()
	rows, err := db.Query(`SELECT "Disk Key", "Raw", "Capacity MiB", "RDM LUN UUID" FROM vdisk ORDER BY "Disk Key"`)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	count := 0
	for rows.Next() {
		var key string
		var raw bool
		var capacity int64
		var uuid sql.NullString
		require.NoError(t, rows.Scan(&key, &raw, &capacity, &uuid))
		require.Equal(t, fmt.Sprint(2000+count), key)
		require.Equal(t, count != 2, raw)
		require.Equal(t, int64(count+1), capacity)
		switch {
		case known && count == 0:
			require.Equal(t, sql.NullString{String: "lun-uuid-A", Valid: true}, uuid)
		case known && count == 1:
			require.Equal(t, sql.NullString{String: "lun-uuid-B", Valid: true}, uuid)
		default:
			require.False(t, uuid.Valid, "disk %s must not get a guessed LUN UUID", key)
		}
		count++
	}
	require.NoError(t, rows.Err())
	require.Equal(t, 6, count, "all disks must survive ingestion")
}

func TestRDMLunUUIDQuery(t *testing.T) {
	p, db, cleanup := setupTestParser(t, nil)
	defer cleanup()
	_, err := db.Exec(`INSERT INTO vinfo ("VM ID", "VM", "Template") VALUES ('vm-1','rdm-vm',false);
 INSERT INTO vdatastore ("Object ID", "Name", "Type", "Backing Devices") VALUES ('ds-1','shared-ds','VMFS','["naa.datastore-extent"]');
 INSERT INTO vdisk ("VM ID", "Disk Key", "Path", "Raw", "Capacity MiB", "Disk UUID", "RDM LUN UUID") VALUES
 ('vm-1','2000','[shared-ds] vm/a.vmdk',true,1,'backing-A','lun-uuid-A'),
 ('vm-1','2001','[shared-ds] vm/b.vmdk',true,2,'backing-B','lun-uuid-B'),
 ('vm-1','2002','[shared-ds] vm/c.vmdk',false,3,'normal-serial','must-not-leak'),
 ('vm-1','2003','[shared-ds] vm/d.vmdk',true,4,'not-a-lun-uuid',NULL);`)
	require.NoError(t, err)
	vms, err := p.VMs(context.Background(), Filters{VmId: "vm-1"}, Options{})
	require.NoError(t, err)
	require.Len(t, vms, 1)
	require.Len(t, vms[0].Disks, 4)
	for _, disk := range vms[0].Disks {
		data, err := json.Marshal(disk)
		require.NoError(t, err)
		var got map[string]any
		require.NoError(t, json.Unmarshal(data, &got))
		switch disk.Key {
		case "2000":
			require.Equal(t, map[string]any{"lunUuid": "lun-uuid-A"}, got["rdmBacking"])
		case "2001":
			require.Equal(t, map[string]any{"lunUuid": "lun-uuid-B"}, got["rdmBacking"])
		default:
			require.NotContains(t, got, "rdmBacking")
		}
		require.Equal(t, "shared-ds", disk.DatastoreName)
		require.Equal(t, []string{"naa.datastore-extent"}, disk.DatastoreBackingDeviceIDs)
	}
}

func TestRDMLunUUIDLegacyReadOnlyCollection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.duckdb")
	db, err := sql.Open("duckdb", path)
	require.NoError(t, err)
	p := New(db, nil)
	require.NoError(t, p.Init())
	_, err = db.Exec(`ALTER TABLE vdisk DROP COLUMN "RDM LUN UUID";
 INSERT INTO vinfo ("VM ID","VM","Template") VALUES ('vm-1','legacy-vm',false);
 INSERT INTO vdisk ("VM ID","Disk Key","Raw","Capacity MiB","Disk UUID") VALUES ('vm-1','2000',true,100,'not-a-lun-uuid');`)
	require.NoError(t, err)
	require.NoError(t, db.Close())
	readonly, err := sql.Open("duckdb", path+"?access_mode=READ_ONLY")
	require.NoError(t, err)
	defer func() { _ = readonly.Close() }()
	vms, err := New(readonly, nil).VMs(context.Background(), Filters{}, Options{})
	require.NoError(t, err)
	require.Len(t, vms, 1)
	require.Len(t, vms[0].Disks, 1)
	require.True(t, vms[0].Disks[0].RDM)
	require.Equal(t, int64(100), vms[0].Disks[0].Capacity)
	data, err := json.Marshal(vms[0].Disks[0])
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(data, &got))
	require.NotContains(t, got, "rdmBacking")
	var columns int
	require.NoError(t, readonly.QueryRow(`SELECT count(*) FROM information_schema.columns WHERE table_name='vdisk' AND column_name='RDM LUN UUID'`).Scan(&columns))
	require.Zero(t, columns, "querying a legacy collection must not alter it")
}

func TestRDMLunUUIDInitUpgradesWritableCollection(t *testing.T) {
	p, db, cleanup := setupTestParser(t, nil)
	defer cleanup()
	_, err := db.Exec(`ALTER TABLE vdisk DROP COLUMN "RDM LUN UUID";
 INSERT INTO vinfo ("VM ID","VM") VALUES ('vm-1','legacy-vm');
 INSERT INTO vdisk ("VM ID","Raw","Capacity MiB") VALUES ('vm-1',true,100);`)
	require.NoError(t, err)
	require.NoError(t, p.Init())
	require.NoError(t, p.Init())
	var capacity int64
	var uuid sql.NullString
	require.NoError(t, db.QueryRow(`SELECT "Capacity MiB", "RDM LUN UUID" FROM vdisk`).Scan(&capacity, &uuid))
	require.Equal(t, int64(100), capacity)
	require.False(t, uuid.Valid)
}
