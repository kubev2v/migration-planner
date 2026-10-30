package duckdb_parser

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kubev2v/migration-planner/pkg/duckdb_parser/models"
	"github.com/kubev2v/migration-planner/pkg/inventory"
	"github.com/kubev2v/migration-planner/pkg/inventory/converters"
)

func TestIngestSqlite_StorageTopology(t *testing.T) {
	ctx := context.Background()
	parser, db, cleanup := setupTestParser(t, &testValidator{})
	defer cleanup()
	sqlitePath := createTestSQLite(t, "vcenter-1",
		[]sqliteCluster{{id: "cluster-1", name: "cluster1", datacenter: "dc1"}},
		[]sqliteVM{{id: "vm-1", name: "vm1", clusterName: "cluster1"}},
	)
	src, err := sql.Open("duckdb", "")
	require.NoError(t, err)
	defer func() { _ = src.Close() }()
	_, err = src.Exec(fmt.Sprintf("ATTACH '%s' AS dst (TYPE sqlite)", sqlitePath))
	require.NoError(t, err)
	exec := func(query string, args ...any) {
		t.Helper()
		_, err := src.Exec(query, args...)
		require.NoError(t, err)
	}
	exec(`UPDATE dst.Host SET Datastores = ?, HbaDiskInfo = ?, HostScsiTopology = ?, HostScsiDisks = ? WHERE ID = 'host-1'`,
		`[{"id":"ds-1"},{"id":"ds-vvol"},{"id":"ds-orphan"},{"id":"ds-invalid"},{"id":"ds-unknown"},{"id":"ds-nfs"}]`,
		`[{"key":"hba-1","hbaDevice":"vmhba64","protocol":"iSCSI","model":"Software Adapter","driver":"iscsi_vmk"},
		  {"key":"hba-2","hbaDevice":"vmhba65","protocol":"iSCSI","model":"Second Adapter"},
		  {"key":"hba-unused","hbaDevice":"vmhba1","protocol":"FibreChannel","model":"FC Adapter","nodeWorldWideName":"2000000000000001","portWorldWideName":"2100000000000001"},
		  {"key":"hba-unknown","hbaDevice":"vmhba66","protocol":"unknown","model":"Unknown Protocol Adapter"}]`,
		`[{"key":"hba-1","scsiDiskKeys":["lun-1","lun-1"]},{"key":"hba-2","scsiDiskKeys":["lun-1"]},{"key":"hba-unknown","scsiDiskKeys":["lun-unknown"]}]`,
		`[{"key":"lun-1","canonicalName":"naa.shared","vendor":" NETAPP ","model":" LUN C-Mode "},
		  {"key":"lun-orphan","canonicalName":"naa.orphan","vendor":"PURE","model":"FlashArray"},
		  {"key":"lun-unknown","canonicalName":"naa.unknown","vendor":"UNKNOWN","model":"Unknown Device"}]`)
	// Identical adapter and disk keys on another host must never contaminate ds-1;
	// malformed host datastore JSON must not abort either datastore parse path.
	exec(`INSERT INTO dst.Host (ID, Cluster, Datastores, HbaDiskInfo, HostScsiTopology, HostScsiDisks)
		VALUES ('host-2', 'cluster-1', 'not-json', ?, ?, ?)`,
		`[{"key":"hba-1","hbaDevice":"vmhba64","protocol":"FibreChannel","model":"Other Adapter"}]`,
		`[{"key":"hba-1","scsiDiskKeys":["lun-1"]}]`,
		`[{"key":"lun-1","canonicalName":"naa.shared","vendor":"WRONG","model":"Wrong Array"}]`)
	exec(`INSERT INTO dst.Host (ID, Cluster, Datastores, HbaDiskInfo, HostScsiTopology, HostScsiDisks)
		VALUES ('host-3', 'cluster-1', ?, ?, ?, ?)`,
		`[{"id":"ds-fc"}]`,
		`[{"key":"hba-fc","hbaDevice":"vmhba70","protocol":"FibreChannel","model":"FC Adapter"}]`,
		`[{"key":"hba-fc","scsiDiskKeys":["lun-fc"]}]`,
		`[{"key":"lun-fc","canonicalName":"naa.fc","vendor":"PURE","model":"FlashArray"}]`)
	exec(`INSERT INTO dst.Host (ID, Cluster, Datastores, HbaDiskInfo, HostScsiTopology, HostScsiDisks)
		VALUES ('host-empty', 'cluster-1', '[]', '[]', '[]', '[]')`)
	for _, ds := range []struct {
		id, name, dsType, filesystemType string
		filesystemMajorVersion           int
		backing, protocolEndpointType    string
	}{
		{"ds-1", "datastore1", "VMFS", "VMFS", 6, `["naa.shared"]`, ""},
		{"ds-vvol", "vvol1", "VVOL", "VVOL", 0, `["naa.vvol"]`, "SCSI"},
		{"ds-orphan", "orphan", "VMFS", "VMFS", 6, `["naa.orphan"]`, ""},
		{"ds-invalid", "invalid", "VMFS", "VMFS", 6, `not-json`, ""},
		{"ds-fc", "fc-ds", "VMFS", "VMFS", 5, `["naa.fc"]`, ""},
		{"ds-unknown", "unknown-ds", "VMFS", "VMFS", 6, `["naa.unknown"]`, ""},
		{"ds-nfs", "nfs-ds", "NFS", "NFS", 0, `[]`, ""},
	} {
		exec(`INSERT INTO dst.Datastore (ID, Name, Type, FilesystemType, FilesystemMajorVersion,
			Free, Capacity, BackingDevicesNames, ProtocolEndpointType)
			VALUES (?, ?, ?, ?, ?, 1073741824, 2147483648, ?, ?)`,
			ds.id, ds.name, ds.dsType, ds.filesystemType, ds.filesystemMajorVersion, ds.backing, ds.protocolEndpointType)
	}
	exec(`UPDATE dst.VM SET Disks = ? WHERE ID = 'vm-1'`,
		`[{"key":2000,"file":"[datastore1] vm1/disk.vmdk","capacity":1073741824,"bus":"scsi","mode":"persistent"}]`)
	exec("DETACH dst")

	result, err := parser.IngestSqlite(ctx, sqlitePath)
	require.NoError(t, err)
	require.True(t, result.IsValid(), "%v", result.Errors)
	assertHostStorage(t, parser, Filters{}, "host-1",
		`[{"device":"vmhba1","key":"hba-unused","protocol":"FibreChannel","model":"FC Adapter","nodeWwn":"2000000000000001","portWwn":"2100000000000001"},{"device":"vmhba64","key":"hba-1","protocol":"iSCSI","model":"Software Adapter","driver":"iscsi_vmk"},{"device":"vmhba65","key":"hba-2","protocol":"iSCSI","model":"Second Adapter"},{"device":"vmhba66","key":"hba-unknown","model":"Unknown Protocol Adapter"}]`,
		`[{"canonicalName":"naa.orphan","key":"lun-orphan","vendor":"PURE","model":"FlashArray"},{"canonicalName":"naa.shared","key":"lun-1","vendor":"NETAPP","model":"LUN C-Mode","adapterDevices":["vmhba64","vmhba65"]},{"canonicalName":"naa.unknown","key":"lun-unknown","vendor":"UNKNOWN","model":"Unknown Device","adapterDevices":["vmhba66"]}]`)
	var source string
	require.NoError(t, db.QueryRow(`SELECT "VI SDK UUID" FROM vhost WHERE "Object ID" = 'host-1'`).Scan(&source))
	require.Equal(t, "vcenter-1", source)
	var count int
	require.NoError(t, db.QueryRow(`SELECT count(*) FROM vhba`).Scan(&count))
	require.Equal(t, 6, count, "adapters without topology must also be collected")
	require.NoError(t, db.QueryRow(`SELECT count(*) FROM vhostscsi`).Scan(&count))
	require.Equal(t, 6, count, "deduplicate repeated targets while preserving distinct adapters")
	type topologyRow struct {
		hostID, hbaDevice, scsiDiskKey, canonicalName, vendor, model string
	}
	rows, err := db.Query(`SELECT "Host ID", COALESCE("HBA Device", ''), "SCSI Disk Key",
		COALESCE("Canonical Name", ''), COALESCE("LUN Vendor", ''), COALESCE("LUN Model", '')
		FROM vhostscsi`)
	require.NoError(t, err)
	var actualTopology []topologyRow
	for rows.Next() {
		var row topologyRow
		require.NoError(t, rows.Scan(&row.hostID, &row.hbaDevice, &row.scsiDiskKey, &row.canonicalName, &row.vendor, &row.model))
		actualTopology = append(actualTopology, row)
	}
	require.NoError(t, rows.Err())
	require.NoError(t, rows.Close())
	require.ElementsMatch(t, []topologyRow{
		{"host-1", "vmhba64", "lun-1", "naa.shared", "NETAPP", "LUN C-Mode"},
		{"host-1", "vmhba65", "lun-1", "naa.shared", "NETAPP", "LUN C-Mode"},
		{"host-1", "vmhba66", "lun-unknown", "naa.unknown", "UNKNOWN", "Unknown Device"},
		{"host-1", "", "lun-orphan", "naa.orphan", "PURE", "FlashArray"},
		{"host-2", "vmhba64", "lun-1", "naa.shared", "WRONG", "Wrong Array"},
		{"host-3", "vmhba70", "lun-fc", "naa.fc", "PURE", "FlashArray"},
	}, actualTopology)
	var emptyHostRows int
	require.NoError(t, db.QueryRow(`SELECT count(*) FROM vhba WHERE "Host ID" = 'host-empty'`).Scan(&emptyHostRows))
	require.Zero(t, emptyHostRows, "empty HBA arrays must not create adapters")
	require.NoError(t, db.QueryRow(`SELECT count(*) FROM vhostscsi WHERE "Host ID" = 'host-empty'`).Scan(&emptyHostRows))
	require.Zero(t, emptyHostRows, "empty topology and SCSI disk arrays must not create paths")
	var unknownProtocol sql.NullString
	require.NoError(t, db.QueryRow(`SELECT "Protocol" FROM vhba WHERE "Host ID" = 'host-1' AND "Device" = 'vmhba66'`).Scan(&unknownProtocol))
	require.False(t, unknownProtocol.Valid, "unknown HBA protocols must be ignored")
	var node, port string
	require.NoError(t, db.QueryRow(`SELECT "Node WWN", "Port WWN" FROM vhba WHERE "Host ID" = 'host-1' AND "Device" = 'vmhba1'`).Scan(&node, &port))
	require.Equal(t, "2000000000000001", node)
	require.Equal(t, "2100000000000001", port)
	var unresolved sql.NullString
	require.NoError(t, db.QueryRow(`SELECT "HBA Device" FROM vhostscsi WHERE "Canonical Name" = 'naa.orphan'`).Scan(&unresolved))
	require.False(t, unresolved.Valid)

	for _, filters := range []Filters{{}, {Cluster: "cluster1"}} {
		datastores, err := parser.Datastores(ctx, filters, Options{})
		require.NoError(t, err)
		require.Len(t, datastores, 7, "missing or malformed backing data must not remove datastores")
		byName := make(map[string]models.Datastore, len(datastores))
		for _, ds := range datastores {
			byName[ds.DiskId] = ds
		}
		for _, expected := range []struct {
			name, protocol, filesystemType string
			filesystemMajorVersion         int32
		}{
			{"datastore1", "iSCSI", "VMFS", 6},
			{"vvol1", "SCSI", "VVOL", 0},
			{"orphan", "N/A", "VMFS", 6},
			{"invalid", "N/A", "VMFS", 6},
			{"fc-ds", "FibreChannel", "VMFS", 5},
			{"unknown-ds", "N/A", "VMFS", 6},
			{"nfs-ds", "NFS", "NFS", 0},
		} {
			ds, ok := byName[expected.name]
			require.True(t, ok, "expected datastore %q", expected.name)
			require.Equal(t, expected.protocol, ds.ProtocolType, "datastore %s protocol", expected.name)
			require.Equal(t, expected.filesystemType, ds.FilesystemType, "datastore %s filesystem type", expected.name)
			require.Equal(t, expected.filesystemMajorVersion, ds.FilesystemMajorVersion, "datastore %s filesystem major version", expected.name)
		}
		require.Equal(t, "NETAPP", byName["datastore1"].Vendor)
		require.Equal(t, "LUN C-Mode", byName["datastore1"].Model)
		require.Equal(t, "PURE", byName["orphan"].Vendor)
		require.Empty(t, byName["invalid"].BackingDeviceIDs)
	}
	vms, err := parser.VMs(ctx, Filters{}, Options{})
	require.NoError(t, err)
	require.Len(t, vms, 1)
	require.Len(t, vms[0].Disks, 1)
	require.Equal(t, "iSCSI", vms[0].Disks[0].DatastoreStorageProtocol)
	require.Equal(t, "NETAPP", vms[0].Disks[0].DatastoreVendor)
	require.Equal(t, "LUN C-Mode", vms[0].Disks[0].DatastoreModel)
}

func TestStorageTopologyQueries_HostReferences(t *testing.T) {
	for _, member := range []string{"host-1", "ESXI1.EXAMPLE.COM"} {
		t.Run(member, func(t *testing.T) {
			ctx := context.Background()
			parser, db, cleanup := setupTestParser(t, nil)
			defer cleanup()
			for _, query := range []string{
				`INSERT INTO vhost ("Object ID", "Host", "Cluster") VALUES ('host-1', 'esxi1.example.com', 'cluster1'), ('host-2', 'esxi2.example.com', 'cluster1')`,
				`INSERT INTO vhba ("Host ID", "Device", "Protocol") VALUES ('host-1', 'vmhba64', 'iSCSI'), ('host-2', 'vmhba64', 'FibreChannel')`,
				`INSERT INTO vhostscsi ("Host ID", "HBA Device", "Canonical Name", "LUN Vendor", "LUN Model") VALUES ('host-1', 'vmhba64', 'naa.shared', 'NETAPP', 'LUN C-Mode'), ('host-2', 'vmhba64', 'naa.shared', 'WRONG', 'Wrong Array')`,
				`INSERT INTO vinfo ("VM ID", "VM", "Host", "Cluster") VALUES ('vm-1', 'vm1', 'esxi1.example.com', 'cluster1')`,
				`INSERT INTO vdisk ("VM ID", "Path") VALUES ('vm-1', '[datastore1] vm1/disk.vmdk')`,
			} {
				_, err := db.Exec(query)
				require.NoError(t, err)
			}
			_, err := db.Exec(`INSERT INTO vdatastore ("Object ID", "Name", "Hosts", "Backing Devices", "Type") VALUES ('ds-1', 'datastore1', ?, '["naa.shared"]', 'VMFS')`, member)
			require.NoError(t, err)
			for _, filters := range []Filters{{}, {Cluster: "cluster1"}} {
				datastores, err := parser.Datastores(ctx, filters, Options{})
				require.NoError(t, err)
				require.Len(t, datastores, 1)
				require.Equal(t, "host-1", datastores[0].HostId)
				require.Equal(t, "iSCSI", datastores[0].ProtocolType)
				require.Equal(t, "NETAPP", datastores[0].Vendor)
			}
			vms, err := parser.VMs(ctx, Filters{}, Options{})
			require.NoError(t, err)
			require.Len(t, vms, 1)
			require.Len(t, vms[0].Disks, 1)
			require.Equal(t, "iSCSI", vms[0].Disks[0].DatastoreStorageProtocol)
			require.Equal(t, "NETAPP", vms[0].Disks[0].DatastoreVendor)
		})
	}
}

func TestIngestRvTools_HbaHostScope(t *testing.T) {
	parser, db, cleanup := setupTestParser(t, nil)
	defer cleanup()
	workbook := createTestExcel(t,
		NewExcelSheet("vInfo", vInfoHeaders, []map[string]string{{"VM ID": "vm-1", "VM": "vm1", "VI SDK UUID": "vc-1", "Cluster": "cluster1"}}),
		NewExcelSheet("vHost", vHostHeaders, []map[string]string{
			{"Host": "esxi1", "Object ID": "host-1"}, {"Host": "esxi2", "Object ID": "host-2"},
			{"Host": "duplicate", "Object ID": "host-3"}, {"Host": " DUPLICATE ", "Object ID": "host-4"},
		}),
		NewExcelSheet("vHBA", []string{"Host", "Device", "Type"}, []map[string]string{
			{"Host": "esxi1", "Device": "vmhba64", "Type": "ISCSI"},
			{"Host": "esxi2", "Device": "vmhba64", "Type": "Fibre Channel"},
			{"Host": "duplicate", "Device": "vmhba65", "Type": "Fibre Channel"},
		}),
	)
	result, err := parser.IngestRvTools(context.Background(), workbook)
	require.NoError(t, err)
	require.True(t, result.IsValid(), "%v", result.Errors)
	var adapterType string
	require.NoError(t, db.QueryRow(`SELECT "Type" FROM vhba WHERE "Host ID" = 'host-1' AND "Device" = 'vmhba64'`).Scan(&adapterType))
	require.Equal(t, "ISCSI", adapterType)
	var ambiguousHostID sql.NullString
	require.NoError(t, db.QueryRow(`SELECT "Host ID" FROM vhba WHERE "Device" = 'vmhba65'`).Scan(&ambiguousHostID))
	require.False(t, ambiguousHostID.Valid, "ambiguous host names must leave adapters unlinked")
	var count int
	require.NoError(t, db.QueryRow(`SELECT count(*) FROM vhostscsi`).Scan(&count))
	require.Zero(t, count, "an absent vMultiPath sheet must leave topology empty")
}

func TestIngestRvTools_StorageMetadataAndTopology(t *testing.T) {
	parser, db, cleanup := setupTestParser(t, nil)
	defer cleanup()

	workbook := createTestExcel(t,
		NewExcelSheet("vInfo", vInfoHeaders, []map[string]string{{
			"VM ID": "vm-1", "VM": "vm1", "VI SDK UUID": "vc-1", "Host": "esxi1", "Cluster": "cluster1",
		}}),
		NewExcelSheet("vHost", vHostHeaders, []map[string]string{{
			"Datacenter": "dc1", "Cluster": "cluster1", "Object ID": "host-1", "Host": "esxi1",
		}}),
		NewExcelSheet("vDatastore", []string{
			"Hosts", "Address", "Name", "Object ID", "Free MiB", "Capacity MiB", "Type", "MHA", "# Extents", "Major Version",
		}, []map[string]string{
			{"Hosts": "esxi1", "Address": "naa.600a0980383139544924583130314c41", "Name": "vmfs6", "Object ID": "ds-vmfs6", "Free MiB": "512", "Capacity MiB": "1024", "Type": "VMFS", "MHA": "false", "# Extents": "1", "Major Version": "6"},
			{"Hosts": "esxi1", "Address": "naa.orphan", "Name": "orphan-ds", "Object ID": "ds-orphan", "Free MiB": "256", "Capacity MiB": "512", "Type": "VMFS", "MHA": "false", "# Extents": "1", "Major Version": "6"},
			{"Hosts": "esxi1", "Address": "10.0.0.10:/exports/vmstore", "Name": "nfs-ds", "Object ID": "ds-nfs", "Free MiB": "256", "Capacity MiB": "512", "Type": "NFS", "MHA": "false"},
			{"Hosts": "esxi1", "Name": "vvol-ds", "Object ID": "ds-vvol", "Free MiB": "256", "Capacity MiB": "512", "Type": "VVOL", "MHA": "false"},
		}),
		NewExcelSheet("vHBA", []string{"Host", "Device", "Type", "Driver", "Model", "WWN"}, []map[string]string{
			{"Host": "esxi1", "Device": "vmhba64", "Type": "ISCSI", "Driver": "iscsi_vmk", "Model": "iSCSI Software Adapter"},
			{"Host": "esxi1", "Device": "vmhba65", "Type": "Fibre Channel", "Driver": "qla2xxx", "Model": "QLE2742", "WWN": "2000000000000001 2100000000000001"},
			{"Host": "esxi1", "Device": "vmhba0", "Type": "VM", "Driver": "VM", "Model": "Local Controller", "WWN": "VM"},
		}),
		NewExcelSheet("vMultiPath", []string{
			"Host", "Disk", "Vendor", "Model", "UUID", "Object ID",
			"Path 1", "Path 2", "Path 3", "Path 4", "Path 5", "Path 6", "Path 7", "Path 8",
		}, []map[string]string{{
			"Host": "esxi1", "Disk": "naa.600a0980383139544924583130314c41", "Vendor": " NETAPP ", "Model": " LUN C-Mode ", "UUID": "lun-uuid-1", "Object ID": "ds-vmfs6",
			"Path 1": "vmhba64", "Path 2": "vmhba64", "Path 3": "vmhba65", "Path 8": "VM",
		}, {
			"Host": "esxi1", "Disk": "naa.orphan", "Vendor": "PURE", "Model": "FlashArray", "UUID": "lun-uuid-orphan", "Object ID": "ds-orphan",
		}}),
		NewExcelSheet("vDisk", []string{
			"VM ID", "Disk Key", "Unit #", "Path", "Disk Path", "Capacity MiB", "Sharing mode", "Raw", "Shared Bus", "Disk Mode", "Disk UUID", "Thin", "Controller", "Label", "SCSI Unit #",
		}, []map[string]string{{
			"VM ID": "vm-1", "Disk Key": "2000", "Unit #": "0", "Path": "[vmfs6] vm1/disk.vmdk", "Capacity MiB": "1024", "Shared Bus": "scsi", "Disk Mode": "persistent",
		}}),
	)

	result, err := parser.IngestRvTools(context.Background(), workbook)
	require.NoError(t, err)
	require.True(t, result.IsValid(), "%v", result.Errors)

	assertHostStorage(t, parser, Filters{Cluster: "cluster1"}, "host-1",
		`[{"device":"vmhba0","model":"Local Controller"},{"device":"vmhba64","type":"ISCSI","protocol":"iSCSI","model":"iSCSI Software Adapter","driver":"iscsi_vmk"},{"device":"vmhba65","type":"Fibre Channel","protocol":"FibreChannel","model":"QLE2742","driver":"qla2xxx","nodeWwn":"2000000000000001","portWwn":"2100000000000001"}]`,
		`[{"canonicalName":"naa.600a0980383139544924583130314c41","lunUuid":"lun-uuid-1","vendor":"NETAPP","model":"LUN C-Mode","adapterDevices":["vmhba64","vmhba65"]},{"canonicalName":"naa.orphan","lunUuid":"lun-uuid-orphan","vendor":"PURE","model":"FlashArray"}]`)

	type adapterRow struct {
		hostID, device, adapterType, protocol, model, driver, nodeWWN, portWWN string
	}
	rows, err := db.Query(`SELECT COALESCE("Host ID", ''), COALESCE("Device", ''), COALESCE("Type", ''), COALESCE("Protocol", ''), COALESCE("Model", ''), COALESCE("Driver", ''), COALESCE("Node WWN", ''), COALESCE("Port WWN", '') FROM vhba`)
	require.NoError(t, err)
	var adapters []adapterRow
	for rows.Next() {
		var row adapterRow
		require.NoError(t, rows.Scan(&row.hostID, &row.device, &row.adapterType, &row.protocol, &row.model, &row.driver, &row.nodeWWN, &row.portWWN))
		adapters = append(adapters, row)
	}
	require.NoError(t, rows.Err())
	require.NoError(t, rows.Close())
	require.ElementsMatch(t, []adapterRow{
		{"host-1", "vmhba64", "ISCSI", "iSCSI", "iSCSI Software Adapter", "iscsi_vmk", "", ""},
		{"host-1", "vmhba65", "Fibre Channel", "FibreChannel", "QLE2742", "qla2xxx", "2000000000000001", "2100000000000001"},
		{"host-1", "vmhba0", "", "", "Local Controller", "", "", ""},
	}, adapters)

	type topologyRow struct {
		hostID, hbaDevice, canonicalName, vendor, model, lunUUID string
	}
	rows, err = db.Query(`SELECT "Host ID", COALESCE("HBA Device", ''), "Canonical Name", "LUN Vendor", "LUN Model", "LUN UUID" FROM vhostscsi`)
	require.NoError(t, err)
	var topology []topologyRow
	for rows.Next() {
		var row topologyRow
		require.NoError(t, rows.Scan(&row.hostID, &row.hbaDevice, &row.canonicalName, &row.vendor, &row.model, &row.lunUUID))
		topology = append(topology, row)
	}
	require.NoError(t, rows.Err())
	require.NoError(t, rows.Close())
	require.ElementsMatch(t, []topologyRow{
		{"host-1", "vmhba64", "naa.600a0980383139544924583130314c41", "NETAPP", "LUN C-Mode", "lun-uuid-1"},
		{"host-1", "vmhba65", "naa.600a0980383139544924583130314c41", "NETAPP", "LUN C-Mode", "lun-uuid-1"},
		{"host-1", "", "naa.orphan", "PURE", "FlashArray", "lun-uuid-orphan"},
	}, topology, "repeated paths through one HBA collapse, but distinct HBAs remain")

	var backingDevices, datastoreProtocol, filesystemType string
	var filesystemMajorVersion int
	require.NoError(t, db.QueryRow(`SELECT "Backing Devices", COALESCE("Storage Protocol", ''), COALESCE("Filesystem Type", ''), COALESCE("Filesystem Major Version", 0) FROM vdatastore WHERE "Object ID" = 'ds-vmfs6'`).Scan(&backingDevices, &datastoreProtocol, &filesystemType, &filesystemMajorVersion))
	require.JSONEq(t, `["naa.600a0980383139544924583130314c41"]`, backingDevices)
	require.Empty(t, datastoreProtocol, "block transport is derived through matching host/HBA/LUN rows")
	require.Equal(t, "VMFS", filesystemType)
	require.Equal(t, 6, filesystemMajorVersion)

	datastores, err := parser.Datastores(context.Background(), Filters{}, Options{})
	require.NoError(t, err)
	byName := make(map[string]models.Datastore, len(datastores))
	for _, datastore := range datastores {
		byName[datastore.DiskId] = datastore
	}
	require.Equal(t, "FibreChannel, iSCSI", byName["vmfs6"].ProtocolType)
	require.Equal(t, "NETAPP", byName["vmfs6"].Vendor)
	require.Equal(t, "LUN C-Mode", byName["vmfs6"].Model)
	require.Equal(t, "NFS", byName["nfs-ds"].ProtocolType)
	require.Equal(t, "NFS", byName["nfs-ds"].FilesystemType)
	require.Equal(t, "VVOL", byName["vvol-ds"].FilesystemType)

	vms, err := parser.VMs(context.Background(), Filters{}, Options{})
	require.NoError(t, err)
	require.Len(t, vms, 1)
	require.Len(t, vms[0].Disks, 1)
	require.Equal(t, "FibreChannel, iSCSI", vms[0].Disks[0].DatastoreStorageProtocol)
	require.Equal(t, "VMFS", vms[0].Disks[0].DatastoreFilesystemType)
	require.EqualValues(t, 6, vms[0].Disks[0].DatastoreFilesystemMajorVersion)
	require.Equal(t, []string{"naa.600a0980383139544924583130314c41"}, vms[0].Disks[0].DatastoreBackingDeviceIDs)
	require.Equal(t, "NETAPP", vms[0].Disks[0].DatastoreVendor)
	require.Equal(t, "LUN C-Mode", vms[0].Disks[0].DatastoreModel)
}

func TestIngestRvTools_StorageMetadataWithoutOptionalSheets(t *testing.T) {
	parser, db, cleanup := setupTestParser(t, nil)
	defer cleanup()

	workbook := createTestExcel(t,
		NewExcelSheet("vInfo", vInfoHeaders, []map[string]string{{
			"VM ID": "vm-1", "VM": "vm1", "VI SDK UUID": "vc-1", "Host": "esxi1", "Cluster": "cluster1",
		}}),
		NewExcelSheet("vHost", vHostHeaders, []map[string]string{{
			"Datacenter": "dc1", "Cluster": "cluster1", "Object ID": "host-1", "Host": "esxi1",
		}}),
		NewExcelSheet("vDatastore", []string{
			"Hosts", "Address", "Name", "Object ID", "Free MiB", "Capacity MiB", "Type", "MHA", "# Extents", "Major Version",
		}, []map[string]string{
			{"Hosts": "esxi1", "Address": "naa.600a0980000000000000000000000001", "Name": "vmfs6", "Object ID": "ds-vmfs6", "Free MiB": "512", "Capacity MiB": "1024", "Type": "VMFS", "MHA": "false", "# Extents": "1", "Major Version": "6"},
			{"Hosts": "esxi1", "Address": "naa.600a0980000000000000000000000002", "Name": "vmfs-multi", "Object ID": "ds-vmfs-multi", "Free MiB": "256", "Capacity MiB": "512", "Type": "VMFS", "MHA": "false", "# Extents": "2", "Major Version": "5"},
			{"Hosts": "esxi1", "Address": "10.0.0.10:/exports/vmstore", "Name": "nfs-ds", "Object ID": "ds-nfs", "Free MiB": "256", "Capacity MiB": "512", "Type": "NFS", "MHA": "false"},
			{"Hosts": "esxi1", "Address": "10.0.0.10:/exports/vmstore41", "Name": "nfs41-ds", "Object ID": "ds-nfs41", "Free MiB": "256", "Capacity MiB": "512", "Type": "NFS 4.1", "MHA": "false"},
			{"Hosts": "esxi1", "Name": "vvol-ds", "Object ID": "ds-vvol", "Free MiB": "256", "Capacity MiB": "512", "Type": "VVOL", "MHA": "false"},
		}),
	)

	result, err := parser.IngestRvTools(context.Background(), workbook)
	require.NoError(t, err)
	require.True(t, result.IsValid(), "%v", result.Errors)

	var count int
	require.NoError(t, db.QueryRow(`SELECT count(*) FROM vdatastore`).Scan(&count))
	require.Equal(t, 5, count, "missing optional storage sheets must not remove datastores")
	var backing string
	require.NoError(t, db.QueryRow(`SELECT "Backing Devices" FROM vdatastore WHERE "Object ID" = 'ds-vmfs6'`).Scan(&backing))
	require.JSONEq(t, `["naa.600a0980000000000000000000000001"]`, backing)
	require.NoError(t, db.QueryRow(`SELECT "Backing Devices" FROM vdatastore WHERE "Object ID" = 'ds-vmfs-multi'`).Scan(&backing))
	require.JSONEq(t, `[]`, backing, "one address must not be presented as all extents")
	var filesystemType, datastoreProtocol string
	var filesystemMajorVersion int
	require.NoError(t, db.QueryRow(`SELECT COALESCE("Filesystem Type", ''), COALESCE("Filesystem Major Version", 0) FROM vdatastore WHERE "Object ID" = 'ds-vmfs-multi'`).Scan(&filesystemType, &filesystemMajorVersion))
	require.Equal(t, "VMFS", filesystemType)
	require.Equal(t, 5, filesystemMajorVersion)
	require.NoError(t, db.QueryRow(`SELECT COALESCE("Storage Protocol", '') FROM vdatastore WHERE "Object ID" = 'ds-nfs'`).Scan(&datastoreProtocol))
	require.Equal(t, "NFS", datastoreProtocol)
	require.NoError(t, db.QueryRow(`SELECT COALESCE("Filesystem Type", ''), COALESCE("Storage Protocol", '') FROM vdatastore WHERE "Object ID" = 'ds-nfs41'`).Scan(&filesystemType, &datastoreProtocol))
	require.Equal(t, "NFS41", filesystemType)
	require.Equal(t, "NFS", datastoreProtocol)
	require.NoError(t, db.QueryRow(`SELECT count(*) FROM vhba`).Scan(&count))
	require.Zero(t, count)
	require.NoError(t, db.QueryRow(`SELECT count(*) FROM vhostscsi`).Scan(&count))
	require.Zero(t, count)

	inv, err := parser.BuildInventory(context.Background(), nil)
	require.NoError(t, err)
	require.Len(t, inv.Clusters, 1)
	scopes := map[string][]inventory.Datastore{"vCenter": inv.VCenter.Infra.Datastores}
	for name, cluster := range inv.Clusters {
		scopes[name] = cluster.Infra.Datastores
	}
	for name, datastores := range scopes {
		t.Run(name, func(t *testing.T) {
			byType := make(map[string]inventory.Datastore)
			for _, ds := range datastores {
				byType[ds.Type] = ds
			}
			ds, ok := byType["NFS 4.1"]
			require.True(t, ok, "the original datastore type must be preserved")
			require.Equal(t, "File", ds.StorageProtocol)
			require.Equal(t, "NFS41", ds.FilesystemType)
			require.Equal(t, "NFS", ds.ProtocolType)
		})
	}
}

func TestIngestRvTools_MultiPathWithoutHBA(t *testing.T) {
	parser, db, cleanup := setupTestParser(t, nil)
	defer cleanup()

	workbook := createTestExcel(t,
		NewExcelSheet("vInfo", vInfoHeaders, []map[string]string{{
			"VM ID": "vm-1", "VM": "vm1", "VI SDK UUID": "vc-1", "Host": "esxi1", "Cluster": "cluster1",
		}}),
		NewExcelSheet("vHost", vHostHeaders, []map[string]string{{
			"Datacenter": "dc1", "Cluster": "cluster1", "Object ID": "host-1", "Host": "esxi1",
		}}),
		NewExcelSheet("vDatastore", []string{
			"Hosts", "Address", "Name", "Object ID", "Free MiB", "Capacity MiB", "Type", "MHA", "# Extents", "Major Version",
		}, []map[string]string{{
			"Hosts": "esxi1", "Address": "naa.600a0980000000000000000000000001", "Name": "vmfs-ds", "Object ID": "ds-1", "Free MiB": "512", "Capacity MiB": "1024", "Type": "VMFS", "MHA": "false", "# Extents": "1", "Major Version": "6",
		}}),
		NewExcelSheet("vMultiPath", []string{"Host", "Disk", "Vendor", "Model", "UUID", "Object ID", "Path 1"}, []map[string]string{{
			"Host": "esxi1", "Disk": "naa.600a0980000000000000000000000001", "Vendor": "PURE", "Model": "FlashArray", "UUID": "lun-uuid-1", "Object ID": "ds-1", "Path 1": "vmhba64",
		}}),
	)

	result, err := parser.IngestRvTools(context.Background(), workbook)
	require.NoError(t, err)
	require.True(t, result.IsValid(), "%v", result.Errors)

	var count int
	require.NoError(t, db.QueryRow(`SELECT count(*) FROM vhba`).Scan(&count))
	require.Zero(t, count)
	require.NoError(t, db.QueryRow(`SELECT count(*) FROM vhostscsi WHERE "Host ID" = 'host-1' AND "HBA Device" = 'vmhba64' AND "Canonical Name" = 'naa.600a0980000000000000000000000001' AND "LUN Vendor" = 'PURE' AND "LUN Model" = 'FlashArray' AND "LUN UUID" = 'lun-uuid-1'`).Scan(&count))
	require.Equal(t, 1, count, "reported LUN metadata must survive without an HBA inventory row")

	datastores, err := parser.Datastores(context.Background(), Filters{}, Options{})
	require.NoError(t, err)
	require.Len(t, datastores, 1)
	require.Equal(t, "N/A", datastores[0].ProtocolType, "a multipath path without HBA metadata does not prove transport")
	require.Equal(t, "PURE", datastores[0].Vendor)
	require.Equal(t, "FlashArray", datastores[0].Model)
}

func TestStorageTopologyQueries_UniqueHostReferences(t *testing.T) {
	for _, tc := range []struct {
		name, members, host1Name, host2Name string
		protocol, vendor, model             string
		hostIDs                             []string
		clusterCounts                       [2]int
		missingHostID, duplicateHostID      bool
	}{
		{name: "ID/name collision", members: "host-1", host1Name: "esxi1.example.com", host2Name: "host-1"},
		{name: "valid name beside collision", members: "host-1, ESXI1.EXAMPLE.COM", host1Name: "esxi1.example.com", host2Name: "host-1", protocol: "iSCSI", vendor: "NETAPP", model: "LUN-A", hostIDs: []string{"host-1"}, clusterCounts: [2]int{1, 0}},
		{name: "valid ID beside collision", members: "host-1,host-2", host1Name: "esxi1.example.com", host2Name: "host-1", protocol: "FibreChannel", vendor: "PURE", model: "LUN-B", hostIDs: []string{"host-2"}, clusterCounts: [2]int{0, 1}},
		{name: "duplicate names", members: "esxi1.example.com", host1Name: "esxi1.example.com", host2Name: " ESXI1.EXAMPLE.COM "},
		{name: "shared datastore", members: "host-1,host-2", host1Name: "esxi1.example.com", host2Name: "esxi2.example.com", protocol: "FibreChannel, iSCSI", vendor: "NETAPP, PURE", model: "LUN-A, LUN-B", hostIDs: []string{"host-1", "host-2"}, clusterCounts: [2]int{1, 1}},
		{name: "same host matches ID and name", members: "host-1", host1Name: "host-1", host2Name: "esxi2.example.com", protocol: "iSCSI", vendor: "NETAPP", model: "LUN-A", hostIDs: []string{"host-1"}, clusterCounts: [2]int{1, 0}},
		{name: "duplicate aliases and whitespace", members: " host-1, ESXI1.EXAMPLE.COM,host-1 ", host1Name: "esxi1.example.com", host2Name: "esxi2.example.com", protocol: "iSCSI", vendor: "NETAPP", model: "LUN-A", hostIDs: []string{"host-1"}, clusterCounts: [2]int{1, 0}},
		{name: "legacy hostname without Object ID", members: "esxi1.example.com", host1Name: "esxi1.example.com", host2Name: "esxi2.example.com", missingHostID: true, clusterCounts: [2]int{1, 0}},
		{name: "unique name with reused Object ID", members: "esxi1.example.com", host1Name: "esxi1.example.com", host2Name: "esxi2.example.com", duplicateHostID: true, hostIDs: []string{"host-1"}, clusterCounts: [2]int{1, 0}},
		{name: "ambiguous reused Object ID", members: "host-1", host1Name: "esxi1.example.com", host2Name: "esxi2.example.com", duplicateHostID: true},
		{name: "unresolved and empty references", members: "missing-host, , ", host1Name: "esxi1.example.com", host2Name: "esxi2.example.com"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			p, db, cleanup := setupTestParser(t, nil)
			defer cleanup()
			_, err := db.Exec(`INSERT INTO vhost ("Object ID", "Host", "Cluster") VALUES ('host-1', ?, 'cluster1'), ('host-2', ?, 'cluster2');`, tc.host1Name, tc.host2Name)
			require.NoError(t, err)
			if tc.missingHostID {
				_, err = db.Exec(`UPDATE vhost SET "Object ID"=NULL WHERE "Host"=?`, tc.host1Name)
				require.NoError(t, err)
			}
			if tc.duplicateHostID {
				_, err = db.Exec(`UPDATE vhost SET "Object ID"='host-1'`)
				require.NoError(t, err)
			}
			_, err = db.Exec(`INSERT INTO vhba ("Host ID", "Device", "Protocol") VALUES ('host-1', 'vmhba64', 'iSCSI'), ('host-2', 'vmhba64', 'FibreChannel');
INSERT INTO vhostscsi ("Host ID", "HBA Device", "Canonical Name", "LUN Vendor", "LUN Model") VALUES
('host-1', 'vmhba64', 'naa.shared', 'NETAPP', 'LUN-A'), ('host-2', 'vmhba64', 'naa.shared', 'PURE', 'LUN-B');
INSERT INTO vinfo ("VM ID", "VM", "Template") VALUES ('vm-1', 'vm1', false);
INSERT INTO vdisk ("VM ID", "Path", "Capacity MiB") VALUES ('vm-1', '[datastore1] vm1/disk.vmdk', 100);`)
			require.NoError(t, err)
			if tc.duplicateHostID {
				_, err = db.Exec(`UPDATE vhba SET "Host ID"='host-1'; UPDATE vhostscsi SET "Host ID"='host-1'`)
				require.NoError(t, err)
			}
			_, err = db.Exec(`INSERT INTO vdatastore ("Object ID", "Name", "Hosts", "Backing Devices", "Type", "Capacity MiB") VALUES ('ds-1', 'datastore1', ?, '["naa.shared"]', 'VMFS', 2048)`, tc.members)
			require.NoError(t, err)
			datastores, err := p.Datastores(ctx, Filters{}, Options{})
			require.NoError(t, err)
			require.Len(t, datastores, 1, "unresolved hosts must not remove the datastore")
			ds := datastores[0]
			require.Equal(t, float64(2), ds.TotalCapacityGB)
			if len(tc.hostIDs) == 0 {
				require.Equal(t, "N/A", ds.HostId)
				require.Equal(t, "N/A", ds.ProtocolType)
				require.Equal(t, "N/A", ds.Vendor)
				require.Equal(t, "N/A", ds.Model)
			} else {
				require.ElementsMatch(t, tc.hostIDs, strings.Split(ds.HostId, ", "))
				wantProtocol, wantVendor, wantModel := tc.protocol, tc.vendor, tc.model
				if tc.duplicateHostID {
					wantProtocol, wantVendor, wantModel = "N/A", "N/A", "N/A"
				}
				require.Equal(t, wantProtocol, ds.ProtocolType)
				require.Equal(t, wantVendor, ds.Vendor)
				require.Equal(t, wantModel, ds.Model)
			}
			for i, cluster := range []string{"cluster1", "cluster2"} {
				rows, err := p.Datastores(ctx, Filters{Cluster: cluster}, Options{})
				require.NoError(t, err)
				require.Len(t, rows, tc.clusterCounts[i], "cluster association must use the same resolved host")
				if tc.duplicateHostID {
					for _, row := range rows {
						require.Equal(t, "N/A", row.ProtocolType)
						require.Equal(t, "N/A", row.Vendor)
						require.Equal(t, "N/A", row.Model)
					}
				}
			}
			vms, err := p.VMs(ctx, Filters{}, Options{})
			require.NoError(t, err)
			require.Len(t, vms, 1)
			require.Len(t, vms[0].Disks, 1)
			disk := vms[0].Disks[0]
			require.Equal(t, "datastore1", disk.DatastoreName)
			require.Equal(t, int64(100), disk.Capacity)
			require.Equal(t, tc.protocol, disk.DatastoreStorageProtocol)
			require.Equal(t, tc.vendor, disk.DatastoreVendor)
			require.Equal(t, tc.model, disk.DatastoreModel)
			var stored string
			require.NoError(t, db.QueryRow(`SELECT "Hosts" FROM vdatastore`).Scan(&stored))
			require.Equal(t, tc.members, stored)
		})
	}
}

func TestStorageQueries_DiskDatastoreIdentity(t *testing.T) {
	for _, tc := range []struct {
		name, vmHost, members1, members2, path, host2Name     string
		secondDatastore, duplicateHost, reusedHostID          bool
		wantID, wantType, wantProtocol, wantVendor, wantModel string
	}{
		{name: "same name resolved by hostname", vmHost: " ESXI1.EXAMPLE.COM ", members1: " host-1 ", members2: "host-2", secondDatastore: true, wantID: "ds-1", wantType: "VMFS", wantProtocol: "iSCSI", wantVendor: "NETAPP", wantModel: "LUN-A"},
		{name: "same name resolved by ID and member hostname", vmHost: "host-2", members1: "ESXI1.EXAMPLE.COM", members2: " ESXI2.EXAMPLE.COM ", secondDatastore: true, wantID: "ds-2", wantType: "NFS", wantProtocol: "FibreChannel", wantVendor: "PURE", wantModel: "LUN-B"},
		{name: "unique name without host", members1: "host-1", wantID: "ds-1", wantType: "VMFS", wantProtocol: "iSCSI", wantVendor: "NETAPP", wantModel: "LUN-A"},
		{name: "VM host collides with another host name", vmHost: "host-1", members1: "esxi1.example.com", members2: "host-2", host2Name: "host-1", secondDatastore: true},
		{name: "same name with unresolved VM host", vmHost: "missing-host", members1: "host-1", members2: "host-2", secondDatastore: true},
		{name: "same name without host", members1: "host-1", members2: "host-2", secondDatastore: true},
		{name: "host belongs to both datastores", vmHost: "host-1", members1: "host-1", members2: "host-1", secondDatastore: true},
		{name: "unmatched datastore", vmHost: "host-1", members1: "host-1", path: "[missing] vm1/disk.vmdk"},
		{name: "normalized duplicate host rows", vmHost: "esxi1.example.com", members1: "host-1", members2: "host-2", secondDatastore: true, duplicateHost: true, wantID: "ds-1", wantType: "VMFS", wantProtocol: "iSCSI", wantVendor: "NETAPP", wantModel: "LUN-A"},
		{name: "reused host ID does not join distinct hosts", vmHost: "esxi1.example.com", members1: "esxi1.example.com", members2: "esxi2.example.com", secondDatastore: true, reusedHostID: true, wantID: "ds-1", wantType: "VMFS"},
		{name: "reused host ID remains ambiguous", vmHost: "host-1", members1: "esxi1.example.com", members2: "esxi2.example.com", secondDatastore: true, reusedHostID: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			p, db, cleanup := setupTestParser(t, nil)
			defer cleanup()
			_, err := db.Exec(`INSERT INTO vhost ("Object ID", "Host", "Cluster") VALUES ('host-1', 'esxi1.example.com', 'cluster1'), ('host-2', 'esxi2.example.com', 'cluster2');
INSERT INTO vhba ("Host ID", "Device", "Protocol") VALUES ('host-1', 'vmhba64', 'iSCSI'), ('host-2', 'vmhba64', 'FibreChannel');
INSERT INTO vhostscsi ("Host ID", "HBA Device", "Canonical Name", "LUN Vendor", "LUN Model") VALUES ('host-1', 'vmhba64', 'naa.a', 'NETAPP', 'LUN-A'), ('host-2', 'vmhba64', 'naa.b', 'PURE', 'LUN-B');`)
			require.NoError(t, err)
			if tc.host2Name != "" {
				_, err = db.Exec(`UPDATE vhost SET "Host"=? WHERE "Object ID"='host-2'`, tc.host2Name)
				require.NoError(t, err)
			}
			if tc.duplicateHost {
				_, err = db.Exec(`INSERT INTO vhost ("Object ID", "Host", "Cluster") VALUES ('host-1', ' ESXI1.EXAMPLE.COM ', 'cluster1')`)
				require.NoError(t, err)
			}
			if tc.reusedHostID {
				_, err = db.Exec(`UPDATE vhost SET "Object ID"='host-1'; UPDATE vhba SET "Host ID"='host-1'; UPDATE vhostscsi SET "Host ID"='host-1'`)
				require.NoError(t, err)
			}
			_, err = db.Exec(`INSERT INTO vdatastore ("Object ID", "Name", "Hosts", "Backing Devices", "Type") VALUES ('ds-1', 'shared', ?, '["naa.a"]', 'VMFS')`, tc.members1)
			require.NoError(t, err)
			if tc.secondDatastore {
				_, err = db.Exec(`INSERT INTO vdatastore ("Object ID", "Name", "Hosts", "Backing Devices", "Type") VALUES ('ds-2', 'shared', ?, '["naa.b"]', 'NFS')`, tc.members2)
				require.NoError(t, err)
			}
			_, err = db.Exec(`INSERT INTO vinfo ("VM ID", "VM", "Host", "Cluster", "Template") VALUES ('vm-1', 'vm1', ?, 'cluster1', false)`, tc.vmHost)
			require.NoError(t, err)
			path := tc.path
			if path == "" {
				path = "[shared] vm1/disk.vmdk"
			}
			_, err = db.Exec(`INSERT INTO vdisk ("VM ID", "Disk Path", "Capacity MiB") VALUES ('vm-1', ?, 1048576), ('vm-1', ?, 1048576)`, path, path)
			require.NoError(t, err)
			vms, err := p.VMs(ctx, Filters{VmId: "vm-1"}, Options{})
			require.NoError(t, err)
			require.Len(t, vms, 1)
			require.Len(t, vms[0].Disks, 2, "each physical source disk must appear once")
			for _, disk := range vms[0].Disks {
				require.Equal(t, tc.wantID, disk.Datastore.ID)
				require.Equal(t, tc.wantType, disk.DatastoreType)
				require.Equal(t, tc.wantProtocol, disk.DatastoreStorageProtocol)
				require.Equal(t, tc.wantVendor, disk.DatastoreVendor)
				require.Equal(t, tc.wantModel, disk.DatastoreModel)
			}
			_, err = db.Exec(`DROP TABLE vhba; DROP TABLE vhostscsi`)
			require.NoError(t, err)
			summary, err := p.DiskTypeSummary(ctx, Filters{VMList: []string{"vm-1"}})
			require.NoError(t, err)
			if tc.wantType == "" {
				require.Empty(t, summary)
			} else {
				require.Equal(t, []inventory.DiskTypeSummary{{Type: tc.wantType, VMCount: 1, TotalSizeTB: 2}}, summary)
			}
		})
	}
}

func TestStorageQueries_DatastoreRowsKeepOwnMetadata(t *testing.T) {
	ctx := context.Background()
	p, db, cleanup := setupTestParser(t, nil)
	defer cleanup()
	_, err := db.Exec(`INSERT INTO vhost ("Object ID", "Host", "Cluster") VALUES ('host-1', 'esxi1', 'cluster1'), ('host-2', 'esxi2', 'cluster1');
INSERT INTO vhba ("Host ID", "Device", "Protocol") VALUES ('host-1', 'vmhba64', 'iSCSI'), ('host-2', 'vmhba64', 'FibreChannel');
INSERT INTO vhostscsi ("Host ID", "HBA Device", "Canonical Name", "LUN Vendor", "LUN Model") VALUES ('host-1', 'vmhba64', 'naa.a', 'NETAPP', 'LUN-A'), ('host-2', 'vmhba64', 'naa.b', 'PURE', 'LUN-B');
INSERT INTO vdatastore ("Object ID", "Name", "Hosts", "Backing Devices", "Type", "Capacity MiB") VALUES
('ds-reused', 'shared', 'host-1', '["naa.a"]', 'VMFS', 1024),
('ds-reused', 'shared', 'host-2', '["naa.b"]', 'VMFS', 2048),
('ds-reused', 'no-host', '', '["naa.orphan"]', 'VMFS', 3072);
INSERT INTO vinfo ("VM ID", "VM", "Host") VALUES ('vm-1', 'vm1', 'esxi1');
INSERT INTO vdisk ("VM ID", "Path") VALUES ('vm-1', '[shared] vm1/disk.vmdk');`)
	require.NoError(t, err)
	for _, filters := range []Filters{{}, {Cluster: "cluster1"}} {
		rows, err := p.Datastores(ctx, filters, Options{})
		require.NoError(t, err)
		wantCount := 3
		if filters.Cluster != "" {
			wantCount = 2
		}
		require.Len(t, rows, wantCount)
		for _, ds := range rows {
			switch ds.TotalCapacityGB {
			case 1:
				require.Equal(t, "host-1", ds.HostId)
				require.Equal(t, "iSCSI", ds.ProtocolType)
				require.Equal(t, "NETAPP", ds.Vendor)
				require.Equal(t, "LUN-A", ds.Model)
				require.Equal(t, models.BackingDeviceIDs{"naa.a"}, ds.BackingDeviceIDs)
			case 2:
				require.Equal(t, "host-2", ds.HostId)
				require.Equal(t, "FibreChannel", ds.ProtocolType)
				require.Equal(t, "PURE", ds.Vendor)
				require.Equal(t, "LUN-B", ds.Model)
				require.Equal(t, models.BackingDeviceIDs{"naa.b"}, ds.BackingDeviceIDs)
			case 3:
				require.Equal(t, "N/A", ds.HostId)
				require.Equal(t, "N/A", ds.ProtocolType)
				require.Equal(t, "N/A", ds.Vendor)
				require.Equal(t, "N/A", ds.Model)
				require.Equal(t, models.BackingDeviceIDs{"naa.orphan"}, ds.BackingDeviceIDs)
			default:
				t.Fatalf("unexpected capacity: %v", ds.TotalCapacityGB)
			}
		}
	}
	vms, err := p.VMs(ctx, Filters{}, Options{})
	require.NoError(t, err)
	require.Len(t, vms, 1)
	require.Len(t, vms[0].Disks, 1)
	require.Equal(t, "iSCSI", vms[0].Disks[0].DatastoreStorageProtocol)
	require.Equal(t, "NETAPP", vms[0].Disks[0].DatastoreVendor)
	require.Equal(t, "LUN-A", vms[0].Disks[0].DatastoreModel)
}

func TestVMStorageTopologyQuery_ScopesDisksWithGlobalHostResolution(t *testing.T) {
	for _, tc := range []struct {
		name                           string
		missingMetadata, ambiguousHost bool
		wantDatastores                 []string
	}{
		{name: "available metadata", wantDatastores: []string{"ds-1"}},
		{name: "missing metadata", missingMetadata: true, wantDatastores: []string{"ds-1"}},
		{name: "host ID collides with another host name", ambiguousHost: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, db, cleanup := setupTestParser(t, nil)
			defer cleanup()
			_, err := db.Exec(`INSERT INTO vhost ("Object ID", "Host", "Cluster") VALUES ('host-1', 'esxi1', 'cluster1'), ('host-2', 'esxi2', 'cluster2');
INSERT INTO vdatastore ("Object ID", "Name", "Hosts") VALUES ('ds-1', 'shared', 'esxi1'), ('ds-2', 'shared', 'esxi2');
INSERT INTO vinfo ("VM ID", "Host") VALUES ('vm-1', 'host-1'), ('vm-2', 'host-2');
INSERT INTO vdisk ("VM ID", "Path") VALUES ('vm-1', '[shared] vm1/disk.vmdk'), ('vm-2', '[shared] vm2/disk.vmdk');`)
			require.NoError(t, err)
			if tc.missingMetadata {
				_, err = db.Exec(`DROP TABLE vhba; DROP TABLE vhostscsi`)
				require.NoError(t, err)
			}
			if tc.ambiguousHost {
				_, err = db.Exec(`INSERT INTO vhost ("Object ID", "Host", "Cluster") VALUES ('host-3', 'host-1', 'cluster3')`)
				require.NoError(t, err)
			}
			topology, err := VMStorageTopologyQuery(context.Background(), db)
			require.NoError(t, err)
			var candidateVMs string
			var candidates int
			err = db.QueryRow(`WITH `+topology+` SELECT string_agg(DISTINCT "VM ID", ','), count(*) FROM storage_disk_candidates`, "vm-1").Scan(&candidateVMs, &candidates)
			require.NoError(t, err)
			require.Equal(t, "vm-1", candidateVMs)
			require.Equal(t, 2, candidates)
			rows, err := db.Query(`WITH `+topology+` SELECT ds."Object ID" FROM storage_disk_datastores resolved JOIN storage_datastores ds ON ds.datastore_row_id = resolved.datastore_row_id`, "vm-1")
			require.NoError(t, err)
			defer func() { require.NoError(t, rows.Close()) }()
			var ids []string
			for rows.Next() {
				var id string
				require.NoError(t, rows.Scan(&id))
				ids = append(ids, id)
			}
			require.NoError(t, rows.Err())
			require.Equal(t, tc.wantDatastores, ids)
		})
	}
}

func TestDatastoreIngestionDiagnostics(t *testing.T) {
	for _, source := range []string{"sqlite", "rvtools"} {
		t.Run(source, func(t *testing.T) {
			ctx := context.Background()
			parser, db, cleanup := setupTestParser(t, nil)
			defer cleanup()
			_, err := db.Exec(`INSERT INTO vdatastore ("Name", "Object ID") VALUES ('existing', 'ds-existing')`)
			require.NoError(t, err)

			var ingest func() (ValidationResult, error)
			if source == "sqlite" {
				path := createTestSQLite(t, "vcenter-1",
					[]sqliteCluster{{id: "cluster-1", name: "cluster1", datacenter: "dc1"}},
					[]sqliteVM{{id: "vm-1", name: "vm1", clusterName: "cluster1"}})
				_, err := db.Exec(`ATTACH '` + escapeSQLString(path) + `' AS setup (TYPE sqlite);
					INSERT INTO setup.Datastore (ID, Name, Type, Capacity, Free, BackingDevicesNames)
					VALUES ('ds-1', 'datastore1', 'VMFS', 1048576, 524288, '[]');
					DETACH setup;`)
				require.NoError(t, err)
				ingest = func() (ValidationResult, error) { return parser.IngestSqlite(ctx, path) }
			} else {
				path := createTestExcel(t, defaultStandardSheets([]map[string]string{
					{"VM ID": "vm-1", "VM": "vm1", "VI SDK UUID": "vcenter-1", "Cluster": "cluster1", "Datacenter": "dc1", "Host": "esxi-host-1"},
				}, []map[string]string{{"Host": "esxi-host-1", "Object ID": "host-1", "Cluster": "cluster1", "Datacenter": "dc1"}})...)
				ingest = func() (ValidationResult, error) { return parser.IngestRvTools(ctx, path) }
			}

			core, logs := observer.New(zap.InfoLevel)
			t.Cleanup(zap.ReplaceGlobals(zap.New(core)))
			for range 2 {
				result, err := ingest()
				require.NoError(t, err)
				require.False(t, result.HasErrors(), "%v", result.Errors)
				for _, warning := range result.Warnings {
					require.NotEqual(t, "DATASTORE_ROW_COUNT_MISMATCH", warning.Code)
				}
			}
			var total int
			require.NoError(t, db.QueryRow(`SELECT count(*) FROM vdatastore`).Scan(&total))
			require.Equal(t, 3, total)

			// A failed optional datastore write must remain nonfatal and visible.
			_, err = db.Exec(`ALTER TABLE vdatastore DROP COLUMN "Name"`)
			require.NoError(t, err)
			result, err := ingest()
			require.NoError(t, err)
			require.False(t, result.HasErrors(), "%v", result.Errors)

			require.Contains(t, result.Warnings, ValidationIssue{
				Code: "DATASTORE_ROW_COUNT_MISMATCH", Table: "vdatastore",
				Message: "Datastore row count mismatch: expected 1, imported 0",
			})

			entries := logs.FilterField(zap.String("source", source)).All()
			require.Len(t, entries, 3)
			for i, entry := range entries {
				require.EqualValues(t, 1, entry.ContextMap()["source_rows"])
				if i < 2 {
					require.Equal(t, zap.InfoLevel, entry.Level)
					require.EqualValues(t, 1, entry.ContextMap()["rows_inserted"], "report each file's added rows, not the accumulated total")
				} else {
					require.Equal(t, zap.WarnLevel, entry.Level)
					require.EqualValues(t, 0, entry.ContextMap()["rows_inserted"])
				}
			}

			for _, stage := range []string{"before", "after", "source"} {
				t.Run(stage+" count fails", func(t *testing.T) {
					parser = New(&datastoreCountFailureDB{DB: db, stage: stage}, nil)
					completedBefore := logs.FilterMessage("datastore ingestion completed").Len()
					result, err := ingest()
					if stage == "source" {
						require.NoError(t, err, "optional source data remains nonfatal")
						require.False(t, result.HasErrors())
						var countWarning *ValidationIssue
						for _, warning := range result.Warnings {
							if warning.Code == "DATASTORE_COUNT_FAILED" {
								countWarning = &warning
							}
						}
						require.NotNil(t, countWarning)
						require.Equal(t, "vdatastore", countWarning.Table)
						require.Contains(t, countWarning.Message, "injected datastore count failure")
					} else {
						require.ErrorContains(t, err, "counting datastores "+stage+" ingestion")
						require.ErrorContains(t, err, "injected datastore count failure")
					}
					require.Equal(t, completedBefore, logs.FilterMessage("datastore ingestion completed").Len())
				})
			}
		})
	}
}

type datastoreCountFailureDB struct {
	*sql.DB
	stage             string
	destinationCounts int
}

func (db *datastoreCountFailureDB) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	fail := false
	if query == "SELECT count(*) FROM vdatastore" {
		db.destinationCounts++
		fail = (db.stage == "before" && db.destinationCounts == 1) ||
			(db.stage == "after" && db.destinationCounts == 2)
	} else if db.stage == "source" {
		fail = strings.Contains(query, "SELECT count(*) FROM read_xlsx") ||
			strings.Contains(query, "SELECT count(*) FROM sqlite_scan")
	}
	if fail {
		return db.DB.QueryRowContext(ctx, "SELECT error('injected datastore count failure')")
	}
	return db.DB.QueryRowContext(ctx, query, args...)
}

func TestStorageQueries_NFSProtocolAliases(t *testing.T) {
	for _, alias := range []string{"NFS", "NFS41", "NFS4.1", "NFS 4.1", "  nfs 4.1  "} {
		t.Run(alias, func(t *testing.T) {
			ctx := context.Background()
			parser, db, cleanup := setupTestParser(t, nil)
			defer cleanup()
			_, err := db.Exec(`INSERT INTO vhost ("Object ID", "Host", "Cluster") VALUES ('host-1', 'esxi', 'cluster1')`)
			require.NoError(t, err)
			_, err = db.Exec(`INSERT INTO vdatastore ("Name", "Hosts", "Type", "Filesystem Type", "Capacity MiB", "Free MiB") VALUES
				('type-only', 'esxi', ?, NULL, 2, 1), ('filesystem', 'esxi', 'NFS', ?, 2, 1), ('blank-filesystem', 'esxi', ?, '   ', 2, 1)`, alias, alias, alias)
			require.NoError(t, err)
			for _, filters := range []Filters{{}, {Cluster: "cluster1"}} {
				datastores, err := parser.Datastores(ctx, filters, Options{})
				require.NoError(t, err)
				require.Len(t, datastores, 3)
				for _, datastore := range datastores {
					require.Equal(t, "NFS", datastore.ProtocolType, datastore.DiskId)
				}
			}

			path := filepath.Join(t.TempDir(), "legacy.duckdb")
			legacy := newPreStorageDatabase(t, path)
			_, err = legacy.Exec(`UPDATE vdatastore SET "Type"=?`, alias)
			require.NoError(t, err)
			require.NoError(t, legacy.Close())
			readonly, err := sql.Open("duckdb", path+"?access_mode=READ_ONLY")
			require.NoError(t, err)
			t.Cleanup(func() { _ = readonly.Close() })
			for _, filters := range []Filters{{}, {Cluster: "cluster1"}} {
				datastores, err := New(readonly, nil).Datastores(ctx, filters, Options{})
				require.NoError(t, err)
				require.Len(t, datastores, 1)
				require.Equal(t, "NFS", datastores[0].ProtocolType)
				require.Equal(t, alias, datastores[0].Type, "preserve the collected datastore type")
			}
		})
	}
}

func TestIngestSqlite_NFSProtocolAliases(t *testing.T) {
	for _, alias := range []string{"NFS", "NFS41", "NFS4.1", "NFS 4.1", "  nfs 4.1  "} {
		t.Run(alias, func(t *testing.T) {
			ctx := context.Background()
			parser, db, cleanup := setupTestParser(t, nil)
			defer cleanup()
			path := createTestSQLite(t, "vcenter-1",
				[]sqliteCluster{{id: "cluster-1", name: "cluster1", datacenter: "dc1"}},
				[]sqliteVM{{id: "vm-1", name: "vm1", clusterName: "cluster1"}})
			_, err := db.Exec("ATTACH '" + escapeSQLString(path) + "' AS setup (TYPE sqlite)")
			require.NoError(t, err)
			_, err = db.Exec(`INSERT INTO setup.Datastore (ID, Name, Type, FilesystemType, Capacity, Free, BackingDevicesNames) VALUES
				('ds-type', 'type-only', ?, NULL, 1048576, 524288, '[]'),
				('ds-fs', 'filesystem', 'NFS', ?, 1048576, 524288, '[]'),
				('ds-blank', 'blank-filesystem', ?, '   ', 1048576, 524288, '[]')`, alias, alias, alias)
			require.NoError(t, err)
			_, err = db.Exec(`UPDATE setup.Host SET Datastores='[{"id":"ds-type"},{"id":"ds-fs"},{"id":"ds-blank"}]';
				UPDATE setup.VM SET Disks='[{"key":2000,"file":"[type-only] vm/disk.vmdk","capacity":1073741824}]';
				DETACH setup`)
			require.NoError(t, err)
			result, err := parser.IngestSqlite(ctx, path)
			require.NoError(t, err)
			require.True(t, result.IsValid(), "%v", result.Errors)
			var protocols string
			require.NoError(t, db.QueryRow(`SELECT string_agg(COALESCE("Storage Protocol", 'missing'), ',' ORDER BY "Name") FROM vdatastore`).Scan(&protocols))
			require.Equal(t, "NFS,NFS,NFS", protocols, "ingestion must retain protocol for VM details as well as inventory")
			for _, filters := range []Filters{{}, {Cluster: "cluster1"}} {
				datastores, err := parser.Datastores(ctx, filters, Options{})
				require.NoError(t, err)
				require.Len(t, datastores, 3)
				for _, datastore := range datastores {
					require.Equal(t, "NFS", datastore.ProtocolType)
				}
			}
			vms, err := parser.VMs(ctx, Filters{VmId: "vm-1"}, Options{})
			require.NoError(t, err)
			require.Len(t, vms, 1)
			require.Len(t, vms[0].Disks, 1)
			require.Equal(t, "NFS", vms[0].Disks[0].DatastoreStorageProtocol)
		})
	}
}

func assertHostStorage(t *testing.T, p *Parser, filters Filters, id, adapters, luns string) {
	t.Helper()
	infra, err := p.buildInfraData(context.Background(), filters, func() (map[string]hostStorageFacts, error) {
		return p.hostStorage(context.Background())
	})
	require.NoError(t, err, "host %q (filters: %+v)", id, filters)
	public := converters.ToAPI(&inventory.Inventory{VCenter: &inventory.InventoryData{Infra: *infra}})
	require.NotNil(t, public.Vcenter.Infra.Hosts, "host %q: hosts missing (filters: %+v)", id, filters)
	for _, host := range *public.Vcenter.Infra.Hosts {
		if host.Id == nil || *host.Id != id {
			continue
		}
		data, err := json.Marshal(host)
		require.NoError(t, err, "host %q (filters: %+v)", id, filters)
		var fields map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(data, &fields))
		for name, want := range map[string]string{"storageAdapters": adapters, "storageLuns": luns} {
			if want == "" {
				require.NotContains(t, fields, name, "host %q: %s should be omitted (filters: %+v)", id, name, filters)
			} else {
				require.Contains(t, fields, name, "host %q: %s missing (filters: %+v)", id, name, filters)
				require.JSONEq(t, want, string(fields[name]), "host %q: %s mismatch (filters: %+v)", id, name, filters)
			}
		}
		return
	}
	t.Fatalf("host %q missing (filters: %+v)", id, filters)
}

func TestHosts_StorageAggregation(t *testing.T) {
	p, db, cleanup := setupTestParser(t, nil)
	defer cleanup()
	_, err := db.Exec(`INSERT INTO vhost ("Object ID", "Host", "Cluster") VALUES ('host-1', 'esxi1', 'cluster1'), ('host-2', 'esxi2', 'cluster2');
INSERT INTO vhba ("Host ID", "Device", "HBA Key", "Type", "Protocol", "Model", "Driver", "Node WWN", "Port WWN") VALUES
('host-1', ' vmhba1 ', 'hba-1', 'FC', 'FibreChannel', 'adapter', 'driver', '000123', '000456'),
('host-1', 'vmhba1', 'hba-1', 'FC', 'FibreChannel', 'conflicting model', 'driver', '000123', '000456'),
('host-1', 'vmhba2', 'hba-2', NULL, 'iSCSI', NULL, NULL, NULL, NULL),
('host-1', NULL, 'fallback', NULL, NULL, NULL, NULL, NULL, NULL),
('host-1', NULL, NULL, NULL, NULL, 'anonymous', NULL, NULL, NULL),
('host-2', 'vmhba1', 'hba-1', NULL, 'iSCSI', 'other host', NULL, NULL, NULL);
INSERT INTO vhostscsi ("Host ID", "HBA Device", "SCSI Disk Key", "Canonical Name", "LUN Vendor", "LUN Model", "LUN UUID") VALUES
('host-1', 'vmhba2', 'lun-1', ' naa.001 ', 'NETAPP', 'model', '000uuid'),
('host-1', 'vmhba1', 'lun-1', 'naa.001', 'NETAPP', 'model', '000uuid'),
('host-1', 'vmhba1', 'lun-1', 'naa.001', 'NETAPP', 'model', '000uuid'),
('host-1', 'unresolved', 'lun-1', 'naa.001', 'NETAPP', 'other model', '000uuid'),
('host-1', NULL, 'orphan', NULL, 'PURE', 'orphan model', NULL),
('host-1', NULL, NULL, NULL, NULL, NULL, 'uuid-only'),
('host-1', NULL, NULL, NULL, 'anonymous', 'anonymous', NULL),
('host-2', 'vmhba9', 'lun-1', 'naa.001', 'OTHER', NULL, NULL);`)
	require.NoError(t, err)
	for _, filters := range []Filters{{}, {Cluster: "cluster1"}, {VMList: []string{"vm-selected"}}} {
		assertHostStorage(t, p, filters, "host-1",
			`[{"key":"fallback"},{"device":"vmhba1","key":"hba-1","type":"FC","protocol":"FibreChannel","driver":"driver","nodeWwn":"000123","portWwn":"000456"},{"device":"vmhba2","key":"hba-2","protocol":"iSCSI"}]`,
			`[{"canonicalName":"naa.001","key":"lun-1","lunUuid":"000uuid","vendor":"NETAPP","adapterDevices":["unresolved","vmhba1","vmhba2"]},{"key":"orphan","vendor":"PURE","model":"orphan model"},{"lunUuid":"uuid-only"}]`)
	}
	assertHostStorage(t, p, Filters{Cluster: "cluster2"}, "host-2", `[{"device":"vmhba1","key":"hba-1","protocol":"iSCSI","model":"other host"}]`, `[{"canonicalName":"naa.001","key":"lun-1","vendor":"OTHER","adapterDevices":["vmhba9"]}]`)
	hosts, err := p.Hosts(context.Background(), Filters{}, Options{})
	require.NoError(t, err)
	require.Len(t, hosts, 2, "child joins must not multiply hosts")
	hosts, err = p.Hosts(context.Background(), Filters{Cluster: "cluster1"}, Options{})
	require.NoError(t, err)
	require.Len(t, hosts, 1)
}

func TestHosts_StorageOwnershipIsResolvedBeforeFiltering(t *testing.T) {
	p, db, cleanup := setupTestParser(t, nil)
	defer cleanup()
	_, err := db.Exec(`INSERT INTO vhost ("Object ID", "Host", "Cluster", "VI SDK UUID") VALUES ('host-1', 'esxi1', 'cluster1', 'vc-1');
INSERT INTO vhba ("Host ID", "Device", "Protocol") VALUES ('host-1', 'vmhba1', 'iSCSI');
INSERT INTO vhostscsi ("Host ID", "Canonical Name") VALUES ('host-1', 'naa.001');`)
	require.NoError(t, err)
	// Identical normalized owner rows are not ambiguous.
	_, err = db.Exec(`INSERT INTO vhost ("Object ID", "Host", "Cluster", "VI SDK UUID") VALUES ('host-1', ' ESXI1 ', 'cluster1', 'vc-1')`)
	require.NoError(t, err)
	assertHostStorage(t, p, Filters{Cluster: "cluster1"}, "host-1", `[{"device":"vmhba1","protocol":"iSCSI"}]`, `[{"canonicalName":"naa.001"}]`)
	// A different owner with a reused raw ID makes the children ambiguous globally.
	_, err = db.Exec(`INSERT INTO vhost ("Object ID", "Host", "Cluster") VALUES ('host-1', 'esxi2', 'cluster2')`)
	require.NoError(t, err)
	for _, filters := range []Filters{{}, {Cluster: "cluster1"}, {Cluster: "cluster2"}} {
		assertHostStorage(t, p, filters, "host-1", "", "")
	}
}

func TestHosts_StorageReadOnlyCompatibility(t *testing.T) {
	for _, tc := range []struct {
		name          string
		minimal       bool
		duplicateHost bool
		adapters      string
		luns          string
	}{
		{name: "no optional storage tables"},
		{name: "minimal storage tables", minimal: true, adapters: `[{"device":"vmhba1","protocol":"iSCSI"}]`, luns: `[{"canonicalName":"naa.001"}]`},
		{name: "duplicate legacy hosts without source identity", minimal: true, duplicateHost: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "collection.duckdb")
			db := newPreStorageDatabase(t, path)
			_, err := db.Exec(`DROP TABLE vhba`)
			require.NoError(t, err)
			if tc.minimal {
				_, err = db.Exec(`CREATE TABLE vhba ("Host ID" VARCHAR, "Device" VARCHAR, "Protocol" VARCHAR);
INSERT INTO vhba VALUES ('host-1', 'vmhba1', 'iSCSI');
CREATE TABLE vhostscsi ("Host ID" VARCHAR, "Canonical Name" VARCHAR);
INSERT INTO vhostscsi VALUES ('host-1', 'naa.001');`)
				require.NoError(t, err)
			}
			if tc.duplicateHost {
				_, err = db.Exec(`INSERT INTO vhost SELECT * FROM vhost`)
				require.NoError(t, err)
			}
			require.NoError(t, db.Close())
			readonly, err := sql.Open("duckdb", path+"?access_mode=READ_ONLY")
			require.NoError(t, err)
			t.Cleanup(func() { _ = readonly.Close() })
			assertHostStorage(t, New(readonly, nil), Filters{}, "host-1", tc.adapters, tc.luns)
		})
	}
}

func TestHosts_StorageQueryErrorsAreNotMissingMetadata(t *testing.T) {
	p, db, cleanup := setupTestParser(t, nil)
	defer cleanup()
	_, err := db.Exec(`INSERT INTO vhost ("Object ID", "Host") VALUES ('host-1', 'esxi1');
DROP TABLE vhba;
CREATE VIEW vhba AS SELECT 'host-1' AS "Host ID", error('storage source failed') AS "Device";`)
	require.NoError(t, err)
	_, err = p.Hosts(context.Background(), Filters{}, Options{})
	require.ErrorContains(t, err, "storage source failed")
	_, err = p.buildInfraData(context.Background(), Filters{}, func() (map[string]hostStorageFacts, error) {
		return p.hostStorage(context.Background())
	})
	require.ErrorContains(t, err, "storage source failed")
}

func TestHosts_StorageOwnershipIgnoresConfigStatus(t *testing.T) {
	for _, tc := range []struct {
		name         string
		firstSource  string
		secondSource string
		status       any
		wantStorage  bool
	}{
		{name: "yellow status", firstSource: "vc-1", secondSource: "vc-1", status: "yellow", wantStorage: true},
		{name: "red status", firstSource: "vc-1", secondSource: "vc-1", status: "red", wantStorage: true},
		{name: "gray status", firstSource: "vc-1", secondSource: "vc-1", status: "gray", wantStorage: true},
		{name: "missing status", firstSource: "vc-1", secondSource: "vc-1", wantStorage: true},
		{name: "empty status", firstSource: "vc-1", secondSource: "vc-1", status: "", wantStorage: true},
		{name: "different vCenters", firstSource: "vc-1", secondSource: "vc-2", status: "yellow"},
		{name: "unknown source", status: "yellow"},
		{name: "one unknown source", firstSource: "vc-1", status: "yellow"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, db, cleanup := setupTestParser(t, nil)
			defer cleanup()
			_, err := db.Exec(`INSERT INTO vhost ("Object ID", "Host", "Cluster", "VI SDK UUID", "Config status") VALUES
('host-1', 'esxi1', 'cluster1', ?, 'green'), ('host-1', 'esxi1', 'cluster1', ?, ?);`, tc.firstSource, tc.secondSource, tc.status)
			require.NoError(t, err)
			_, err = db.Exec(`INSERT INTO vhba ("Host ID", "Device") VALUES ('host-1', 'vmhba1');
INSERT INTO vhostscsi ("Host ID", "Canonical Name") VALUES ('host-1', 'naa.001');`)
			require.NoError(t, err)
			var adapters, luns string
			if tc.wantStorage {
				adapters = `[{"device":"vmhba1"}]`
				luns = `[{"canonicalName":"naa.001"}]`
			}
			assertHostStorage(t, p, Filters{Cluster: "cluster1"}, "host-1", adapters, luns)
		})
	}
}

func TestHosts_StorageOwnershipRejectsDifferentHostFacts(t *testing.T) {
	for _, change := range []string{`"Model" = 'different'`, `"Vendor" = 'different'`, `"# Cores" = 64`, `"# CPU" = 8`, `"# Memory" = 1024`, `"VMotion support" = true`, `"Storage VMotion support" = true`} {
		t.Run(change, func(t *testing.T) {
			p, db, cleanup := setupTestParser(t, nil)
			defer cleanup()
			_, err := db.Exec(`INSERT INTO vhost ("Object ID", "Host", "Cluster", "Datacenter", "VI SDK UUID") VALUES ('host-1', 'host-1', 'cluster1', 'dc1', 'vc-1');
INSERT INTO vhost SELECT * FROM vhost;
INSERT INTO vhba ("Host ID", "Device") VALUES ('host-1', 'vmhba1');
INSERT INTO vhostscsi ("Host ID", "Canonical Name") VALUES ('host-1', 'naa.001');
UPDATE vhost SET ` + change + ` WHERE rowid = (SELECT MAX(rowid) FROM vhost);`)
			require.NoError(t, err)
			assertHostStorage(t, p, Filters{Cluster: "cluster1"}, "host-1", "", "")
		})
	}
}

func TestIngestRvTools_HostStorageSourceOwnership(t *testing.T) {
	for _, tc := range []struct {
		name         string
		sources      []string
		sourceHeader string
		wantStorage  bool
	}{
		{name: "different vCenters with identical host facts", sources: []string{"vc-1", "vc-2"}},
		{name: "same vCenter duplicate host", sources: []string{"vc-1", "vc-1"}, wantStorage: true},
		{name: "source UUID from vHost", sources: []string{"vc-1", "vc-2"}, sourceHeader: "VI SDK UUID"},
		{name: "legacy source UUID from vHost", sources: []string{"vc-1", "vc-2"}, sourceHeader: "vCenter UUID"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, db, cleanup := setupTestParser(t, nil)
			defer cleanup()
			for i, source := range tc.sources {
				vmSource := source
				headers := append([]string(nil), vHostHeaders...)
				host := map[string]string{"Host": "esxi1", "Object ID": "host-1", "Cluster": "cluster1", "Datacenter": "dc1"}
				if tc.sourceHeader != "" {
					headers = append(headers, tc.sourceHeader)
					host[tc.sourceHeader] = source
					vmSource = "vc-metadata"
				}
				workbook := createTestExcel(t,
					NewExcelSheet("vInfo", vInfoHeaders, []map[string]string{{"VM ID": fmt.Sprintf("vm-%d", i), "VM": fmt.Sprintf("vm%d", i), "VI SDK UUID": vmSource, "Host": "esxi1", "Cluster": "cluster1"}}),
					NewExcelSheet("vHost", headers, []map[string]string{host}),
					NewExcelSheet("vHBA", []string{"Host", "Device", "Type", "WWN"}, []map[string]string{{"Host": "esxi1", "Device": "vmhba1", "Type": "Fibre Channel", "WWN": fmt.Sprintf("%016x %016x", i*2+1, i*2+2)}}),
					NewExcelSheet("vMultiPath", []string{"Host", "Disk", "UUID", "Path 1"}, []map[string]string{{"Host": "esxi1", "Disk": "naa.001", "UUID": fmt.Sprintf("lun-%d", i), "Path 1": "vmhba1:C0:T0:L0"}}),
				)
				result, err := p.IngestRvTools(context.Background(), workbook)
				require.NoError(t, err)
				require.True(t, result.IsValid(), "%v", result.Errors)
				var hostSource string
				require.NoError(t, db.QueryRow(`SELECT "VI SDK UUID" FROM vhost ORDER BY rowid DESC LIMIT 1`).Scan(&hostSource))
				require.Equal(t, source, hostSource, "explicit host source takes precedence over workbook fallback")
			}
			for _, filters := range []Filters{{}, {Cluster: "cluster1"}} {
				hosts, err := p.Hosts(context.Background(), filters, Options{})
				require.NoError(t, err)
				require.Len(t, hosts, 2, "storage enrichment preserves host membership")
				for _, host := range hosts {
					if tc.wantStorage {
						require.Len(t, host.StorageAdapters, 1)
						require.Equal(t, "0000000000000001", host.StorageAdapters[0].NodeWWN)
						require.Len(t, host.StorageLuns, 1)
					} else {
						require.Empty(t, host.StorageAdapters, "storage must not cross unknown or different source owners")
						require.Empty(t, host.StorageLuns, "storage must not cross unknown or different source owners")
					}
				}
			}
		})
	}
}
