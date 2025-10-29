package service

import (
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

type ErrFileCorrupted struct {
	error
}

func NewErrFileCorrupted(message string) *ErrFileCorrupted {
	return &ErrFileCorrupted{fmt.Errorf("bad request: %s", message)}
}

func NewErrRVToolsFileCorrupted(message string) *ErrFileCorrupted {
	return NewErrFileCorrupted(fmt.Sprintf("The provided RVTools file is corrupted: %s", message))
}

type ErrAgentUpdateForbidden struct {
	error
}

func NewErrAgentUpdateForbidden(sourceID, agentID uuid.UUID) *ErrAgentUpdateForbidden {
	return &ErrAgentUpdateForbidden{fmt.Errorf("agent %s is not associated with source %s", agentID, sourceID)}
}

func NewErrAssessmentNotFound(id uuid.UUID) *ErrResourceNotFound {
	return NewErrResourceNotFound(id, "assessment")
}

type ErrAssessmentCreationForbidden struct {
	error
}

func NewErrAssessmentCreationForbidden(sourceID uuid.UUID) *ErrAssessmentCreationForbidden {
	return &ErrAssessmentCreationForbidden{fmt.Errorf("forbidden to create assessment from source id:%s", sourceID)}
}

type ErrSourceHasNoInventory struct {
	error
}

func NewErrSourceHasNoInventory(sourceID uuid.UUID) *ErrSourceHasNoInventory {
	return &ErrSourceHasNoInventory{fmt.Errorf("source has no inventory: %s", sourceID)}
}
