package duckdb_parser

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHosts_VInfoFallbackUsesOnlyHostIDs(t *testing.T) {
	tests := []struct {
		name        string
		hostID      string
		wantHosts   int
		wantCluster string
		wantDC      string
	}{
		{
			name:        "host ID with conflicting datacenters",
			hostID:      "host-id-1",
			wantHosts:   1,
			wantCluster: "cluster-a",
		},
		{
			name:      "missing host ID does not fall back to hostname",
			wantHosts: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser, db, cleanup := setupTestParser(t, &testValidator{})
			defer cleanup()

			_, err := db.Exec(`INSERT INTO vinfo ("VM ID", "VM", "Host", "Host ID", "Cluster", "Datacenter") VALUES
('vm-1', 'vm-1', 'esx-secret.example.com', ?, 'cluster-a', 'dc-1'),
('vm-2', 'vm-2', 'esx-secret.example.com', ?, 'cluster-a', 'dc-2')`, tt.hostID, tt.hostID)
			require.NoError(t, err)
			_, err = db.Exec(`DELETE FROM vhost`)
			require.NoError(t, err)

			hosts, err := parser.Hosts(context.Background(), Filters{}, Options{})
			require.NoError(t, err)
			require.Len(t, hosts, tt.wantHosts)

			if tt.wantHosts == 0 {
				return
			}

			assert.Equal(t, tt.hostID, hosts[0].Id)
			assert.Equal(t, tt.wantCluster, hosts[0].Cluster)
			assert.Equal(t, tt.wantDC, hosts[0].Datacenter, "conflicting VM datacenters leave the host datacenter unknown")
			assert.Empty(t, hosts[0].CpuModel)
			assert.Empty(t, hosts[0].SerialNumber)
		})
	}
}
