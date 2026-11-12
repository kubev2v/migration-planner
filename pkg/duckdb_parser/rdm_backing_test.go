package duckdb_parser

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kubev2v/migration-planner/pkg/duckdb_parser/models"
)

// TestRDMBackingIngestSQLite proves source-faithful ingestion of the Forklift
// disk backing fields: explicit mode precedence, the conservative historical
// fallback, identifier scoping, nullable flags, and match provenance.
func TestRDMBackingIngestSQLite(t *testing.T) {
	p, db, cleanup := setupTestParser(t, nil)
	defer cleanup()
	path := createTestSQLite(t, "vcenter-1", []sqliteCluster{{id: "c1", name: "cluster", datacenter: "dc"}}, []sqliteVM{{id: "vm-1", name: "rdm-vm", clusterName: "cluster"}})
	source, err := sql.Open("duckdb", "")
	require.NoError(t, err)
	_, err = source.Exec(fmt.Sprintf("ATTACH '%s' AS src (TYPE sqlite)", path))
	require.NoError(t, err)
	_, err = source.Exec(`UPDATE src.VM SET Disks = ?`, `[
 {"key":1000,"rdm":true,"compatibilityMode":"physicalMode","backingUuid":"backing-A","deviceName":"/vmfs/devices/disks/naa.A","lunUuid":"lun-A","capacity":1048576,"file":"[datastore1] vm/a.vmdk"},
 {"key":1001,"rdm":true,"compatibilityMode":"virtualMode","capacity":2097152,"file":"[datastore1] vm/b.vmdk"},
 {"key":1002,"rdm":true,"compatibilityMode":"virtualMode","physicalMode":true,"capacity":3145728,"file":"[datastore1] vm/c.vmdk"},
 {"key":1003,"rdm":true,"physicalMode":true,"capacity":4194304,"file":"[datastore1] vm/d.vmdk"},
 {"key":1004,"rdm":true,"physicalMode":false,"capacity":5242880,"file":"[datastore1] vm/e.vmdk"},
 {"key":1005,"rdm":true,"capacity":6291456,"file":"[datastore1] vm/f.vmdk"},
 {"key":1006,"rdm":true,"compatibilityMode":"   ","physicalMode":true,"capacity":7340032,"file":"[datastore1] vm/g.vmdk"},
 {"key":1007,"rdm":true,"compatibilityMode":"bogus","physicalMode":true,"capacity":8388608,"file":"[datastore1] vm/h.vmdk"},
 {"key":1008,"rdm":true,"lunUuid":"lun-B","thinProvisioned":true,"eagerlyScrub":false,"capacity":9437184,"file":"[datastore1] vm/i.vmdk"},
 {"key":1009,"rdm":false,"compatibilityMode":"physicalMode","backingUuid":"backing-X","deviceName":"/vmfs/devices/disks/naa.X","lunUuid":"must-not-leak","thinProvisioned":false,"eagerlyScrub":true,"capacity":10485760,"file":"[datastore1] vm/j.vmdk"},
 {"key":1010,"rdm":false,"thinProvisioned":false,"capacity":11534336,"file":"[datastore1] vm/k.vmdk"}
 ]`)
	require.NoError(t, err)
	// The host reports a SCSI LUN whose uuid equals the first disk's lunUuid;
	// the round trip through ingestion must retain it and match that disk.
	_, err = source.Exec(`UPDATE src.Host SET HostScsiDisks = ? WHERE ID = 'host-1'`, `[
 {"key":"key-vmlun-1","canonicalName":"naa.001","vendor":"PURE","model":"FlashArray","uuid":"lun-A"}
 ]`)
	require.NoError(t, err)
	require.NoError(t, source.Close())
	result, err := p.IngestSqlite(context.Background(), path)
	require.NoError(t, err)
	require.True(t, result.IsValid(), "%v", result.Errors)

	type expectation struct {
		mode        sql.NullString
		deviceName  sql.NullString
		backingUUID sql.NullString
		lunUUID     sql.NullString
		lunUUIDSrc  sql.NullString
		thin        sql.NullBool
		eager       sql.NullBool
	}
	cases := map[string]expectation{
		"1000": {mode: ns("physical"), deviceName: ns("/vmfs/devices/disks/naa.A"), backingUUID: ns("backing-A"), lunUUID: ns("lun-A"), lunUUIDSrc: ns("forklift-rdm")},
		"1001": {mode: ns("virtual")},
		"1002": {mode: ns("virtual")},
		"1003": {mode: ns("physical")},
		"1004": {mode: ns("unknown")},
		"1005": {mode: ns("unknown")},
		"1006": {mode: ns("physical")},
		"1007": {mode: ns("unknown")},
		"1008": {mode: ns("unknown"), lunUUID: ns("lun-B"), lunUUIDSrc: ns("forklift-rdm"), thin: nb(true), eager: nb(false)},
		"1009": {thin: nb(false), eager: nb(true)},
		"1010": {thin: nb(false)},
	}
	// Disks that must expose a backing object, keyed to the normalized mode.
	backingModes := map[string]string{
		"1000": "physical",
		"1001": "virtual",
		"1002": "virtual",
		"1003": "physical",
		"1006": "physical",
		"1008": "unknown",
	}
	// The matched host LUN, reached through the ingested host SCSI uuid.
	hostLuns := map[string]*models.RDMHostLun{
		"1000": {HostID: "host-1", Key: "key-vmlun-1", CanonicalName: "naa.001", LunUUID: "lun-A"},
	}

	rows, err := db.Query(`SELECT "Disk Key", "RDM Compatibility Mode", "RDM Device Name", "RDM Backing UUID", "RDM LUN UUID", "RDM LUN UUID Source", "Thin Provisioned", "Eagerly Scrubbed" FROM vdisk`)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	count := 0
	for rows.Next() {
		var key string
		var got expectation
		require.NoError(t, rows.Scan(&key, &got.mode, &got.deviceName, &got.backingUUID, &got.lunUUID, &got.lunUUIDSrc, &got.thin, &got.eager))
		require.Equal(t, cases[key], got, "disk %s", key)
		count++
	}
	require.NoError(t, rows.Err())
	require.Equal(t, len(cases), count, "all disks must survive ingestion")

	// The SQLite host SCSI uuid must land in vhostscsi with the same value.
	var hostLunUUID string
	require.NoError(t, db.QueryRow(`SELECT "LUN UUID" FROM vhostscsi WHERE "Host ID" = 'host-1'`).Scan(&hostLunUUID))
	require.Equal(t, "lun-A", hostLunUUID)

	vms, err := p.VMs(context.Background(), Filters{VmId: "vm-1"}, Options{})
	require.NoError(t, err)
	require.Len(t, vms, 1)
	require.Len(t, vms[0].Disks, len(cases))
	for _, disk := range vms[0].Disks {
		want := cases[disk.Key]
		wantMode, wantBacking := backingModes[disk.Key]
		if !wantBacking {
			require.Nil(t, disk.RDMBacking, "disk %s must not get an empty backing object", disk.Key)
		} else {
			require.NotNil(t, disk.RDMBacking, "disk %s", disk.Key)
			require.Equal(t, wantMode, disk.RDMBacking.CompatibilityMode, "disk %s", disk.Key)
		}

		var gotHostLun *models.RDMHostLun
		if disk.RDMBacking != nil {
			gotHostLun = disk.RDMBacking.HostLun
		}
		require.Equal(t, hostLuns[disk.Key], gotHostLun, "disk %s", disk.Key)
		if want.thin.Valid {
			require.Equal(t, want.thin.Bool, *disk.ThinProvisioned, "disk %s", disk.Key)
		} else {
			require.Nil(t, disk.ThinProvisioned, "disk %s", disk.Key)
		}
		if want.eager.Valid {
			require.Equal(t, want.eager.Bool, *disk.EagerlyScrubbed, "disk %s", disk.Key)
		} else {
			require.Nil(t, disk.EagerlyScrubbed, "disk %s", disk.Key)
		}
	}
}

// TestRDMBackingIngestRVTools proves RVTools mode and nullable-flag parsing,
// including the established Thin spellings and the conservative Eagerly Scrub
// contract, and that no device name or backing UUID is invented.
func TestRDMBackingIngestRVTools(t *testing.T) {
	p, db, cleanup := setupTestParser(t, nil)
	defer cleanup()
	sheets := defaultStandardSheets([]map[string]string{{"VM ID": "vm-1", "VM": "rdm-vm", "VI SDK UUID": "vcenter-1", "Host": "esxi-host-1", "Cluster": "cluster", "Datacenter": "dc", "Template": "false"}}, []map[string]string{{"Host": "esxi-host-1", "Object ID": "host-1", "Cluster": "cluster", "Datacenter": "dc"}})
	headers := []string{"VM ID", "Disk Key", "Unit #", "Path", "Disk Path", "Capacity MiB", "Sharing mode", "Raw", "Shared Bus", "Disk Mode", "Disk UUID", "Raw LUN ID", "Raw Comp. Mode", "Thin", "Eagerly Scrub", "Controller", "Label", "SCSI Unit #"}
	rows := []map[string]string{
		{"Disk Key": "2000", "Raw": "true", "Raw LUN ID": "lun-A", "Raw Comp. Mode": "physicalMode", "Thin": "1", "Eagerly Scrub": "true"},
		{"Disk Key": "2001", "Raw": "true", "Raw LUN ID": "lun-B", "Raw Comp. Mode": " VirtualMode ", "Thin": "yes", "Eagerly Scrub": "false"},
		{"Disk Key": "2002", "Raw": "true", "Raw LUN ID": "lun-C", "Raw Comp. Mode": "physical", "Thin": "0", "Eagerly Scrub": "1"},
		{"Disk Key": "2003", "Raw": "true", "Raw LUN ID": "", "Raw Comp. Mode": "", "Thin": "no", "Eagerly Scrub": ""},
		{"Disk Key": "2004", "Raw": "true", "Raw LUN ID": "lun-D", "Raw Comp. Mode": "bogus", "Thin": "bogus-thin", "Eagerly Scrub": "bogus-scrub"},
		{"Disk Key": "2005", "Raw": "false", "Raw LUN ID": "must-not-leak", "Raw Comp. Mode": "physicalMode", "Thin": "false", "Eagerly Scrub": "false"},
		{"Disk Key": "2006", "Raw Comp. Mode": "physicalMode"},
	}
	for _, row := range rows {
		row["VM ID"] = "vm-1"
		row["Path"] = fmt.Sprintf("[datastore1] vm/%s.vmdk", row["Disk Key"])
		row["Capacity MiB"] = "1"
	}
	sheets = append(sheets, NewExcelSheet("vDisk", headers, rows))
	result, err := p.IngestRvTools(context.Background(), createTestExcel(t, sheets...))
	require.NoError(t, err)
	require.True(t, result.IsValid(), "%v", result.Errors)

	type expectation struct {
		mode       sql.NullString
		lunUUID    sql.NullString
		lunUUIDSrc sql.NullString
		thin       sql.NullBool
		eager      sql.NullBool
	}
	cases := map[string]expectation{
		"2000": {mode: ns("physical"), lunUUID: ns("lun-A"), lunUUIDSrc: ns("rvtools-raw-lun-id"), thin: nb(true), eager: nb(true)},
		"2001": {mode: ns("virtual"), lunUUID: ns("lun-B"), lunUUIDSrc: ns("rvtools-raw-lun-id"), thin: nb(true), eager: nb(false)},
		"2002": {mode: ns("physical"), lunUUID: ns("lun-C"), lunUUIDSrc: ns("rvtools-raw-lun-id"), thin: nb(false)},
		"2003": {mode: ns("unknown"), thin: nb(false)},
		"2004": {mode: ns("unknown"), lunUUID: ns("lun-D"), lunUUIDSrc: ns("rvtools-raw-lun-id")},
		"2005": {thin: nb(false), eager: nb(false)},
		// A disk without a reported Raw value never stores an RDM mode.
		"2006": {},
	}

	queryRows, err := db.Query(`SELECT "Disk Key", "RDM Compatibility Mode", "RDM LUN UUID", "RDM LUN UUID Source", "Thin Provisioned", "Eagerly Scrubbed", "RDM Device Name", "RDM Backing UUID" FROM vdisk`)
	require.NoError(t, err)
	defer func() { _ = queryRows.Close() }()
	count := 0
	for queryRows.Next() {
		var key string
		var got expectation
		var deviceName, backingUUID sql.NullString
		require.NoError(t, queryRows.Scan(&key, &got.mode, &got.lunUUID, &got.lunUUIDSrc, &got.thin, &got.eager, &deviceName, &backingUUID))
		require.Equal(t, cases[key], got, "disk %s", key)
		require.False(t, deviceName.Valid, "disk %s must not get an invented device name", key)
		require.False(t, backingUUID.Valid, "disk %s must not get an invented backing UUID", key)
		count++
	}
	require.NoError(t, queryRows.Err())
	require.Equal(t, len(cases), count)
}

// TestRDMHostLunMatching proves the verified host-LUN resolution: matches stay
// within the VM's uniquely resolved running host, ambiguity suppresses only
// the matched-LUN reference, and unrelated provenance never joins.
func TestRDMHostLunMatching(t *testing.T) {
	p, db, cleanup := setupTestParser(t, nil)
	defer cleanup()
	_, err := db.Exec(`INSERT INTO vhost ("Object ID", "Host", "Cluster", "Datacenter", "VI SDK UUID") VALUES
 ('host-1', 'esxi-1', 'cluster', 'dc', 'vc-1'),
 ('host-2', 'esxi-2', 'cluster', 'dc', 'vc-1'),
 ('host-3', 'esxi-1', 'cluster', 'dc', 'vc-1');
INSERT INTO vhostscsi ("Host ID", "SCSI Disk Key", "Canonical Name", "LUN UUID") VALUES
 ('host-1', 'key-1', 'naa.001', 'lun-uuid-A'),
 ('host-1', 'key-1', 'naa.001', 'lun-uuid-A'),
 ('host-2', 'key-2', 'naa.002', 'lun-uuid-B'),
 ('host-2', 'key-3', 'naa.003', 'lun-uuid-B'),
 ('host-2', 'key-4', 'naa.004', 'lun-uuid-C');
INSERT INTO vinfo ("VM ID", "VM", "Host", "Host ID", "VI SDK UUID") VALUES
 ('vm-1', 'same-host', 'esxi-1', 'host-1', 'vc-1'),
 ('vm-2', 'cross-host', 'esxi-2', 'host-2', 'vc-1'),
 ('vm-3', 'no-host', '', NULL, 'vc-1'),
 ('vm-4', 'name-only', 'esxi-2', NULL, 'vc-1'),
 ('vm-5', 'name-ambiguous', 'esxi-1', NULL, 'vc-1'),
 ('vm-6', 'explicit-missing', 'esxi-1', 'host-9', 'vc-1');
INSERT INTO vdisk ("VM ID", "Disk Key", "Path", "Raw", "RDM LUN UUID", "RDM LUN UUID Source") VALUES
 ('vm-1', '1000', '[shared] vm/a.vmdk', true, 'lun-uuid-A', 'forklift-rdm'),
 ('vm-1', '1001', '[shared] vm/b.vmdk', true, 'lun-uuid-B', 'forklift-rdm'),
 ('vm-2', '2000', '[shared] vm/c.vmdk', true, 'lun-uuid-A', 'forklift-rdm'),
 ('vm-3', '3000', '[shared] vm/d.vmdk', true, 'lun-uuid-A', 'forklift-rdm'),
 ('vm-4', '4000', '[shared] vm/e.vmdk', true, 'lun-uuid-B', 'forklift-rdm'),
 ('vm-4', '4001', '[shared] vm/f.vmdk', true, 'lun-uuid-A', 'rvtools-raw-lun-id'),
 ('vm-4', '4002', '[shared] vm/g.vmdk', true, 'lun-uuid-C', 'forklift-rdm'),
 ('vm-5', '5000', '[shared] vm/h.vmdk', true, 'lun-uuid-A', 'forklift-rdm'),
 ('vm-6', '6000', '[shared] vm/i.vmdk', true, 'lun-uuid-A', 'forklift-rdm'),
 ('vm-1', '1002', '[shared] vm/j.vmdk', true, NULL, NULL);`)
	require.NoError(t, err)

	vms, err := p.VMs(context.Background(), Filters{}, Options{})
	require.NoError(t, err)
	var returned []models.Disk
	for _, vm := range vms {
		returned = append(returned, vm.Disks...)
	}
	require.Len(t, returned, 10, "no disks may be multiplied by joins")

	disks := make(map[string]models.Disk, len(returned))
	for _, disk := range returned {
		_, duplicate := disks[disk.Key]
		require.False(t, duplicate, "duplicate disk key %s", disk.Key)
		disks[disk.Key] = disk
	}

	// vm-1 on host-1: identical duplicate LUN rows collapse into one match.
	require.Equal(t, &models.RDMHostLun{HostID: "host-1", Key: "key-1", CanonicalName: "naa.001", LunUUID: "lun-uuid-A"}, disks["1000"].RDMBacking.HostLun)
	// The same VM's second RDM disk has no LUN on host-1; no host is inferred
	// from the shared datastore and no match leaks from host-2.
	require.Nil(t, disks["1001"].RDMBacking.HostLun)
	require.Equal(t, "lun-uuid-B", disks["1001"].RDMBacking.LunUUID)
	// vm-2 runs on host-2, where only lun-uuid-B exists; lun-uuid-A is on host-1.
	require.Nil(t, disks["2000"].RDMBacking.HostLun)
	require.Equal(t, "lun-uuid-A", disks["2000"].RDMBacking.LunUUID)
	// A VM without a host association never matches.
	require.Nil(t, disks["3000"].RDMBacking.HostLun)
	// vm-4 resolves by name to host-2; conflicting candidates for lun-uuid-B
	// suppress the match, and RVTools provenance never joins at all.
	require.Nil(t, disks["4000"].RDMBacking.HostLun)
	require.Nil(t, disks["4001"].RDMBacking.HostLun)
	// The unique lun-uuid-C on host-2 matches through name resolution.
	require.Equal(t, &models.RDMHostLun{HostID: "host-2", Key: "key-4", CanonicalName: "naa.004", LunUUID: "lun-uuid-C"}, disks["4002"].RDMBacking.HostLun)
	// An ambiguous host name (esxi-1 on host-1 and host-3) suppresses matching.
	require.Nil(t, disks["5000"].RDMBacking.HostLun)
	// An explicit host ID that does not resolve to a unique owner stays unresolved.
	require.Nil(t, disks["6000"].RDMBacking.HostLun)
	// A disk with no backing details keeps rdm: true without a backing object.
	require.Nil(t, disks["1002"].RDMBacking)
}

// TestRDMHostLunMatchingDuplicateDiskKeys proves the match key is the disk
// row, not the reported Disk Key: two VMs may share one key with different
// LUNs, and each keeps its own match without row inflation.
func TestRDMHostLunMatchingDuplicateDiskKeys(t *testing.T) {
	p, db, cleanup := setupTestParser(t, nil)
	defer cleanup()
	_, err := db.Exec(`INSERT INTO vhost ("Object ID", "Host", "Cluster", "Datacenter", "VI SDK UUID") VALUES
 ('host-1', 'esxi-1', 'cluster', 'dc', 'vc-1'),
 ('host-2', 'esxi-2', 'cluster', 'dc', 'vc-1');
INSERT INTO vhostscsi ("Host ID", "SCSI Disk Key", "Canonical Name", "LUN UUID") VALUES
 ('host-1', 'key-1', 'naa.001', 'lun-uuid-A'),
 ('host-2', 'key-2', 'naa.002', 'lun-uuid-B');
INSERT INTO vinfo ("VM ID", "VM", "Host", "Host ID", "VI SDK UUID") VALUES
 ('vm-1', 'dup-key-a', 'esxi-1', 'host-1', 'vc-1'),
 ('vm-2', 'dup-key-b', 'esxi-2', 'host-2', 'vc-1');
INSERT INTO vdisk ("VM ID", "Disk Key", "Raw", "RDM LUN UUID", "RDM LUN UUID Source") VALUES
 ('vm-1', '2000', true, 'lun-uuid-A', 'forklift-rdm'),
 ('vm-2', '2000', true, 'lun-uuid-B', 'forklift-rdm');`)
	require.NoError(t, err)

	vms, err := p.VMs(context.Background(), Filters{}, Options{})
	require.NoError(t, err)
	require.Len(t, vms, 2)
	byVM := make(map[string]models.Disk)
	for _, vm := range vms {
		require.Len(t, vm.Disks, 1, "no disks may be multiplied by joins")
		require.Equal(t, "2000", vm.Disks[0].Key)
		byVM[vm.ID] = vm.Disks[0]
	}
	require.Equal(t, &models.RDMHostLun{HostID: "host-1", Key: "key-1", CanonicalName: "naa.001", LunUUID: "lun-uuid-A"}, byVM["vm-1"].RDMBacking.HostLun)
	require.Equal(t, &models.RDMHostLun{HostID: "host-2", Key: "key-2", CanonicalName: "naa.002", LunUUID: "lun-uuid-B"}, byVM["vm-2"].RDMBacking.HostLun)
}

// TestRDMHostLunMatchingScopesVMSource proves a host owner from another
// vCenter never receives the match, while a legacy owner without source
// identity stays eligible.
func TestRDMHostLunMatchingScopesVMSource(t *testing.T) {
	p, db, cleanup := setupTestParser(t, nil)
	defer cleanup()
	_, err := db.Exec(`INSERT INTO vhost ("Object ID", "Host", "Cluster", "Datacenter", "VI SDK UUID") VALUES
 ('host-1', 'esxi-1', 'cluster', 'dc', 'vc-1'),
 ('host-4', 'esxi-4', 'cluster', 'dc', NULL);
INSERT INTO vhostscsi ("Host ID", "SCSI Disk Key", "Canonical Name", "LUN UUID") VALUES
 ('host-1', 'key-1', 'naa.001', 'lun-uuid-A'),
 ('host-4', 'key-4', 'naa.004', 'lun-uuid-D');
INSERT INTO vinfo ("VM ID", "VM", "Host", "Host ID", "VI SDK UUID") VALUES
 ('vm-1', 'same-source', 'esxi-1', 'host-1', 'vc-1'),
 ('vm-2', 'other-source', 'esxi-1', 'host-1', 'vc-2'),
 ('vm-3', 'legacy-owner', 'esxi-4', 'host-4', 'vc-1');
INSERT INTO vdisk ("VM ID", "Disk Key", "Raw", "RDM LUN UUID", "RDM LUN UUID Source") VALUES
 ('vm-1', '1000', true, 'lun-uuid-A', 'forklift-rdm'),
 ('vm-2', '2000', true, 'lun-uuid-A', 'forklift-rdm'),
 ('vm-3', '3000', true, 'lun-uuid-D', 'forklift-rdm');`)
	require.NoError(t, err)

	vms, err := p.VMs(context.Background(), Filters{}, Options{})
	require.NoError(t, err)
	byVM := make(map[string]models.Disk)
	for _, vm := range vms {
		require.Len(t, vm.Disks, 1)
		byVM[vm.ID] = vm.Disks[0]
	}
	// The VM's own vCenter source matches the owner.
	require.NotNil(t, byVM["vm-1"].RDMBacking.HostLun)
	// A host owned by another vCenter never receives the match; the disk
	// keeps its source backing values.
	require.Nil(t, byVM["vm-2"].RDMBacking.HostLun)
	require.Equal(t, "lun-uuid-A", byVM["vm-2"].RDMBacking.LunUUID)
	// An owner without source identity stays eligible.
	require.NotNil(t, byVM["vm-3"].RDMBacking.HostLun)
	require.Equal(t, "host-4", byVM["vm-3"].RDMBacking.HostLun.HostID)
}

// TestRDMHostLunMatchingRequiresUniqueHostOwner proves a host ID repeated
// across two vCenters cannot establish LUN ownership, even when the LUN UUID
// itself has a single candidate.
func TestRDMHostLunMatchingRequiresUniqueHostOwner(t *testing.T) {
	p, db, cleanup := setupTestParser(t, nil)
	defer cleanup()
	_, err := db.Exec(`INSERT INTO vhost ("Object ID", "Host", "Cluster", "Datacenter", "VI SDK UUID") VALUES
 ('host-1', 'esxi-1', 'cluster', 'dc', 'vc-1'),
 ('host-1', 'esxi-other', 'cluster', 'other-dc', 'vc-2');
INSERT INTO vhostscsi ("Host ID", "SCSI Disk Key", "Canonical Name", "LUN UUID") VALUES
 ('host-1', 'key-1', 'naa.001', 'lun-uuid-A');
INSERT INTO vinfo ("VM ID", "VM", "Host", "Host ID", "VI SDK UUID") VALUES
 ('vm-1', 'shared-id', 'esxi-1', 'host-1', 'vc-1');
INSERT INTO vdisk ("VM ID", "Disk Key", "Raw", "RDM LUN UUID", "RDM LUN UUID Source") VALUES
 ('vm-1', '1000', true, 'lun-uuid-A', 'forklift-rdm');`)
	require.NoError(t, err)

	vms, err := p.VMs(context.Background(), Filters{}, Options{})
	require.NoError(t, err)
	require.Len(t, vms, 1)
	require.Len(t, vms[0].Disks, 1)
	backing := vms[0].Disks[0].RDMBacking
	require.NotNil(t, backing)
	require.Equal(t, "lun-uuid-A", backing.LunUUID)
	require.Nil(t, backing.HostLun, "a host ID shared across vCenters must not establish LUN ownership")
}

func ns(value string) sql.NullString {
	return sql.NullString{String: value, Valid: true}
}

func nb(value bool) sql.NullBool {
	return sql.NullBool{Bool: value, Valid: true}
}
