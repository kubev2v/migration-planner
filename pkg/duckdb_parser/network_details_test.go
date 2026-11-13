package duckdb_parser

import (
	"context"
	"database/sql"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Network topology", func() {
	DescribeTable("retains VMkernel interfaces and topology from each source",
		func(source string) {
			rvtools := source == "rvtools" || source == "rvtools-legacy"
			ctx := context.Background()
			parser, db, cleanup := setupTestParser(GinkgoTB(), nil)
			DeferCleanup(cleanup)

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
				_, err := parser.IngestRvTools(ctx, createTestExcel(GinkgoTB(), sheets...))
				Expect(err).To(Succeed())
			} else {
				path := createTestSQLite(GinkgoTB(), "vc-1", []sqliteCluster{{id: "c1", name: "cluster1", datacenter: "dc1"}}, []sqliteVM{{id: "vm-1", name: "first", clusterName: "cluster1", nics: `[{"network":{"id":"net-std"},"mac":"01","deviceKey":1},{"network":{"id":"dpg-id"},"mac":"02","deviceKey":2}]`}})
				_, err := db.Exec(fmt.Sprintf("ATTACH '%s' AS fixture (TYPE sqlite)", escapeSQLString(path)))
				Expect(err).To(Succeed())
				_, err = db.Exec(`INSERT INTO fixture.Network (ID,Name,DVSwitch,VlanId,Variant,"Key") VALUES ('net-std','management',NULL,'','Standard',NULL),('dvs-1','distributed',NULL,'','DvSwitch',NULL),('dpg-id','storage','{"id":"dvs-1"}','','DvPortGroup','dpg-key'),('blank-group',' unlinked ','{"id":"source-switch"}','','DvPortGroup','');
UPDATE fixture.Host SET Network = '{"switches":[{"key":"switch-key","name":"vSwitch0","mtu":1500}],"portGroups":[{"key":"","name":"management","vSwitch":"switch-key","vlanId":0}],"vNICs":[{"key":"k0","device":"vmk0","portGroup":"management","mtu":1500},{"key":"k1","device":"vmk1","dPortGroup":"dpg-key","mtu":9000},{"key":"k2","device":"vmk2","portGroup":"missing","mtu":0},{"key":"","device":"vmk3","dPortGroup":"","portGroup":"","mtu":0}]}'`)
				Expect(err).To(Succeed())
				if source == "sqlite" {
					_, err = db.Exec(`UPDATE fixture.Network SET MaxMTU=9000 WHERE ID='dvs-1'`)
					Expect(err).To(Succeed())
				}
				_, err = db.Exec("DETACH fixture")
				Expect(err).To(Succeed())
				_, err = parser.IngestSqlite(ctx, path)
				Expect(err).To(Succeed())
			}

			var count int
			Expect(db.QueryRow(`SELECT COUNT(*) FROM vsc_vmk`).Scan(&count)).To(Succeed())
			Expect(count).To(Equal(4))

			var unknown sql.NullInt64
			Expect(db.QueryRow(`SELECT "MTU" FROM vsc_vmk WHERE "Device"='vmk2'`).Scan(&unknown)).To(Succeed())
			Expect(unknown.Valid).To(BeFalse())
			Expect(db.QueryRow(`SELECT "Max MTU" FROM dvswitch WHERE "Switch"='distributed'`).Scan(&unknown)).To(Succeed())
			Expect(unknown.Valid).To(Equal(source != "sqlite-unknown-mtu"))
			if source == "rvtools-legacy" {
				var id sql.NullString
				Expect(db.QueryRow(`SELECT "Object ID" FROM dvswitch WHERE "Switch"='distributed'`).Scan(&id)).To(Succeed())
				Expect(id.Valid).To(BeFalse())
				Expect(db.QueryRow(`SELECT "Object ID" FROM dvport WHERE "Port"='storage'`).Scan(&id)).To(Succeed())
				Expect(id.Valid).To(BeFalse())
			}

			var reference, identity, switchRef sql.NullString
			Expect(db.QueryRow(`SELECT "Port Group", "Key" FROM vsc_vmk WHERE "Device"='vmk3'`).Scan(&reference, &identity)).To(Succeed())
			Expect(reference.Valid).To(BeFalse())
			Expect(identity.Valid).To(BeFalse())
			Expect(db.QueryRow(`SELECT "Switch", "Switch ID", "Key" FROM dvport WHERE "Port"=' unlinked '`).Scan(&reference, &switchRef, &identity)).To(Succeed())
			Expect(identity.Valid).To(BeFalse())
			if rvtools {
				Expect(reference.String).To(Equal(" source-switch "))
				Expect(switchRef.Valid).To(BeFalse())
			} else {
				Expect(reference.Valid).To(BeFalse())
				Expect(switchRef.String).To(Equal("source-switch"))
			}
			Expect(db.QueryRow(`SELECT "Type", "Switch", "Network" FROM vnetwork WHERE "Mac Address"='01'`).Scan(&reference, &switchRef, &identity)).To(Succeed())
			if rvtools {
				Expect(reference.String).To(Equal("assigned"))
				vms, err := parser.VMs(ctx, Filters{}, Options{})
				Expect(err).To(Succeed())
				Expect(vms).To(HaveLen(1))
				Expect(vms[0].NICs).To(HaveLen(3))
				assignmentTypes := map[string]string{"01": "assigned", "02": "generated", "03": "manual"}

				for _, nic := range vms[0].NICs {
					Expect(nic.Type).To(Equal(assignmentTypes[nic.MAC]))
				}
				Expect(identity.String).To(Equal(" management "))
				Expect(switchRef.String).To(Equal("vSwitch0"))
				Expect(db.QueryRow(`SELECT "Type" FROM vnetwork WHERE "Mac Address"='02'`).Scan(&reference)).To(Succeed())
				Expect(reference.String).To(Equal("generated"))
			} else {
				Expect(reference.String).To(Equal("standard"))
				var groupType string
				Expect(db.QueryRow(`SELECT "Port Group Type" FROM vnetwork WHERE "Mac Address"='01'`).Scan(&groupType)).To(Succeed())
				Expect(groupType).To(Equal("standard"))
				Expect(db.QueryRow(`SELECT "Port Group Type" FROM vnetwork WHERE "Mac Address"='02'`).Scan(&groupType)).To(Succeed())
				Expect(groupType).To(Equal("distributed"))
				Expect(identity.String).To(Equal("management"))
				Expect(switchRef.Valid).To(BeFalse())
				Expect(db.QueryRow(`SELECT "Key" FROM vport WHERE "Port Group"='management'`).Scan(&identity)).To(Succeed())
				Expect(identity.Valid).To(BeFalse())
			}

			query := NetworkTopologyQuery()
			Expect(db.QueryRowContext(ctx, "WITH "+query+" SELECT COUNT(*) FROM network_vmkernel_links WHERE (interface->'portGroup') IS NOT NULL").Scan(&count)).To(Succeed())

			Expect(count).To(Equal(2))
			Expect(db.QueryRowContext(ctx, "WITH "+query+" SELECT COUNT(*) FROM network_adapter_links WHERE port_group IS NOT NULL").Scan(&count)).To(Succeed())
			if rvtools {
				Expect(count).To(Equal(3))
			} else {
				Expect(count).To(Equal(2))
			}

			Expect(parser.Cleanup()).To(Succeed())
			Expect(db.QueryRow(`SELECT COUNT(*) FROM vsc_vmk`).Scan(&count)).To(Succeed())
			Expect(count).To(BeZero())
		},
		Entry("RVTools with object IDs", "rvtools"),

		Entry("RVTools without object IDs", "rvtools-legacy"),

		Entry("Forklift with MTU", "sqlite"),

		Entry("Forklift without MTU", "sqlite-unknown-mtu"),
	)

	It("resolves host-scoped links without multiplying adapters or interfaces", func() {
		ctx := context.Background()
		_, db, cleanup := setupTestParser(GinkgoTB(), nil)
		DeferCleanup(cleanup)

		_, err := db.Exec(`INSERT INTO vinfo ("VM ID", "VM", "Host")
VALUES
    ('vm-1','first','host-1'),
    ('vm-2','second','host-2');

INSERT INTO vport ("Host", "Port Group", "Switch")
VALUES
    ('host-1','management','vSwitch0'),
    ('host-2','management','vSwitch0');

INSERT INTO vswitch
VALUES
    ('host-1','vSwitch0','key-1',1500),
    ('host-2','vSwitch0','key-2',9000);

INSERT INTO dvswitch
VALUES
    ('distributed','distributed','dvs-1',9000);

INSERT INTO dvport ("Port","Switch","Object ID","Key","Switch ID")
VALUES
    ('storage','distributed','dpg-id','dpg-key','dvs-1');

INSERT INTO dvport ("Port","Switch","Object ID")
VALUES
    ('management','vSwitch0','other-group');

INSERT INTO vsc_vmk
VALUES
    ('host-1','vmk0','key','management','standard',1500),
    ('host-1','vmk1','key2','dpg-key','distributed',9000),
    ('host-1','vmk2','key3','missing',NULL,NULL);

INSERT INTO vnetwork ("VM ID","Network","Mac Address","Switch","Type","Port Group Type")
VALUES
    ('vm-1','management','01','vSwitch0','assigned','standard'),
    ('vm-1','management','02','vSwitch0','generated','standard'),
    ('vm-2','management','03','vSwitch0','manual','standard');

INSERT INTO vsc_vmk SELECT * FROM vsc_vmk;`)
		Expect(err).To(Succeed())

		query := NetworkTopologyQuery()
		var interfaces string
		Expect(db.QueryRowContext(ctx, "WITH "+query+" SELECT interfaces FROM network_host_interfaces WHERE host='host-1'").Scan(&interfaces)).To(Succeed())
		Expect(interfaces).To(MatchJSON(`
[
    {
        "device": "vmk0",
        "mtu": 1500,
        "portGroupRef": "management",
        "portGroup": {
            "name": "management",
            "type": "standard",
            "switch": {
                "id": "key-1",
                "name": "vSwitch0",
                "type": "standard",
                "mtu": 1500
            }
        }
    },
    {
        "device": "vmk1",
        "mtu": 9000,
        "portGroupRef": "dpg-key",
        "portGroup": {
            "id": "dpg-id",
            "name": "storage",
            "type": "distributed",
            "switch": {
                "id": "dvs-1",
                "name": "distributed",
                "type": "distributed",
                "maxMtu": 9000
            }
        }
    },
    {
        "device": "vmk2",
        "portGroupRef": "missing"
    }
]
`))

		var count int
		Expect(db.QueryRowContext(ctx, "WITH "+query+" SELECT COUNT(*) FROM network_adapter_links WHERE port_group IS NOT NULL").Scan(&count)).To(Succeed())
		Expect(count).To(Equal(3))
	})

	DescribeTable("retains adapter details and clears previous port groups",
		func(withPorts bool) {
			ctx := context.Background()
			parser, _, cleanup := setupTestParser(GinkgoTB(), &testValidator{})
			DeferCleanup(cleanup)

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
			result, err := parser.IngestRvTools(ctx, createTestExcel(GinkgoTB(), sheets...))
			Expect(err).To(Succeed())
			Expect(result.IsValid()).To(BeTrue(), "%+v", result.Errors)
			got, err := parser.VMs(ctx, Filters{}, Options{})
			Expect(err).To(Succeed())
			Expect(got).To(HaveLen(2))
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
					Expect(nic.VlanID).To(Equal(wantVLAN), "MAC %s", nic.MAC)
					if nic.MAC == "01" {
						Expect(nic.IPv4Addresses).To(Equal([]string{"10.0.0.1", "10.0.0.2"}))
						Expect(nic.IPv6Addresses).To(Equal([]string{"2001:db8::1", "fe80::1"}))
					} else {
						Expect(nic.IPv4Addresses).To(BeEmpty())
						Expect(nic.IPv6Addresses).To(BeEmpty())
					}
				}
			}
			Expect(count).To(Equal(9), "VLAN lookup must not multiply or remove NICs")

			Expect(parser.Cleanup()).To(Succeed())
			result, err = parser.IngestRvTools(ctx, createTestExcel(GinkgoTB(), sheets[:2]...))
			Expect(err).To(Succeed())
			Expect(result.IsValid()).To(BeTrue(), "%+v", result.Errors)
			got, err = parser.VMs(ctx, Filters{}, Options{})
			Expect(err).To(Succeed())
			Expect(got).To(HaveLen(2))
			for _, vm := range got {
				for _, nic := range vm.NICs {
					Expect(nic.VlanID).To(BeEmpty(), "previous import's port groups must be cleared")
				}
			}
		},
		Entry("with port-group sheets", true),

		Entry("without port-group sheets", false),
	)

	DescribeTable("retains adapter details with partial host inventory",
		func(hostNetwork string) {
			ctx := context.Background()
			parser, _, cleanup := setupTestParser(GinkgoTB(), &testValidator{})
			DeferCleanup(cleanup)

			sqlitePath := createTestSQLite(GinkgoTB(), "vc-1",
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
			Expect(err).To(Succeed())
			DeferCleanup(src.Close)
			_, err = src.Exec(fmt.Sprintf("ATTACH '%s' AS dst (TYPE sqlite)", sqlitePath))
			Expect(err).To(Succeed())
			_, err = src.Exec(`UPDATE dst.Host SET Network = '{"portGroups":[{"name":"shared","vlanId":0},{"name":"management","vlanId":100},{"name":"ambiguous","vlanId":200},{"name":"ambiguous","vlanId":300}]}'`)
			Expect(err).To(Succeed())
			_, err = src.Exec(`INSERT INTO dst.Network (ID, Name, DVSwitch, VlanId) VALUES
		('standard', 'shared', '{"kind":"","id":""}', ''),
		('distributed', 'shared', '{"id":"dvs-1"}', '10-20,30-40'),
		('unknown', 'shared', '{"id":"dvs-2"}', ''),
		('management', 'management', NULL, ''),
		('ambiguous', 'ambiguous', NULL, '')`)
			Expect(err).To(Succeed())
			switch hostNetwork {
			case "null":
				_, err = src.Exec(`UPDATE dst.Host SET Network = NULL`)
			case "empty":
				_, err = src.Exec(`UPDATE dst.Host SET Network = '{}'`)
			case "missing-host":
				_, err = src.Exec(`UPDATE dst.VM SET Host = 'missing-host'`)
			}
			Expect(err).To(Succeed())
			_, err = src.Exec("DETACH dst")
			Expect(err).To(Succeed())
			result, err := parser.IngestSqlite(ctx, sqlitePath)
			Expect(err).To(Succeed())
			if hostNetwork == "missing-host" {
				Expect(result.Errors).To(HaveLen(1))
				Expect(result.Errors[0].Code).To(Equal(CodeMissingCluster))
			} else {
				Expect(result.IsValid()).To(BeTrue(), "%+v", result.Errors)
			}
			got, err := parser.VMs(ctx, Filters{}, Options{})
			Expect(err).To(Succeed())
			Expect(got).To(HaveLen(1))
			Expect(got[0].NICs).To(HaveLen(7))
			standardVLAN := ""
			if hostNetwork == "present" {
				standardVLAN = "0"
			}
			gotMACs := make([]string, 0, len(got[0].NICs))
			for _, nic := range got[0].NICs {
				gotMACs = append(gotMACs, nic.MAC)
				switch nic.MAC {
				case "AA:01":
					Expect(nic.Type).To(Equal("standard"))
					Expect(nic.VlanID).To(Equal(standardVLAN))
					Expect(nic.IPv4Addresses).To(Equal([]string{"10.0.0.1/24", "10.0.0.2/25", "10.0.0.3"}))
					Expect(nic.IPv6Addresses).To(Equal([]string{"::ffff:192.0.2.1/128", "fe80::1/64"}))
				case "aa:02":
					Expect(nic.Type).To(Equal("distributed"))
					Expect(nic.VlanID).To(Equal("10-20,30-40"))
					Expect(nic.IPv4Addresses).To(Equal([]string{"10.0.1.1/0"}))
					Expect(nic.IPv6Addresses).To(BeEmpty())
				case "aa:03":
					Expect(nic.VlanID).To(BeEmpty())
					Expect(nic.IPv4Addresses).To(BeEmpty())
					Expect(nic.IPv6Addresses).To(BeEmpty())
				case "aa:05":
					Expect(nic.Type).To(Equal("standard"))
					if hostNetwork == "present" {
						Expect(nic.VlanID).To(Equal("100"))
					} else {
						Expect(nic.VlanID).To(BeEmpty())
					}
					Expect(nic.IPv4Addresses).To(BeEmpty())
					Expect(nic.IPv6Addresses).To(BeEmpty())
				case "aa:04":
					Expect(nic.VlanID).To(Equal(standardVLAN))
					Expect(nic.IPv4Addresses).To(BeEmpty(), "duplicate MACs must not receive ambiguous addresses")
				case "aa:06":
					Expect(nic.Type).To(Equal("standard"))
					Expect(nic.VlanID).To(BeEmpty(), "duplicate port-group names must leave VLAN unknown without dropping adapters")
					Expect(nic.IPv4Addresses).To(Equal([]string{"10.0.6.1/24"}))
					Expect(nic.IPv6Addresses).To(BeEmpty())
				}
			}
			Expect(gotMACs).To(ConsistOf([]string{"AA:01", "aa:02", "aa:03", "aa:04", "aa:04", "aa:05", "aa:06"}))
		},
		Entry("host network present", "present"),

		Entry("host network null", "null"),

		Entry("host network empty", "empty"),

		Entry("host missing", "missing-host"),
	)

})
