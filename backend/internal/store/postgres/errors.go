package postgres

import "errors"

// ErrNotFound is returned when a requested row does not exist.
var ErrNotFound = errors.New("not found")
