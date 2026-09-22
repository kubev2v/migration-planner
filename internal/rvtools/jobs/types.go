package jobs

import (
	"fmt"
	"strings"
)

type RVToolParsingError struct {
	Errors map[string]error
}

func NewRVToolParsingError() *RVToolParsingError {
	return &RVToolParsingError{Errors: make(map[string]error)}
}

func (e *RVToolParsingError) Add(name string, err error) {
	e.Errors[name] = err
}

func (e *RVToolParsingError) HasErrors() bool {
	return len(e.Errors) > 0
}

func (e *RVToolParsingError) Error() string {
	msgs := make([]string, 0, len(e.Errors))
	for name, err := range e.Errors {
		msgs = append(msgs, fmt.Sprintf("%s: %v", name, err))
	}
	return fmt.Sprintf("rvtools parsing failed: %s", strings.Join(msgs, "; "))
}
