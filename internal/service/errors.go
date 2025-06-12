package service

import (
	"fmt"

	"github.com/google/uuid"
)

type ErrInvalidVCenterID struct {
	error
}

func NewErrInvalidVCenterID(sourceID uuid.UUID, vCenterID string) *ErrInvalidVCenterID {
	return &ErrInvalidVCenterID{fmt.Errorf("source %q vcenter id is different than the one specied %q", sourceID, vCenterID)}
}

type ErrSourceNotFound struct {
	error
}

func NewErrSourceNotFound(sourceID uuid.UUID) *ErrSourceNotFound {
	return &ErrSourceNotFound{fmt.Errorf("source %q not found", sourceID)}
}
