package duckdb_parser

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVMkernelSourceRetention(t *testing.T) {
	for _, source := range []string{"rvtools", "rvtools-legacy", "sqlite", "sqlite-unknown-mtu"} {
		t.Run(source, func(t *testing.T) {
			rvtools := source == "rvtools" || source == "rvtools-legacy"
			ctx := context.Background()
			parser, db, cleanup := setupTestParser(t, nil)
			defer cleanup()
			if rvtools {
				vms := []map[string]string{{"VM ID": "vm-1", "VM": "first", "Host": "host-1", "Cluster": "cluster1", "VI SDK UUID": "vc-1"}}
				sheets := defaultStandardSheets(vms, nil)
				var kept []ExcelSheet
				for _, sheet := range sheets {
					if sheet.Name != "dvSwitch" && sheet.Name != "dvPort" && sheet.Name != "vPort" && sheet.Name != "vNetwork" {
						kept = append(kept, sheet)
					}
				}
				switchHeaders := []string{"Name", "Switch", "Max MTU"}
				portHeaders := []string{"Port", "Switch", "VLAN"}
				if source == "rvtools" {
					switchHeaders = append(switchHeaders, "Object ID")
					portHeaders = append(portHeaders, "Object ID")
				}
				sheets = append(kept,
					NewExcelSheet("vNetwork", []string{"VM ID", "Network", "Mac Address", "NIC label", "Adapter", "Switch", "Connected", "Starts Connected", "Type", "IPv4 Address", "IPv6 Address", "Cluster"}, []map[string]string{
						{"VM ID": "vm-1", "Network": " management ", "Mac Address": "01", "Switch": "vSwitch0", "Type": "assigned"},
						{"VM ID": "vm-1", "Network": "storage", "Mac Address": "02", "Switch": "distributed", "Type": "generated"},
						{"VM ID": "vm-1", "Network": " management ", "Mac Address": "03", "Switch": "vSwitch0", "Type": "manual"},
					}),
					NewExcelSheet("vSwitch", []string{"Host", "Switch", "MTU"}, []map[string]string{{"Host": "host-1", "Switch": "vSwitch0", "MTU": "1500"}}),
					NewExcelSheet("vPort", []string{"Host", "Port Group", "Switch", "VLAN"}, []map[string]string{{"Host": "host-1", "Port Group": " management ", "Switch": "vSwitch0"}}),
					NewExcelSheet("dvSwitch", switchHeaders, []map[string]string{{"Name": "distributed", "Switch": "distributed", "Object ID": "dvs-1", "Max MTU": "9000"}}),
					NewExcelSheet("dvPort", portHeaders, []map[string]string{{"Port": "storage", "Switch": "distributed", "Object ID": "dpg-id"}, {"Port": " unlinked ", "Switch": " source-switch ", "Object ID": ""}}),
					NewExcelSheet("vSC_VMK", []string{"Host", "Device", "Port Group", "MTU"}, []map[string]string{
						{"Host": "host-1", "Device": "vmk0", "Port Group": " management ", "MTU": "1500"},
						{"Host": "host-1", "Device": "vmk1", "Port Group": "storage", "MTU": "9000"},
						{"Host": "host-1", "Device": "vmk2", "Port Group": "missing"},
						{"Host": "host-1", "Device": "vmk3", "Port Group": ""},
					}))
				_, err := parser.IngestRvTools(ctx, createTestExcel(t, sheets...))
				require.NoError(t, err)
			} else {
				path := createTestSQLite(t, "vc-1", []sqliteCluster{{id: "c1", name: "cluster1", datacenter: "dc1"}}, []sqliteVM{{id: "vm-1", name: "first", clusterName: "cluster1", nics: `[{"network":{"id":"net-std"},"mac":"01","deviceKey":1},{"network":{"id":"dpg-id"},"mac":"02","deviceKey":2}]`}})
				_, err := db.Exec(fmt.Sprintf("ATTACH '%s' AS fixture (TYPE sqlite)", escapeSQLString(path)))
				require.NoError(t, err)
				_, err = db.Exec(`INSERT INTO fixture.Network (ID,Name,DVSwitch,VlanId,Variant,"Key") VALUES ('net-std','management',NULL,'','Standard',NULL),('dvs-1','distributed',NULL,'','DvSwitch',NULL),('dpg-id','storage','{"id":"dvs-1"}','','DvPortGroup','dpg-key'),('blank-group',' unlinked ','{"id":"source-switch"}','','DvPortGroup','');
UPDATE fixture.Host SET Network = '{"switches":[{"key":"switch-key","name":"vSwitch0","mtu":1500}],"portGroups":[{"key":"","name":"management","vSwitch":"switch-key","vlanId":0}],"vNICs":[{"key":"k0","device":"vmk0","portGroup":"management","mtu":1500},{"key":"k1","device":"vmk1","dPortGroup":"dpg-key","mtu":9000},{"key":"k2","device":"vmk2","portGroup":"missing","mtu":0},{"key":"","device":"vmk3","dPortGroup":"","portGroup":"","mtu":0}]}'`)
				require.NoError(t, err)
				if source == "sqlite" {
					_, err = db.Exec(`UPDATE fixture.Network SET MaxMTU=9000 WHERE ID='dvs-1'`)
					require.NoError(t, err)
				}
				_, err = db.Exec("DETACH fixture")
				require.NoError(t, err)
				_, err = parser.IngestSqlite(ctx, path)
				require.NoError(t, err)
			}
			var count int
			require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM vsc_vmk`).Scan(&count))
			require.Equal(t, 4, count)
			var unknown sql.NullInt64
			require.NoError(t, db.QueryRow(`SELECT "MTU" FROM vsc_vmk WHERE "Device"='vmk2'`).Scan(&unknown))
			require.False(t, unknown.Valid)
			require.NoError(t, db.QueryRow(`SELECT "Max MTU" FROM dvswitch WHERE "Switch"='distributed'`).Scan(&unknown))
			require.Equal(t, source != "sqlite-unknown-mtu", unknown.Valid)
			if source == "rvtools-legacy" {
				var id sql.NullString
				require.NoError(t, db.QueryRow(`SELECT "Object ID" FROM dvswitch WHERE "Switch"='distributed'`).Scan(&id))
				require.False(t, id.Valid)
				require.NoError(t, db.QueryRow(`SELECT "Object ID" FROM dvport WHERE "Port"='storage'`).Scan(&id))
				require.False(t, id.Valid)
			}
			var reference, identity, switchRef sql.NullString
			require.NoError(t, db.QueryRow(`SELECT "Port Group", "Key" FROM vsc_vmk WHERE "Device"='vmk3'`).Scan(&reference, &identity))
			require.False(t, reference.Valid)
			require.False(t, identity.Valid)
			require.NoError(t, db.QueryRow(`SELECT "Switch", "Switch ID", "Key" FROM dvport WHERE "Port"=' unlinked '`).Scan(&reference, &switchRef, &identity))
			require.False(t, identity.Valid)
			if rvtools {
				require.Equal(t, " source-switch ", reference.String)
				require.False(t, switchRef.Valid)
			} else {
				require.False(t, reference.Valid)
				require.Equal(t, "source-switch", switchRef.String)
			}
			require.NoError(t, db.QueryRow(`SELECT "Type", "Switch", "Network" FROM vnetwork WHERE "Mac Address"='01'`).Scan(&reference, &switchRef, &identity))
			if rvtools {
				require.Equal(t, "assigned", reference.String)
				vms, err := parser.VMs(ctx, Filters{}, Options{})
				require.NoError(t, err)
				require.Len(t, vms, 1)
				require.Len(t, vms[0].NICs, 3)
				for _, nic := range vms[0].NICs {
					require.Equal(t, map[string]string{"01": "assigned", "02": "generated", "03": "manual"}[nic.MAC], nic.Type)
				}
				require.Equal(t, " management ", identity.String)
				require.Equal(t, "vSwitch0", switchRef.String)
				require.NoError(t, db.QueryRow(`SELECT "Type" FROM vnetwork WHERE "Mac Address"='02'`).Scan(&reference))
				require.Equal(t, "generated", reference.String)
			} else {
				require.Equal(t, "standard", reference.String)
				var groupType string
				require.NoError(t, db.QueryRow(`SELECT "Port Group Type" FROM vnetwork WHERE "Mac Address"='01'`).Scan(&groupType))
				require.Equal(t, "standard", groupType)
				require.NoError(t, db.QueryRow(`SELECT "Port Group Type" FROM vnetwork WHERE "Mac Address"='02'`).Scan(&groupType))
				require.Equal(t, "distributed", groupType)
				require.Equal(t, "management", identity.String)
				require.False(t, switchRef.Valid)
				require.NoError(t, db.QueryRow(`SELECT "Key" FROM vport WHERE "Port Group"='management'`).Scan(&identity))
				require.False(t, identity.Valid)
			}
			query := NetworkTopologyQuery()
			require.NoError(t, db.QueryRowContext(ctx, "WITH "+query+" SELECT COUNT(*) FROM network_vmkernel_links WHERE (interface->'portGroup') IS NOT NULL").Scan(&count))

			require.Equal(t, 2, count)
			require.NoError(t, db.QueryRowContext(ctx, "WITH "+query+" SELECT COUNT(*) FROM network_adapter_links WHERE port_group IS NOT NULL").Scan(&count))
			if rvtools {
				require.Equal(t, 3, count)
			} else {
				require.Equal(t, 2, count)
			}
			require.NoError(t, parser.Cleanup())
			require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM vsc_vmk`).Scan(&count))
			require.Zero(t, count)
		})
	}
}

func TestNetworkTopologyRelationships(t *testing.T) {
	ctx := context.Background()
	_, db, cleanup := setupTestParser(t, nil)
	defer cleanup()
	_, err := db.Exec(`INSERT INTO vinfo ("VM ID", "VM", "Host") VALUES ('vm-1','first','host-1'),('vm-2','second','host-2');
INSERT INTO vport ("Host", "Port Group", "Switch") VALUES ('host-1','management','vSwitch0'),('host-2','management','vSwitch0');
INSERT INTO vswitch VALUES ('host-1','vSwitch0','key-1',1500),('host-2','vSwitch0','key-2',9000);
INSERT INTO dvswitch VALUES ('distributed','distributed','dvs-1',9000);
INSERT INTO dvport ("Port","Switch","Object ID","Key","Switch ID") VALUES ('storage','distributed','dpg-id','dpg-key','dvs-1');
INSERT INTO dvport ("Port","Switch","Object ID") VALUES ('management','vSwitch0','other-group');
INSERT INTO vsc_vmk VALUES ('host-1','vmk0','key','management','standard',1500),('host-1','vmk1','key2','dpg-key','distributed',9000),('host-1','vmk2','key3','missing',NULL,NULL);
INSERT INTO vnetwork ("VM ID","Network","Mac Address","Switch","Type","Port Group Type") VALUES ('vm-1','management','01','vSwitch0','assigned','standard'),('vm-1','management','02','vSwitch0','generated','standard'),('vm-2','management','03','vSwitch0','manual','standard');
INSERT INTO vsc_vmk SELECT * FROM vsc_vmk;`)
	require.NoError(t, err)
	query := NetworkTopologyQuery()
	var interfaces string
	require.NoError(t, db.QueryRowContext(ctx, "WITH "+query+" SELECT interfaces FROM network_host_interfaces WHERE host='host-1'").Scan(&interfaces))
	require.JSONEq(t, `[{"device":"vmk0","mtu":1500,"portGroupRef":"management","portGroup":{"name":"management","type":"standard","switch":{"id":"key-1","name":"vSwitch0","type":"standard","mtu":1500}}},{"device":"vmk1","mtu":9000,"portGroupRef":"dpg-key","portGroup":{"id":"dpg-id","name":"storage","type":"distributed","switch":{"id":"dvs-1","name":"distributed","type":"distributed","maxMtu":9000}}},{"device":"vmk2","portGroupRef":"missing"}]`, interfaces)
	var count int
	require.NoError(t, db.QueryRowContext(ctx, "WITH "+query+" SELECT COUNT(*) FROM network_adapter_links WHERE port_group IS NOT NULL").Scan(&count))
	require.Equal(t, 3, count)

}

func TestIngestRvTools_NetworkDetails(t *testing.T) {
	for _, withPorts := range []bool{true, false} {
		t.Run(fmt.Sprintf("portSheets=%t", withPorts), func(t *testing.T) {
			ctx := context.Background()
			parser, _, cleanup := setupTestParser(t, &testValidator{})
			defer cleanup()
			vms := []map[string]string{
				{"VM ID": "vm-1", "VM": "first", "Host": "host-1", "VI SDK UUID": "vc-1"},
				{"VM ID": "vm-2", "VM": "second", "Host": "host-2", "VI SDK UUID": "vc-1"},
			}
			for _, vm := range vms {
				vm["Cluster"] = "cluster1"
				vm["CPUs"] = "2"
				vm["Memory"] = "4096"
				vm["OS according to the configuration file"] = "Linux"
			}
			headers := []string{"VM ID", "Network", "Mac Address", "NIC label", "Adapter", "Switch", "Connected", "Starts Connected", "Type", "IPv4 Address", "IPv6 Address", "Cluster"}
			nics := []map[string]string{
				{"VM ID": "vm-1", "Network": "shared", "Mac Address": "01", "Switch": "vSwitch0", "IPv4 Address": " 10.0.0.1, 10.0.0.2, ,VM ", "IPv6 Address": "2001:db8::1, fe80::1"},
				{"VM ID": "vm-2", "Network": "shared", "Mac Address": "02", "Switch": "vSwitch0"},
				{"VM ID": "vm-1", "Network": "shared", "Mac Address": "03", "Switch": "dvs-1"},
				{"VM ID": "vm-1", "Network": "management", "Mac Address": "04", "Switch": "dvs-1"},
				{"VM ID": "vm-1", "Network": "unknown", "Mac Address": "05", "Switch": "dvs-1", "IPv4 Address": "VM"},
				{"VM ID": "vm-1", "Network": "ambiguous-standard", "Mac Address": "06", "Switch": "vSwitch0"},
				{"VM ID": "vm-1", "Network": "ambiguous-distributed", "Mac Address": "07", "Switch": "dvs-1"},
				{"VM ID": "vm-1", "Network": "overlap", "Mac Address": "08", "Switch": "same-switch"},
				{"VM ID": "vm-1", "Network": "blank-standard", "Mac Address": "09", "Switch": "same-switch"},
			}
			sheets := []ExcelSheet{
				NewExcelSheet("vInfo", vInfoHeaders, vms),
				NewExcelSheet("vNetwork", headers, nics),
			}
			if withPorts {
				sheets = append(sheets,
					NewExcelSheet("vPort", []string{"Host", "Port Group", "Switch", "VLAN"}, []map[string]string{
						{"Host": "host-1", "Port Group": "shared", "Switch": "vSwitch0", "VLAN": "0"},
						{"Host": "host-1", "Port Group": "shared", "Switch": "vSwitch0", "VLAN": "0"},
						{"Host": "host-2", "Port Group": "shared", "Switch": "vSwitch0", "VLAN": "200"},
						{"Host": "host-1", "Port Group": "ambiguous-standard", "Switch": "vSwitch0", "VLAN": "10"},
						{"Host": "host-1", "Port Group": "ambiguous-standard", "Switch": "vSwitch0", "VLAN": "20"},
						{"Host": "host-1", "Port Group": "overlap", "Switch": "same-switch", "VLAN": "50"},
						{"Host": "host-1", "Port Group": "blank-standard", "Switch": "same-switch", "VLAN": ""},
						{"Host": "host-2", "Port Group": "management", "Switch": "dvs-1", "VLAN": "200"},
					}),
					NewExcelSheet("dvPort", []string{"Port", "Switch", "VLAN"}, []map[string]string{
						{"Port": "shared", "Switch": "dvs-1", "VLAN": "10-20,30-40"},
						{"Port": "shared", "Switch": "dvs-2", "VLAN": "300"},
						{"Port": "management", "Switch": "dvs-1", "VLAN": "100"},
						{"Port": "management", "Switch": "dvs-1", "VLAN": "VM"},
						{"Port": "unknown", "Switch": "dvs-1", "VLAN": ""},
						{"Port": "ambiguous-distributed", "Switch": "dvs-1", "VLAN": "30"},
						{"Port": "ambiguous-distributed", "Switch": "dvs-1", "VLAN": "40"},
						{"Port": "overlap", "Switch": "same-switch", "VLAN": "60"},
						{"Port": "blank-standard", "Switch": "same-switch", "VLAN": "70"},
					}),
				)
			}
			result, err := parser.IngestRvTools(ctx, createTestExcel(t, sheets...))
			require.NoError(t, err)
			require.True(t, result.IsValid(), "%+v", result.Errors)
			got, err := parser.VMs(ctx, Filters{}, Options{})
			require.NoError(t, err)
			require.Len(t, got, 2)
			count := 0
			for _, vm := range got {
				for _, nic := range vm.NICs {
					count++
					wantVLAN := ""
					if withPorts {
						wantVLAN = map[string]string{
							"01": "0",
							"02": "200",
							"03": "10-20,30-40",
							"04": "100",
							"08": "", // Both port-group types match.
							"09": "", // A blank standard row still blocks the distributed VLAN.
						}[nic.MAC]
					}
					assert.Equal(t, wantVLAN, nic.VlanID, "MAC %s", nic.MAC)
					if nic.MAC == "01" {
						assert.Equal(t, []string{"10.0.0.1", "10.0.0.2"}, nic.IPv4Addresses)
						assert.Equal(t, []string{"2001:db8::1", "fe80::1"}, nic.IPv6Addresses)
					} else {
						assert.Empty(t, nic.IPv4Addresses)
						assert.Empty(t, nic.IPv6Addresses)
					}
				}
			}
			assert.Equal(t, 9, count, "VLAN lookup must not multiply or remove NICs")

			require.NoError(t, parser.Cleanup())
			result, err = parser.IngestRvTools(ctx, createTestExcel(t, sheets[:2]...))
			require.NoError(t, err)
			require.True(t, result.IsValid(), "%+v", result.Errors)
			got, err = parser.VMs(ctx, Filters{}, Options{})
			require.NoError(t, err)
			require.Len(t, got, 2)
			for _, vm := range got {
				for _, nic := range vm.NICs {
					assert.Empty(t, nic.VlanID, "previous import's port groups must be cleared")
				}
			}
		})
	}
}

func TestIngestSqlite_NetworkDetails(t *testing.T) {
	for _, hostNetwork := range []string{"present", "null", "empty", "missing-host"} {
		t.Run(hostNetwork, func(t *testing.T) {
			ctx := context.Background()
			parser, _, cleanup := setupTestParser(t, &testValidator{})
			defer cleanup()
			sqlitePath := createTestSQLite(t, "vc-1",
				[]sqliteCluster{{id: "c1", name: "cluster1", datacenter: "dc1"}},
				[]sqliteVM{{id: "vm-1", name: "first", clusterName: "cluster1", ipAddress: "192.0.2.99",
					nics: `[
				{"network":{"id":"standard"},"mac":"AA:01","deviceKey":100},
				{"network":{"id":"distributed"},"mac":"aa:02","deviceKey":101},
				{"network":{"id":"unknown"},"mac":"aa:03","deviceKey":102},
				{"network":{"id":"standard"},"mac":"aa:04","deviceKey":103},
				{"network":{"id":"standard"},"mac":"aa:04","deviceKey":104},
				{"network":{"id":"management"},"mac":"aa:05","deviceKey":105},
				{"network":{"id":"ambiguous"},"mac":"aa:06","deviceKey":106}
			]`,
					guestNetworks: `[
				{"mac":"different-mac","ip":"10.0.0.2","prefix":25,"deviceConfigId":100},
				{"mac":"aa:01","ip":"10.0.0.1","prefix":24,"deviceConfigId":100},
				{"mac":"aa:01","ip":"10.0.0.1","prefix":24,"deviceConfigId":100},
				{"mac":"aa:01","ip":"10.0.0.3","deviceConfigId":100},
				{"mac":"aa:01","ip":"fe80::1","prefix":64,"deviceConfigId":100},
				{"mac":"aa:01","ip":"::ffff:192.0.2.1","prefix":128,"deviceConfigId":100},
				{"mac":"AA:02","ip":"10.0.1.1","prefix":0,"deviceConfigId":-1},
				{"mac":"aa:02","ip":"10.0.9.9","prefix":24,"deviceConfigId":999},
				{"mac":"aa:04","ip":"10.0.4.1","prefix":24,"deviceConfigId":-1},
				{"mac":"aa:06","ip":"10.0.6.1","prefix":24,"deviceConfigId":106}
			]`,
				}})
			src, err := sql.Open("duckdb", "")
			require.NoError(t, err)
			defer func() { _ = src.Close() }()
			_, err = src.Exec(fmt.Sprintf("ATTACH '%s' AS dst (TYPE sqlite)", sqlitePath))
			require.NoError(t, err)
			_, err = src.Exec(`UPDATE dst.Host SET Network = '{"portGroups":[{"name":"shared","vlanId":0},{"name":"management","vlanId":100},{"name":"ambiguous","vlanId":200},{"name":"ambiguous","vlanId":300}]}'`)
			require.NoError(t, err)
			_, err = src.Exec(`INSERT INTO dst.Network (ID, Name, DVSwitch, VlanId) VALUES
		('standard', 'shared', '{"kind":"","id":""}', ''),
		('distributed', 'shared', '{"id":"dvs-1"}', '10-20,30-40'),
		('unknown', 'shared', '{"id":"dvs-2"}', ''),
		('management', 'management', NULL, ''),
		('ambiguous', 'ambiguous', NULL, '')`)
			require.NoError(t, err)
			switch hostNetwork {
			case "null":
				_, err = src.Exec(`UPDATE dst.Host SET Network = NULL`)
			case "empty":
				_, err = src.Exec(`UPDATE dst.Host SET Network = '{}'`)
			case "missing-host":
				_, err = src.Exec(`UPDATE dst.VM SET Host = 'missing-host'`)
			}
			require.NoError(t, err)
			_, err = src.Exec("DETACH dst")
			require.NoError(t, err)
			result, err := parser.IngestSqlite(ctx, sqlitePath)
			require.NoError(t, err)
			if hostNetwork == "missing-host" {
				require.Len(t, result.Errors, 1)
				require.Equal(t, CodeMissingCluster, result.Errors[0].Code)
			} else {
				require.True(t, result.IsValid(), "%+v", result.Errors)
			}
			got, err := parser.VMs(ctx, Filters{}, Options{})
			require.NoError(t, err)
			require.Len(t, got, 1)
			require.Len(t, got[0].NICs, 7)
			standardVLAN := ""
			if hostNetwork == "present" {
				standardVLAN = "0"
			}
			gotMACs := make([]string, 0, len(got[0].NICs))
			for _, nic := range got[0].NICs {
				gotMACs = append(gotMACs, nic.MAC)
				switch nic.MAC {
				case "AA:01":
					assert.Equal(t, "standard", nic.Type)
					assert.Equal(t, standardVLAN, nic.VlanID)
					assert.Equal(t, []string{"10.0.0.1/24", "10.0.0.2/25", "10.0.0.3"}, nic.IPv4Addresses)
					assert.Equal(t, []string{"::ffff:192.0.2.1/128", "fe80::1/64"}, nic.IPv6Addresses)
				case "aa:02":
					assert.Equal(t, "distributed", nic.Type)
					assert.Equal(t, "10-20,30-40", nic.VlanID)
					assert.Equal(t, []string{"10.0.1.1/0"}, nic.IPv4Addresses)
					assert.Empty(t, nic.IPv6Addresses)
				case "aa:03":
					assert.Empty(t, nic.VlanID)
					assert.Empty(t, nic.IPv4Addresses)
					assert.Empty(t, nic.IPv6Addresses)
				case "aa:05":
					assert.Equal(t, "standard", nic.Type)
					if hostNetwork == "present" {
						assert.Equal(t, "100", nic.VlanID)
					} else {
						assert.Empty(t, nic.VlanID)
					}
					assert.Empty(t, nic.IPv4Addresses)
					assert.Empty(t, nic.IPv6Addresses)
				case "aa:04":
					assert.Equal(t, standardVLAN, nic.VlanID)
					assert.Empty(t, nic.IPv4Addresses, "duplicate MACs must not receive ambiguous addresses")
				case "aa:06":
					assert.Equal(t, "standard", nic.Type)
					assert.Empty(t, nic.VlanID, "duplicate port-group names must leave VLAN unknown without dropping adapters")
					assert.Equal(t, []string{"10.0.6.1/24"}, nic.IPv4Addresses)
					assert.Empty(t, nic.IPv6Addresses)
				}
			}
			assert.ElementsMatch(t, []string{"AA:01", "aa:02", "aa:03", "aa:04", "aa:04", "aa:05", "aa:06"}, gotMACs)
		})
	}
}
