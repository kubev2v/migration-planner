package mappers_test

import (
	"encoding/json"
	"testing"
	"time"

	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/handlers/v1alpha1/mappers"
	"github.com/kubev2v/migration-planner/internal/store/model"
)

func TestAssessmentInventoryV2Metadata(t *testing.T) {
	storedAt := time.Date(2026, 10, 14, 10, 0, 0, 0, time.UTC)
	collectedAt := time.Date(2026, 10, 13, 9, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name       string
		version    uint
		raw        string
		created    time.Time
		apiVersion string
	}{
		{
			name:       "v2 preserves collection metadata",
			version:    model.SnapshotVersionV2,
			raw:        `{"vcenter_id":"vc-id","vcenter_version":"8.0.3.0","created_at":"2026-10-13T09:00:00Z","vcenter":{"vms":{"total":5}},"clusters":{"domain-c34":{"vms":{"total":2}}}}`,
			created:    collectedAt,
			apiVersion: "8.0.3.0",
		},
		{
			name:    "v2 falls back to stored timestamp",
			version: model.SnapshotVersionV2,
			raw:     `{"vcenter_id":"vc-id","vcenter":{"vms":{"total":5}},"clusters":{}}`,
			created: storedAt,
		},
		{
			name:    "legacy inventory uses stored timestamp",
			version: model.SnapshotVersionV1,
			raw:     `{"vcenter":{"id":"vc-id"},"vms":{"total":5}}`,
			created: storedAt,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, clusters, err := mappers.AssessmentInventoryToV2(model.AssessmentInventory{
				VCenterID: "vc-id", Version: tc.version, CreatedAt: storedAt, Inventory: []byte(tc.raw),
			})
			if err != nil {
				t.Fatal(err)
			}
			raw, err := json.Marshal(data)
			if err != nil {
				t.Fatal(err)
			}
			var result api.InventoryV2
			if err := json.Unmarshal(raw, &result); err != nil {
				t.Fatal(err)
			}
			if result.CreatedAt == nil || !result.CreatedAt.Equal(tc.created) {
				t.Fatalf("created_at = %v, want %v", result.CreatedAt, tc.created)
			}
			if result.Data.Vms.Total != 5 {
				t.Fatalf("wrapped VM count = %d, want 5", result.Data.Vms.Total)
			}
			if tc.apiVersion != "" && (result.VcenterVersion == nil || *result.VcenterVersion != tc.apiVersion) {
				t.Fatalf("vcenter_version = %v, want %q", result.VcenterVersion, tc.apiVersion)
			}
			if tc.apiVersion == "" && result.VcenterVersion != nil {
				t.Fatalf("unexpected vcenter_version = %v", result.VcenterVersion)
			}
			if result.Data.Infra.Datastores == nil || result.Data.Vms.PowerStates == nil {
				t.Fatal("inventory collections must be normalized")
			}
			if tc.name == "v2 preserves collection metadata" && clusters["domain-c34"].Vms.Total != 2 {
				t.Fatal("string cluster ID or cluster data was lost")
			}
		})
	}
}

func TestAssessmentInventoryV2RejectsMalformedData(t *testing.T) {
	for _, version := range []uint{model.SnapshotVersionV1, model.SnapshotVersionV2} {
		if _, _, err := mappers.AssessmentInventoryToV2(model.AssessmentInventory{Version: version, Inventory: []byte(`{`)}); err == nil {
			t.Fatalf("version %d accepted malformed JSON", version)
		}
	}
}
