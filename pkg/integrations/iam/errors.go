package iam

import "errors"

// ErrOrgNotFound is returned when the User Service has no organization
// (account relationship) for the requested username.
var ErrOrgNotFound = errors.New("no org_id found for username")

// ErrAccountNotFound is returned when the User Service cannot find the requested account.
var ErrAccountNotFound = errors.New("account not found")
