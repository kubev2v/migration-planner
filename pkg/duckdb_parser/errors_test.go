package duckdb_parser_test

import (
	"archive/zip"
	"errors"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/xuri/excelize/v2"

	"github.com/kubev2v/migration-planner/pkg/duckdb_parser"
)

var _ = Describe("RVTools error types", func() {
	DescribeTable("selects safe messages through diagnostic wrappers", func(cause error, want string) {
		err := fmt.Errorf("ingesting /private/upload.xlsx: %w", cause)
		workbook := &duckdb_parser.XLSXError{Cause: err}
		Expect(workbook.Error()).To(Equal(want))
		Expect(errors.Is(workbook, cause)).To(BeTrue())
		Expect(errors.Is(err, cause)).To(BeTrue())
		Expect(err.Error()).To(ContainSubstring("/private/upload.xlsx"))
	},
		Entry("unknown error", errors.New("SELECT private_column FROM /private/database"), duckdb_parser.RVToolsFailureMessage),
		Entry("untrusted readable text", errors.New("No VMs found in vInfo sheet"), duckdb_parser.RVToolsFailureMessage),
		Entry("DuckDB invalid workbook", errors.New("SQL: No xl/workbook.xml found"), "The file is corrupted or not a valid Excel file"),
		Entry("DuckDB missing vInfo", errors.New("SQL: \"vInfo\" not found in xlsx file"), "File is not a valid RVTools export (missing required 'vInfo' sheet)"),
		Entry("Excelize invalid ZIP", zip.ErrFormat, "The file is corrupted or not a valid Excel file"),
		Entry("Excelize missing vInfo", excelize.ErrSheetNotExist{SheetName: "vInfo"}, "File is not a valid RVTools export (missing required 'vInfo' sheet)"),
		Entry("different missing sheet", excelize.ErrSheetNotExist{SheetName: "private"}, duckdb_parser.RVToolsFailureMessage),
	)

	DescribeTable("retains the operation context", func(stage duckdb_parser.RVToolsStage) {
		original := fmt.Errorf("opening /private/upload.xlsx: %w", zip.ErrFormat)
		err := &duckdb_parser.RVToolsProcessingError{Stage: stage, Filename: "/private/upload.xlsx", Cause: original}
		wrapped := fmt.Errorf("job failed: %w", err)
		Expect(errors.Is(wrapped, zip.ErrFormat)).To(BeTrue())
		var processing *duckdb_parser.RVToolsProcessingError
		Expect(errors.As(wrapped, &processing)).To(BeTrue())
		Expect(processing.Stage).To(Equal(stage))
		Expect(processing.Filename).To(Equal("/private/upload.xlsx"))
		Expect(wrapped.Error()).To(ContainSubstring(original.Error()))
	},
		Entry("parsing", duckdb_parser.RVToolsStageParsing),
		Entry("initialization", duckdb_parser.RVToolsStageInitialization),
		Entry("cleanup", duckdb_parser.RVToolsStageCleanup),
		Entry("unknown stage", duckdb_parser.RVToolsStage("unknown")),
	)

	It("returns all prepared validation messages together", func() {
		result := duckdb_parser.ValidationResult{Errors: []duckdb_parser.ValidationIssue{
			{Code: duckdb_parser.CodeMissingVMID, Message: "First issue: missing VM ID"},
			{Code: duckdb_parser.CodeMissingVMName, Message: "Second issue: missing name"},
		}}
		err := result.Error()
		Expect(err.Error()).To(Equal("First issue: missing VM ID\nSecond issue: missing name"))
		var validation *duckdb_parser.SchemaValidationError
		Expect(errors.As(err, &validation)).To(BeTrue())
		Expect(validation.Result).To(Equal(result))
	})

	DescribeTable("leaves unexpected issues for the caller to interpret", func(issue duckdb_parser.ValidationIssue) {
		result := duckdb_parser.ValidationResult{Errors: []duckdb_parser.ValidationIssue{
			{Code: duckdb_parser.CodeMissingVMID, Message: "Missing VM ID"},
			issue,
		}}
		err := result.Error()
		Expect(err).To(HaveOccurred())
		var validation *duckdb_parser.SchemaValidationError
		Expect(errors.As(err, &validation)).To(BeFalse())
		Expect(err.Error()).To(ContainSubstring("Missing VM ID"))
		Expect(err.Error()).To(ContainSubstring(issue.Code))
		Expect(err.Error()).NotTo(ContainSubstring(duckdb_parser.RVToolsFailureMessage))
	},
		Entry("query failure", duckdb_parser.ValidationIssue{Code: duckdb_parser.CodeColumnValidationFailed, Message: "SELECT private_column FROM /private/database"}),
		Entry("unknown code", duckdb_parser.ValidationIssue{Code: "NEW_CODE", Message: "/private/database"}),
		Entry("empty message", duckdb_parser.ValidationIssue{Code: duckdb_parser.CodeNoVMs}),
	)

	It("keeps empty validation details empty and no error for a valid result", func() {
		Expect((duckdb_parser.ValidationResult{}).Error()).To(Succeed())
		Expect((&duckdb_parser.SchemaValidationError{}).Error()).To(BeEmpty())
		result := duckdb_parser.ValidationResult{Errors: []duckdb_parser.ValidationIssue{{Code: duckdb_parser.CodeNoVMs}}}
		Expect(result.Error()).To(HaveOccurred())
		Expect((&duckdb_parser.XLSXError{}).Error()).To(Equal(duckdb_parser.RVToolsFailureMessage))
		err := &duckdb_parser.RVToolsProcessingError{Stage: duckdb_parser.RVToolsStageCleanup}
		Expect(err.Error()).To(Equal("RVTools cleanup failed"))
	})
})
