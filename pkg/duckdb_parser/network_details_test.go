package duckdb_parser

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOMA2508RVToolsNetworkDetails(t *testing.T) {
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
				{"VM ID": "vm-1", "Network": "shared", "Mac Address": "01", "Switch": "vSwitch0", "IPv4 Address": "10.0.0.1;10.0.0.2", "IPv6 Address": "2001:db8::1;fe80::1"},
				{"VM ID": "vm-2", "Network": "shared", "Mac Address": "02", "Switch": "vSwitch0"},
				{"VM ID": "vm-1", "Network": "shared", "Mac Address": "03", "Switch": "dvs-1"},
				{"VM ID": "vm-1", "Network": "management", "Mac Address": "04", "Switch": "dvs-1"},
				{"VM ID": "vm-1", "Network": "unknown", "Mac Address": "05", "Switch": "dvs-1", "IPv4 Address": "VM"},
			}
			sheets := []ExcelSheet{
				NewExcelSheet("vInfo", vInfoHeaders, vms),
				NewExcelSheet("vNetwork", headers, nics),
			}
			if withPorts {
				sheets = append(sheets,
					NewExcelSheet("vPort", []string{"Host", "Port Group", "Switch", "VLAN"}, []map[string]string{
						{"Host": "host-1", "Port Group": "shared", "Switch": "vSwitch0", "VLAN": "0"},
						{"Host": "host-2", "Port Group": "shared", "Switch": "vSwitch0", "VLAN": "200"},
					}),
					NewExcelSheet("dvPort", []string{"Port", "Switch", "VLAN"}, []map[string]string{
						{"Port": "shared", "Switch": "dvs-1", "VLAN": "10-20,30-40"},
						{"Port": "shared", "Switch": "dvs-2", "VLAN": "300"},
						{"Port": "management", "Switch": "dvs-1", "VLAN": "100"},
						{"Port": "unknown", "Switch": "dvs-1", "VLAN": ""},
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
					assert.Empty(t, nic.IPv4CIDR)
					assert.Empty(t, nic.IPv6CIDR)
					wantVLAN := ""
					if withPorts {
						wantVLAN = map[string]string{"01": "0", "02": "200", "03": "10-20,30-40", "04": "100"}[nic.MAC]
					}
					assert.Equal(t, wantVLAN, nic.VlanID, "MAC %s", nic.MAC)
					if nic.MAC == "01" {
						assert.Equal(t, "10.0.0.1;10.0.0.2", nic.IPv4Address)
						assert.Equal(t, "2001:db8::1;fe80::1", nic.IPv6Address)
					} else {
						assert.Empty(t, nic.IPv4Address)
						assert.Empty(t, nic.IPv6Address)
					}
				}
			}
			assert.Equal(t, 5, count, "VLAN lookup must not multiply or remove NICs")

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

func TestOMA2508ForkliftNetworkDetails(t *testing.T) {
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
	_, err = src.Exec(`INSERT INTO dst.Network VALUES
		('standard', 'shared', '{"kind":"","id":""}', ''),
		('distributed', 'shared', '{"id":"dvs-1"}', '10-20,30-40'),
		('unknown', 'shared', '{"id":"dvs-2"}', ''),
		('management', 'management', NULL, ''),
		('ambiguous', 'ambiguous', NULL, '')`)
	require.NoError(t, err)
	_, err = src.Exec("DETACH dst")
	require.NoError(t, err)
	result, err := parser.IngestSqlite(ctx, sqlitePath)
	require.NoError(t, err)
	require.True(t, result.IsValid(), "%+v", result.Errors)
	got, err := parser.VMs(ctx, Filters{}, Options{})
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Len(t, got[0].NICs, 7)
	gotMACs := make([]string, 0, len(got[0].NICs))
	for _, nic := range got[0].NICs {
		gotMACs = append(gotMACs, nic.MAC)
		switch nic.MAC {
		case "AA:01":
			assert.Equal(t, "standard", nic.Type)
			assert.Equal(t, "0", nic.VlanID)
			assert.Equal(t, "10.0.0.1,10.0.0.2,10.0.0.3", nic.IPv4Address)
			assert.Equal(t, "::ffff:192.0.2.1,fe80::1", nic.IPv6Address)
			assert.Equal(t, "10.0.0.1/24,10.0.0.2/25", nic.IPv4CIDR)
			assert.Equal(t, "::ffff:192.0.2.1/128,fe80::1/64", nic.IPv6CIDR)
		case "aa:02":
			assert.Equal(t, "distributed", nic.Type)
			assert.Equal(t, "10-20,30-40", nic.VlanID)
			assert.Equal(t, "10.0.1.1", nic.IPv4Address)
			assert.Empty(t, nic.IPv6Address)
			assert.Equal(t, "10.0.1.1/0", nic.IPv4CIDR)
			assert.Empty(t, nic.IPv6CIDR)
		case "aa:03":
			assert.Empty(t, nic.VlanID)
			assert.Empty(t, nic.IPv4Address)
			assert.Empty(t, nic.IPv6Address)
			assert.Empty(t, nic.IPv4CIDR)
			assert.Empty(t, nic.IPv6CIDR)
		case "aa:05":
			assert.Equal(t, "standard", nic.Type)
			assert.Equal(t, "100", nic.VlanID)
			assert.Empty(t, nic.IPv4Address)
			assert.Empty(t, nic.IPv6Address)
			assert.Empty(t, nic.IPv4CIDR)
			assert.Empty(t, nic.IPv6CIDR)
		case "aa:04":
			assert.Equal(t, "0", nic.VlanID)
			assert.Empty(t, nic.IPv4Address, "duplicate MACs must not receive ambiguous addresses")
			assert.Empty(t, nic.IPv4CIDR)
			assert.Empty(t, nic.IPv6CIDR)
		case "aa:06":
			assert.Equal(t, "standard", nic.Type)
			assert.Empty(t, nic.VlanID, "duplicate port-group names must leave VLAN unknown without dropping adapters")
			assert.Equal(t, "10.0.6.1", nic.IPv4Address)
			assert.Equal(t, "10.0.6.1/24", nic.IPv4CIDR)
			assert.Empty(t, nic.IPv6Address)
			assert.Empty(t, nic.IPv6CIDR)
		}
	}
	assert.ElementsMatch(t, []string{"AA:01", "aa:02", "aa:03", "aa:04", "aa:04", "aa:05", "aa:06"}, gotMACs)
}
