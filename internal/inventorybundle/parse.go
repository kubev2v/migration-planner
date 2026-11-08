// Package inventorybundle parses disconnected inventory uploads: a single JSON
// inventory or a ZIP (inventory.json + subsets/<groupId>.json).
package inventorybundle

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/api/v1alpha1"
	svcmodel "github.com/kubev2v/migration-planner/internal/model"
	"github.com/kubev2v/migration-planner/internal/util"
)

const (
	// MaxFileSize is the cap on an uploaded JSON or ZIP file (50 MiB).
	MaxFileSize = 50 << 20
	// MaxRequestSize is MaxFileSize plus allowance for multipart wrappers.
	MaxRequestSize = MaxFileSize + (1 << 20)

	maxZipEntries        = 512
	maxDecompressedEntry = MaxFileSize
	maxDecompressedTotal = 4 * MaxFileSize
)

// Subset is one group inventory from a disconnected ZIP bundle.
type Subset struct {
	ID        uuid.UUID          `json:"-"`
	Name      string             `json:"name"`
	VCenterID string             `json:"vcenterId,omitempty"`
	VMsCount  int                `json:"vmsCount,omitempty"`
	Inventory v1alpha1.Inventory `json:"inventory"`
}

// Parsed is the result of Parse. It is the full new inventory state from the
// uploaded file: main inventory plus exactly the subsets present in the file
// (none for JSON or a ZIP without subsets/).
type Parsed struct {
	MainInventory v1alpha1.Inventory
	VCenterID     string
	AgentID       *uuid.UUID
	Subsets       []Subset
}

// Parse reads a JSON inventory or a disconnected ZIP bundle.
func Parse(data []byte) (Parsed, error) {
	if len(data) == 0 {
		return Parsed{}, fmt.Errorf("file is required")
	}
	if isZip(data) {
		return parseZip(data)
	}
	return parseJSON(data)
}

func isZip(data []byte) bool {
	return len(data) >= 4 && data[0] == 'P' && data[1] == 'K' && data[2] == 0x03 && data[3] == 0x04
}

func parseJSON(data []byte) (Parsed, error) {
	agentID := extractAgentID(data)
	main, vcenterID, err := normalizeMainInventory(data)
	if err != nil {
		return Parsed{}, err
	}
	return Parsed{
		MainInventory: main,
		VCenterID:     vcenterID,
		AgentID:       agentID,
		Subsets:       []Subset{},
	}, nil
}

func parseZip(data []byte) (Parsed, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return Parsed{}, fmt.Errorf("invalid zip archive: %w", err)
	}
	if len(zr.File) > maxZipEntries {
		return Parsed{}, fmt.Errorf("zip archive has too many entries (max %d)", maxZipEntries)
	}

	var totalRead int64
	var mainRaw []byte
	var subsets []Subset
	seenSubsets := make(map[uuid.UUID]struct{})
	for _, f := range zr.File {
		name, err := zipEntryName(f.Name)
		if err != nil {
			return Parsed{}, err
		}
		if f.FileInfo().IsDir() {
			continue
		}

		isMain := name == "inventory.json"
		isSubset := strings.HasPrefix(name, "subsets/") && strings.HasSuffix(name, ".json")
		if !isMain && !isSubset {
			continue
		}

		content, err := readZipEntry(f, &totalRead)
		if err != nil {
			return Parsed{}, err
		}

		if isMain {
			if mainRaw != nil {
				return Parsed{}, fmt.Errorf("zip archive contains more than one inventory.json")
			}
			mainRaw = content
			continue
		}

		subsetID, err := subsetIDFromName(name)
		if err != nil {
			return Parsed{}, err
		}
		if _, dup := seenSubsets[subsetID]; dup {
			return Parsed{}, fmt.Errorf("zip archive contains duplicate subset %s", subsetID)
		}
		seenSubsets[subsetID] = struct{}{}
		var subset Subset
		if err := json.Unmarshal(content, &subset); err != nil {
			return Parsed{}, fmt.Errorf("invalid subset inventory %q: %w", name, err)
		}
		subset.ID = subsetID
		if subset.Inventory.Clusters == nil {
			subset.Inventory.Clusters = make(map[string]v1alpha1.InventoryData)
		}
		subsets = append(subsets, subset)
	}

	if mainRaw == nil {
		return Parsed{}, fmt.Errorf("zip archive is missing inventory.json")
	}
	agentID := extractAgentID(mainRaw)
	main, vcenterID, err := normalizeMainInventory(mainRaw)
	if err != nil {
		return Parsed{}, err
	}
	if subsets == nil {
		subsets = []Subset{}
	}
	return Parsed{
		MainInventory: main,
		VCenterID:     vcenterID,
		AgentID:       agentID,
		Subsets:       subsets,
	}, nil
}

func zipEntryName(name string) (string, error) {
	name = strings.ReplaceAll(name, "\\", "/")
	if name == "" || strings.Contains(name, "..") || strings.HasPrefix(name, "/") || path.IsAbs(name) {
		return "", fmt.Errorf("zip archive contains illegal path: %q", name)
	}
	cleaned := path.Clean(name)
	if cleaned == "." || cleaned == ".." || strings.Contains(cleaned, "..") || strings.HasPrefix(cleaned, "/") || path.IsAbs(cleaned) {
		return "", fmt.Errorf("zip archive contains illegal path: %q", name)
	}
	return cleaned, nil
}

func subsetIDFromName(name string) (uuid.UUID, error) {
	base := strings.TrimSuffix(strings.TrimPrefix(name, "subsets/"), ".json")
	id, err := uuid.Parse(base)
	if err != nil {
		return uuid.Nil, fmt.Errorf("subset filename %q must be subsets/<uuid>.json", name)
	}
	return id, nil
}

func extractAgentID(data []byte) *uuid.UUID {
	var probe struct {
		AgentId *uuid.UUID `json:"agentId"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return nil
	}
	return probe.AgentId
}

func normalizeMainInventory(data []byte) (v1alpha1.Inventory, string, error) {
	var probe struct {
		Inventory json.RawMessage `json:"inventory"`
	}
	inventoryBytes := data
	if err := json.Unmarshal(data, &probe); err == nil && len(probe.Inventory) > 0 {
		inventoryBytes = probe.Inventory
		// Unwrap a nested UpdateInventory envelope ({agentId, inventory}).
		var nested struct {
			Inventory json.RawMessage `json:"inventory"`
		}
		if err := json.Unmarshal(inventoryBytes, &nested); err == nil && len(nested.Inventory) > 0 {
			inventoryBytes = nested.Inventory
		}
	}

	if util.GetInventoryVersion(inventoryBytes) != svcmodel.InventoryVersionV2 {
		return v1alpha1.Inventory{}, "", fmt.Errorf("only v2 inventories are supported")
	}

	var inv v1alpha1.Inventory
	if err := json.Unmarshal(inventoryBytes, &inv); err != nil {
		return v1alpha1.Inventory{}, "", fmt.Errorf("invalid inventory JSON: %w", err)
	}
	if inv.CreatedAt == nil {
		now := time.Now().UTC()
		inv.CreatedAt = &now
	}

	if inv.Clusters == nil {
		inv.Clusters = make(map[string]v1alpha1.InventoryData)
	}
	return inv, inv.VcenterId, nil
}

func checkDecompressedSize(name string, uncompressed uint64, totalRead int64) error {
	if uncompressed > uint64(maxDecompressedEntry) {
		return fmt.Errorf("zip entry %q exceeds maximum size of %d MiB", name, maxDecompressedEntry>>20)
	}
	if totalRead+int64(uncompressed) > maxDecompressedTotal {
		return fmt.Errorf("zip archive exceeds maximum decompressed size of %d MiB", maxDecompressedTotal>>20)
	}
	return nil
}

func readZipEntry(f *zip.File, totalRead *int64) ([]byte, error) {
	if err := checkDecompressedSize(f.Name, f.UncompressedSize64, *totalRead); err != nil {
		return nil, err
	}

	rc, err := f.Open()
	if err != nil {
		return nil, fmt.Errorf("failed to open zip entry %q: %w", f.Name, err)
	}
	defer func() { _ = rc.Close() }()

	content, err := io.ReadAll(io.LimitReader(rc, maxDecompressedEntry+1))
	if err != nil {
		return nil, fmt.Errorf("failed to read zip entry %q: %w", f.Name, err)
	}
	if int64(len(content)) > maxDecompressedEntry {
		return nil, fmt.Errorf("zip entry %q exceeds maximum size of %d MiB", f.Name, maxDecompressedEntry>>20)
	}
	*totalRead += int64(len(content))
	if *totalRead > maxDecompressedTotal {
		return nil, fmt.Errorf("zip archive exceeds maximum decompressed size of %d MiB", maxDecompressedTotal>>20)
	}
	return content, nil
}
