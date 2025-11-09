package validator

import (
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"regexp"
	"strconv"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/api/v1alpha1"
)

var (
	sshRegex = []*regexp.Regexp{
		regexp.MustCompile(`^ssh-rsa AAAAB3NzaC1yc2[0-9A-Za-z+/]+[=]{0,3}(\s.*)?$`),
		regexp.MustCompile(`^ssh-ed25519 AAAAC3NzaC1lZDI1NTE5[0-9A-Za-z+/]+[=]{0,3}(\s.*)?$`),
		regexp.MustCompile(`^ssh-dss AAAAB3NzaC1kc3[0-9A-Za-z+/]+[=]{0,3}(\s.*)?$`),
		regexp.MustCompile(`^ecdsa-sha2-nistp256 AAAAE2VjZHNhLXNoYTItbmlzdHAyNT[0-9A-Za-z+/]+[=]{0,3}(\\s.*)?$`),
	}

	sourceNameValidRegex = regexp.MustCompile("^[a-zA-Z0-9+-_.]+$")
	labelRegex           = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9._-]*[a-zA-Z0-9])?$`)
)

func nameValidator(fl validator.FieldLevel) bool {
	val, ok := fl.Field().Interface().(string)
	if !ok {
		return false
	}

	return sourceNameValidRegex.MatchString(val)
}

func sshKeyValidator(fl validator.FieldLevel) bool {
	val, ok := fl.Field().Addr().Interface().(*string)
	if !ok {
		return false
	}

	if val == nil {
		return true
	}

	for _, r := range sshRegex {
		if r.MatchString(*val) {
			return true
		}
	}

	return false
}

func certificateValidator(fl validator.FieldLevel) bool {
	val, ok := fl.Field().Addr().Interface().(*string)
	if !ok {
		return false
	}

	if val == nil {
		return true
	}

	block, _ := pem.Decode([]byte(*val))
	if block == nil {
		return false
	}

	_, err := x509.ParseCertificate(block.Bytes)
	return err == nil
}

func agentStatusValidator(fl validator.FieldLevel) bool {
	val, ok := fl.Field().Interface().(string)
	if !ok {
		return false
	}
	switch val {
	case "not-connected":
		fallthrough
	case "waiting-for-credentials":
		fallthrough
	case "error":
		fallthrough
	case "gathering-initial-inventory":
		fallthrough
	case "up-to-date":
		fallthrough
	case "source-gone":
		return true
	default:
		return false
	}
}

func uuidValidator(fl validator.FieldLevel) bool {
	val, ok := fl.Field().Interface().(uuid.UUID)
	if !ok {
		return false
	}
	return val != uuid.UUID{}
}

func labelValidator(fl validator.FieldLevel) bool {
	val, ok := fl.Field().Interface().(string)
	if !ok {
		return false
	}

	// Label key/value should not be empty
	// Allow alphanumeric characters, hyphens, underscores, and dots
	// Must start and end with alphanumeric character
	return labelRegex.MatchString(val)
}

func startsWithValidator(fl validator.FieldLevel) bool {
	val, ok := fl.Field().Addr().Interface().(*string)
	if !ok {
		return false
	}

	if val == nil {
		return true
	}

	param := fl.Param()
	return strings.HasPrefix(*val, param)
}

func subnetMaskValidator(fl validator.FieldLevel) bool {
	val, ok := fl.Field().Interface().(string)
	if !ok {
		return false
	}

	maskInt, err := strconv.Atoi(val)
	if err != nil {
		return false
	}
	return maskInt >= 0 && maskInt <= 32
}

func startsNotWithValidator(fl validator.FieldLevel) bool {
	val, ok := fl.Field().Addr().Interface().(*string)
	if !ok {
		return false
	}

	if val == nil {
		return true
	}

	param := fl.Param()
	return !strings.HasPrefix(*val, param)
}

func inventoryNotEmptyValidator(fl validator.FieldLevel) bool {
	val, ok := fl.Field().Addr().Interface().(*v1alpha1.Inventory)
	if !ok {
		return false
	}

	// If inventory is nil, validation should pass here
	// The required_if validation should handle the nil case
	if val == nil {
		return true
	}

	// Marshal the inventory to JSON and compare against empty JSON object
	inventoryJSON, err := json.Marshal(*val)
	if err != nil {
		// If marshalling fails, consider it invalid
		return false
	}

	// Return false if inventory marshals to empty JSON object
	return string(inventoryJSON) != "{}"
}

func AssessmentFormValidator() validator.StructLevelFunc {
	return func(sl validator.StructLevel) {
		val, _ := sl.Current().Interface().(v1alpha1.AssessmentForm)

		// If sourceType is "inventory", validate that inventory is provided and not empty
		if val.SourceType == "inventory" {
			// Check if inventory is provided
			if val.Inventory == nil {
				sl.ReportError("inventory", "inventory", "inventory", "inventory is missing", "")
				return
			}

			// Check if inventory is not empty by marshaling to JSON
			_, err := json.Marshal(*val.Inventory)
			if err != nil {
				sl.ReportError("inventory", "inventory", "inventory", "failed to marshal", "")

			}

			// Check if inventory has a vCenter
			if val.Inventory.Vcenter.Id == "" {
				sl.ReportError("inventory", "inventory", "inventory", "inventory has no vCenterID", "")
			}
		}

		// If sourceType is "agent", validate that sourceId is provided
		if val.SourceType == "agent" && val.SourceId == nil {
			sl.ReportError("SourceType", "SourceType", "sourceType", "agent", "")
		}
	}
}

// ValidateRVToolsFile validates that the uploaded file is a valid Excel format
// by checking the magic bytes signature
func ValidateRVToolsFile(fileData []byte) error {
	if len(fileData) == 0 {
		return &ValidationError{Field: "file", Message: "rvtools file body is empty"}
	}

	if len(fileData) < 4 {
		return &ValidationError{Field: "file", Message: "file is too small to be a valid Excel file"}
	}

	// Excel 2007+ (.xlsx) files start with PK (ZIP signature: 0x50 0x4B)
	// Excel 97-2003 (.xls) files start with D0 CF (OLE signature: 0xD0 0xCF)
	isXLSX := fileData[0] == 0x50 && fileData[1] == 0x4B
	isXLS := fileData[0] == 0xD0 && fileData[1] == 0xCF

	if !isXLSX && !isXLS {
		return &ValidationError{Field: "file", Message: "file is not a valid Excel file format"}
	}

	return nil
}

// ValidationError represents a validation error with field information
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return e.Message
}
