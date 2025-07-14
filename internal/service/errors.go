package service

import (
	"errors"
	"fmt"

	"github.com/google/uuid"
)

type ErrInvalidVCenterID struct {
	error
}

func NewErrInvalidVCenterID(sourceID uuid.UUID, vCenterID string) *ErrInvalidVCenterID {
	return &ErrInvalidVCenterID{fmt.Errorf("source %s vcenter id is different than the one specified %s", sourceID, vCenterID)}
}

type ErrResourceNotFound struct {
	error
}

func NewErrResourceNotFound(id uuid.UUID, resourceType string) *ErrResourceNotFound {
	return &ErrResourceNotFound{fmt.Errorf("%s %s not found", resourceType, id)}
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

type ErrAgentUpdateForbidden struct {
	error
}

func NewErrAgentUpdateForbidden(sourceID, agentID uuid.UUID) *ErrAgentUpdateForbidden {
	return &ErrAgentUpdateForbidden{fmt.Errorf("agent %s is not associated with source %s", agentID, sourceID)}
}
