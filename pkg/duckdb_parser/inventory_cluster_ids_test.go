package duckdb_parser

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kubev2v/migration-planner/pkg/inventory"
	pkgstore "github.com/kubev2v/migration-planner/pkg/store"
	_ "github.com/marcboeker/go-duckdb/v2"
	"github.com/stretchr/testify/require"
)

func newClusterIDTestParser(t *testing.T) (*Parser, *sql.DB, pkgstore.Transactor) {
	t.Helper()
	db, err := sql.Open("duckdb", "")
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	parser := New(pkgstore.NewQueryInterceptor(db), nil)
	require.NoError(t, parser.Init())
	return parser, db, pkgstore.NewTransactor(db)
}

func seedClusterIDVM(t *testing.T, db *sql.DB, id, name, dc string) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO vinfo
        ("VM ID", "VM", "Cluster", "Datacenter", "CPUs", "Memory", "Template")
        VALUES (?, ?, ?, ?, 2, 4096, false)`, id, id, name, dc)
	require.NoError(t, err)
}

func buildClusterIDTestInventory(
	ctx context.Context, p *Parser, tx pkgstore.Transactor, vmIDs []string,
) (*inventory.Inventory, error) {
	var inv *inventory.Inventory
	err := tx.WithTx(ctx, func(txCtx context.Context) error {
		var err error
		inv, err = p.BuildInventory(txCtx, vmIDs)
		return err
	})
	return inv, err
}

// Intercept only identity writes. All other operations retain transaction routing.
type clusterIDTestInterceptor struct {
	pkgstore.QueryInterceptor
	writes        int
	failAt        int
	failQuery     string
	failVCenter   bool
	failAmbiguity bool
}

func (q *clusterIDTestInterceptor) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	upper := strings.ToUpper(query)
	if strings.Contains(upper, "UPDATE VCLUSTER") || strings.Contains(upper, "INSERT INTO VCLUSTER") {
		q.writes++
		if q.failAt > 0 && q.writes == q.failAt {
			return nil, errors.New("injected cluster ID write failure")
		}
	}
	return q.QueryInterceptor.ExecContext(ctx, query, args...)
}

func (q *clusterIDTestInterceptor) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	if q.failQuery != "" && strings.Contains(query, q.failQuery) {
		return nil, errors.New("injected identity lookup failure")
	}
	return q.QueryInterceptor.QueryContext(ctx, query, args...)
}

func (q *clusterIDTestInterceptor) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	if q.failAmbiguity && strings.Contains(query, "-- cluster ID identity ambiguity") {
		// Cause a Scan error without aborting the caller transaction.
		return q.QueryInterceptor.QueryRowContext(ctx, `SELECT 'injected ambiguity lookup failure'`)
	}
	if q.failVCenter && strings.Contains(query, "FROM about") {
		// about scans three columns; this one-column result causes a Scan
		// error without issuing invalid SQL that could abort the transaction.
		return q.QueryInterceptor.QueryRowContext(ctx, `SELECT 'injected identity lookup failure'`)
	}
	return q.QueryInterceptor.QueryRowContext(ctx, query, args...)
}

func TestBuildInventory_ClusterIDsRepairMissingAndPreserveValid(t *testing.T) {
	values := []any{nil, "", " ", "\t\n", "VM", " VM ", "\tVM\n",
		"domain-c34", " domain-c34 ", "VM-domain-c34"}
	for i, objectID := range values {
		t.Run(fmt.Sprintf("case-%d", i), func(t *testing.T) {
			p, db, tx := newClusterIDTestParser(t)
			seedClusterIDVM(t, db, "vm-a", "cluster-a", "dc-a")
			_, err := db.Exec(`INSERT INTO about ("InstanceUuid") VALUES ('vc-a')`)
			require.NoError(t, err)
			_, err = db.Exec(`INSERT INTO vcluster
                ("Name", "Object ID", "DrsEnabled", "DrsDefaultVmBehavior", "DasEnabled")
                VALUES ('cluster-a', ?, true, 'fullyAutomated', true)`, objectID)
			require.NoError(t, err)
			recorder := &clusterIDTestInterceptor{QueryInterceptor: p.db}
			p.db = recorder
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			inv, err := buildClusterIDTestInventory(ctx, p, tx, nil)
			require.NoError(t, err)
			expected := generateClusterID("cluster-a", "dc-a", "vc-a")
			missing := true
			if id, ok := objectID.(string); ok && strings.TrimSpace(id) != "" && strings.TrimSpace(id) != "VM" {
				expected = id
				missing = false
			}
			require.Contains(t, inv.Clusters, expected)
			if missing {
				require.Equal(t, 2, recorder.writes)
			} else {
				require.Zero(t, recorder.writes)
			}
			var id, behavior string
			var drs, ha bool
			require.NoError(t, db.QueryRow(`SELECT "Object ID", "DrsEnabled",
                "DrsDefaultVmBehavior", "DasEnabled" FROM vcluster`).Scan(&id, &drs, &behavior, &ha))
			require.Equal(t, expected, id)
			require.True(t, drs)
			require.True(t, ha)
			require.Equal(t, "fullyAutomated", behavior)
		})
	}
}

func TestBuildInventory_ClusterIDsInsertNamesAndRebuildWithoutWrites(t *testing.T) {
	p, db, tx := newClusterIDTestParser(t)
	seedClusterIDVM(t, db, "vm-a", "cluster-a", "dc-a")
	seedClusterIDVM(t, db, "vm-b", "cluster-b", "dc-b")
	recorder := &clusterIDTestInterceptor{QueryInterceptor: p.db}
	p.db = recorder
	ctx := context.Background()
	inv, err := buildClusterIDTestInventory(ctx, p, tx, nil)
	require.NoError(t, err)
	require.Equal(t, 4, recorder.writes)
	for name, dc := range map[string]string{"cluster-a": "dc-a", "cluster-b": "dc-b"} {
		var id string
		var behavior sql.NullString
		var drs, ha sql.NullBool
		require.NoError(t, db.QueryRow(`SELECT "Object ID", "DrsEnabled",
            "DrsDefaultVmBehavior", "DasEnabled" FROM vcluster WHERE "Name" = ?`, name).
			Scan(&id, &drs, &behavior, &ha))
		require.Equal(t, generateClusterID(name, dc, ""), id)
		require.Contains(t, inv.Clusters, id)
		require.False(t, drs.Valid)
		require.False(t, ha.Valid)
		require.False(t, behavior.Valid)
		require.Nil(t, inv.Clusters[id].ClusterFeatures)
	}
	for _, vmIDs := range [][]string{nil, {"vm-a"}} {
		recorder.writes = 0
		rebuilt, err := buildClusterIDTestInventory(ctx, p, tx, vmIDs)
		require.NoError(t, err)
		require.Contains(t, rebuilt.Clusters, generateClusterID("cluster-a", "dc-a", ""))
		require.Zero(t, recorder.writes)
		for _, cluster := range rebuilt.Clusters {
			require.Nil(t, cluster.ClusterFeatures)
		}
	}
	var n int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM vcluster`).Scan(&n))
	require.Equal(t, 2, n)
}

func TestBuildInventory_ClusterIDsPreserveDisabledSourceFeatures(t *testing.T) {
	for _, objectID := range []any{nil, "domain-c34"} {
		t.Run(fmt.Sprintf("id=%v", objectID), func(t *testing.T) {
			p, db, tx := newClusterIDTestParser(t)
			seedClusterIDVM(t, db, "vm-a", "cluster-a", "dc-a")
			_, err := db.Exec(`INSERT INTO vcluster
                ("Name", "Object ID", "DrsEnabled", "DrsDefaultVmBehavior", "DasEnabled")
                VALUES ('cluster-a', ?, false, 'None', false)`, objectID)
			require.NoError(t, err)
			for i := 0; i < 2; i++ {
				inv, err := buildClusterIDTestInventory(context.Background(), p, tx, nil)
				require.NoError(t, err)
				expected := generateClusterID("cluster-a", "dc-a", "")
				if objectID != nil {
					expected = objectID.(string)
				}
				require.Contains(t, inv.Clusters, expected)
				features := inv.Clusters[expected].ClusterFeatures
				require.NotNil(t, features)
				require.NotNil(t, features.DrsEnabled)
				require.NotNil(t, features.HaEnabled)
				require.False(t, *features.DrsEnabled)
				require.False(t, *features.HaEnabled)
				var drs, ha bool
				var behavior string
				require.NoError(t, db.QueryRow(`SELECT "DrsEnabled", "DrsDefaultVmBehavior",
                    "DasEnabled" FROM vcluster WHERE "Name" = 'cluster-a'`).Scan(&drs, &behavior, &ha))
				require.False(t, drs)
				require.False(t, ha)
				require.Equal(t, "None", behavior)
			}
		})
	}
}

func TestBuildInventory_ClusterIDsRollbackOnWriteFailure(t *testing.T) {
	p, db, tx := newClusterIDTestParser(t)
	seedClusterIDVM(t, db, "vm-a", "cluster-a", "dc-a")
	seedClusterIDVM(t, db, "vm-b", "cluster-b", "dc-b")
	// First cluster INSERT succeeds on write 2; second INSERT fails on write 4.
	recorder := &clusterIDTestInterceptor{QueryInterceptor: p.db, failAt: 4}
	p.db = recorder
	inv, err := buildClusterIDTestInventory(context.Background(), p, tx, nil)
	require.ErrorContains(t, err, "injected cluster ID write failure")
	require.Nil(t, inv)
	require.Equal(t, 4, recorder.writes)
	var n int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM vcluster`).Scan(&n))
	require.Zero(t, n)
}

func TestBuildInventory_ClusterIDsLookupErrorsContinueWithoutWrites(t *testing.T) {
	cases := []struct {
		name, query string
		vcenter     bool
	}{
		{name: "Object IDs", query: "FROM vcluster"},
		{name: "datacenters", query: `SELECT DISTINCT "Cluster", "Datacenter"`},
		{name: "vCenter", vcenter: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, db, tx := newClusterIDTestParser(t)
			seedClusterIDVM(t, db, "vm-a", "cluster-a", "dc-a")
			seedClusterIDVM(t, db, "vm-b", "cluster-b", "dc-b")
			_, err := db.Exec(`INSERT INTO about ("InstanceUuid") VALUES ('vc-a')`)
			require.NoError(t, err)
			_, err = db.Exec(`INSERT INTO vcluster ("Name", "Object ID")
                VALUES ('cluster-a', 'domain-c34')`)
			require.NoError(t, err)
			recorder := &clusterIDTestInterceptor{QueryInterceptor: p.db,
				failQuery: tc.query, failVCenter: tc.vcenter}
			p.db = recorder
			inv, err := buildClusterIDTestInventory(context.Background(), p, tx, nil)
			require.NoError(t, err)
			require.NotNil(t, inv)
			require.Zero(t, recorder.writes)
			vcenter, datacenter := "vc-a", "dc-b"
			if tc.vcenter {
				vcenter = ""
			}
			if tc.name == "datacenters" {
				datacenter = ""
			}
			require.Contains(t, inv.Clusters, generateClusterID("cluster-b", datacenter, vcenter))
			if tc.name == "Object IDs" {
				// Existing ID remains stored, although the failed lookup uses
				// a generated ID in this transient inventory.
				require.Contains(t, inv.Clusters, generateClusterID("cluster-a", "dc-a", "vc-a"))
			} else {
				require.Contains(t, inv.Clusters, "domain-c34")
			}
			var id string
			require.NoError(t, db.QueryRow(`SELECT "Object ID" FROM vcluster`).Scan(&id))
			require.Equal(t, "domain-c34", id)
			var n int
			require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM vcluster`).Scan(&n))
			require.Equal(t, 1, n)

			// Recovery must use the complete metadata and persist only the
			// genuinely missing cluster; no degraded ID was made permanent.
			recorder.failQuery = ""
			recorder.failVCenter = false
			recovered, err := buildClusterIDTestInventory(context.Background(), p, tx, nil)
			require.NoError(t, err)
			require.Contains(t, recovered.Clusters, "domain-c34")
			expected := generateClusterID("cluster-b", "dc-b", "vc-a")
			require.Contains(t, recovered.Clusters, expected)
			require.Equal(t, 2, recorder.writes)
			require.NoError(t, db.QueryRow(`SELECT "Object ID" FROM vcluster
                WHERE "Name" = 'cluster-b'`).Scan(&id))
			require.Equal(t, expected, id)
		})
	}
}

func TestBuildInventory_ClusterIDsReadOnlyDatabase(t *testing.T) {
	for _, existingID := range []bool{false, true} {
		t.Run(fmt.Sprintf("persisted=%t", existingID), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "collection.duckdb")
			writable, err := sql.Open("duckdb", path)
			require.NoError(t, err)
			t.Cleanup(func() { _ = writable.Close() })
			p := New(pkgstore.NewQueryInterceptor(writable), nil)
			require.NoError(t, p.Init())
			seedClusterIDVM(t, writable, "vm-a", "cluster-a", "dc-a")
			if existingID {
				_, err = writable.Exec(`INSERT INTO vcluster ("Name", "Object ID")
                    VALUES ('cluster-a', 'domain-c34')`)
				require.NoError(t, err)
			}
			require.NoError(t, writable.Close())
			readDB, err := sql.Open("duckdb", "")
			require.NoError(t, err)
			t.Cleanup(func() { _ = readDB.Close() })
			readDB.SetMaxOpenConns(1)
			_, err = readDB.Exec(fmt.Sprintf("ATTACH '%s' AS snapshot (READ_ONLY)", strings.ReplaceAll(path, "'", "''")))
			require.NoError(t, err)
			_, err = readDB.Exec("USE snapshot")
			require.NoError(t, err)
			recorder := &clusterIDTestInterceptor{QueryInterceptor: pkgstore.NewQueryInterceptor(readDB)}
			parser := New(recorder, nil) // schema already exists; do not call Init
			inv, err := parser.BuildInventory(context.Background(), nil)
			if existingID {
				require.NoError(t, err)
				require.Contains(t, inv.Clusters, "domain-c34")
				require.Zero(t, recorder.writes)
			} else {
				require.ErrorContains(t, err, "persisting ID for cluster")
				require.Nil(t, inv)
				require.Equal(t, 1, recorder.writes)
			}
		})
	}
}

func TestBuildInventory_ClusterIDsSkipAmbiguousNames(t *testing.T) {
	cases := []struct {
		name, secondDC, secondVC string
		ambiguous                bool
	}{
		{"different datacenters", "dc-b", "vc-a", true},
		{"different vCenters", "dc-a", "vc-b", true},
		{"same identity repeated", "dc-a", "vc-a", false},
	}
	for _, tc := range cases {
		for _, missingRow := range []bool{false, true} {
			for _, subset := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/missing=%t/subset=%t", tc.name, missingRow, subset), func(t *testing.T) {
					p, db, tx := newClusterIDTestParser(t)
					seedClusterIDVM(t, db, "vm-a", "production", "dc-a")
					seedClusterIDVM(t, db, "vm-b", "production", tc.secondDC)
					_, err := db.Exec(`UPDATE vinfo SET "VI SDK UUID" =
                        CASE WHEN "VM ID" = 'vm-a' THEN 'vc-a' ELSE ? END`, tc.secondVC)
					require.NoError(t, err)
					if !missingRow {
						_, err = db.Exec(`INSERT INTO vcluster ("Name", "Object ID")
                            VALUES ('production', NULL), ('production', ' VM ')`)
						require.NoError(t, err)
					}
					recorder := &clusterIDTestInterceptor{QueryInterceptor: p.db}
					p.db = recorder
					var vmIDs []string
					if subset {
						vmIDs = []string{"vm-a"}
					}
					inv, err := buildClusterIDTestInventory(context.Background(), p, tx, vmIDs)
					require.NoError(t, err)
					require.NotNil(t, inv) // Existing name-based inventory remains available.
					if tc.ambiguous {
						require.Zero(t, recorder.writes)
						var total, nulls, placeholders int
						require.NoError(t, db.QueryRow(`SELECT COUNT(*),
                            COUNT(*) FILTER (WHERE "Object ID" IS NULL),
                            COUNT(*) FILTER (WHERE "Object ID" = ' VM ')
                            FROM vcluster WHERE "Name" = 'production'`).Scan(&total, &nulls, &placeholders))
						if missingRow {
							require.Zero(t, total)
						} else {
							require.Equal(t, 2, total)
							require.Equal(t, 1, nulls)
							require.Equal(t, 1, placeholders)
						}
					} else {
						require.Equal(t, 2, recorder.writes)
						var ids int
						require.NoError(t, db.QueryRow(`SELECT COUNT(DISTINCT "Object ID")
                            FROM vcluster WHERE "Name" = 'production'`).Scan(&ids))
						require.Equal(t, 1, ids)
					}
				})
			}
		}
	}
}

func TestBuildInventory_ClusterIDsAmbiguityLookupFailureAndRecovery(t *testing.T) {
	p, db, tx := newClusterIDTestParser(t)
	seedClusterIDVM(t, db, "vm-a", "cluster-a", "dc-a")
	recorder := &clusterIDTestInterceptor{QueryInterceptor: p.db, failAmbiguity: true}
	p.db = recorder
	inv, err := buildClusterIDTestInventory(context.Background(), p, tx, nil)
	require.NoError(t, err)
	require.NotNil(t, inv)
	require.Zero(t, recorder.writes)
	var n int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM vcluster`).Scan(&n))
	require.Zero(t, n)
	recorder.failAmbiguity = false
	recovered, err := buildClusterIDTestInventory(context.Background(), p, tx, nil)
	require.NoError(t, err)
	expected := generateClusterID("cluster-a", "dc-a", "")
	require.Contains(t, recovered.Clusters, expected)
	require.Equal(t, 2, recorder.writes)
}

func TestClusterObjectIDs_PlaceholderClassificationMatchesPersistence(t *testing.T) {
	cases := []struct {
		id      any
		missing bool
	}{
		{nil, true}, {"", true}, {" ", true}, {"\t\n", true},
		{"VM", true}, {" VM ", true}, {"\tVM\n", true},
		{"domain-c34", false}, {" domain-c34 ", false},
		{"VM-domain-c34", false}, {"vm", false}, {"VMVM", false},
	}
	for i, tc := range cases {
		t.Run(fmt.Sprintf("case-%d", i), func(t *testing.T) {
			p, db, _ := newClusterIDTestParser(t)
			seedClusterIDVM(t, db, "vm-a", "cluster-a", "dc-a")
			_, err := db.Exec(`INSERT INTO vcluster ("Name", "Object ID")
                VALUES ('cluster-a', ?)`, tc.id)
			require.NoError(t, err)
			ids, err := p.ClusterObjectIDs(context.Background())
			require.NoError(t, err)
			_, found := ids["cluster-a"]
			require.Equal(t, !tc.missing, found)
			// Call the helper directly even for valid IDs to exercise its own
			// UPDATE classification independently of BuildInventory's guard.
			require.NoError(t, p.persistClusterID(context.Background(), "cluster-a", "cluster-generated"))
			var stored string
			require.NoError(t, db.QueryRow(`SELECT "Object ID" FROM vcluster`).Scan(&stored))
			if tc.missing {
				require.Equal(t, "cluster-generated", stored)
			} else {
				require.Equal(t, tc.id.(string), stored)
				require.Equal(t, stored, ids["cluster-a"])
			}
		})
	}
}

func TestClusterObjectIDs_IgnoresMissingIDs(t *testing.T) {
	p, db, _ := newClusterIDTestParser(t)
	_, err := db.Exec(`INSERT INTO vcluster ("Name", "Object ID") VALUES
        ('cluster-null', NULL), ('cluster-empty', ''), ('cluster-space', ' '),
        ('cluster-placeholder', ' VM '), ('cluster-real', ' domain-c34 '),
        ('cluster-substring', 'VM-domain-c35')`)
	require.NoError(t, err)
	ids, err := p.ClusterObjectIDs(context.Background())
	require.NoError(t, err)
	require.Equal(t, map[string]string{
		"cluster-real": " domain-c34 ", "cluster-substring": "VM-domain-c35",
	}, ids)
}
