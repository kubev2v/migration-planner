package store

import "errors"

var (
	ErrRecordNotFound = errors.New("record not found")
	ErrDuplicateKey   = errors.New("already exists")
)
