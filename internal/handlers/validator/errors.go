package validator

import (
	"errors"
	"fmt"

	"github.com/go-playground/validator/v10"
)

type ErrInvalidName struct {
	error
}

func NewErrInvalidName(format string, args ...any) *ErrInvalidName {
	return &ErrInvalidName{fmt.Errorf(format, args...)}
}

type ErrInvalidFile struct {
	message string
}

var manualCountFieldLabels = map[string]string{
	"PerpetualLicensesCount": "Number of perpetual licenses",
	"CoresLicensingCount":    "Number of cores licensing",
	"EnvironmentCount":       "Number of environments",
	"OtherNodesCount":        "Number of other nodes",
	"PhysicalLocationsCount": "Number of physical locations/datacenters",
}

func NewErrInvalidFile(format string, args ...any) *ErrInvalidFile {
	return &ErrInvalidFile{message: fmt.Sprintf(format, args...)}
}

func (e *ErrInvalidFile) Error() string {
	return e.message
}

func ManualCountError(field string) error {
	label, ok := manualCountFieldLabels[field]
	if !ok {
		return nil
	}
	return fmt.Errorf("%s must be a whole number within the supported range", label)
}

// TransformValidationError converts validation errors into user-friendly messages
func TransformValidationError(err error) error {
	if err == nil {
		return nil
	}

	var validationErrs validator.ValidationErrors
	if !errors.As(err, &validationErrs) {
		// Not a validation error, return as-is
		return err
	}

	// Look for errors and replace with friendly message
	var finalErrors []error
	for _, fieldErr := range validationErrs {
		switch fieldErr.Tag() {
		case TagIP4Addr.String():
			finalErrors = append(finalErrors,
				fmt.Errorf("invalid %s format. Please use format like 192.168.1.100", fieldErr.Field()))
		default:
			// Fallback: return original error
			finalErrors = append(finalErrors, fieldErr)
		}

	}

	return errors.Join(finalErrors...)
}
