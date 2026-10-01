package duckdb_parser

import (
	"context"
	"fmt"
	"strings"

	"github.com/xuri/excelize/v2"
	"go.uber.org/zap"

	"github.com/kubev2v/migration-planner/pkg/duckdb_parser/models"
)

// readRVToolsMetadata reads original cells to distinguish blanks from literal "VM".
func readRVToolsMetadata(ctx context.Context, filePath string) (map[string]models.SourceMetadata, error) {
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
		zap.S().Warn("RVTools source metadata skipped: expected unambiguous Annotation before Datacenter")
		return nil, nil
	}
	if vmID == -1 || end == start+1 {
		return nil, nil
	}
	metadata := make(map[string]models.SourceMetadata)
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
			metadata[cells[vmID]] = append(metadata[cells[vmID]], models.SourceMetadataEntry{
				Key: headers[i], Value: cells[i], Kind: "unknown",
			})
		}
	}
	if err := rows.Error(); err != nil {
		return nil, fmt.Errorf("reading vInfo metadata rows: %w", err)
	}
	return metadata, nil
}
