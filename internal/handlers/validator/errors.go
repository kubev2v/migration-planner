package validator

import (
	"fmt"
)

type ErrInvalidName struct {
	error
}

func NewErrInvalidName(format string, args ...any) *ErrInvalidName {
	return &ErrInvalidName{fmt.Errorf(format, args...)}
}
