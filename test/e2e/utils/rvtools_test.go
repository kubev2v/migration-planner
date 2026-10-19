package utils

import (
	"bytes"
	"testing"

	"github.com/google/uuid"
	"github.com/xuri/excelize/v2"
)

func TestRVToolsBuildersUseSuppliedVCenterIDForEveryVM(t *testing.T) {
	vcenterID := uuid.MustParse("44a88f55-9b2b-4a61-9dc2-a3cd3b04b2bf")
	builders := map[string]func(uuid.UUID) ([]byte, error){
		"vInfo only":    CreateExcelOnlyWithVInfo,
		"large":         CreateLargeExcel,
		"valid":         CreateValidTestExcel,
		"example":       CreateExample1Excel,
		"multi cluster": CreateMultiClusterTestExcel,
		"with concerns": CreateExcelWithConcerns,
	}

	for name, build := range builders {
		t.Run(name, func(t *testing.T) {
			content, err := build(vcenterID)
			if err != nil {
				t.Fatalf("build workbook: %v", err)
			}

			workbook, err := excelize.OpenReader(bytes.NewReader(content))
			if err != nil {
				t.Fatalf("open workbook: %v", err)
			}
			defer func() { _ = workbook.Close() }()

			rows, err := workbook.GetRows("vInfo")
			if err != nil {
				t.Fatalf("read vInfo rows: %v", err)
			}
			if len(rows) < 2 {
				t.Fatalf("expected at least one VM row, got %d rows", len(rows))
			}

			vcenterColumn := -1
			for index, header := range rows[0] {
				if header == "VI SDK UUID" {
					vcenterColumn = index
					break
				}
			}
			if vcenterColumn < 0 {
				t.Fatal("vInfo sheet has no VI SDK UUID column")
			}

			for rowIndex, row := range rows[1:] {
				if len(row) <= vcenterColumn {
					t.Errorf("VM row %d has no VI SDK UUID cell", rowIndex+2)
					continue
				}
				if got := row[vcenterColumn]; got != vcenterID.String() {
					t.Errorf("VM row %d has VI SDK UUID %q, want supplied vCenter ID %q", rowIndex+2, got, vcenterID)
				}
			}
		})
	}
}
