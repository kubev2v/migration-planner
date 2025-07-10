package service

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/google/uuid"
)

// HTTPStatusCodeError interface allows errors to specify their HTTP status code
//
// To add a new HTTP-aware error:
//   - For custom errors: implement HTTPStatusCode() method
//   - For quick errors: use service.NewHTTPError() or service.NewHTTPErrorf()
type HTTPStatusCodeError interface {
	HTTPStatusCode() int
}

// HTTPError is a generic error that includes an HTTP status code
type HTTPError struct {
	Err        error
	StatusCode int
}

func (e *HTTPError) Error() string {
	return e.Err.Error()
}

func (e *HTTPError) HTTPStatusCode() int {
	return e.StatusCode
}

func (e *HTTPError) Unwrap() error {
	return e.Err
}

// NewHTTPError creates a new HTTP-aware error
func NewHTTPError(statusCode int, message string) *HTTPError {
	return &HTTPError{
		Err:        errors.New(message),
		StatusCode: statusCode,
	}
}

// NewHTTPErrorf creates a new HTTP-aware error with formatting
func NewHTTPErrorf(statusCode int, format string, args ...interface{}) *HTTPError {
	return &HTTPError{
		Err:        fmt.Errorf(format, args...),
		StatusCode: statusCode,
	}
}

type ErrInvalidVCenterID struct {
	error
}

func NewErrInvalidVCenterID(sourceID uuid.UUID, vCenterID string) *ErrInvalidVCenterID {
	return &ErrInvalidVCenterID{fmt.Errorf("source %s vcenter id is different than the one specified %s", sourceID, vCenterID)}
}

func (e *ErrInvalidVCenterID) HTTPStatusCode() int {
	return http.StatusBadRequest
}

type ErrResourceNotFound struct {
	error
}

func NewErrResourceNotFound(id uuid.UUID, resourceType string) *ErrResourceNotFound {
	return &ErrResourceNotFound{fmt.Errorf("%s %s not found", resourceType, id)}
}

func (e *ErrResourceNotFound) HTTPStatusCode() int {
	return http.StatusNotFound
}

func NewErrSourceNotFound(id uuid.UUID) *ErrResourceNotFound {
	return NewErrResourceNotFound(id, "source")
}

func NewErrAgentNotFound(id uuid.UUID) *ErrResourceNotFound {
	return NewErrResourceNotFound(id, "agent")
}

type ErrExcelFileNotValid struct {
	error
}

func NewErrExcelFileNotValid() *ErrExcelFileNotValid {
	return &ErrExcelFileNotValid{errors.New("the uploaded file is not a valid Excel (.xlsx) file. Please upload an RVTools export in Excel format")}
}

func (e *ErrExcelFileNotValid) HTTPStatusCode() int {
	return http.StatusBadRequest
}

type ErrAgentUpdateForbidden struct {
	error
}

func NewErrAgentUpdateForbidden(sourceID, agentID uuid.UUID) *ErrAgentUpdateForbidden {
	return &ErrAgentUpdateForbidden{fmt.Errorf("agent %s is not associated with source %s", agentID, sourceID)}
}

func (e *ErrAgentUpdateForbidden) HTTPStatusCode() int {
	return http.StatusForbidden
}
