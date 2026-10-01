package duckdb_parser

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/xuri/excelize/v2"
	"go.uber.org/zap"
)

// readRVToolsMetadata reads original cells to distinguish blanks from literal "VM".
func readRVToolsMetadata(ctx context.Context, filePath string) (map[string]map[string][]string, error) {
	workbook, err := excelize.OpenFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("opening RVTools workbook: %w", err)
	}
	defer func() { _ = workbook.Close() }()
	rows, err := workbook.Rows("vInfo")
	if err != nil {
		return nil, fmt.Errorf("reading vInfo headers: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var headers []string
	if rows.Next() {
		headers, err = rows.Columns()
	} else {
		err = rows.Error()
	}
	if err != nil {
		return nil, fmt.Errorf("reading vInfo headers: %w", err)
	}
	start, end, vmID := -1, -1, -1
	annotations, datacenters := 0, 0
	for i, header := range headers {
		switch header {
		case "Annotation":
			start = i
			annotations++
		case "Datacenter":
			end = i
			datacenters++
		case "VM ID":
			if vmID == -1 {
				vmID = i
			}
		}
	}
	if annotations != 1 || datacenters != 1 || end <= start {
		zap.S().Warn("RVTools metadata skipped: expected unambiguous Annotation before Datacenter")
		return nil, nil
	}
	if vmID == -1 || end == start+1 {
		return nil, nil
	}
	metadata := make(map[string]map[string][]string)
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		cells, err := rows.Columns()
		if err != nil {
			return nil, fmt.Errorf("reading vInfo metadata cells: %w", err)
		}
		if vmID >= len(cells) || strings.TrimSpace(cells[vmID]) == "" {
			continue
		}
		for i := start + 1; i < end && i < len(cells); i++ {
			if strings.TrimSpace(cells[i]) == "" {
				continue
			}
			if metadata[cells[vmID]] == nil {
				metadata[cells[vmID]] = make(map[string][]string)
			}
			values := metadata[cells[vmID]]
			values[headers[i]] = append(values[headers[i]], cells[i])
		}
	}
	if err := rows.Error(); err != nil {
		return nil, fmt.Errorf("reading vInfo metadata rows: %w", err)
	}
	for _, entries := range metadata {
		for key, values := range entries {
			slices.Sort(values)
			entries[key] = slices.Compact(values)
		}
	}
	return metadata, nil
}
