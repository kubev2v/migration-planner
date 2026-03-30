package util

import (
	"testing"

	"github.com/kubev2v/migration-planner/internal/store/model"
)

func TestGetInventoryVersion(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		json string
		want int
	}{
		{
			name: "legacy v1 shape without top-level clusters or vcenter_id",
			json: `{"vms":{"total":10},"infra":{"totalHosts":5},"vcenter":{"id":"x","name":"n"}}`,
			want: model.SnapshotVersionV1,
		},
		{
			name: "v2 with clusters and vcenter_id",
			json: `{"vcenter_id":"uuid","vcenter":{"vms":{"total":1},"infra":{}},"clusters":{}}`,
			want: model.SnapshotVersionV2,
		},
		{
			name: "v2 with empty vcenter_id and clusters",
			json: `{"vcenter_id":"","vcenter":{"vms":{"total":5},"infra":{"totalHosts":2}},"clusters":{"c1":{"vms":{"total":5},"infra":{}}}}`,
			want: model.SnapshotVersionV2,
		},
		{
			name: "v2 with vcenter_id only no clusters key",
			json: `{"vcenter_id":"test-vcenter","vcenter":{"vms":{"total":10},"infra":{"totalHosts":5}}}`,
			want: model.SnapshotVersionV2,
		},
		{
			name: "v2 with clusters only",
			json: `{"vcenter":{"vms":{"total":1},"infra":{}},"clusters":{}}`,
			want: model.SnapshotVersionV2,
		},
		{
			name: "empty JSON object defaults to v1",
			json: `{}`,
			want: model.SnapshotVersionV1,
		},
		{
			name: "empty input defaults to v1",
			json: "",
			want: model.SnapshotVersionV1,
		},
		{
			name: "invalid JSON defaults to v1",
			json: `{`,
			want: model.SnapshotVersionV1,
		},
		{
			name: "JSON null unmarshals to nil map defaults to v1",
			json: `null`,
			want: model.SnapshotVersionV1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := GetInventoryVersion([]byte(tt.json)); got != tt.want {
				t.Fatalf("GetInventoryVersion() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestGetInventoryVersion_nilSlice(t *testing.T) {
	t.Parallel()
	if got := GetInventoryVersion(nil); got != model.SnapshotVersionV1 {
		t.Fatalf("GetInventoryVersion(nil) = %d, want v1", got)
	}
}
