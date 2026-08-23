package inventorybundle

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/api/v1alpha1"
	agentAPI "github.com/kubev2v/migration-planner/api/v1alpha1/agent"
)

func TestParse_JSON(t *testing.T) {
	inv := v1alpha1.Inventory{VcenterId: "vc-1"}
	raw, _ := json.Marshal(inv)

	parsed, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if parsed.VCenterID != "vc-1" {
		t.Errorf("VCenterID = %q, want vc-1", parsed.VCenterID)
	}
	if parsed.Subsets == nil {
		t.Fatal("Subsets is nil, want empty slice for JSON")
	}
	if len(parsed.Subsets) != 0 {
		t.Errorf("Subsets len = %d, want 0", len(parsed.Subsets))
	}
}

func TestParse_UpdateInventoryWrapper(t *testing.T) {
	agentID := uuid.New()
	wrapped := v1alpha1.UpdateInventory{
		AgentId:   agentID,
		Inventory: v1alpha1.Inventory{VcenterId: "vc-wrapped"},
	}
	raw, _ := json.Marshal(wrapped)

	parsed, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if parsed.VCenterID != "vc-wrapped" {
		t.Errorf("VCenterID = %q, want vc-wrapped", parsed.VCenterID)
	}
	if parsed.AgentID == nil || *parsed.AgentID != agentID {
		t.Errorf("AgentID = %v, want %s", parsed.AgentID, agentID)
	}
}

func TestParse_Zip(t *testing.T) {
	groupID := uuid.New()
	vms := 3
	vcenter := "vc-a"
	bundle := buildZip(t, v1alpha1.Inventory{VcenterId: "vc-main"}, map[string]agentAPI.SourceSubsetUpdate{
		groupID.String(): {
			Name:      "group-a",
			VcenterId: &vcenter,
			VmsCount:  &vms,
			Inventory: v1alpha1.Inventory{VcenterId: "vc-a"},
		},
	})

	parsed, err := Parse(bundle)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if parsed.VCenterID != "vc-main" {
		t.Errorf("VCenterID = %q, want vc-main", parsed.VCenterID)
	}
	if len(parsed.Subsets) != 1 {
		t.Fatalf("Subsets len = %d, want 1", len(parsed.Subsets))
	}
	if parsed.Subsets[0].ID != groupID {
		t.Errorf("subset ID = %s, want %s", parsed.Subsets[0].ID, groupID)
	}
	if parsed.Subsets[0].Name != "group-a" {
		t.Errorf("subset Name = %q, want group-a", parsed.Subsets[0].Name)
	}
	if parsed.Subsets[0].VMsCount != 3 {
		t.Errorf("VMsCount = %d, want 3", parsed.Subsets[0].VMsCount)
	}
}

func TestParse_ZipMissingMain(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("subsets/" + uuid.New().String() + ".json")
	_, _ = w.Write([]byte(`{"name":"orphan"}`))
	_ = zw.Close()

	_, err := Parse(buf.Bytes())
	if err == nil || err.Error() != "zip archive is missing inventory.json" {
		t.Fatalf("err = %v, want missing inventory.json", err)
	}
}

func TestParse_ZipEmptySubsets(t *testing.T) {
	bundle := buildZip(t, v1alpha1.Inventory{VcenterId: "vc-only"}, nil)
	parsed, err := Parse(bundle)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if parsed.Subsets == nil {
		t.Fatal("Subsets is nil, want empty slice for zip")
	}
	if len(parsed.Subsets) != 0 {
		t.Errorf("Subsets len = %d, want 0", len(parsed.Subsets))
	}
}

func TestParse_ZipIllegalPath(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	mainBytes, _ := json.Marshal(v1alpha1.Inventory{VcenterId: "vc-1"})
	w, _ := zw.Create("../inventory.json")
	_, _ = w.Write(mainBytes)
	_ = zw.Close()

	_, err := Parse(buf.Bytes())
	if err == nil || !strings.Contains(err.Error(), "illegal path") {
		t.Fatalf("err = %v, want illegal path", err)
	}
}

func TestParse_ZipNormalizedDotPath(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	mainBytes, _ := json.Marshal(v1alpha1.Inventory{VcenterId: "vc-dot"})
	w, _ := zw.Create("./inventory.json")
	_, _ = w.Write(mainBytes)
	_ = zw.Close()

	parsed, err := Parse(buf.Bytes())
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if parsed.VCenterID != "vc-dot" {
		t.Errorf("VCenterID = %q, want vc-dot", parsed.VCenterID)
	}
}

func TestParse_ZipDuplicateMain(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	mainBytes, _ := json.Marshal(v1alpha1.Inventory{VcenterId: "vc-1"})
	w, _ := zw.Create("inventory.json")
	_, _ = w.Write(mainBytes)
	w, _ = zw.Create("inventory.json")
	_, _ = w.Write(mainBytes)
	_ = zw.Close()

	_, err := Parse(buf.Bytes())
	if err == nil || !strings.Contains(err.Error(), "more than one inventory.json") {
		t.Fatalf("err = %v, want duplicate inventory.json", err)
	}
}

func TestParse_ZipDuplicateSubset(t *testing.T) {
	groupID := uuid.New()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	mainBytes, _ := json.Marshal(v1alpha1.Inventory{VcenterId: "vc-main"})
	mw, _ := zw.Create("inventory.json")
	_, _ = mw.Write(mainBytes)
	sb, _ := json.Marshal(agentAPI.SourceSubsetUpdate{Name: "group-a", Inventory: v1alpha1.Inventory{VcenterId: "vc-a"}})
	name := "subsets/" + groupID.String() + ".json"
	sw, _ := zw.Create(name)
	_, _ = sw.Write(sb)
	sw, _ = zw.Create(name)
	_, _ = sw.Write(sb)
	_ = zw.Close()

	_, err := Parse(buf.Bytes())
	if err == nil || !strings.Contains(err.Error(), "duplicate subset") {
		t.Fatalf("err = %v, want duplicate subset", err)
	}
}

func TestCheckDecompressedSize(t *testing.T) {
	if err := checkDecompressedSize("inventory.json", uint64(maxDecompressedEntry)+1, 0); err == nil {
		t.Fatal("expected error for oversized entry")
	}
	if err := checkDecompressedSize("inventory.json", 1, maxDecompressedTotal); err == nil {
		t.Fatal("expected error for oversized archive")
	}
	if err := checkDecompressedSize("inventory.json", 1, 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestZipEntryName(t *testing.T) {
	got, err := zipEntryName("./inventory.json")
	if err != nil {
		t.Fatalf("zipEntryName: %v", err)
	}
	if got != "inventory.json" {
		t.Errorf("got %q, want inventory.json", got)
	}
	got, err = zipEntryName(`subsets\` + "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa.json")
	if err != nil {
		t.Fatalf("zipEntryName windows slash: %v", err)
	}
	if got != "subsets/aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa.json" {
		t.Errorf("got %q", got)
	}
	if _, err := zipEntryName("../inventory.json"); err == nil {
		t.Fatal("expected error for .. path")
	}
	if _, err := zipEntryName("subsets/../inventory.json"); err == nil {
		t.Fatal("expected error for nested .. path")
	}
	if _, err := zipEntryName("/inventory.json"); err == nil {
		t.Fatal("expected error for absolute path")
	}
}

func TestParse_Empty(t *testing.T) {
	_, err := Parse(nil)
	if err == nil {
		t.Fatal("expected error for empty file")
	}
}

func buildZip(t *testing.T, main v1alpha1.Inventory, subsets map[string]agentAPI.SourceSubsetUpdate) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	mainBytes, _ := json.Marshal(main)
	mw, _ := zw.Create("inventory.json")
	_, _ = mw.Write(mainBytes)
	for id, subset := range subsets {
		sb, _ := json.Marshal(subset)
		sw, _ := zw.Create("subsets/" + id + ".json")
		_, _ = sw.Write(sb)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
