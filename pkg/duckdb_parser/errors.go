package duckdb_parser

import (
	"archive/zip"
	"errors"
	"fmt"
	"strings"

	"github.com/xuri/excelize/v2"
)

type RVToolsStage string

const (
	RVToolsStageParsing        RVToolsStage = "parsing"
	RVToolsStageInitialization RVToolsStage = "initialization"
	RVToolsStageCleanup        RVToolsStage = "cleanup"
)

// RVToolsProcessingError identifies the failed operation without choosing its public response.
type RVToolsProcessingError struct {
	Stage    RVToolsStage
	Filename string
	Cause    error
}

func (e *RVToolsProcessingError) Error() string {
	message := fmt.Sprintf("RVTools %s failed", e.Stage)
	if e.Filename != "" {
		message += " for " + e.Filename
	}
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", message, e.Cause)
	}
	return message
}

func (e *RVToolsProcessingError) Unwrap() error { return e.Cause }

// SchemaValidationError retains every issue and formats the recognized validation messages.
type SchemaValidationError struct {
	Result ValidationResult
}

func (e *SchemaValidationError) Error() string {
	messages := make([]string, 0, len(e.Result.Errors))
	for _, issue := range e.Result.Errors {
		messages = append(messages, issue.Message)
	}
	return strings.Join(messages, "\n")
}

const RVToolsFailureMessage = "RVTools file could not be processed."

var xlsxErrorMappings = map[string]string{
	"No xl/workbook.xml found":         "The file is corrupted or not a valid Excel file",
	"\"vInfo\" not found in xlsx file": "File is not a valid RVTools export (missing required 'vInfo' sheet)",
}

// XLSXError retains the workbook-reading failure and formats recognized XLSX errors.
type XLSXError struct {
	Cause error
}

func (e *XLSXError) Error() string {
	if e.Cause == nil {
		return RVToolsFailureMessage
	}
	if errors.Is(e.Cause, zip.ErrFormat) {
		return xlsxErrorMappings["No xl/workbook.xml found"]
	}
	var missingSheet excelize.ErrSheetNotExist
	if errors.As(e.Cause, &missingSheet) && missingSheet.SheetName == "vInfo" {
		return xlsxErrorMappings["\"vInfo\" not found in xlsx file"]
	}
	for pattern, message := range xlsxErrorMappings {
		if strings.Contains(e.Cause.Error(), pattern) {
			return message
		}
	}
	return RVToolsFailureMessage
}

func (e *XLSXError) Unwrap() error { return e.Cause }
