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

type ErrInvalidFile struct {
	message string
}

func NewErrInvalidFile(format string, args ...any) *ErrInvalidFile {
	return &ErrInvalidFile{message: fmt.Sprintf(format, args...)}
}

func (e *ErrInvalidFile) Error() string {
	return e.message
}
